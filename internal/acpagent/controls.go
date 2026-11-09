//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strconv"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrControl = errors.New("the agent did not confirm that setting")

// ErrSafety means an adapter did not confirm a safety option the profile
// requires before a turn, such as Cline's auto-approve switch being off.
var ErrSafety = errors.New("the agent did not confirm a required safety setting")

// optionValue is a config option's current value. ACP adapters report most as
// strings, but Cline reports its boolean auto-approve option as a JSON boolean,
// so one text form keeps a single comparison rule. JSON null is the empty value,
// as an absent string was before.
type optionValue string

func (v *optionValue) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*v = ""
		return nil
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		*v = optionValue(text)
		return nil
	}
	var flag bool
	if json.Unmarshal(data, &flag) == nil {
		*v = optionValue(strconv.FormatBool(flag))
		return nil
	}
	return errors.New("unsupported config option value")
}

// readSettings admits only non-permission selectors from the live session.
// Static installation descriptors never authorise a configuration change.
// readSettings admits only non-permission selectors from the live session. The
// provider option is a billing source only for an agent whose spec says so.
func readSettings(raw json.RawMessage, provider bool) ([]terminalproto.ACPSetting, bool) {
	var reply struct {
		Options json.RawMessage `json:"configOptions"`
	}
	if json.Unmarshal(raw, &reply) != nil || len(reply.Options) == 0 {
		return nil, false
	}
	var options []struct {
		ID       string          `json:"id"`
		Category string          `json:"category"`
		Current  optionValue     `json:"currentValue"`
		Options  json.RawMessage `json:"options"`
	}
	if json.Unmarshal(reply.Options, &options) != nil {
		return nil, false
	}
	out := []terminalproto.ACPSetting{}
	for _, o := range options {
		category := o.Category
		if category == "" && o.ID == "model" {
			category = "model"
		}
		// Cline reports its provider (its billing source) with category
		// "_provider"; reet surfaces it as the "provider" setting.
		if category == "_provider" || (category == "" && o.ID == "provider") {
			if !provider {
				continue
			}
			category = "provider"
		}
		isMode := o.ID == "mode"
		isModel := o.ID == "model" && category != "model"
		if isMode || isModel {
			continue
		}
		if category != "model" && category != "thought_level" && category != "provider" {
			continue
		}
		current := string(o.Current)
		if current != "" && !terminalproto.ValidModel(current) {
			// An adapter may report a current value reet cannot represent (one
			// Cline alias starts with "~"). Keep the selector but do not claim
			// a confirmed value it cannot name.
			current = ""
		}
		setting := terminalproto.ACPSetting{ID: o.ID, Category: category, Current: current, Values: []string{}}
		var choices []struct {
			Value   string `json:"value"`
			Options []struct {
				Value string `json:"value"`
			} `json:"options"`
		}
		if len(o.Options) > 0 && json.Unmarshal(o.Options, &choices) != nil {
			return nil, false
		}
		add := func(v string) {
			// A model may be the whole catalogue (Cline offers 307 ids); the
			// connection trims it to the values its peer can read.
			limit := terminalproto.MaxACPThoughtLevelValues
			if category == "model" {
				limit = terminalproto.MaxACPSettingValues
			}
			canAdd := len(setting.Values) < limit && v != "" && terminalproto.ValidModel(v)
			if canAdd && !slices.Contains(setting.Values, v) {
				setting.Values = append(setting.Values, v)
			}
		}
		for _, c := range choices {
			add(c.Value)
			for _, v := range c.Options {
				add(v.Value)
			}
		}
		out = append(out, setting)
	}
	if !(terminalproto.ACPControls{Options: out}).Valid() {
		return nil, false
	}
	return out, true
}

func (s *Session) updateSettings(raw json.RawMessage) {
	if options, ok := readSettings(raw, s.opts.Agent.Provider); ok {
		s.mu.Lock()
		s.settings = options
		for _, o := range options {
			if o.Category == "model" {
				s.model = o.Current
			}
		}
		s.mu.Unlock()
	} else {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil && fields["configOptions"] != nil {
			// An explicit replacement that cannot be validated invalidates the
			// old selectors; they may now describe a different model.
			s.mu.Lock()
			s.settings = nil
			s.model = ""
			s.mu.Unlock()
		}
	}
}

func (s *Session) Controls() terminalproto.ACPControls {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	state := terminalproto.ACPControls{Configuring: s.configuring, Working: s.working || s.nativeBusy, Waiting: len(s.pending) > 0 || len(s.questions) > 0, Compacting: s.compacting, Steering: s.steering, Model: s.model, Options: []terminalproto.ACPSetting{}}
	for _, o := range s.settings {
		o.Values = slices.Clone(o.Values)
		if o.Category == "model" {
			o.Current = s.model
		}
		state.Options = append(state.Options, o)
	}
	return state
}

