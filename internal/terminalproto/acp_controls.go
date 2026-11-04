package terminalproto

import (
	"encoding/json"
	"slices"
	"strings"
)

const ACPLiveControlsFeature = "acp-live-controls-v1"

// ACPStartOptionsFeature negotiates structured create effort. A connector with
// live controls alone still uses the older create validation contract.
const ACPStartOptionsFeature = "acp-start-options-v1"

func SupportsACPStartOptions(features string) bool {
	for _, f := range strings.Split(features, ",") {
		if strings.TrimSpace(f) == ACPStartOptionsFeature {
			return true
		}
	}
	return false
}

func SupportsACPLiveControls(features string) bool {
	for _, f := range strings.Split(features, ",") {
		if strings.TrimSpace(f) == ACPLiveControlsFeature {
			return true
		}
	}
	return false
}

// ACPSetting is a session's current non-permission selector. Values are IDs,
// never commands. Missing current means the last switch was not confirmed.
type ACPSetting struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Current  string   `json:"current"`
	Values   []string `json:"values"`
}

type ACPControls struct {
	Configuring bool         `json:"configuring"`
	Working     bool         `json:"working"`
	Waiting     bool         `json:"waiting"`
	Compacting  bool         `json:"compacting"`
	Steering    bool         `json:"steering"`
	Model       string       `json:"model"`
	Options     []ACPSetting `json:"options"`
}

func (s ACPControls) Valid() bool {
	tooMany := len(s.Options) > 2
	invalidModel := s.Model != "" && !ValidModel(s.Model)
	if tooMany || invalidModel {
		return false
	}
	ids, categories := map[string]bool{}, map[string]bool{}
	for _, o := range s.Options {
		validID := validAgentOptionToken(o.ID) && !ids[o.ID]
		validCategory := !categories[o.Category] &&
			(o.Category == "model" || o.Category == "thought_level")
		validValues := len(o.Values) <= 16
		valid := validID && validCategory && validValues
		if !valid {
			return false
		}
		ids[o.ID], categories[o.Category] = true, true
		if o.Current != "" && !ValidModel(o.Current) {
			return false
		}
		seen := map[string]bool{}
		for _, v := range o.Values {
			valid := v != "" && ValidModel(v) && !seen[v]
			if !valid {
				return false
			}
			seen[v] = true
		}
	}
	b, err := json.Marshal(s)
	return err == nil && len(b) <= MaxACPEventBytes
}

type ACPControl struct {
	Action   string   `json:"action"`
	ConfigID string   `json:"configId,omitempty"`
	Value    string   `json:"value,omitempty"`
	Text     string   `json:"text,omitempty"`
	Images   []string `json:"images,omitempty"`
}

func (ACPControl) kind() string { return "acp_control" }
func (c ACPControl) valid() bool {
	switch c.Action {
	case "get":
		return c.ConfigID == "" && c.Value == "" && c.Text == "" && len(c.Images) == 0
	case "set":
		return validAgentOptionToken(c.ConfigID) && c.Value != "" && ValidModel(c.Value) && c.Text == "" && len(c.Images) == 0
	case "steer":
		return c.ConfigID == "" && c.Value == "" && (ACPPrompt{Text: c.Text, Images: c.Images}).valid()
	}
	return false
}

type ACPControlResult struct {
	Error    string      `json:"error,omitempty"`
	Controls ACPControls `json:"controls"`
	Outcome  string      `json:"outcome,omitempty"`
}

func (ACPControlResult) kind() string { return "acp_control_result" }
func (r ACPControlResult) valid() bool {
	return validChatText(r.Error, 500) && r.Controls.Valid() && slices.Contains([]string{"", "injected", "promptRequired", "rejected", "unknown"}, r.Outcome)
}
