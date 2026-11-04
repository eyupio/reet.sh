package terminalproto

import "strings"

const ACPPreviewsFeature = "acp-previews-v1"
const MaxACPPreviewBytes = 5 << 20
const MaxACPPreviewChunk = 32 << 10

func ValidACPPreviewID(id string) bool { return validUploadSHA256(id) }

func SupportsACPPreviews(features string) bool {
	for _, f := range strings.Split(features, ",") {
		if strings.TrimSpace(f) == ACPPreviewsFeature {
			return true
		}
	}
	return false
}

// ACPPreviewRef describes a snapshot supplied by the agent, never a browser-supplied file path.
type ACPPreviewRef struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	MIME   string `json:"mime,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (p ACPPreviewRef) Valid() bool {
	if p.Name == "" || !plainText(p.Name, 200) {
		return false
	}
	if p.Reason != "" {
		return p.ID == "" && p.MIME == "" && p.Size == 0 && (p.Reason == "unavailable" || p.Reason == "too_large" || p.Reason == "unsupported")
	}
	return validUploadSHA256(p.ID) && validUploadMIME(p.MIME) && p.Size > 0 && p.Size <= MaxACPPreviewBytes
}

// ACPPreview fetches one bounded part of a session's saved preview snapshot.
type ACPPreview struct {
	ID     string `json:"id"`
	Offset int64  `json:"offset"`
}

func (ACPPreview) kind() string { return "acp_preview" }
func (p ACPPreview) valid() bool {
	return validUploadSHA256(p.ID) && p.Offset >= 0 && p.Offset < MaxACPPreviewBytes
}

type ACPPreviewResult struct {
	ID     string `json:"id"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	MIME   string `json:"mime"`
	Data   []byte `json:"data"`
}

func (ACPPreviewResult) kind() string { return "acp_preview_result" }
func (p ACPPreviewResult) valid() bool {
	return validUploadSHA256(p.ID) && validUploadMIME(p.MIME) && p.Size > 0 && p.Size <= MaxACPPreviewBytes && p.Offset >= 0 && len(p.Data) > 0 && len(p.Data) <= MaxACPPreviewChunk && p.Offset+int64(len(p.Data)) <= p.Size
}
