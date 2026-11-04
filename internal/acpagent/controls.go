//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrControl = errors.New("the agent did not confirm that setting")

// readSettings admits only non-permission selectors from the live session.
// Static installation descriptors never authorise a configuration change.
func readSettings(raw json.RawMessage) ([]terminalproto.ACPSetting, bool) {
	var reply struct {
		Options json.RawMessage `json:"configOptions"`
	}
	if json.Unmarshal(raw, &reply) != nil || len(reply.Options) == 0 {
		return nil, false
	}
	var options []struct {
		ID       string          `json:"id"`
		Category string          `json:"category"`
		Current  string          `json:"currentValue"`
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
		if o.ID == "mode" || (o.ID == "model" && category != "model") {
			continue
		}
		if category != "model" && category != "thought_level" {
			continue
		}
		setting := terminalproto.ACPSetting{ID: o.ID, Category: category, Current: o.Current, Values: []string{}}
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
			if len(setting.Values) < 16 && v != "" && !slices.Contains(setting.Values, v) {
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
	if options, ok := readSettings(raw); ok {
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
	state := terminalproto.ACPControls{Configuring: s.configuring, Working: s.working, Waiting: len(s.pending) > 0 || len(s.questions) > 0, Compacting: s.compacting, Steering: s.steering, Model: s.model, Options: []terminalproto.ACPSetting{}}
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
	if setting == nil || value == "" || !terminalproto.ValidModel(value) || setting.Category == "thought_level" && !slices.Contains(setting.Values, value) {
		return ErrControl
	}
	raw, err := s.request(ctx, "session/set_config_option", map[string]string{"sessionId": acpID, "configId": id, "value": value})
	if err != nil {
		var refusal *rpcError
		if !errors.As(err, &refusal) || refusal.Code != -32601 && refusal.Code != -32602 {
			s.forgetSetting(id)
		}
		return err
	}
	options, ok := readSettings(raw)
	if ok {
		s.updateSettings(raw)
		for _, o := range options {
			if o.ID == id && o.Category == setting.Category && o.Current == value {
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
	if !state.Steering || state.Waiting || state.Compacting || state.Configuring {
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
		if errors.As(err, &refusal) && (refusal.Code == -32601 || refusal.Code == -32602) {
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