// SetOption serialises setting mutations while leaving Cancel and ACP updates
// responsive. An idle session cannot begin a turn in the middle of a switch.
func (s *Session) SetOption(ctx context.Context, id, value string) error {
	if !s.configMu.TryLock() {
		return ErrBusy
	}
	defer s.configMu.Unlock()
	s.turnMu.Lock()
	if s.forceClosing {
		s.turnMu.Unlock()
		return ErrClosed
	}
	s.configuring = true
	busy := s.working || s.nativeBusy
	s.turnMu.Unlock()
	defer func() { s.turnMu.Lock(); s.configuring = false; s.turnMu.Unlock() }()
	s.mu.Lock()
	var setting *terminalproto.ACPSetting
	for _, o := range s.settings {
		if o.ID == id {
			copy := o
			setting = &copy
			break
		}
	}
	acpID, closed := s.acpID, s.closed
	s.mu.Unlock()
	if closed {
		return ErrClosed
	}
	listedOnly := setting != nil && (setting.Category == "thought_level" || setting.Category == "provider")
	invalid := setting == nil || value == "" || !terminalproto.ValidModel(value) ||
		(listedOnly && !slices.Contains(setting.Values, value))
	if invalid {
		return ErrControl
	}
	// A source switch replaces the model catalogue, so it never lands under a
	// turn that is already on its way.
	if setting.Category == "provider" && busy {
		return ErrBusy
	}
	ackErr := ErrControl
	_, err := s.requestWithReply(ctx, "session/set_config_option", map[string]string{"sessionId": acpID, "configId": id, "value": value}, func(raw json.RawMessage) {
		ackErr = s.applySettingReply(*setting, value, raw)
	})
	if err != nil {
		var refusal *rpcError
		known := errors.As(err, &refusal) && (refusal.Code == -32601 || refusal.Code == -32602)
		if !known {
			s.forgetSetting(id)
		}
		return err
	}
	return ackErr
}

func (s *Session) applySettingReply(setting terminalproto.ACPSetting, value string, raw json.RawMessage) error {
	id := setting.ID
	options, ok := readSettings(raw, s.opts.Agent.Provider)
	if ok {
		previousModel := s.modelSetting()
		s.updateSettings(raw)
		if setting.Category == "provider" {
			s.keepModelValues(options, previousModel)
		}
		for _, o := range options {
			matches := o.ID == id && o.Category == setting.Category
			if matches && o.Current == value {
				return nil
			}
		}
	}
	// Only the measured legacy model acknowledgement permits omission.
	if setting.Category == "model" && s.opts.Agent.AllowOmittedModelAck {
		if applied, confirmed := acknowledgedModel(raw, value, true); confirmed && applied == value {
			s.mu.Lock()
			s.model = applied
			s.mu.Unlock()
			return nil
		}
	}
	if !ok {
		s.forgetSetting(id)
	} else if setting.Category == "model" {
		found := false
		for _, o := range options {
			if o.Category == "model" {
				found = true
			}
		}
		if !found {
			s.mu.Lock()
			s.model = ""
			s.mu.Unlock()
		}
	}
	return ErrControl
}

// modelSetting is a copy of the current model setting, or nil.
func (s *Session) modelSetting() *terminalproto.ACPSetting {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.settings {
		if o.Category == "model" {
			copy := o
			return &copy
		}
	}
	return nil
}

// keepModelValues handles a provider reply that carries no model option: there
// is no list for the new source to learn, so the previous values stay and the
// current model is forgotten rather than guessed.
func (s *Session) keepModelValues(reply []terminalproto.ACPSetting, previous *terminalproto.ACPSetting) {
	if previous == nil {
		return
	}
	for _, o := range reply {
		if o.Category == "model" {
			return
		}
	}
	kept := *previous
	kept.Current = ""
	s.mu.Lock()
	s.settings = append(s.settings, kept)
	s.model = ""
	s.mu.Unlock()
}

func (s *Session) forgetSetting(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.settings {
		if s.settings[i].ID == id {
			s.settings[i].Current = ""
			if s.settings[i].Category == "model" {
				s.model = ""
			}
		}
	}
}

// Steer never resends an uncertain delivery. promptRequired is the adapter's
// explicit acknowledgement that these bytes were not consumed.
func (s *Session) Steer(ctx context.Context, text string, images []Attachment) (string, error) {
	if !s.steerMu.TryLock() {
		return "rejected", nil
	}
	defer s.steerMu.Unlock()
	state := s.Controls()
	valid := state.Steering && !state.Waiting && !state.Compacting && !state.Configuring
	if !valid {
		return "rejected", nil
	}
	if !state.Working {
		return "promptRequired", nil
	}
	s.turnMu.Lock()
	cancelled := s.cancelledSeq == s.turnSeq
	s.turnMu.Unlock()
	if cancelled {
		return "rejected", nil
	}
	s.mu.Lock()
	id, allowed := s.acpID, s.promptImages
	s.mu.Unlock()
	if len(images) > 0 && !allowed {
		return "rejected", ErrImages
	}
	blocks := []map[string]string{}
	if text != "" {
		blocks = append(blocks, map[string]string{"type": "text", "text": text})
	}
	for _, im := range images {
		blocks = append(blocks, map[string]string{"type": "image", "mimeType": im.MIME, "data": base64.StdEncoding.EncodeToString(im.Data)})
	}
	raw, err := s.request(ctx, "_session/steering", map[string]any{"sessionId": id, "prompt": blocks, "_meta": map[string]any{"steering": map[string]string{"idleBehavior": "promptRequired"}}})
	if err != nil {
		var refusal *rpcError
		unsupported := errors.As(err, &refusal) && (refusal.Code == -32601 || refusal.Code == -32602)
		if unsupported {
			return "rejected", nil
		}
		return "unknown", err
	}
	var result struct {
		Outcome string `json:"outcome"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return "unknown", nil
	}
	if result.Outcome == "injected" {
		s.markProgress()
		return "injected", nil
	}
	if result.Outcome == "promptRequired" {
		return "promptRequired", nil
	}
	return "unknown", nil
}
