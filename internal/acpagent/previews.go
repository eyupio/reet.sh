//go:build linux || darwin

package acpagent

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Preview is image output already returned or explicitly named by the agent.
// Path is consumed only inside the connector; it never becomes a browser fetch path.
type Preview struct {
	Data                     []byte
	MIME, Path, Name, Reason string
}

func previewPath(uri string) string {
	if strings.HasPrefix(uri, "file:") {
		u, err := url.Parse(uri)
		if err != nil || u.Host != "" {
			return ""
		}
		uri = u.Path
	}
	if strings.Contains(uri, "://") || strings.ContainsAny(uri, "\x00\r\n") {
		return ""
	}
	switch strings.ToLower(filepath.Ext(uri)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return uri
	}
	return ""
}

func contentPreviews(raw json.RawMessage) []Preview {
	var out []Preview
	var visit func(json.RawMessage, int)
	visit = func(raw json.RawMessage, depth int) {
		tooDeep := depth > 3
		tooMany := len(out) >= 8
		atLimit := tooDeep || tooMany
		if atLimit || len(raw) == 0 {
			return
		}
		if raw[0] == '[' {
			var entries []json.RawMessage
			if json.Unmarshal(raw, &entries) == nil {
				for i, entry := range entries {
					if i >= 32 {
						break
					}
					visit(entry, depth+1)
				}
			}
			return
		}
		var b struct {
			Type     string          `json:"type"`
			MIME     string          `json:"mimeType"`
			Data     string          `json:"data"`
			URI      string          `json:"uri"`
			Name     string          `json:"name"`
			Content  json.RawMessage `json:"content"`
			Resource *struct {
				URI  string `json:"uri"`
				MIME string `json:"mimeType"`
				Blob string `json:"blob"`
			} `json:"resource"`
		}
		if json.Unmarshal(raw, &b) != nil {
			return
		}
		if b.Type == "content" {
			visit(b.Content, depth+1)
			return
		}
		if b.Type == "resource" && b.Resource != nil {
			b.MIME, b.Data, b.URI = b.Resource.MIME, b.Resource.Blob, b.Resource.URI
		}
		valid := b.Type == "image" || b.Type == "resource" || b.Type == "resource_link"
		if !valid {
			return
		}
		path := previewPath(b.URI)
		if !strings.HasPrefix(b.MIME, "image/") && path == "" {
			return
		}
		name := b.Name
		if name == "" && path != "" {
			name = filepath.Base(path)
		}
		if name == "" {
			name = "Agent image"
		}
		p := Preview{Name: name, MIME: b.MIME, Path: path}
		if b.Data != "" {
			if len(b.Data) > base64.StdEncoding.EncodedLen(terminalproto.MaxACPPreviewBytes) {
				p.Reason = "too_large"
			} else if data, err := base64.StdEncoding.Strict().DecodeString(b.Data); err == nil {
				mime, _, ok := terminalproto.SniffUploadMIME(data)
				validMime := b.MIME == "" || mime == b.MIME
				if !ok || !validMime {
					p.Reason = "unsupported"
				} else {
					p.Data = data
					p.MIME = mime
				}
			} else {
				p.Reason = "unavailable"
			}
		} else if path == "" {
			p.Reason = "unavailable"
		}
		out = append(out, p)
	}
	visit(raw, 0)
	return out
}

var markdownPreview = regexp.MustCompile(`!\[[^\]\r\n]{0,200}\]\(([^)\r\n]{1,1000})\)`)

func (s *Session) messagePreviews(id, text string) []Preview {
	// Bound partial Markdown retained for streamed references independently of conversation history.
	s.mu.Lock()
	if s.previewText == nil {
		s.previewText = map[string]string{}
	}
	if len(s.previewText) >= 32 && s.previewText[id] == "" {
		clear(s.previewText)
	}
	previous := s.previewText[id]
	current := previous + text
	if len(current) > 16<<10 {
		current = current[len(current)-(16<<10):]
	}
	s.previewText[id] = current
	s.mu.Unlock()
	var out []Preview
	for _, match := range markdownPreview.FindAllStringSubmatch(current, 8) {
		if strings.Contains(previous, match[0]) {
			continue
		}
		if path := previewPath(strings.Trim(match[1], "<>")); path != "" {
			out = append(out, Preview{Path: path, Name: filepath.Base(path)})
		}
	}
	return out
}

func (s *Session) emitPreviews(messageID, toolID string, pictures []Preview) {
	for i := range pictures {
		p := pictures[i]
		s.emit(Event{Kind: "preview", MessageID: messageID, ToolCallID: toolID, Preview: &p})
	}
}

type previewTarget struct{ path, kind string }

func (s *Session) previewTarget(id, kind, path, status string) string {
	if id == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.previewTools == nil {
		s.previewTools = map[string]previewTarget{}
	}
	target := s.previewTools[id]
	if kind != "" {
		target.kind = kind
	}
	if path != "" {
		target.path = path
	}
	final := status == "completed" || status == "failed" || status == "cancelled"
	if final {
		delete(s.previewTools, id)
		kindOK := target.kind == "read" || target.kind == "edit"
		if status == "completed" && kindOK {
			return previewPath(target.path)
		}
		return ""
	}
	if len(s.previewTools) >= 512 {
		clear(s.previewTools)
	}
	s.previewTools[id] = target
	return ""
}
