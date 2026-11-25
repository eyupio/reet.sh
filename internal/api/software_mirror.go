package api

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/softwaremirror"
)

const (
	softwareMirrorKey   = "software_mirror_enabled"
	softwareMirrorByKey = "software_mirror_by"
	softwareMirrorAtKey = "software_mirror_at"
	// softwareMirrorFirstKey makes machines try the mirror before the registry.
	// It only counts while the mirror is on, and turning the mirror off clears it.
	softwareMirrorFirstKey = "software_mirror_first"
)

type softwareMirrorView struct {
	Enabled   bool   `json:"enabled"`
	Preferred bool   `json:"preferred"`
	Cached    bool   `json:"cached"`
	Size      int64  `json:"size,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Version   string `json:"version,omitempty"`
	ChangedBy string `json:"changedBy,omitempty"`
	ChangedAt int64  `json:"changedAt,omitempty"`
}

func (s *server) softwareMirrorEnabled() (bool, error) {
	v, err := s.st.GetSetting(softwareMirrorKey)
	return v == "1", err
}

// softwareMirrorPreferred reports whether machines should try the mirror
// first: only while the mirror is on and an owner or admin asked for it.
func (s *server) softwareMirrorPreferred() bool {
	enabled, err := s.softwareMirrorEnabled()
	first, _ := s.st.GetSetting(softwareMirrorFirstKey)
	return err == nil && enabled && first == "1"
}

func (s *server) softwareMirrorViewNow() (softwareMirrorView, error) {
	out := softwareMirrorView{Version: agentsoftware.OpenCodeReviewedVersion}
	enabled, err := s.softwareMirrorEnabled()
	if err != nil {
		return out, err
	}
	out.Enabled = enabled
	out.Preferred = s.softwareMirrorPreferred()
	if s.mirror != nil {
		cached := s.mirror.Status()
		out.Cached, out.Size, out.Digest = cached.Present, cached.Size, cached.Digest
	}
	out.ChangedBy, _ = s.st.GetSetting(softwareMirrorByKey)
	at, _ := s.st.GetSetting(softwareMirrorAtKey)
	out.ChangedAt, _ = strconv.ParseInt(at, 10, 64)
	return out, nil
}

func (s *server) softwareMirrorGet(w http.ResponseWriter, r *http.Request) {
	out, err := s.softwareMirrorViewNow()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) softwareMirrorPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled   *bool `json:"enabled"`
		Preferred *bool `json:"preferred"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled == nil && in.Preferred == nil {
		writeErr(w, http.StatusBadRequest, "enabled or preferred is required")
		return
	}
	enabled, err := s.softwareMirrorEnabled()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.Preferred != nil && *in.Preferred && !enabled {
		writeErrCode(w, http.StatusConflict, "software_mirror_off", "", "Turn the artifact mirror on first.", "")
		return
	}
	settings := map[string]string{
		softwareMirrorByKey: s.currentUser(r).Name,
		softwareMirrorAtKey: strconv.FormatInt(time.Now().UnixMilli(), 10),
	}
	if in.Enabled != nil {
		settings[softwareMirrorKey] = settingFlag(*in.Enabled)
		if !*in.Enabled {
			settings[softwareMirrorFirstKey] = "0"
		}
	}
	if in.Preferred != nil {
		settings[softwareMirrorFirstKey] = settingFlag(*in.Preferred)
	}
	for key, v := range settings {
		if err := s.st.SetSetting(key, v); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if in.Enabled != nil {
		action := "software-mirror-off"
		if *in.Enabled {
			action = "software-mirror-on"
		}
		s.eventRequest(r, action, "", map[string]any{"enabled": *in.Enabled})
	}
	if in.Preferred != nil {
		action := "software-mirror-first-off"
		if *in.Preferred {
			action = "software-mirror-first-on"
		}
		s.eventRequest(r, action, "", map[string]any{"preferred": *in.Preferred})
	}
	out, err := s.softwareMirrorViewNow()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func settingFlag(on bool) string {
	if on {
		return "1"
	}
	return "0"
}

// softwareMirrorCache downloads and verifies the reviewed artifact now, so an
// operator can prepare the mirror before a machine needs it.
func (s *server) softwareMirrorCache(w http.ResponseWriter, r *http.Request) {
	if s.mirror == nil {
		writeErr(w, http.StatusServiceUnavailable, "the artifact mirror is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), softwaremirror.DownloadTimeout+time.Minute)
	defer cancel()
	cached, err := s.mirror.Ensure(ctx)
	if err != nil {
		s.eventRequest(r, "software-mirror-cache", "", map[string]any{"cached": false, "error": err.Error()})
		writeErr(w, http.StatusBadGateway, "The reviewed artifact could not be cached. Check that this controller can reach registry.npmjs.org, then try again.")
		return
	}
	s.eventRequest(r, "software-mirror-cache", "", map[string]any{"cached": cached.Present, "size": cached.Size, "digest": cached.Digest})
	out, err := s.softwareMirrorViewNow()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// privateSoftwareArtifact serves the cached archive to an enrolled machine over
// the private connection. The machine verifies the pinned digest itself; this
// route never accepts a path the controller did not pin.
func (s *server) privateSoftwareArtifact(w http.ResponseWriter, r *http.Request) {
	if !s.privateCredentials() || s.mirror == nil {
		http.NotFound(w, r)
		return
	}
	if enabled, err := s.softwareMirrorEnabled(); err != nil || !enabled {
		http.NotFound(w, r)
		return
	}
	if strings.TrimPrefix(r.URL.Path, "/private/v1/software/artifact/") != s.mirror.Digest() {
		http.NotFound(w, r)
		return
	}
	if _, _, err := s.st.AuthenticateRemoteAgent(r.Header.Get("X-Reet-Agent"), r.Header.Get("X-Reet-Agent-Key"), r.Header.Get("X-Reet-Version")); err != nil {
		refuseMachine(w, err)
		return
	}
	select {
	case s.mirrorServes <- struct{}{}:
		defer func() { <-s.mirrorServes }()
	case <-r.Context().Done():
		return
	}
	cached, err := s.mirror.Ensure(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "the reviewed artifact could not be cached")
		return
	}
	f, err := os.Open(s.mirror.Path())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "the cached artifact could not be read")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "the cached artifact could not be read")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(cached.Size, 10))
	http.ServeContent(w, r, "", info.ModTime(), f)
}
