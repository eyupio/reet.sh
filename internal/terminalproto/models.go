package terminalproto

import (
	"encoding/json"
	"strings"
)

// ModelsFeature lets a connector list the models its agent reports, without a
// controller-supplied argument or a launch.
const ModelsFeature = "models-v1"

// ModelsRefreshFeature lets a person-pressed refresh ask the connector to run
// a fresh listing instead of answering its short cache. The field is sent only
// to a connector that negotiated it: an older connector's strict decoder would
// refuse the request.
const ModelsRefreshFeature = "models-refresh-v1"

const (
	// MaxModels bounds one machine's listed models.
	MaxModels = 500
	// MaxModelsReplyBytes keeps a list inside the control frame with its
	// envelope; a longer list is trimmed and More is set.
	MaxModelsReplyBytes = 12 << 10
)

// SupportsModels reports whether the peer advertised models-v1.
func SupportsModels(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ModelsFeature {
			return true
		}
	}
	return false
}

// SupportsModelsRefresh reports whether the peer advertised models-refresh-v1.
func SupportsModelsRefresh(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ModelsRefreshFeature {
			return true
		}
	}
	return false
}

// Models asks the connector to run its own listing for one tool. Refresh asks
// for a fresh listing even when the connector's short cache would answer the
// last result; it is sent only to a connector that negotiated
// models-refresh-v1, because an older connector's strict decoder would refuse
// the field.
type Models struct {
	Tool    string `json:"tool"`
	Refresh bool   `json:"refresh,omitempty"`
}

func (Models) kind() string { return "models" }
func (m Models) valid() bool {
	return ValidToolV2(m.Tool)
}

// ModelEntry is one model the agent listed. Provider is the text before the
// first slash in ID, or empty when the id has no provider prefix.
type ModelEntry struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

// ModelsReason is a closed refusal or failure reason. A result with a reason
// carries no models, version or More flag.
type ModelsReason string

const (
	ModelsNotInstalled ModelsReason = "not_installed"
	ModelsUnsupported  ModelsReason = "unsupported"
	ModelsTimedOut     ModelsReason = "timed_out"
	ModelsTooLarge     ModelsReason = "too_large"
	ModelsExited       ModelsReason = "exited"
	ModelsUnreadable   ModelsReason = "unreadable"
	ModelsDenied       ModelsReason = "denied"
	ModelsConflict     ModelsReason = "conflict"
	ModelsOff          ModelsReason = "off"
)

func (r ModelsReason) Valid() bool {
	switch r {
	case ModelsNotInstalled, ModelsUnsupported, ModelsTimedOut, ModelsTooLarge, ModelsExited, ModelsUnreadable, ModelsDenied, ModelsConflict, ModelsOff:
		return true
	}
	return false
}

// ModelsResult is one connector listing, or the reason it could not list.
type ModelsResult struct {
	Tool    string       `json:"tool"`
	Models  []ModelEntry `json:"models,omitempty"`
	More    bool         `json:"more,omitempty"`
	Version string       `json:"version,omitempty"`
	Reason  ModelsReason `json:"reason,omitempty"`
}

func (ModelsResult) kind() string { return "models_result" }

func (r ModelsResult) valid() bool {
	invalidReason := r.Reason != "" && !r.Reason.Valid()
	if !ValidToolV2(r.Tool) || invalidReason {
		return false
	}
	if r.Reason != "" {
		return len(r.Models) == 0 && !r.More && r.Version == ""
	}
	validCount := len(r.Models) <= MaxModels
	validMore := !r.More || len(r.Models) > 0
	validVersion := r.Version == "" || plainText(r.Version, 64)
	valid := validCount && validMore && validVersion
	if !valid {
		return false
	}
	seen := make(map[string]bool, len(r.Models))
	for _, entry := range r.Models {
		validID := ValidModel(entry.ID) && !seen[entry.ID]
		if !validID || !plainText(entry.Provider, 64) {
			return false
		}
		provider := entry.ID
		if index := strings.IndexByte(provider, '/'); index >= 0 {
			provider = provider[:index]
		} else {
			provider = ""
		}
		if entry.Provider != provider {
			return false
		}
		seen[entry.ID] = true
	}
	data, err := json.Marshal(r)
	return err == nil && len(data) <= MaxModelsReplyBytes
}
