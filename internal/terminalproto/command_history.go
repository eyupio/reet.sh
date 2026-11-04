package terminalproto

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

const FeatureHeader = "X-Reet-Terminal-Features"
const CommandHistoryFeature = "command-history-v1"
const MaxCommandHistoryEntries = 500
const MaxCommandHistoryBytes = 256 << 10
const MaxCommandBytes = 16 << 10

// The authenticated HTTP upgrade advertises extensions before the strict V1
// handshake. New agents omit new capability fields when talking to old servers.
func SupportsCommandHistory(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == CommandHistoryFeature {
			return true
		}
	}
	return false
}

type CommandHistory struct {
	StreamToken string `json:"streamToken"`
}

func (CommandHistory) kind() string  { return "command_history" }
func (h CommandHistory) valid() bool { return ValidID(h.StreamToken) }

type CommandHistoryEntry struct {
	Command   string `json:"command"`
	Source    string `json:"source"`
	Timestamp int64  `json:"timestamp,omitempty"`
}
type CommandHistorySource struct {
	Source    string `json:"source"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}
type CommandHistorySnapshot struct {
	Entries   []CommandHistoryEntry  `json:"entries"`
	Sources   []CommandHistorySource `json:"sources"`
	Truncated bool                   `json:"truncated"`
	Skipped   int                    `json:"skipped"`
}

func ValidCommandText(text string) bool {
	valid := utf8.ValidString(text) && len(text) <= MaxCommandBytes && strings.TrimSpace(text) != ""
	if !valid {
		return false
	}
	for _, r := range text {
		allowedWhitespace := r == '\n' || r == '\r' || r == '\t'
		if unicode.IsControl(r) && !allowedWhitespace {
			return false
		}
	}
	return true
}
func (h CommandHistorySnapshot) Validate() error {
	validSlices := h.Entries != nil && h.Sources != nil
	validCounts := len(h.Entries) <= MaxCommandHistoryEntries && len(h.Sources) <= 2
	validSkipped := h.Skipped >= 0 && h.Skipped <= 1<<20
	valid := validSlices && validCounts && validSkipped
	if !valid {
		return ErrInvalid
	}
	sources := map[string]bool{}
	seen := map[string]bool{}
	for _, source := range h.Sources {
		validSource := source.Source == "bash" || source.Source == "zsh"
		uniqueSource := !seen[source.Source]
		validReason := plainText(source.Reason, 200)
		valid := validSource && uniqueSource && validReason
		if !valid {
			return ErrInvalid
		}
		seen[source.Source] = true
		sources[source.Source] = source.Available
	}
	for _, entry := range h.Entries {
		knownSource := sources[entry.Source]
		validCommand := ValidCommandText(entry.Command)
		validTime := entry.Timestamp >= 0 && entry.Timestamp <= 253402300799999
		valid := knownSource && validCommand && validTime
		if !valid {
			return ErrInvalid
		}
	}
	return nil
}
func EncodeCommandHistory(h CommandHistorySnapshot) ([]byte, error) {
	if h.Validate() != nil {
		return nil, ErrInvalid
	}
	data, err := json.Marshal(h)
	if err != nil || len(data) > MaxCommandHistoryBytes {
		return nil, ErrInvalid
	}
	return data, nil
}
func DecodeCommandHistory(data []byte) (CommandHistorySnapshot, error) {
	var h CommandHistorySnapshot
	validData := len(data) > 0 && len(data) <= MaxCommandHistoryBytes && utf8.Valid(data)
	validJSON := uniqueKeys(data) && strictJSON(data, &h) == nil && h.Validate() == nil
	if !validData || !validJSON {
		return h, ErrInvalid
	}
	return h, nil
}
