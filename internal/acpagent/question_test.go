//go:build linux || darwin

package acpagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func form(schema string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"mode":"form","sessionId":"s1","toolCallId":"t1","message":"Pick","requestedSchema":%s}`, schema))
}

// The shape captured from the Claude adapter's AskUserQuestion.
const capturedSchema = `{"type":"object","properties":{
 "question_0":{"type":"string","title":"Probe","oneOf":[{"const":"Option A","title":"Option A","description":"First synthetic option"},{"const":"Option B","title":"Option B","description":"Second synthetic option"}]},
 "question_0_custom":{"type":"string","title":"Other","description":"Type your own answer"}}}`

func TestParseCapturedClaudeQuestion(t *testing.T) {
	q, session, err := parseQuestion(form(capturedSchema))
	if err != nil || session != "s1" || q.ToolCallID != "t1" || q.Message != "Pick" {
		t.Fatalf("%+v %q %v", q, session, err)
	}
	if len(q.Fields) != 2 || q.Fields[0].Key != "question_0" || q.Fields[1].Key != "question_0_custom" {
		t.Fatalf("fields %+v", q.Fields)
	}
	choice := q.Fields[0]
	if choice.Kind != QuestionSingle || len(choice.Options) != 2 || choice.Options[0].Value != "Option A" || choice.Options[0].Description != "First synthetic option" {
		t.Fatalf("choice %+v", choice)
	}
	if text := q.Fields[1]; text.Kind != QuestionText || text.Required || text.MaxLength != maxQuestionTextAnswer {
		t.Fatalf("text %+v", text)
	}
}

func TestParseMultiSelectAndEnumForms(t *testing.T) {
	q, _, err := parseQuestion(form(`{"type":"object","required":["pick","tags"],"properties":{
	 "pick":{"type":"string","enum":["x","y"],"enumNames":["Ex","Why"]},
	 "tags":{"type":"array","minItems":1,"maxItems":2,"items":{"anyOf":[{"const":"a","title":"A"},{"const":"b"},{"const":"c"}]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var pick, tags QuestionField
	for _, f := range q.Fields {
		if f.Key == "pick" {
			pick = f
		} else {
			tags = f
		}
	}
	if pick.Kind != QuestionSingle || pick.Options[1].Label != "Why" || !pick.Required {
		t.Fatalf("pick %+v", pick)
	}
	if tags.Kind != QuestionMulti || tags.MinItems != 1 || tags.MaxItems != 2 || tags.Options[1].Label != "b" {
		t.Fatalf("tags %+v", tags)
	}
}

func TestUnsupportedFormsAreRefusedWhole(t *testing.T) {
	long := strings.Repeat("x", maxQuestionLabel+1)
	var many []string
	for i := 0; i <= maxQuestionFields; i++ {
		many = append(many, fmt.Sprintf(`"f%d":{"type":"string"}`, i))
	}
	var options []string
	for i := 0; i <= maxQuestionOptions; i++ {
		options = append(options, fmt.Sprintf(`{"const":"o%d"}`, i))
	}
	cases := map[string]string{
		"number":           `{"type":"object","properties":{"n":{"type":"number"}}}`,
		"integer":          `{"type":"object","properties":{"n":{"type":"integer"}}}`,
		"boolean":          `{"type":"object","properties":{"b":{"type":"boolean"}}}`,
		"nested object":    `{"type":"object","properties":{"o":{"type":"object"}}}`,
		"pattern":          `{"type":"object","properties":{"s":{"type":"string","pattern":"^a$"}}}`,
		"format":           `{"type":"object","properties":{"s":{"type":"string","format":"email"}}}`,
		"default":          `{"type":"object","properties":{"s":{"type":"string","default":"x"}}}`,
		"minLength":        `{"type":"object","properties":{"s":{"type":"string","minLength":2}}}`,
		"unknown keyword":  `{"type":"object","properties":{"s":{"type":"string","x-weird":1}}}`,
		"not an object":    `{"type":"array"}`,
		"no fields":        `{"type":"object","properties":{}}`,
		"too many fields":  `{"type":"object","properties":{` + strings.Join(many, ",") + `}}`,
		"too many options": `{"type":"object","properties":{"s":{"type":"string","oneOf":[` + strings.Join(options, ",") + `]}}}`,
		"empty option":     `{"type":"object","properties":{"s":{"type":"string","oneOf":[{"const":""}]}}}`,
		"repeated option":  `{"type":"object","properties":{"s":{"type":"string","enum":["a","a"]}}}`,
		"long title":       `{"type":"object","properties":{"s":{"type":"string","title":"` + long + `"}}}`,
		"unknown required": `{"type":"object","required":["nope"],"properties":{"s":{"type":"string"}}}`,
		"oneOf and enum":   `{"type":"object","properties":{"s":{"type":"string","oneOf":[{"const":"a"}],"enum":["a"]}}}`,
		"array of objects": `{"type":"object","properties":{"a":{"type":"array","items":{"type":"object"}}}}`,
		"array no items":   `{"type":"object","properties":{"a":{"type":"array"}}}`,
		"bad max items":    `{"type":"object","properties":{"a":{"type":"array","maxItems":9,"items":{"enum":["a"]}}}}`,
		"control char":     "{\"type\":\"object\",\"properties\":{\"s\":{\"type\":\"string\",\"title\":\"a\\u0000b\"}}}",
		"names mismatch":   `{"type":"object","properties":{"s":{"type":"string","enum":["a","b"],"enumNames":["A"]}}}`,
		"oversized":        `{"type":"object","description":"` + strings.Repeat("x", maxQuestionSchemaBytes) + `","properties":{"s":{"type":"string"}}}`,
	}
	for name, schema := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := parseQuestion(form(schema))
			var refused *UnsupportedQuestionError
			if !errors.As(err, &refused) || !errors.Is(err, ErrQuestionUnsupported) {
				t.Fatalf("accepted: %v", err)
			}
		})
	}
	for name, raw := range map[string]string{
		"url mode":      `{"mode":"url","message":"x","requestedSchema":{}}`,
		"no message":    `{"mode":"form","requestedSchema":{"type":"object","properties":{"s":{"type":"string"}}}}`,
		"bad json":      `{`,
		"bad tool call": `{"mode":"form","toolCallId":"a b","message":"x","requestedSchema":{"type":"object","properties":{"s":{"type":"string"}}}}`,
	} {
		if _, _, err := parseQuestion(json.RawMessage(raw)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestKeyOrder(t *testing.T) {
	keys := []string{"zeta", "question_1", "question_0_custom", "alpha", "question_10", "question_0", "question_2"}
	sortQuestionKeys(keys)
	want := "question_0,question_0_custom,question_1,question_2,question_10,alpha,zeta"
	if got := strings.Join(keys, ","); got != want {
		t.Fatalf("order %s", got)
	}
}

func mustParse(t *testing.T) Question {
	t.Helper()
	q, _, err := parseQuestion(form(`{"type":"object","required":["pick","tags"],"properties":{
	 "pick":{"type":"string","oneOf":[{"const":"a"},{"const":"b"}]},
	 "tags":{"type":"array","minItems":1,"maxItems":2,"items":{"enum":["x","y","z"]}},
	 "note":{"type":"string","maxLength":5}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func decode(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	a, err := DecodeAnswers([]byte(raw))
	if err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return a
}

func TestValidateAcceptsAndNormalizes(t *testing.T) {
	q := mustParse(t)
	content, err := q.Validate(decode(t, `{"pick":"a","tags":["x","z"],"note":"hi"}`))
	if err != nil || content["pick"] != "a" || content["note"] != "hi" || len(content["tags"].([]string)) != 2 {
		t.Fatalf("%v %v", content, err)
	}
	// Optional text left blank, or null, is omitted rather than sent empty.
	content, err = q.Validate(decode(t, `{"pick":"b","tags":["y"],"note":"   "}`))
	if err != nil || len(content) != 2 {
		t.Fatalf("%v %v", content, err)
	}
	content, err = q.Validate(decode(t, `{"pick":"b","tags":["y"],"note":null}`))
	if err != nil || len(content) != 2 {
		t.Fatalf("%v %v", content, err)
	}
}

func TestValidateRejects(t *testing.T) {
	q := mustParse(t)
	for name, raw := range map[string]string{
		"missing required":      `{"tags":["x"]}`,
		"missing required list": `{"pick":"a"}`,
		"empty required list":   `{"pick":"a","tags":[]}`,
		"unknown key":           `{"pick":"a","tags":["x"],"other":"1"}`,
		"unoffered choice":      `{"pick":"c","tags":["x"]}`,
		"case differs":          `{"pick":"A","tags":["x"]}`,
		"choice as list":        `{"pick":["a"],"tags":["x"]}`,
		"list as choice":        `{"pick":"a","tags":"x"}`,
		"number as choice":      `{"pick":1,"tags":["x"]}`,
		"object as text":        `{"pick":"a","tags":["x"],"note":{}}`,
		"too many items":        `{"pick":"a","tags":["x","y","z"]}`,
		"repeated item":         `{"pick":"a","tags":["x","x"]}`,
		"unoffered item":        `{"pick":"a","tags":["q"]}`,
		"text too long":         `{"pick":"a","tags":["x"],"note":"123456"}`,
		"control char text":     "{\"pick\":\"a\",\"tags\":[\"x\"],\"note\":\"a\\u0000\"}",
		"answers too large":     `{"pick":"a","tags":["x"],"note":"` + strings.Repeat("y", 6) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			answers, err := DecodeAnswers([]byte(raw))
			if err != nil {
				return // rejected at decode, also fine
			}
			var answerErr *AnswerError
			if _, err := q.Validate(answers); !errors.As(err, &answerErr) {
				t.Fatalf("accepted: %v", err)
			}
		})
	}
	big := mustParse(t)
	big.Fields = append(big.Fields, QuestionField{Key: "long", Kind: QuestionText, MaxLength: maxQuestionTextAnswer})
	huge := map[string]json.RawMessage{"long": json.RawMessage(`"` + strings.Repeat("a", maxQuestionAnswerBytes) + `"`)}
	if _, err := big.Validate(huge); err == nil {
		t.Fatal("oversized answer set accepted")
	}
}

func TestDecodeAnswersRefusesDuplicatesAndNonObjects(t *testing.T) {
	for _, raw := range []string{`{"a":"1","a":"2"}`, `[]`, `"x"`, `{"a":`, `{"a":"1"} {"b":"2"}`, `{} x`, ``} {
		if _, err := DecodeAnswers([]byte(raw)); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if got, err := DecodeAnswers([]byte(`{}`)); err != nil || len(got) != 0 {
		t.Fatalf("empty object: %v %v", got, err)
	}
}

func TestFormOverTheWireBoundParsesButIsNotWireValid(t *testing.T) {
	var properties []string
	for f := 0; f < 6; f++ {
		var options []string
		for o := 0; o < 12; o++ {
			options = append(options, fmt.Sprintf(`{"const":"v%02d%02d","title":"%s"}`, f, o, strings.Repeat("t", 150)))
		}
		properties = append(properties, fmt.Sprintf(`"question_%d":{"type":"string","oneOf":[%s]}`, f, strings.Join(options, ",")))
	}
	q, _, err := parseQuestion(form(`{"type":"object","properties":{` + strings.Join(properties, ",") + `}}`))
	if err != nil {
		t.Fatalf("the machine parser should accept it: %v", err)
	}
	if q.Wire().Valid() {
		t.Fatal("a form over the control-frame bound was wire valid")
	}
}
