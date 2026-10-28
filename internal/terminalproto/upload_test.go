package terminalproto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func validUpload() Upload {
	return Upload{
		StreamToken: testID,
		Size:        1024,
		SHA256:      strings.Repeat("a", 64),
		MIME:        "image/png",
	}
}

func TestUploadFeatureToken(t *testing.T) {
	for _, value := range []string{"", "launch-v1", "upload-v10", "upload-v1x", "prefix-upload-v1", "Upload-v1"} {
		if SupportsUpload(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsUpload("launch-v1, upload-v1") || !SupportsUpload(" upload-v1 ") || UploadFeature != "upload-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestUploadRoundTripMaximalIsExact(t *testing.T) {
	u := Upload{StreamToken: testID, Size: MaxUploadBytes, SHA256: strings.Repeat("0123456789abcdef", 4), MIME: "image/webp"}
	encoded, err := Encode(testMessage(u))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.Body.(*Upload)
	if !ok {
		t.Fatalf("wrong body type %T", decoded.Body)
	}
	if *got != u {
		t.Fatalf("round trip changed body: %+v", got)
	}
	again, err := Encode(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("round trip changed", string(encoded), string(again), err)
	}
}

func TestUploadValidationBoundaries(t *testing.T) {
	valid := validUpload()
	cases := map[string]func(*Upload){
		"valid minimum size": func(u *Upload) { u.Size = 1 },
		"valid maximum size": func(u *Upload) { u.Size = MaxUploadBytes },
		"valid png":          func(u *Upload) { u.MIME = "image/png" },
		"valid jpeg":         func(u *Upload) { u.MIME = "image/jpeg" },
		"valid gif":          func(u *Upload) { u.MIME = "image/gif" },
		"valid webp":         func(u *Upload) { u.MIME = "image/webp" },
		"valid sha":          func(u *Upload) { u.SHA256 = strings.Repeat("0", 63) + "f" },

		"size zero":            func(u *Upload) { u.Size = 0 },
		"size over maximum":    func(u *Upload) { u.Size = MaxUploadBytes + 1 },
		"size negative":        func(u *Upload) { u.Size = -1 },
		"empty token":          func(u *Upload) { u.StreamToken = "" },
		"short token":          func(u *Upload) { u.StreamToken = "short" },
		"unsafe token":         func(u *Upload) { u.StreamToken = "../../etc/passwd000000" },
		"sha 63":               func(u *Upload) { u.SHA256 = strings.Repeat("a", 63) },
		"sha 65":               func(u *Upload) { u.SHA256 = strings.Repeat("a", 65) },
		"sha uppercase":        func(u *Upload) { u.SHA256 = strings.Repeat("A", 64) },
		"sha mixed uppercase":  func(u *Upload) { u.SHA256 = strings.Repeat("a", 63) + "A" },
		"sha non hex":          func(u *Upload) { u.SHA256 = strings.Repeat("g", 64) },
		"mime svg":             func(u *Upload) { u.MIME = "image/svg+xml" },
		"mime html":            func(u *Upload) { u.MIME = "text/html" },
		"mime empty":           func(u *Upload) { u.MIME = "" },
		"mime uppercase":       func(u *Upload) { u.MIME = "IMAGE/PNG" },
		"mime with parameters": func(u *Upload) { u.MIME = "image/png; charset=utf-8" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			u := valid
			mutate(&u)
			_, err := Encode(testMessage(u))
			wantValid := strings.HasPrefix(name, "valid")
			if wantValid && err != nil {
				t.Fatalf("valid upload refused: %v", err)
			}
			if !wantValid && err == nil {
				t.Fatal("invalid upload accepted")
			}
		})
	}
}

func TestUploadStrictnessAndBinding(t *testing.T) {
	u := validUpload()
	encoded, err := Encode(testMessage(u))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(u)
	cases := map[string][]byte{
		"unknown body field": bytes.Replace(encoded, []byte(`"size":`), []byte(`"extra":1,"size":`), 1),
		"unknown envelope":   bytes.Replace(encoded, []byte(`"version":1`), []byte(`"version":1,"token":"secret"`), 1),
		"null body":          bytes.Replace(encoded, body, []byte(`null`), 1),
		"duplicate body":     bytes.Replace(encoded, []byte(`"size":`), []byte(`"size":1024,"size":`), 1),
		"case alias body":    bytes.Replace(encoded, []byte(`"size":`), []byte(`"size":1024,"SIZE":`), 1),
		"trailing json":      append(append([]byte{}, encoded...), []byte(` {}`)...),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(data); err == nil {
				t.Fatal("ambiguous upload accepted")
			}
		})
	}

	withoutSession := testMessage(u)
	withoutSession.SessionID = ""
	if err := withoutSession.Validate(); err == nil {
		t.Fatal("upload without a session accepted")
	}
	withoutAttachment := testMessage(u)
	withoutAttachment.AttachmentID = ""
	withoutAttachment.AttachmentGeneration = 0
	if err := withoutAttachment.Validate(); err == nil {
		t.Fatal("upload without an attachment fence accepted")
	}
	if _, err := Encode(withoutAttachment); err == nil {
		t.Fatal("encoded upload without an attachment fence")
	}
}

