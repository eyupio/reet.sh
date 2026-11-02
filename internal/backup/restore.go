package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// RestoreOptions chooses what survives a restore.
type RestoreOptions struct {
	// KeepTokens leaves personal API tokens and MCP connections working.
	// Browser sign-ins always end.
	KeepTokens bool
}

// RestoreReport says what a copy held and what was done to it so it is safe
// on the controller it lands on.
type RestoreReport struct {
	ID            string `json:"id"`
	Release       string `json:"release,omitempty"`
	TakenAt       int64  `json:"takenAt"`
	SchemaVersion int    `json:"schemaVersion"`
	Users         int    `json:"users"`
	Skills        int    `json:"skills"`
	Lists         int    `json:"lists"`
	Tags          int    `json:"tags"`
	Environments  int    `json:"environments"`

	SessionsEnded         int  `json:"sessionsEnded"`
	TokensRevoked         int  `json:"tokensRevoked"`
	MCPConnectionsRevoked int  `json:"mcpConnectionsRevoked"`
	TokensKept            bool `json:"tokensKept"`
	IdentityDropped       bool `json:"identityDropped"`

	// Unreadable names each stored secret the key on this controller cannot
	// open. They are left in place, so the right key still opens them, and
	// are entered again otherwise.
	Unreadable []string `json:"unreadable"`
}

// Lines is the report as a person reads it.
func (r RestoreReport) Lines() []string {
	var l []string
	l = append(l, fmt.Sprintf("Came across: %d accounts, %d skills on shelves, %d lists, %d tags, %d environments.",
		r.Users, r.Skills, r.Lists, r.Tags, r.Environments))
	l = append(l, fmt.Sprintf("Ended %d browser sign-in(s).", r.SessionsEnded))
	if r.TokensKept {
		l = append(l, "Kept personal API tokens and MCP connections, as asked. The old controller's tokens open this one.")
	} else {
		l = append(l, fmt.Sprintf("Revoked %d personal API token(s) and %d MCP connection(s); make new ones here.", r.TokensRevoked, r.MCPConnectionsRevoked))
	}
	if r.IdentityDropped {
		l = append(l, "Dropped the private-connection identity; this controller makes its own, so two never share one.")
	}
	if len(r.Unreadable) == 0 {
		l = append(l, "Every stored secret opens with this controller's backup key.")
	} else {
		l = append(l, "These stored secrets were sealed under a different REET_BACKUP_KEY and cannot be opened here; enter them again, or restart with the old key:")
		for _, u := range r.Unreadable {
			l = append(l, "  - "+u)
		}
	}
	return l
}

// RestoreResult names the copy that was restored and where the previous
// database was kept.
type RestoreResult struct {
	ID       string
	Previous string
	Message  string
	Report   RestoreReport
}

// ErrNewerSchema is returned for a copy taken by a newer reet.
var ErrNewerSchema = errors.New("this copy was taken by a newer reet than this one and may hold data this version cannot read; upgrade reet, then restore it")

// prepare checks a copy's payload, then adapts it in place at path. Nothing
// live is touched, so a refusal leaves the controller as it was.
func (s *Service) prepare(ctx context.Context, path string, m Manifest, payload []byte, opts RestoreOptions) (RestoreReport, error) {
	rep := RestoreReport{ID: m.ID, Release: m.Version, TakenAt: m.TakenAt, TokensKept: opts.KeepTokens, Unreadable: []string{}}
	if m.Kind != "instance" {
		return rep, errors.New("that is not an instance backup")
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return rep, err
	}
	if err := store.CheckDatabaseFile(ctx, path); err != nil {
		return rep, fmt.Errorf("the backup does not pass an integrity check: %w", err)
	}
	sum, err := store.InspectDatabaseFile(ctx, path)
	if err != nil {
		return rep, err
	}
	if sum.SchemaVersion > store.SchemaVersion {
		return rep, ErrNewerSchema
	}
	ad, err := store.AdaptDatabaseFile(ctx, path, store.AdaptOptions{KeepTokens: opts.KeepTokens})
	if err != nil {
		return rep, fmt.Errorf("adapting the copy: %w", err)
	}
	rep.SchemaVersion = sum.SchemaVersion
	rep.Users, rep.Skills, rep.Lists, rep.Tags, rep.Environments = sum.Users, sum.Skills, sum.Lists, sum.Tags, sum.Environments
	rep.SessionsEnded, rep.TokensRevoked, rep.MCPConnectionsRevoked, rep.IdentityDropped =
		ad.SessionsEnded, ad.TokensRevoked, ad.MCPConnectionsRevoked, ad.IdentityDropped
	for _, sec := range sum.Secrets {
		if s.cfg.Sealer == nil {
			rep.Unreadable = append(rep.Unreadable, sec.Where)
		} else if _, err := s.cfg.Sealer.OpenSecret(sec.Sealed); err != nil {
			rep.Unreadable = append(rep.Unreadable, sec.Where)
		}
	}
	return rep, nil
}

