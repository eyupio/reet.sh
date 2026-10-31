//go:build linux || darwin

package acpagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Typed agent questions (SC3). An adapter asks with a form-mode
// elicitation/create request. This file normalizes the supported subset into a
// bounded internal model, independent of raw adapter JSON, and validates
// answers against it. The browser's own validation is feedback only: the
// machine keeps the normalized question and is the authority.
//
// Supported subset, taken from what the Claude adapter was observed to send
// (SC2 probe): bounded text, single select and multi select. Everything else —
// URL mode, numbers, booleans, patterns, formats, defaults, nested objects —
// is refused as a whole form rather than guessed at.

// Question limits. A form over any of them is refused, not truncated, so a
// person never answers a different question than the agent asked.
const (
	maxQuestionSchemaBytes = 16 << 10
	maxQuestionFields      = 8
	maxQuestionOptions     = 12
	maxQuestionMessage     = 1000
	maxQuestionLabel       = 200
	maxQuestionDescription = 500
	maxQuestionKey         = 64
	maxQuestionValue       = 200
	maxQuestionTextAnswer  = 2000
	maxQuestionAnswerBytes = 8 << 10
	maxPendingQuestions    = 4
)

// QuestionKind is one supported field shape.
type QuestionKind string

const (
	QuestionText   QuestionKind = "text"
	QuestionSingle QuestionKind = "single"
	QuestionMulti  QuestionKind = "multi"
)

// QuestionOption is one choice. Value is the exact string echoed back to the
// agent; Label is what the person reads.
type QuestionOption struct {
	Value       string
	Label       string
	Description string
}

// QuestionField is one answerable field of a form.
type QuestionField struct {
	Key         string
	Title       string
	Description string
	Kind        QuestionKind
	Required    bool
	Options     []QuestionOption
	// MinItems and MaxItems bound a multi select. MaxItems is never zero.
	MinItems int
	MaxItems int
	// MaxLength bounds a text answer in bytes.
	MaxLength int
}

// Question is a normalized, validated form.
type Question struct {
	Message    string
	ToolCallID string
	Fields     []QuestionField
}

// ErrQuestionUnsupported means the agent asked for a form this client cannot
// render or validate. The request is answered with a cancellation.
var ErrQuestionUnsupported = errors.New("the agent asked a question this version cannot show")

// UnsupportedQuestionError says why a form was refused. Reason is for logs and
// tests; it is not shown as an answer anywhere.
type UnsupportedQuestionError struct{ Reason string }

func (e *UnsupportedQuestionError) Error() string {
	return ErrQuestionUnsupported.Error() + ": " + e.Reason
}
func (e *UnsupportedQuestionError) Unwrap() error { return ErrQuestionUnsupported }

func unsupported(format string, args ...any) error {
	return &UnsupportedQuestionError{Reason: fmt.Sprintf(format, args...)}
}

type elicitationParams struct {
	Mode            string          `json:"mode"`
	SessionID       string          `json:"sessionId"`
	ToolCallID      string          `json:"toolCallId"`
	Message         string          `json:"message"`
	RequestedSchema json.RawMessage `json:"requestedSchema"`
}

