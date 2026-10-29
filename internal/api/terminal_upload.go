package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	// uploadRateLimit is the images one account may send per minute.
	uploadRateLimit = 20
	// uploadInFlightLimit bounds the images one account may upload at once.
	uploadInFlightLimit = 2
	uploadRateWindow    = time.Minute
	// uploadRequestTimeout bounds one image from the controller's side.
	uploadRequestTimeout = 30 * time.Second
)

var uploadSafePath = regexp.MustCompile(`^[A-Za-z0-9_./:@%+-]+$`)

// uploadLimits holds the per-account rate and concurrency limits. Its zero
// value is ready: the limiter is made on first use, and nothing is global.
type uploadLimits struct {
	once     sync.Once
	mu       sync.Mutex
	rate     *auth.Limiter
	inFlight map[int64]int
}

func (l *uploadLimits) init() {
	l.once.Do(func() {
		l.rate = auth.NewLimiter(uploadRateLimit, uploadRateWindow)
		l.inFlight = map[int64]int{}
	})
}

// begin takes an in-flight slot and a rate hit for uid. A non-empty reason is
// the error code to answer with; otherwise release must be called when the
// request is done.
func (l *uploadLimits) begin(uid int64) (release func(), reason string) {
	l.init()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight[uid] >= uploadInFlightLimit {
		return nil, "upload_busy"
	}
	key := strconv.FormatInt(uid, 10)
	if !l.rate.Allow(key) {
		return nil, "upload_rate_limited"
	}
	l.rate.Hit(key)
	l.inFlight[uid]++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.inFlight[uid]--
		if l.inFlight[uid] == 0 {
			delete(l.inFlight, uid)
		}
	}, ""
}

// terminalUploadPath names the routes allowed to write a non-JSON body: one
// session's images and a machine's launch images.
func terminalUploadPath(path string) bool {
	if strings.HasPrefix(path, "/api/v2/machines/") && strings.HasSuffix(path, "/uploads") {
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v2/machines/"), "/uploads")
		return id != "" && !strings.Contains(id, "/")
	}
	const prefix = "/api/v2/terminal/sessions/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, "/uploads") {
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), "/uploads")
	return id != "" && !strings.Contains(id, "/")
}

func (s *server) terminalUpload(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	declared := r.Header.Get("Content-Type")
	switch declared {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		uploadError(w, terminalbroker.ErrUploadType)
		return
	}
	uid := s.uid(r)
	session, err := s.st.TerminalSessionFor(uid, r.PathValue("id"))
	if err != nil || session.State != terminalproto.Running {
		terminalError(w, terminalbroker.ErrDenied)
		return
	}
	release, reason := s.uploadLimits.begin(uid)
	if reason != "" {
		if reason == "upload_busy" {
			uploadError(w, terminalbroker.ErrUploadBusy)
		} else {
			writeErrCode(w, http.StatusTooManyRequests, reason, "", "Too many images at once. Slow down a moment, then try again.", "")
		}
		return
	}
	defer release()
	// Refuse an old or switched-off machine before reading a five-megabyte
	// body. The broker repeats this check against its live peer generation.
	caps, err := s.st.RemoteCapabilitiesFor(uid, session.AgentID)
	if err != nil {
		uploadError(w, terminalbroker.ErrOffline)
		return
	}
	if !caps.Capabilities.Uploads {
		if caps.Capabilities.UploadsOff {
			uploadError(w, terminalbroker.ErrUploadsOff)
		} else {
			uploadError(w, terminalbroker.ErrUploadUnsupported)
		}
		return
	}
	data, mime, ok := s.readUploadImage(w, r, declared)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), uploadRequestTimeout)
	defer cancel()
	path, err := s.terminals.Upload(ctx, uid, session.ID, data, mime)
	if err != nil {
		uploadError(w, err)
		return
	}
	s.auditTerminalUpload(uid, session, mime, len(data))
	// The path is machine-generated and validated as plain text. It travels in
	// ASCII-only headers, escaped, so the browser gets it without the shared
	// JSON response writer carrying a machine value into the body.
	w.Header().Set("X-Reet-Upload-Path", url.PathEscape(path))
	w.Header().Set("X-Reet-Upload-Reference", url.PathEscape(uploadReference(path)))
	writeJSON(w, http.StatusOK, map[string]string{"uploaded": "true"})
}

