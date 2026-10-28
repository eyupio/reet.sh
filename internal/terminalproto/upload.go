package terminalproto

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// UploadFeature lets the controller send image bytes to a machine's private
// per-session upload folder.
const UploadFeature = "upload-v1"

const (
	// MaxUploadBytes is the largest image one upload may carry.
	MaxUploadBytes = 5 << 20
	// MaxUploadsPerSession and MaxUploadSessionBytes bound one session's folder.
	MaxUploadsPerSession  = 20
	MaxUploadSessionBytes = 50 << 20
	MaxUploadMachineBytes = 200 << 20
	// MaxUploadPathBytes bounds the absolute path an upload result may return.
	MaxUploadPathBytes = 4096
)

// SupportsUpload reports whether the peer advertised upload-v1 in
// X-Reet-Terminal-Features. Capabilities.Uploads and Upload messages are never
// set without it.
func SupportsUpload(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == UploadFeature {
			return true
		}
	}
	return false
}

// Upload asks the machine to accept Size bytes as a private image file. The
// bytes follow as binary frames on the reverse stream; the controller supplies
// no name and no path.
type Upload struct {
	StreamToken string `json:"streamToken"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	MIME        string `json:"mime"`
}

func (Upload) kind() string { return "upload" }
func (u Upload) valid() bool {
	return ValidID(u.StreamToken) && u.Size >= 1 && u.Size <= MaxUploadBytes &&
		validUploadSHA256(u.SHA256) && validUploadMIME(u.MIME)
}

func validUploadSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validUploadMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// UploadCode is the outcome of an upload, reported in the final text frame on
// the reverse stream.
type UploadCode string

const (
	UploadOK       UploadCode = "ok"
	UploadDisabled UploadCode = "disabled"
	UploadTooLarge UploadCode = "too_large"
	UploadType     UploadCode = "type"
	UploadQuota    UploadCode = "quota"
	UploadMismatch UploadCode = "mismatch"
	UploadFailed   UploadCode = "failed"
	UploadConflict UploadCode = "conflict"
)

func (c UploadCode) Valid() bool {
	switch c {
	case UploadOK, UploadDisabled, UploadTooLarge, UploadType, UploadQuota, UploadMismatch, UploadFailed, UploadConflict:
		return true
	}
	return false
}

// UploadResult is the final text frame on the reverse stream. Path is the
// absolute location of the stored image and is set only when Code is ok.
type UploadResult struct {
	Code UploadCode `json:"code"`
	Path string     `json:"path,omitempty"`
}

func (r UploadResult) Validate() error {
	if !r.Code.Valid() {
		return ErrInvalid
	}
	if r.Code == UploadOK {
		if !strings.HasPrefix(r.Path, "/") || len(r.Path) > MaxUploadPathBytes || !plainText(r.Path, MaxUploadPathBytes) {
			return ErrInvalid
		}
	} else if r.Path != "" {
		return ErrInvalid
	}
	return nil
}

// DecodeUploadResult decodes and validates the upload's final text frame.
func DecodeUploadResult(data []byte) (UploadResult, error) {
	var r UploadResult
	if len(data) == 0 || len(data) > MaxControlBytes || !utf8.Valid(data) || !uniqueKeys(data) ||
		strictJSON(data, &r) != nil || r.Validate() != nil {
		return UploadResult{}, ErrInvalid
	}
	return r, nil
}

// SniffUploadMIME identifies the accepted image types from their magic bytes.
// It rejects SVG, HTML and anything else, including a truncated header.
func SniffUploadMIME(header []byte) (mime, ext string, ok bool) {
	switch {
	case bytes.HasPrefix(header, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", "png", true
	case bytes.HasPrefix(header, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg", "jpg", true
	case bytes.HasPrefix(header, []byte("GIF87a")), bytes.HasPrefix(header, []byte("GIF89a")):
		return "image/gif", "gif", true
	case len(header) >= 12 && bytes.Equal(header[:4], []byte("RIFF")) && bytes.Equal(header[8:12], []byte("WEBP")):
		return "image/webp", "webp", true
	}
	return "", "", false
}
