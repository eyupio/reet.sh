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
)

type softwareMirrorView struct {
	Enabled   bool   `json:"enabled"`
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

func (s *server) softwareMirrorViewNow() (softwareMirrorView, error) {
	out := softwareMirrorView{Version: agentsoftware.OpenCodeReviewedVersion}
	enabled, err := s.softwareMirrorEnabled()
	if err != nil {
		return out, err
	}
	out.Enabled = enabled
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
		Enabled *bool `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled == nil {
		writeErr(w, http.StatusBadRequest, "enabled is required")
		return
	}
	value := "0"
	if *in.Enabled {
		value = "1"
	}
	name := s.currentUser(r).Name
	at := time.Now().UnixMilli()
	for key, v := range map[string]string{
		softwareMirrorKey:   value,
		softwareMirrorByKey: name,
		softwareMirrorAtKey: strconv.FormatInt(at, 10),
	} {
		if err := s.st.SetSetting(key, v); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	action := "software-mirror-off"
	if *in.Enabled {
		action = "software-mirror-on"
	}
	s.eventRequest(r, action, "", map[string]any{"enabled": *in.Enabled})
	out, err := s.softwareMirrorViewNow()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
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
		writeErr(w, http.StatusForbidden, "machine enrollment is missing or revoked")
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