// RestoreInstance downloads an instance copy and replaces the live database
// with it. closeStore is called before any live file changes so SQLite is not
// writing while its files are swapped. The store stays open when the download
// needs a password, so a caller can retry with one.
func RestoreInstance(ctx context.Context, svc *Service, dataDir, id, password string, opts RestoreOptions, closeStore func() error) (RestoreResult, error) {
	if closeStore == nil {
		closeStore = func() error { return nil }
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	manifest, payload, err := svc.DownloadWithPassword(ctx, 0, id, password)
	if err != nil {
		return RestoreResult{}, err
	}
	temp, err := os.CreateTemp(dataDir, ".reet-restore-*.db")
	if err != nil {
		return RestoreResult{}, err
	}
	tempName := temp.Name()
	temp.Close()
	defer os.Remove(tempName)
	rep, err := svc.prepare(ctx, tempName, manifest, payload, opts)
	if err != nil {
		return RestoreResult{Report: rep}, err
	}
	if err := closeStore(); err != nil {
		return RestoreResult{}, err
	}
	previous, err := swapIn(dataDir, tempName)
	if err != nil {
		return RestoreResult{}, err
	}
	result := RestoreResult{ID: id, Previous: previous, Report: rep}
	var message strings.Builder
	if previous != "" {
		fmt.Fprintf(&message, "The database that was live is now %s.\n", previous)
	}
	fmt.Fprintf(&message, "Restored %s.\n", id)
	for _, l := range rep.Lines() {
		message.WriteString(l + "\n")
	}
	message.WriteString("Start reet again; it will open the restored database.\n")
	result.Message = message.String()
	return result, nil
}

// swapIn puts the database at src in place of the live one, keeping the old
// one beside it, and returns where it went.
func swapIn(dataDir, src string) (string, error) {
	live := filepath.Join(dataDir, "reet.db")
	var previous string
	if _, err := os.Stat(live); err == nil {
		previous = live + ".before-restore-" + time.Now().UTC().Format("20060102T150405Z")
		if err := os.Rename(live, previous); err != nil {
			return "", err
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(live + suffix)
	}
	if err := os.Rename(src, live); err != nil {
		return "", err
	}
	return previous, os.Chmod(live, 0o600)
}

// --- import into a running controller -------------------------------------

// A running controller cannot swap the database it serves, so an import is
// staged beside it, already opened, checked and adapted, and put in place by
// ApplyPendingImport at the next start.
const (
	pendingDir   = "pending-import"
	pendingDB    = "reet.db"
	pendingState = "report.json"
	// MaxImportBytes matches the largest copy this build can seal.
	MaxImportBytes = 512 << 20
)

// sealedFrom accepts the sealed bytes themselves or the .zip the Backups page
// downloads, which holds them as <id>.sealed.
func sealedFrom(raw []byte) ([]byte, error) {
	if !bytes.HasPrefix(raw, []byte("PK\x03\x04")) {
		return raw, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, errors.New("that file is a damaged zip")
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".sealed") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, MaxImportBytes+1))
	}
	return nil, errors.New("that zip holds no .sealed copy")
}

// StageImport opens an uploaded copy and stages it for the next start. A
// second import replaces the first. The copy is opened with this controller's
// key, or the password given; a password-protected copy needs one.
func (s *Service) StageImport(ctx context.Context, dataDir string, raw []byte, password string, opts RestoreOptions) (RestoreReport, error) {
	raw, err := sealedFrom(raw)
	if err != nil {
		return RestoreReport{}, err
	}
	m, payload, err := s.openArchive(0, raw, password)
	if err != nil {
		return RestoreReport{}, err
	}
	stage, err := os.MkdirTemp(dataDir, ".reet-import-*")
	if err != nil {
		return RestoreReport{}, err
	}
	defer os.RemoveAll(stage)
	rep, err := s.prepare(ctx, filepath.Join(stage, pendingDB), m, payload, opts)
	if err != nil {
		return rep, err
	}
	state, _ := json.Marshal(rep)
	if err := os.WriteFile(filepath.Join(stage, pendingState), state, 0o600); err != nil {
		return rep, err
	}
	final := filepath.Join(dataDir, pendingDir)
	if err := os.RemoveAll(final); err != nil {
		return rep, err
	}
	if err := os.Rename(stage, final); err != nil {
		return rep, err
	}
	return rep, nil
}

// PendingImport reports the staged import, or nil when there is none.
func PendingImport(dataDir string) (*RestoreReport, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, pendingDir, pendingState))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rep RestoreReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

// CancelImport discards a staged import.
func CancelImport(dataDir string) error {
	return os.RemoveAll(filepath.Join(dataDir, pendingDir))
}

// ApplyPendingImport swaps a staged import in. It runs before the database is
// opened, at the start of serve. It returns nil when nothing is staged. A
// stage that cannot be applied is moved aside so the controller still starts.
func ApplyPendingImport(dataDir string) (*RestoreResult, error) {
	rep, err := PendingImport(dataDir)
	if err != nil || rep == nil {
		return nil, err
	}
	dir := filepath.Join(dataDir, pendingDir)
	previous, err := swapIn(dataDir, filepath.Join(dir, pendingDB))
	if err != nil {
		aside := dir + ".failed-" + time.Now().UTC().Format("20060102T150405Z")
		_ = os.Rename(dir, aside)
		return nil, fmt.Errorf("the staged import could not be applied and was moved to %s: %w", aside, err)
	}
	_ = os.RemoveAll(dir)
	return &RestoreResult{ID: rep.ID, Previous: previous, Report: *rep}, nil
}
