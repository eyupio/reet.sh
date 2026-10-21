package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) registerBackupRoutes(mux *http.ServeMux) {
	// The instance's copies and destinations: the owner configures, admins read.
	mux.HandleFunc("GET /api/v2/admin/backups", s.requireAdmin(s.adminBackups))
	mux.HandleFunc("PATCH /api/v2/admin/backups", s.requireOwner(s.adminBackupSettings))
	mux.HandleFunc("POST /api/v2/admin/backups/run", s.requireOwner(s.adminBackupRun))
	mux.HandleFunc("GET /api/v2/admin/backups/remotes", s.requireAdmin(s.adminBackupRemotes))
	mux.HandleFunc("POST /api/v2/admin/backups/remotes", s.requireOwner(s.adminBackupRemoteCreate))
	mux.HandleFunc("POST /api/v2/admin/backups/remotes/test", s.requireOwner(s.adminBackupRemoteTestDraft))
	mux.HandleFunc("PATCH /api/v2/admin/backups/remotes/{name}", s.requireOwner(s.adminBackupRemoteUpdate))
	mux.HandleFunc("DELETE /api/v2/admin/backups/remotes/{name}", s.requireOwner(s.adminBackupRemoteDelete))
	mux.HandleFunc("POST /api/v2/admin/backups/remotes/{name}/test", s.requireOwner(s.adminBackupRemoteTest))
	mux.HandleFunc("GET /api/v2/admin/backups/remotes/{name}/copies", s.requireAdmin(s.adminBackupRemoteCopies))
	mux.HandleFunc("GET /api/v2/admin/backups/items/{id}/download", s.requireOwner(s.adminBackupDownload))
	mux.HandleFunc("POST /api/v2/admin/backups/items/{id}/verify", s.requireAdmin(s.adminBackupVerify))
	mux.HandleFunc("DELETE /api/v2/admin/backups/items/{id}", s.requireOwner(s.adminBackupDelete))

	// A person's own copies. A viewer may read; only a writer may change anything.
	mux.HandleFunc("GET /api/v2/backups", s.userBackups)
	mux.HandleFunc("PATCH /api/v2/backups", s.requireWriter(s.userBackupSettings))
	mux.HandleFunc("POST /api/v2/backups/run", s.requireWriter(s.userBackupRun))
	mux.HandleFunc("GET /api/v2/backups/remotes", s.userBackupRemotes)
	mux.HandleFunc("POST /api/v2/backups/remotes", s.requireWriter(s.userBackupRemoteCreate))
	mux.HandleFunc("POST /api/v2/backups/remotes/test", s.requireWriter(s.userBackupRemoteTestDraft))
	mux.HandleFunc("PATCH /api/v2/backups/remotes/{name}", s.requireWriter(s.userBackupRemoteUpdate))
	mux.HandleFunc("DELETE /api/v2/backups/remotes/{name}", s.requireWriter(s.userBackupRemoteDelete))
	mux.HandleFunc("POST /api/v2/backups/remotes/{name}/test", s.requireWriter(s.userBackupRemoteTest))
	mux.HandleFunc("GET /api/v2/backups/remotes/{name}/copies", s.userBackupRemoteCopies)
	mux.HandleFunc("GET /api/v2/backups/items/{id}/download", s.userBackupDownload)
	mux.HandleFunc("POST /api/v2/backups/items/{id}/verify", s.userBackupVerify)
	mux.HandleFunc("POST /api/v2/backups/items/{id}/restore", s.requireWriter(s.userBackupRestore))
	mux.HandleFunc("DELETE /api/v2/backups/items/{id}", s.requireWriter(s.userBackupDelete))
}

func (s *server) backupsOr(w http.ResponseWriter) *backup.Service {
	if s.backups == nil {
		writeErr(w, http.StatusServiceUnavailable, "backups are not configured on this instance")
		return nil
	}
	return s.backups
}

func adminOwner() int64 { return 0 }

func (s *server) adminBackups(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	view, err := svc.Status(r.Context(), adminOwner())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, view)
}

func (s *server) adminBackupSettings(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.saveBackupSettings(w, r, svc, adminOwner())
}

func (s *server) adminBackupRun(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	item, err := svc.RunInstance(r.Context(), "manual")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "backup", "backup-run", "", map[string]any{"scope": "instance", "id": item.ID, "bytes": item.Bytes})
	writeJSON(w, 200, item)
}

func (s *server) adminBackupRemotes(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	view, err := svc.Status(r.Context(), adminOwner())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"remotes": view.Remotes})
}

func (s *server) adminBackupRemoteCreate(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.createBackupRemote(w, r, svc, adminOwner())
}

func (s *server) adminBackupRemoteUpdate(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.updateBackupRemote(w, r, svc, adminOwner())
}

func (s *server) adminBackupRemoteDelete(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.deleteBackupRemote(w, r, svc, adminOwner())
}

func (s *server) adminBackupRemoteTestDraft(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.testBackupRemoteDraft(w, r, svc, adminOwner())
}

func (s *server) adminBackupRemoteTest(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.testBackupRemote(w, r, svc, adminOwner())
}

func (s *server) adminBackupRemoteCopies(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.backupRemoteCopies(w, r, svc, adminOwner())
}

func (s *server) adminBackupDownload(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.downloadBackup(w, r, svc, adminOwner())
}

func (s *server) adminBackupVerify(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.verifyBackup(w, r, svc, adminOwner())
}

func (s *server) adminBackupDelete(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.deleteBackup(w, r, svc, adminOwner())
}

func (s *server) userBackups(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	view, err := svc.Status(r.Context(), s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, view)
}

func (s *server) userBackupSettings(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.saveBackupSettings(w, r, svc, s.uid(r))
}