func TestUploadResultValidation(t *testing.T) {
	path := "/home/u/.local/state/reet/uploads/0123456789abcdef/img-0123456789abcdef0123456789abcdef.png"
	if err := (UploadResult{Code: UploadOK, Path: path}).Validate(); err != nil {
		t.Fatal(err)
	}
	allCodes := []UploadCode{UploadOK, UploadDisabled, UploadTooLarge, UploadType, UploadQuota, UploadMismatch, UploadFailed, UploadConflict}
	for _, code := range allCodes {
		if !code.Valid() {
			t.Fatal("code not valid", code)
		}
	}
	for _, code := range allCodes[1:] {
		if err := (UploadResult{Code: code}).Validate(); err != nil {
			t.Fatal("empty path refused", code, err)
		}
		if err := (UploadResult{Code: code, Path: path}).Validate(); err == nil {
			t.Fatal("non-ok result with a path accepted", code)
		}
	}
	longest := "/" + strings.Repeat("a", MaxUploadPathBytes-1)
	if err := (UploadResult{Code: UploadOK, Path: longest}).Validate(); err != nil {
		t.Fatal("path at the limit refused", err)
	}
	invalid := map[string]UploadResult{
		"ok empty path":       {Code: UploadOK},
		"ok relative path":    {Code: UploadOK, Path: "home/u/x.png"},
		"ok control char":     {Code: UploadOK, Path: "/home/u/\x00x.png"},
		"ok format char":      {Code: UploadOK, Path: "/home/u/\u200bx.png"},
		"ok invalid utf8":     {Code: UploadOK, Path: "/home/u/\xff.png"},
		"ok too long":         {Code: UploadOK, Path: "/" + strings.Repeat("a", MaxUploadPathBytes)},
		"unknown code":        {Code: UploadCode("unknown")},
		"empty code":          {},
		"ok line separator":   {Code: UploadOK, Path: "/home/u/\u2028x.png"},
		"disabled with path":  {Code: UploadDisabled, Path: path},
		"too large with path": {Code: UploadTooLarge, Path: path},
	}
	for name, result := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := result.Validate(); err == nil {
				t.Fatal("invalid result accepted")
			}
		})
	}
}

func TestDecodeUploadResultIsStrict(t *testing.T) {
	valid, err := json.Marshal(UploadResult{Code: UploadOK, Path: "/home/u/x.png"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeUploadResult(valid)
	if err != nil || got != (UploadResult{Code: UploadOK, Path: "/home/u/x.png"}) {
		t.Fatalf("decode failed: %+v %v", got, err)
	}
	failure, _ := json.Marshal(UploadResult{Code: UploadQuota})
	if got, err := DecodeUploadResult(failure); err != nil || got.Code != UploadQuota || got.Path != "" {
		t.Fatalf("failure decode: %+v %v", got, err)
	}
	cases := map[string][]byte{
		"unknown field":  bytes.Replace(valid, []byte(`{"code"`), []byte(`{"reason":"x","code"`), 1),
		"duplicate":      bytes.Replace(valid, []byte(`"code":"ok"`), []byte(`"code":"ok","code":"ok"`), 1),
		"case alias":     bytes.Replace(valid, []byte(`"path":`), []byte(`"path":"/a","PATH":"/b","path":`), 1),
		"trailing":       append(append([]byte{}, valid...), []byte(` {}`)...),
		"null":           []byte(`null`),
		"empty":          nil,
		"unknown code":   []byte(`{"code":"unknown"}`),
		"ok no path":     []byte(`{"code":"ok"}`),
		"relative path":  []byte(`{"code":"ok","path":"home/u/x.png"}`),
		"failure path":   []byte(`{"code":"failed","path":"/home/u/x.png"}`),
		"oversized":      bytes.Repeat([]byte(" "), MaxControlBytes+1),
		"not an object":  []byte(`[]`),
		"no code string": []byte(`{"code":1}`),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeUploadResult(data); err == nil {
				t.Fatal("invalid upload result accepted")
			}
		})
	}
}

