package api

import (
	"encoding/json"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
	"io"
	"net/http"
	"strings"
)

func (s *server) registerRemoteImports(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/sync/from-machine", s.privateSkillAccess(s.syncFromMachine))
	for _, route := range []string{"GET /api/v2/skill-imports", "GET /api/v2/skill-imports/{id}", "POST /api/v2/skill-imports", "POST /api/v2/skill-imports/{id}/approve", "POST /api/v2/skill-imports/{id}/cancel", "DELETE /api/v2/skill-imports/{id}"} {
		mux.HandleFunc(route, s.privateSkillAccess(s.browserSkillImports))
	}
	for _, route := range []string{"POST /api/v2/sync/skill-import/work", "POST /api/v2/sync/skill-import/preview", "POST /api/v2/sync/skill-import/transfer", "POST /api/v2/sync/skill-import/failure"} {
		mux.HandleFunc(route, s.privateSkillAccess(s.connectorSkillImports))
	}
}
func importDecode(w http.ResponseWriter, r *http.Request, limit int64, in any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(in) != nil || d.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "Invalid or oversized import request.")
		return false
	}
	return true
}
func (s *server) browserSkillImports(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	switch {
	case r.Method == "GET" && r.PathValue("id") != "":
		result, err := s.st.RemoteImportFor(uid, r.PathValue("id"))
		if err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, result)
	case r.Method == "GET":
		requests, caps, err := s.st.RemoteImportsFor(uid)
		if err != nil {
			privateSkillError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"requests": requests, "capabilities": caps})
	case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/cancel"):
		var in struct{}
		if !importDecode(w, r, 4096, &in) {
			return
		}
		out, err := s.st.StopRemoteImportFor(uid, r.PathValue("id"))
		if err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, out)
	case r.Method == "DELETE":
		if err := s.st.CancelRemoteImportFor(uid, r.PathValue("id")); err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case r.PathValue("id") != "":
		var in struct {
			PreviewHash string   `json:"previewHash"`
			Excluded    []string `json:"excluded"`
		}
		if !importDecode(w, r, 1<<20, &in) {
			return
		}
		out, err := s.st.ApproveRemoteImportFor(uid, r.PathValue("id"), in.PreviewHash, in.Excluded)
		if err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, out)
	default:
		var in store.RemoteImportCreate
		if !importDecode(w, r, 16<<10, &in) {
			return
		}
		out, err := s.st.CreateRemoteImportFor(uid, in)
		if err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, out)
	}
}
func remoteImportError(w http.ResponseWriter, err error) {
	if err == store.ErrRemoteImportCapacity {
		writeErr(w, 409, "Transfer history is full. Finish or cancel pending work, then dismiss finished request history before creating another.")
		return
	}
	if err == store.ErrRemoteImport || err == store.ErrContentObservationStale || err == store.ErrRemoteNotFound {
		writeErr(w, 409, "Import is unavailable, blocked, changed or expired. Refresh the machine and request a new preview.")
		return
	}
	privateSkillError(w, err)
}
func (s *server) connectorSkillImports(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	uid := s.uid(r)
	switch r.URL.Path {
	case "/api/v2/sync/skill-import/work":
		var in struct {
			SchemaVersion int    `json:"schemaVersion"`
			Instance      string `json:"instanceId"`
		}
		if !importDecode(w, r, 4096, &in) {
			return
		}
		if in.SchemaVersion != skillimport.RemoteVersion {
			writeErr(w, 400, "Unsupported import protocol.")
			return
		}
		work, err := s.st.ClaimRemoteImportFor(uid, a.ID, in.Instance)
		if err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"schemaVersion": skillimport.RemoteVersion, "work": work})
	case "/api/v2/sync/skill-import/preview":
		var in struct {
			ID       string              `json:"id"`
			Instance string              `json:"instanceId"`
			Preview  skillimport.Preview `json:"preview"`
		}
		if !importDecode(w, r, 1<<20, &in) {
			return
		}
		if err := s.st.RecordRemotePreviewFor(uid, a.ID, in.ID, in.Instance, in.Preview); err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"recorded": true})
	case "/api/v2/sync/skill-import/transfer":
		var in struct {
			ID           string                    `json:"id"`
			Instance     string                    `json:"instanceId"`
			SourceDigest string                    `json:"sourceDigest"`
			Files        []skillbundle.ContentFile `json:"files"`
		}
		if !importDecode(w, r, 16<<20, &in) {
			return
		}
		out, err := s.st.CommitRemoteImportFor(uid, a.ID, in.ID, in.Instance, in.SourceDigest, in.Files)
		if err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, out)
	case "/api/v2/sync/skill-import/failure":
		var in struct {
			ID       string `json:"id"`
			Instance string `json:"instanceId"`
			Code     string `json:"code"`
		}
		if !importDecode(w, r, 4096, &in) {
			return
		}
		if err := s.st.FailRemoteImportFor(uid, a.ID, in.ID, in.Instance, in.Code); err != nil {
			remoteImportError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"recorded": true})
	}
}

func (s *server) syncFromMachine(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SkillID       string `json:"skillId"`
		DestinationID string `json:"destinationId"`
		FollowShelf   bool   `json:"followShelf"`
		Destination   string `json:"destination"`
		SourceID      string `json:"sourceId"`
		RequestKey    string `json:"requestKey"`
	}
	if !importDecode(w, r, 16384, &in) {
		return
	}
	result, err := s.st.RequestMachineSkillSync(s.uid(r), in.SkillID, in.Destination, in.SourceID, in.RequestKey, in.DestinationID, in.FollowShelf)
	if err != nil {
		if err == store.ErrRemoteImport {
			writeErr(w, 409, "The source machine is not ready to transfer files. Connect it and update reet, then try again.")
		} else if err == store.ErrRemoteImportCapacity {
			remoteImportError(w, err)
		} else {
			writeErr(w, 409, err.Error())
		}
		return
	}
	writeJSON(w, 200, result)
}