func (s *server) userBackupRun(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	item, err := svc.RunUser(r.Context(), s.uid(r), "manual")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "backup", "backup-run", "", map[string]any{"scope": "user", "id": item.ID, "bytes": item.Bytes})
	writeJSON(w, 200, item)
}

func (s *server) userBackupRemotes(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	view, err := svc.Status(r.Context(), s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"remotes": view.Remotes})
}

func (s *server) userBackupRemoteCreate(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.createBackupRemote(w, r, svc, s.uid(r))
}

func (s *server) userBackupRemoteUpdate(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.updateBackupRemote(w, r, svc, s.uid(r))
}

func (s *server) userBackupRemoteDelete(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.deleteBackupRemote(w, r, svc, s.uid(r))
}

func (s *server) userBackupRemoteTestDraft(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.testBackupRemoteDraft(w, r, svc, s.uid(r))
}

func (s *server) userBackupRemoteTest(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.testBackupRemote(w, r, svc, s.uid(r))
}

func (s *server) userBackupRemoteCopies(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.backupRemoteCopies(w, r, svc, s.uid(r))
}

func (s *server) userBackupDownload(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.downloadBackup(w, r, svc, s.uid(r))
}

func (s *server) userBackupVerify(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.verifyBackup(w, r, svc, s.uid(r))
}

func (s *server) userBackupRestore(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	id := r.PathValue("id")
	n, err := svc.RestoreUser(r.Context(), s.uid(r), id)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event(s.uid(r), "backup", "backup-restore", "", map[string]any{"id": id, "restored": n})
	writeJSON(w, 200, map[string]any{"restored": n})
}

func (s *server) userBackupDelete(w http.ResponseWriter, r *http.Request) {
	svc := s.backupsOr(w)
	if svc == nil {
		return
	}
	s.deleteBackup(w, r, svc, s.uid(r))
}

// --- shared handlers -------------------------------------------------------

func (s *server) saveBackupSettings(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	var in struct {
		EverySeconds *int64  `json:"everySeconds"`
		Keep         *int    `json:"keep"`
		Destination  *string `json:"destination"`
	}
	if !decode(w, r, &in) {
		return
	}
	current, err := svc.Settings(r.Context(), ownerID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	every, keep := current.EverySeconds, current.Keep
	destination := current.Destination
	if in.EverySeconds != nil {
		every = *in.EverySeconds
	}
	if in.Keep != nil {
		keep = *in.Keep
	}
	if in.Destination != nil {
		destination = *in.Destination
	}
	saved, err := svc.SaveSettings(ownerID, every, keep, destination)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	scope := "user"
	if strings.HasPrefix(r.URL.Path, "/api/v2/admin/") {
		scope = "instance"
	}
	s.event(s.uid(r), "backup", "backup-settings", "", map[string]any{"scope": scope, "everySeconds": saved.EverySeconds, "keep": saved.Keep, "destination": saved.Destination})
	writeJSON(w, 200, saved)
}

func (s *server) createBackupRemote(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	var in backup.RemoteInput
	if !decode(w, r, &in) {
		return
	}
	view, err := svc.CreateRemote(ownerID, in)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event(s.uid(r), "backup", "backup-remote-add", "", map[string]any{"name": view.Name, "where": view.Where})
	writeJSON(w, 200, view)
}

func (s *server) updateBackupRemote(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	var in backup.RemoteInput
	if !decode(w, r, &in) {
		return
	}
	view, err := svc.UpdateRemote(ownerID, r.PathValue("name"), in)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "destination not found")
		return
	} else if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event(s.uid(r), "backup", "backup-remote-change", "", map[string]any{"name": view.Name, "where": view.Where})
	writeJSON(w, 200, view)
}

func (s *server) deleteBackupRemote(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	name := r.PathValue("name")
	if err := svc.DeleteRemote(ownerID, name); errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "destination not found")
		return
	} else if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event(s.uid(r), "backup", "backup-remote-remove", "", map[string]any{"name": name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) testBackupRemoteDraft(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	var in backup.RemoteInput
	if !decode(w, r, &in) {
		return
	}
	steps, err := svc.TestRemoteDraft(r.Context(), ownerID, in)
	out := map[string]any{"name": valueOr(in.Name, ""), "ok": err == nil, "steps": steps}
	if err != nil {
		out["error"] = err.Error()
	}
	writeJSON(w, 200, out)
}

func valueOr(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}

func (s *server) testBackupRemote(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	name := r.PathValue("name")
	steps, err := svc.TestRemote(r.Context(), ownerID, name)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "destination not found")
		return
	}
	out := map[string]any{"name": name, "ok": err == nil, "steps": steps}
	if err != nil {
		out["error"] = err.Error()
	}
	writeJSON(w, 200, out)
}

func (s *server) backupRemoteCopies(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	items, err := svc.RemoteCopies(ctx, ownerID, r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "destination not found")
		return
	} else if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *server) downloadBackup(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	id := r.PathValue("id")
	payload, err := svc.Archive(r.Context(), ownerID, id)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	name := id + ".reetbackup"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(200)
	_, _ = w.Write(payload)
}

func (s *server) verifyBackup(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	result := svc.Verify(r.Context(), ownerID, r.PathValue("id"))
	writeJSON(w, 200, result)
}

func (s *server) deleteBackup(w http.ResponseWriter, r *http.Request, svc *backup.Service, ownerID int64) {
	if err := svc.DeleteLocal(ownerID, r.PathValue("id")); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	s.event(s.uid(r), "backup", "backup-delete", "", map[string]any{"id": r.PathValue("id")})
	w.WriteHeader(http.StatusNoContent)
}