type schemaOption struct {
	Const       string          `json:"const"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Meta        json.RawMessage `json:"_meta"`
}

type schemaItems struct {
	Type  string         `json:"type"`
	Enum  []string       `json:"enum"`
	AnyOf []schemaOption `json:"anyOf"`
	OneOf []schemaOption `json:"oneOf"`
}

type schemaField struct {
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	OneOf       []schemaOption  `json:"oneOf"`
	Enum        []string        `json:"enum"`
	EnumNames   []string        `json:"enumNames"`
	Items       *schemaItems    `json:"items"`
	MinItems    *int            `json:"minItems"`
	MaxItems    *int            `json:"maxItems"`
	MaxLength   *int            `json:"maxLength"`
	UniqueItems *bool           `json:"uniqueItems"`
	Meta        json.RawMessage `json:"_meta"`
}

type schemaObject struct {
	Schema      string                     `json:"$schema"`
	Type        string                     `json:"type"`
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	Properties  map[string]json.RawMessage `json:"properties"`
	Required    []string                   `json:"required"`
}

func strictDecode(data []byte, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("trailing data")
	}
	return nil
}

// plainQuestionText accepts valid UTF-8 within max bytes with no control
// characters other than newline and tab.
func plainQuestionText(s string, max int) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return false
		}
	}
	return true
}

// parseQuestion normalizes an elicitation/create request. The returned session
// ID is the one the agent named; the caller must check it against the session
// that is actually running.
func parseQuestion(raw json.RawMessage) (Question, string, error) {
	var params elicitationParams
	if json.Unmarshal(raw, &params) != nil {
		return Question{}, "", unsupported("the request could not be read")
	}
	if params.Mode != "form" {
		return Question{}, "", unsupported("mode %q is not form", params.Mode)
	}
	if len(params.RequestedSchema) == 0 || len(params.RequestedSchema) > maxQuestionSchemaBytes {
		return Question{}, "", unsupported("the schema is missing or larger than %d bytes", maxQuestionSchemaBytes)
	}
	if !plainQuestionText(params.Message, maxQuestionMessage) || params.Message == "" {
		return Question{}, "", unsupported("the message is empty or not plain text within %d bytes", maxQuestionMessage)
	}
	if params.ToolCallID != "" && !validACPQuestionID(params.ToolCallID) {
		return Question{}, "", unsupported("the tool call id is not valid")
	}
	var object schemaObject
	if strictDecode(params.RequestedSchema, &object) != nil || object.Type != "object" {
		return Question{}, "", unsupported("the schema is not a plain object schema")
	}
	if len(object.Properties) == 0 || len(object.Properties) > maxQuestionFields {
		return Question{}, "", unsupported("the form has %d fields; 1 to %d are supported", len(object.Properties), maxQuestionFields)
	}
	required := map[string]bool{}
	for _, key := range object.Required {
		if _, ok := object.Properties[key]; !ok || required[key] {
			return Question{}, "", unsupported("required field %q is unknown or repeated", key)
		}
		required[key] = true
	}
	keys := make([]string, 0, len(object.Properties))
	for key := range object.Properties {
		keys = append(keys, key)
	}
	sortQuestionKeys(keys)
	q := Question{Message: params.Message, ToolCallID: params.ToolCallID}
	for _, key := range keys {
		if key == "" || !plainQuestionText(key, maxQuestionKey) || strings.ContainsAny(key, "\n\t") {
			return Question{}, "", unsupported("field name %q is not allowed", key)
		}
		field, err := parseQuestionField(key, object.Properties[key], required[key])
		if err != nil {
			return Question{}, "", err
		}
		q.Fields = append(q.Fields, field)
	}
	return q, params.SessionID, nil
}

func validACPQuestionID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':') {
			return false
		}
	}
	return true
}

// sortQuestionKeys orders question_0, question_0_custom, question_1, ... as the
// adapter intends, then any other name alphabetically. JSON object order is not
// preserved by the decoder, so this is the only stable display order.
func sortQuestionKeys(keys []string) {
	order := func(key string) (int, int, bool) {
		rest, ok := strings.CutPrefix(key, "question_")
		if !ok {
			return 0, 0, false
		}
		custom := 0
		if base, found := strings.CutSuffix(rest, "_custom"); found {
			rest, custom = base, 1
		}
		n, err := strconv.Atoi(rest)
		return n, custom, err == nil && n >= 0
	}
	sort.SliceStable(keys, func(i, j int) bool {
		ai, ac, aok := order(keys[i])
		bi, bc, bok := order(keys[j])
		switch {
		case aok && bok:
			if ai != bi {
				return ai < bi
			}
			return ac < bc
		case aok:
			return true
		case bok:
			return false
		}
		return keys[i] < keys[j]
	})
}

func parseQuestionField(key string, raw json.RawMessage, required bool) (QuestionField, error) {
	var schema schemaField
	if strictDecode(raw, &schema) != nil {
		return QuestionField{}, unsupported("field %q uses a schema keyword that is not supported", key)
	}
	if !plainQuestionText(schema.Title, maxQuestionLabel) || !plainQuestionText(schema.Description, maxQuestionDescription) {
		return QuestionField{}, unsupported("field %q has an oversized or non-plain title or description", key)
	}
	field := QuestionField{Key: key, Title: schema.Title, Description: schema.Description, Required: required}
	switch schema.Type {
	case "string":
		if schema.Items != nil || schema.MinItems != nil || schema.MaxItems != nil || schema.UniqueItems != nil {
			return QuestionField{}, unsupported("field %q mixes string and array keywords", key)
		}
		options, err := stringOptions(key, schema)
		if err != nil {
			return QuestionField{}, err
		}
		if len(options) > 0 {
			if schema.MaxLength != nil {
				return QuestionField{}, unsupported("field %q is a choice with a length limit", key)
			}
			field.Kind, field.Options = QuestionSingle, options
			return field, nil
		}
		field.Kind, field.MaxLength = QuestionText, maxQuestionTextAnswer
		if schema.MaxLength != nil {
			if *schema.MaxLength < 1 {
				return QuestionField{}, unsupported("field %q has a non-positive length limit", key)
			}
			field.MaxLength = min(*schema.MaxLength, maxQuestionTextAnswer)
		}
		return field, nil
	case "array":
		if len(schema.OneOf) > 0 || len(schema.Enum) > 0 || schema.MaxLength != nil || schema.Items == nil {
			return QuestionField{}, unsupported("field %q is not a plain multi select", key)
		}
		options, err := itemOptions(key, *schema.Items)
		if err != nil {
			return QuestionField{}, err
		}
		field.Kind, field.Options, field.MinItems, field.MaxItems = QuestionMulti, options, 0, len(options)
		if schema.MinItems != nil {
			if *schema.MinItems < 0 || *schema.MinItems > len(options) {
				return QuestionField{}, unsupported("field %q has an impossible minimum", key)
			}
			field.MinItems = *schema.MinItems
		}
		if schema.MaxItems != nil {
			if *schema.MaxItems < 1 || *schema.MaxItems > len(options) || *schema.MaxItems < field.MinItems {
				return QuestionField{}, unsupported("field %q has an impossible maximum", key)
			}
			field.MaxItems = *schema.MaxItems
		}
		if required && field.MinItems < 1 {
			field.MinItems = 1
		}
		return field, nil
	}
	return QuestionField{}, unsupported("field %q has type %q", key, schema.Type)
}

func toOptions(key string, titled []schemaOption) ([]QuestionOption, error) {
	if len(titled) == 0 || len(titled) > maxQuestionOptions {
		return nil, unsupported("field %q has %d options; 1 to %d are supported", key, len(titled), maxQuestionOptions)
	}
	seen := map[string]bool{}
	options := make([]QuestionOption, 0, len(titled))
	for _, option := range titled {
		if option.Const == "" || !plainQuestionText(option.Const, maxQuestionValue) || seen[option.Const] {
			return nil, unsupported("field %q has an empty, oversized or repeated option value", key)
		}
		if !plainQuestionText(option.Title, maxQuestionLabel) || !plainQuestionText(option.Description, maxQuestionDescription) {
			return nil, unsupported("field %q has an oversized or non-plain option label", key)
		}
		seen[option.Const] = true
		label := option.Title
		if label == "" {
			label = option.Const
		}
		options = append(options, QuestionOption{Value: option.Const, Label: label, Description: option.Description})
	}
	return options, nil
}

func stringOptions(key string, schema schemaField) ([]QuestionOption, error) {
	switch {
	case len(schema.OneOf) > 0 && len(schema.Enum) > 0:
		return nil, unsupported("field %q has both oneOf and enum", key)
	case len(schema.OneOf) > 0:
		if len(schema.EnumNames) > 0 {
			return nil, unsupported("field %q has enumNames beside oneOf", key)
		}
		return toOptions(key, schema.OneOf)
	case len(schema.Enum) > 0:
		return bareOptions(key, schema.Enum, schema.EnumNames)
	}
	if len(schema.EnumNames) > 0 {
		return nil, unsupported("field %q has enumNames without enum", key)
	}
	return nil, nil
}

func bareOptions(key string, values, names []string) ([]QuestionOption, error) {
	if len(names) != 0 && len(names) != len(values) {
		return nil, unsupported("field %q has %d names for %d values", key, len(names), len(values))
	}
	titled := make([]schemaOption, len(values))
	for i, value := range values {
		titled[i].Const = value
		if len(names) > 0 {
			titled[i].Title = names[i]
		}
	}
	return toOptions(key, titled)
}

func itemOptions(key string, items schemaItems) ([]QuestionOption, error) {
	count := 0
	for _, present := range []bool{len(items.Enum) > 0, len(items.AnyOf) > 0, len(items.OneOf) > 0} {
		if present {
			count++
		}
	}
	if count != 1 || items.Type != "" && items.Type != "string" {
		return nil, unsupported("field %q items are not a plain list of string choices", key)
	}
	switch {
	case len(items.Enum) > 0:
		return bareOptions(key, items.Enum, nil)
	case len(items.AnyOf) > 0:
		return toOptions(key, items.AnyOf)
	}
	return toOptions(key, items.OneOf)
}

// AnswerError is a rejected answer set. The pending question stays answerable.
type AnswerError struct {
	Field  string
	Reason string
}

func (e *AnswerError) Error() string {
	if e.Field == "" {
		return "answer rejected: " + e.Reason
	}
	return "answer rejected for " + e.Field + ": " + e.Reason
}

func rejected(field, format string, args ...any) error {
	return &AnswerError{Field: field, Reason: fmt.Sprintf(format, args...)}
}

// DecodeAnswers parses a JSON object of answers and refuses duplicate keys,
// which a plain map decode would silently resolve to the last one.
func DecodeAnswers(data []byte) (map[string]json.RawMessage, error) {
	if len(data) > maxQuestionAnswerBytes*2 {
		return nil, rejected("", "the answers are too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if delim, ok := token.(json.Delim); err != nil || !ok || delim != '{' {
		return nil, rejected("", "the answers are not a JSON object")
	}
	answers := map[string]json.RawMessage{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok {
			return nil, rejected("", "the answers are not a JSON object")
		}
		if _, dup := answers[key]; dup {
			return nil, rejected(key, "answered more than once")
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, rejected(key, "the value could not be read")
		}
		answers[key] = value
	}
	if _, err := decoder.Token(); err != nil || decoder.More() {
		return nil, rejected("", "the answers are not a JSON object")
	}
	if _, err := decoder.Token(); err == nil {
		return nil, rejected("", "trailing data after the answers")
	}
	return answers, nil
}

// Validate checks submitted answers against the normalized question and returns
// the content to send to the agent. It rejects unknown keys, wrong types,
// options that were not offered, duplicates, missing required answers and
// anything out of bounds. Optional text left empty is omitted.
func (q Question) Validate(answers map[string]json.RawMessage) (map[string]any, error) {
	fields := make(map[string]QuestionField, len(q.Fields))
	for _, field := range q.Fields {
		fields[field.Key] = field
	}
	total := 0
	for key, value := range answers {
		if _, ok := fields[key]; !ok {
			return nil, rejected(key, "no such question")
		}
		total += len(key) + len(value)
	}
	if total > maxQuestionAnswerBytes {
		return nil, rejected("", "the answers are larger than %d bytes", maxQuestionAnswerBytes)
	}
	content := map[string]any{}
	for _, field := range q.Fields {
		raw, answered := answers[field.Key]
		if !answered || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			if field.Required {
				return nil, rejected(field.Key, "an answer is required")
			}
			continue
		}
		switch field.Kind {
		case QuestionText:
			var text string
			if json.Unmarshal(raw, &text) != nil {
				return nil, rejected(field.Key, "expected text")
			}
			if !plainQuestionText(text, field.MaxLength) {
				return nil, rejected(field.Key, "the text is not plain or is longer than %d bytes", field.MaxLength)
			}
			if strings.TrimSpace(text) == "" {
				if field.Required {
					return nil, rejected(field.Key, "an answer is required")
				}
				continue
			}
			content[field.Key] = text
		case QuestionSingle:
			var choice string
			if json.Unmarshal(raw, &choice) != nil {
				return nil, rejected(field.Key, "expected one choice")
			}
			if !field.offers(choice) {
				return nil, rejected(field.Key, "that choice was not offered")
			}
			content[field.Key] = choice
		case QuestionMulti:
			var choices []string
			if json.Unmarshal(raw, &choices) != nil {
				return nil, rejected(field.Key, "expected a list of choices")
			}
			if len(choices) < field.MinItems {
				return nil, rejected(field.Key, "choose at least %d", field.MinItems)
			}
			if len(choices) > field.MaxItems {
				return nil, rejected(field.Key, "choose at most %d", field.MaxItems)
			}
			seen := map[string]bool{}
			for _, choice := range choices {
				if !field.offers(choice) {
					return nil, rejected(field.Key, "a choice was not offered")
				}
				if seen[choice] {
					return nil, rejected(field.Key, "a choice was repeated")
				}
				seen[choice] = true
			}
			if len(choices) == 0 {
				continue // an optional multi select left empty is omitted
			}
			content[field.Key] = choices
		}
	}
	return content, nil
}

func (f QuestionField) offers(value string) bool {
	for _, option := range f.Options {
		if option.Value == value {
			return true
		}
	}
	return false
}

// Wire is the question in the private protocol's shape. Its Valid method is the
// authority on whether the form fits the control frame.
func (q Question) Wire() terminalproto.ACPQuestion {
	wire := terminalproto.ACPQuestion{Message: q.Message, Fields: make([]terminalproto.ACPQuestionField, 0, len(q.Fields))}
	for _, f := range q.Fields {
		field := terminalproto.ACPQuestionField{Key: f.Key, Title: f.Title, Description: f.Description, Kind: string(f.Kind),
			Required: f.Required, MinItems: f.MinItems, MaxItems: f.MaxItems, MaxLength: f.MaxLength}
		if f.Kind == QuestionSingle {
			field.MinItems, field.MaxItems = 0, 0
		}
		for _, o := range f.Options {
			field.Options = append(field.Options, terminalproto.ACPQuestionOption{Value: o.Value, Label: o.Label, Description: o.Description})
		}
		wire.Fields = append(wire.Fields, field)
	}
	return wire
}