// terminalLaunchUpload accepts an image for a session that is about to start
// on the named machine. Nothing is stored on the controller: the bytes go
// straight to the machine's private launch folder and the reply names the path
// the opening prompt should carry.
func (s *server) terminalLaunchUpload(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	declared := r.Header.Get("Content-Type")
	switch declared {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		uploadError(w, terminalbroker.ErrUploadType)
		return
	}
	uid := s.uid(r)
	agent := r.PathValue("agentId")
	release, reason := s.uploadLimits.begin(uid)
	if reason != "" {
		if reason == "upload_busy" {
			uploadError(w, terminalbroker.ErrUploadBusy)
		} else {
			writeErrCode(w, http.StatusTooManyRequests, reason, "", "Too many images at once. Slow down a moment, then try again.", "")
		}
		return
	}
	defer release()
	caps, err := s.st.RemoteCapabilitiesFor(uid, agent)
	if err != nil {
		uploadError(w, terminalbroker.ErrOffline)
		return
	}
	if !caps.Capabilities.Uploads {
		if caps.Capabilities.UploadsOff {
			uploadError(w, terminalbroker.ErrUploadsOff)
		} else {
			uploadError(w, terminalbroker.ErrUploadUnsupported)
		}
		return
	}
	data, mime, ok := s.readUploadImage(w, r, declared)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), uploadRequestTimeout)
	defer cancel()
	path, err := s.terminals.UploadLaunch(ctx, uid, agent, data, mime)
	if err != nil {
		uploadError(w, err)
		return
	}
	s.auditTerminalLaunchUpload(uid, agent, mime, len(data))
	w.Header().Set("X-Reet-Upload-Path", url.PathEscape(path))
	w.Header().Set("X-Reet-Upload-Reference", url.PathEscape(uploadReference(path)))
	writeJSON(w, http.StatusOK, map[string]string{"uploaded": "true"})
}

// readUploadImage reads, bounds and sniffs one image body. It writes the
// refusal itself and reports false when it did.
func (s *server) readUploadImage(w http.ResponseWriter, r *http.Request, declared string) ([]byte, string, bool) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadRequestTimeout))
	r.Body = http.MaxBytesReader(w, r.Body, terminalproto.MaxUploadBytes+1)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			uploadError(w, terminalbroker.ErrUploadTooLarge)
			return nil, "", false
		}
		terminalError(w, terminalproto.ErrInvalid)
		return nil, "", false
	}
	if len(data) == 0 {
		terminalError(w, terminalproto.ErrInvalid)
		return nil, "", false
	}
	if len(data) > terminalproto.MaxUploadBytes {
		uploadError(w, terminalbroker.ErrUploadTooLarge)
		return nil, "", false
	}
	mime, _, ok := terminalproto.SniffUploadMIME(data)
	if !ok || mime != declared {
		uploadError(w, terminalbroker.ErrUploadType)
		return nil, "", false
	}
	return data, mime, true
}

// uploadReference is the text a browser inserts after an upload. It is the
// absolute path unchanged unless it needs POSIX single quoting.
func uploadReference(path string) string {
	if uploadSafePath.MatchString(path) {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func uploadSizeBucket(size int) string {
	switch {
	case size < 256<<10:
		return "<256 KiB"
	case size < 1<<20:
		return "<1 MiB"
	}
	return "<=5 MiB"
}

func (s *server) auditTerminalUpload(uid int64, session store.TerminalSession, mime string, size int) {
	s.event(uid, "ui", "terminal-upload", "", map[string]string{
		"machine": session.AgentID,
		"host":    s.machineHost(uid, session.AgentID),
		"tool":    session.Tool,
		"mime":    mime,
		"size":    uploadSizeBucket(size),
	})
}

func (s *server) auditTerminalLaunchUpload(uid int64, agent, mime string, size int) {
	s.event(uid, "ui", "terminal-upload", "", map[string]string{
		"machine": agent,
		"host":    s.machineHost(uid, agent),
		"launch":  "true",
		"mime":    mime,
		"size":    uploadSizeBucket(size),
	})
}

// uploadError maps an upload failure to one sentence. No sentence carries a
// path, file name or hash.
func uploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, terminalbroker.ErrUploadUnsupported):
		writeErrCode(w, http.StatusConflict, "upload_unsupported", "", "Update reet on that machine to attach images.", "")
	case errors.Is(err, terminalbroker.ErrUploadsOff):
		writeErrCode(w, http.StatusConflict, "uploads_off", "", "Image uploads are off on this machine.", "")
	case errors.Is(err, terminalbroker.ErrUploadTooLarge):
		writeErrCode(w, http.StatusRequestEntityTooLarge, "upload_too_large", "", "That image is larger than the 5 MiB limit.", "")
	case errors.Is(err, terminalbroker.ErrUploadType):
		writeErrCode(w, http.StatusUnsupportedMediaType, "upload_type", "", "Attach a PNG, JPEG, GIF or WebP image.", "")
	case errors.Is(err, terminalbroker.ErrUploadQuota):
		writeErrCode(w, http.StatusConflict, "upload_quota", "", "That session has no room for more images.", "")
	case errors.Is(err, terminalbroker.ErrUploadBusy):
		writeErrCode(w, http.StatusTooManyRequests, "upload_busy", "", "Two images are already uploading. Try again in a moment.", "")
	case errors.Is(err, terminalbroker.ErrDenied), errors.Is(err, store.ErrRemoteNotFound):
		terminalError(w, err)
	case errors.Is(err, terminalbroker.ErrOffline), errors.Is(err, terminalbroker.ErrUploadTimeout):
		writeErrCode(w, http.StatusServiceUnavailable, "machine_unavailable", "", "That machine is reconnecting. Try again in a moment.", "")
	default:
		writeErrCode(w, http.StatusBadGateway, "upload_failed", "", "The image could not be uploaded.", "")
	}
}