func TestSniffUploadMIME(t *testing.T) {
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBP"), make([]byte, 4)...)
	valid := []struct {
		name string
		data []byte
		mime string
		ext  string
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n"), "image/png", "png"},
		{"jpeg", []byte{0xff, 0xd8, 0xff, 0xe0}, "image/jpeg", "jpg"},
		{"gif87a", []byte("GIF87a\x01\x00"), "image/gif", "gif"},
		{"gif89a", []byte("GIF89a\x01\x00"), "image/gif", "gif"},
		{"webp", webp, "image/webp", "webp"},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			mime, ext, ok := SniffUploadMIME(test.data)
			if !ok || mime != test.mime || ext != test.ext {
				t.Fatalf("got %q %q %v", mime, ext, ok)
			}
		})
	}
	invalid := map[string][]byte{
		"svg xml":        []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"svg tag":        []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"html":           []byte(`<!doctype html><html></html>`),
		"empty":          nil,
		"short png":      []byte("\x89PNG"),
		"short webp":     []byte("RIFF\x00\x00\x00\x00WEB"),
		"wrong riff":     []byte("RIFX\x00\x00\x00\x00WEBP"),
		"wrong webp tag": []byte("RIFF\x00\x00\x00\x00WEBX"),
		"jpeg too short": []byte{0xff, 0xd8},
	}
	for name, data := range invalid {
		t.Run(name, func(t *testing.T) {
			if mime, ext, ok := SniffUploadMIME(data); ok || mime != "" || ext != "" {
				t.Fatalf("non-image accepted: %q %q %v", mime, ext, ok)
			}
		})
	}
}

func TestCapabilitiesUploadsNeedTerminalControlAndNeverCoexist(t *testing.T) {
	c := testCapabilities()
	legacy, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(legacy, []byte(`"uploads"`)) || bytes.Contains(legacy, []byte(`"uploadsOff"`)) {
		t.Fatal("upload capabilities sent when unset", string(legacy))
	}

	c.Uploads = true
	if err := c.Validate(); err == nil {
		t.Fatal("uploads without terminal control accepted")
	}
	c.TerminalControl = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	// The negotiated token is checked where capabilities are built, not in
	// Validate or ValidateFor: capability validation runs before a feature
	// list exists on several paths.
	if err := c.ValidateFor(""); err != nil {
		t.Fatal("ValidateFor enforced the upload token", err)
	}
	on, _ := json.Marshal(c)
	if !bytes.Contains(on, []byte(`"uploads":true`)) || bytes.Contains(on, []byte(`"uploadsOff"`)) {
		t.Fatal(string(on))
	}

	c.Uploads = false
	c.UploadsOff = true
	if err := c.Validate(); err != nil {
		t.Fatal("uploadsOff refused", err)
	}
	off, _ := json.Marshal(c)
	if !bytes.Contains(off, []byte(`"uploadsOff":true`)) || bytes.Contains(off, []byte(`"uploads":`)) {
		t.Fatal(string(off))
	}

	c.Uploads = true
	if err := c.Validate(); err == nil {
		t.Fatal("uploads and uploadsOff together accepted")
	}
	c.Uploads, c.TerminalControl = false, false
	if err := c.Validate(); err == nil {
		t.Fatal("uploadsOff without terminal control accepted")
	}
}

func TestStreamHelloAcceptsUploadOperation(t *testing.T) {
	h := StreamHello{
		Version: Version, RequestID: testID, SessionID: testID, AttachmentID: testID,
		Generation: 1, AttachmentGeneration: 1, Token: testID, Operation: "upload",
	}
	if err := h.Validate(); err != nil {
		t.Fatal("upload stream hello refused", err)
	}
	for _, operation := range []string{"attach", "history"} {
		h.Operation = operation
		if err := h.Validate(); err != nil {
			t.Fatal("existing operation refused", operation, err)
		}
	}
	encoded, err := json.Marshal(StreamHello{
		Version: Version, RequestID: testID, SessionID: testID, AttachmentID: testID,
		Generation: 1, AttachmentGeneration: 1, Token: testID, Operation: "upload",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeStreamHello(encoded); err != nil {
		t.Fatal(err)
	}
	h.Operation = "copy"
	if err := h.Validate(); err == nil {
		t.Fatal("unknown operation accepted")
	}
	if _, err := DecodeStreamHello(bytes.Replace(encoded, []byte(`"operation":"upload"`), []byte(`"operation":"copy"`), 1)); err == nil {
		t.Fatal("unknown operation decoded")
	}
	if _, err := DecodeStreamHello(bytes.Replace(encoded, []byte(`{"version"`), []byte(`{"extra":1,"version"`), 1)); err == nil {
		t.Fatal("unknown stream hello field accepted")
	}
}
