package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// RestoreOptions chooses what is checked and what survives a restore. Sessions
// and personal tokens come across unless dropped.
type RestoreOptions struct {
	// DropSessions ends every browser sign-in and password-reset link.
	DropSessions bool
	// DropTokens revokes every personal API token and MCP connection.
	DropTokens bool
	// OwnerEmail, when set, must be an owner account in the copy that can sign
	// in, or the restore is refused before anything is replaced.
	OwnerEmail string
	// CheckOnly runs every check and reports, and replaces nothing.
	CheckOnly bool
	// Before is called with the inventory once the copy has passed every
	// check, and before anything live changes.
	Before func(RestoreReport)
	// Source is where the request came from ("cli" or "api"), for the
	// activity log.
	Source string
	// Replace allows an import into a controller that already holds data.
	Replace bool
	// AllowLiveSource accepts a copy whose source was not fenced as it was
	// taken, or that does not say. Without it such a copy is refused, because
	// two live controllers would split one set of data.
	AllowLiveSource bool
}

// RestoreReport says what a copy holds and what was done to it so it is safe
// on the controller it lands on.
type RestoreReport struct {
	ID             string         `json:"id"`
	Release        string         `json:"release,omitempty"`
	TakenAt        int64          `json:"takenAt"`
	SchemaVersion  int            `json:"schemaVersion"`
	Users          int            `json:"users"`
	UsersByRole    map[string]int `json:"usersByRole"`
	Shelves        int            `json:"shelves"`
	Skills         int            `json:"skills"`
	LibraryItems   int            `json:"libraryItems"`
	PrivateBundles int            `json:"privateBundles"`
	Lists          int            `json:"lists"`
	Tags           int            `json:"tags"`
	Environments   int            `json:"environments"`
	Machines       int            `json:"machines"`
	Destinations   int            `json:"destinations"`

	SessionsKept          int  `json:"sessionsKept"`
	SessionsEnded         int  `json:"sessionsEnded"`
	TokensRevoked         int  `json:"tokensRevoked"`
	MCPConnectionsRevoked int  `json:"mcpConnectionsRevoked"`
	TokensKept            bool `json:"tokensKept"`
	IdentityDropped       bool `json:"identityDropped"`

	// SafetyCopy is where the live database was copied before an import was
	// staged, so that what it replaces can be put back. Empty for a restore,
	// which moves the live database aside instead.
	SafetyCopy string `json:"safetyCopy,omitempty"`

	// Unreadable names each stored secret the key on this controller cannot
	// open, with what to do about it. Each is cleared in the copy.
	Unreadable []string `json:"unreadable"`

	// StagedSHA256 is the digest of the adapted database a staged import
	// holds, taken when it was staged, so the next start can tell the file
	// was not changed in between.
	StagedSHA256 string `json:"stagedSha256,omitempty"`
	// Refusal says why a staged import was not applied at the next start. It
	// is written beside the refused stage, never to a live database.
	Refusal string `json:"refusal,omitempty"`
}

// Inventory is what the copy holds, as a person reads it.
func (r RestoreReport) Inventory() []string {
	roles := make([]string, 0, len(r.UsersByRole))
	for role := range r.UsersByRole {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for i, role := range roles {
		roles[i] = fmt.Sprintf("%d %s", r.UsersByRole[role], role)
	}
	accounts := fmt.Sprintf("%d accounts", r.Users)
	if len(roles) > 0 {
		accounts += " (" + strings.Join(roles, ", ") + ")"
	}
	lines := []string{
		fmt.Sprintf("The copy %s holds %s, %d shelves, %d skills, %d lists, %d tags, %d environments, %d connected machines and %d backup destinations.",
			r.ID, accounts, r.Shelves, r.Skills, r.Lists, r.Tags, r.Environments, r.Machines, r.Destinations),
	}
	if r.LibraryItems > 0 || r.PrivateBundles > 0 {
		lines = append(lines, fmt.Sprintf("It also holds %d library items and %d private content bundles.", r.LibraryItems, r.PrivateBundles))
	}
	return lines
}

// Lines is the whole report as a person reads it.
func (r RestoreReport) Lines() []string { return append(r.Inventory(), r.Outcome()...) }

// Outcome is what was, or would be, done to the copy.
func (r RestoreReport) Outcome() []string {
	var l []string
	if r.SessionsEnded > 0 || r.SessionsKept == 0 {
		l = append(l, fmt.Sprintf("Ended %d browser sign-in(s) and any password-reset links, as asked.", r.SessionsEnded))
	} else {
		l = append(l, fmt.Sprintf("Kept %d browser sign-in(s); they open this controller. Use --drop-sessions to end them.", r.SessionsKept))
	}
	if r.TokensKept {
		l = append(l, "Kept personal API tokens and MCP connections; the old controller's tokens open this one. Use --drop-tokens to revoke them.")
	} else {
		l = append(l, fmt.Sprintf("Revoked %d personal API token(s) and %d MCP connection(s), as asked; make new ones here.", r.TokensRevoked, r.MCPConnectionsRevoked))
	}
	if r.SafetyCopy != "" {
		l = append(l, "The database that is live now was copied to "+r.SafetyCopy+" first.")
	}
	if r.IdentityDropped {
		l = append(l, "Dropped the private-connection identity; this controller makes its own, so two never share one.")
	}
	if len(r.Unreadable) == 0 {
		l = append(l, "Every stored secret opens with this controller's backup key.")
	} else {
		l = append(l, "These stored secrets were sealed under a different REET_BACKUP_KEY. They cannot be opened here, so they were cleared:")
		for _, u := range r.Unreadable {
			l = append(l, "  - "+u)
		}
		l = append(l, "The cleared values are still in the database that was replaced, which opens with the old key.")
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

// ErrLiveSource is returned for a copy whose source controller was not fenced
// as the copy was taken, or whose copy does not say.
var ErrLiveSource = errors.New("this copy was taken from a controller that was not stopped")

// sourceState says in a word what a copy records about its source: "fenced",
// "live" or "unknown" (taken by a reet that did not record it).
func sourceState(m Manifest) string {
	switch {
	case m.Fenced == nil:
		return "unknown"
	case *m.Fenced:
		return "fenced"
	}
	return "live"
}

// checkLiveSource refuses a copy of a controller that may still be running.
func checkLiveSource(m Manifest, allow bool) error {
	state := sourceState(m)
	if state == "fenced" || allow {
		return nil
	}
	what := "it says its source controller was still running"
	if state == "unknown" {
		what = "it does not say whether its source controller was stopped (an older reet took it)"
	}
	return fmt.Errorf("%w: %s. If that controller is still live, two controllers would work from one set of data, and this copy may already be out of date. Stop the old controller, take a final copy of it once it is stopped, and use that; or, if you know the old one is gone, say so with --allow-live-source", ErrLiveSource, what)
}

// ErrNewerSchema is returned for a copy taken by a newer reet.
var ErrNewerSchema = errors.New("this copy was taken by a newer reet than this one")

// prepare checks a copy's payload, then adapts it in place at path. Nothing
// live is touched, so a refusal leaves the controller as it was. The checks
// run in this order: the copy is a whole-instance copy that SQLite accepts, it
// is not newer than this build reads, and it has the owner the operator wants
// to sign in as. Only then is the copy changed, and only the file at path.
func (s *Service) prepare(ctx context.Context, path string, m Manifest, payload []byte, opts RestoreOptions) (RestoreReport, error) {
	rep := RestoreReport{ID: m.ID, Release: m.Version, TakenAt: m.TakenAt, UsersByRole: map[string]int{}, Unreadable: []string{}}
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
		release := m.Version
		if release == "" {
			release = "unknown"
		}
		return rep, fmt.Errorf("%w: it is data format %d (release %s) and this reet reads up to data format %d; upgrade reet, then restore it", ErrNewerSchema, sum.SchemaVersion, release, store.SchemaVersion)
	}
	if err := checkLiveSource(m, opts.AllowLiveSource); err != nil {
		return rep, err
	}
	if email := strings.TrimSpace(opts.OwnerEmail); email != "" {
		found := false
		for _, o := range sum.Owners {
			if strings.EqualFold(o, email) {
				found = true
			}
		}
		if !found {
			return rep, fmt.Errorf("the copy has no owner account for %s that can sign in, so nobody could sign in with it; nothing was changed (owners in the copy: %s)", email, strings.Join(sum.Owners, ", "))
		}
	}
	ad, err := store.AdaptDatabaseFile(ctx, path, store.AdaptOptions{
		DropSessions: opts.DropSessions,
		DropTokens:   opts.DropTokens,
		Unreadable: func(sealed string) bool {
			if s.cfg.Sealer == nil {
				return true
			}
			_, err := s.cfg.Sealer.OpenSecret(sealed)
			return err != nil
		},
		EventSource: opts.Source,
		EventAction: "backup-restored",
		EventDetail: map[string]any{"id": m.ID, "release": m.Version, "ownerChecked": strings.TrimSpace(opts.OwnerEmail) != "",
			"source": m.Source, "sourceState": sourceState(m), "allowLiveSource": opts.AllowLiveSource},
	})
	if err != nil {
		return rep, fmt.Errorf("adapting the copy: %w", err)
	}
	rep.SchemaVersion = sum.SchemaVersion
	rep.Users, rep.UsersByRole, rep.Shelves, rep.Skills = sum.Users, sum.UsersByRole, sum.Shelves, sum.Skills
	rep.LibraryItems, rep.PrivateBundles = sum.LibraryItems, sum.PrivateBundles
	rep.Lists, rep.Tags, rep.Environments, rep.Machines, rep.Destinations = sum.Lists, sum.Tags, sum.Environments, sum.Machines, sum.Destinations
	rep.SessionsKept = sum.Sessions - ad.SessionsEnded
	rep.TokensKept = !opts.DropTokens
	rep.SessionsEnded, rep.TokensRevoked, rep.MCPConnectionsRevoked, rep.IdentityDropped =
		ad.SessionsEnded, ad.TokensRevoked, ad.MCPConnectionsRevoked, ad.IdentityDropped
	rep.Unreadable = ad.Cleared
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
	if opts.Source == "" {
		opts.Source = "cli"
	}
	rep, err := svc.prepare(ctx, tempName, manifest, payload, opts)
	if err != nil {
		return RestoreResult{Report: rep}, err
	}
	if opts.Before != nil {
		opts.Before(rep)
	}
	if opts.CheckOnly {
		return RestoreResult{ID: id, Report: rep}, nil
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
	for _, l := range rep.Outcome() {
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

// ErrNotEmpty is returned when an import would replace data a controller
// already holds and the operator did not say to.
var ErrNotEmpty = errors.New("this controller already holds data")

// holds says what a controller holds that an import would replace. A new
// controller has its first owner and nothing else: no other account and no
// skills, shelves, library content, lists, tags, environments or connected machines. The topic
// tags the daily sync adds and the server's own machine report are not
// counted: a controller makes them for itself before anyone uses it. Backup
// destinations and settings are configuration, set up before an import on
// purpose, so they do not count.
func holds(h store.RestoreSummary) []string {
	var what []string
	add := func(n int, noun string) {
		if n > 0 {
			what = append(what, fmt.Sprintf("%d %s", n, noun))
		}
	}
	if h.Users > 1 {
		what = append(what, fmt.Sprintf("%d accounts", h.Users))
	}
	add(h.Shelves, "shelves")
	add(h.Skills, "skills")
	add(h.LibraryItems, "library items")
	add(h.PrivateBundles, "private content bundles")
	add(h.Lists, "lists")
	add(h.MadeTags, "tags")
	add(h.MadeEnvironments, "environments")
	add(h.Machines, "connected machines")
	return what
}

// StageImportFile stages the sealed copy in the file at path, which an upload
// was streamed to. The format opens a copy in one piece, so the file is read
// whole here and no more than MaxImportBytes of it.
func (s *Service) StageImportFile(ctx context.Context, dataDir, path, password string, opts RestoreOptions) (RestoreReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return RestoreReport{}, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, MaxImportBytes+1))
	if err != nil {
		return RestoreReport{}, err
	}
	if len(raw) > MaxImportBytes {
		return RestoreReport{}, errors.New("that copy is larger than this build opens")
	}
	return s.StageImport(ctx, dataDir, raw, password, opts)
}

// StageImport opens an uploaded copy and stages it for the next start. A
// second import replaces the first. The copy is opened with this controller's
// key, or the password given; a password-protected copy needs one.
//
// Everything that can refuse runs before anything changes: whether this
// controller holds data (unless opts.Replace), opening the copy, and every
// check a restore makes. Then the live database is copied aside, and only
// then is the import staged.
func (s *Service) StageImport(ctx context.Context, dataDir string, raw []byte, password string, opts RestoreOptions) (RestoreReport, error) {
	if !opts.Replace {
		if what := holds(s.st.Holdings(ctx)); len(what) > 0 {
			return RestoreReport{}, fmt.Errorf("%w (%s); importing would replace it, so say you want to replace it, or import into a new controller", ErrNotEmpty, strings.Join(what, ", "))
		}
	}
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
	safety := filepath.Join(dataDir, "reet.db.before-import-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := s.st.VacuumInto(ctx, safety); err != nil {
		os.Remove(safety)
		return rep, fmt.Errorf("copying the live database aside before the import: %w", err)
	}
	if err := os.Chmod(safety, 0o600); err != nil {
		return rep, err
	}
	rep.SafetyCopy = safety
	if rep.StagedSHA256, err = fileDigest(filepath.Join(stage, pendingDB)); err != nil {
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

// fileDigest is the SHA-256 of a file, as hex.
func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// recheckStaged asks of a staged import what was asked of the copy when it was
// staged, now that time has passed: it is still the file that was checked, it
// still opens and passes SQLite's own check, and it is no newer than this
// binary reads. The copy's own password is not asked for again. It was spent
// opening the copy, and what is staged is the opened database, so the digest
// taken at staging stands in for "still opens with its password".
func recheckStaged(ctx context.Context, path string, rep *RestoreReport) error {
	if rep.StagedSHA256 != "" {
		got, err := fileDigest(path)
		if err != nil {
			return fmt.Errorf("the staged database cannot be read: %w", err)
		}
		if got != rep.StagedSHA256 {
			return errors.New("the staged database changed after it was checked, so it is no longer the copy that was staged")
		}
	}
	if err := store.CheckDatabaseFile(ctx, path); err != nil {
		return fmt.Errorf("the staged database no longer passes an integrity check: %w", err)
	}
	sum, err := store.InspectDatabaseFile(ctx, path)
	if err != nil {
		return fmt.Errorf("the staged database cannot be opened: %w", err)
	}
	if sum.SchemaVersion > store.SchemaVersion {
		return fmt.Errorf("%w: it is data format %d and this reet reads up to data format %d; upgrade reet, then stage it again", ErrNewerSchema, sum.SchemaVersion, store.SchemaVersion)
	}
	return nil
}

// refuseStaged moves a stage that failed its re-check aside, with the reason
// written into its report, and leaves the live database alone.
func refuseStaged(dir string, rep *RestoreReport, why error) error {
	rep.Refusal = why.Error()
	aside := dir + ".refused-" + time.Now().UTC().Format("20060102T150405Z")
	if err := os.Rename(dir, aside); err != nil {
		return fmt.Errorf("the staged import was not applied (%v), and could not be moved aside: %w", why, err)
	}
	if state, err := json.Marshal(rep); err == nil {
		_ = os.WriteFile(filepath.Join(aside, pendingState), state, 0o600)
	}
	return fmt.Errorf("the staged import %s was not applied and the live database was left as it is; it was moved to %s: %w", rep.ID, aside, why)
}

// ApplyPendingImport swaps a staged import in. It runs before the database is
// opened, at the start of serve. It returns nil when nothing is staged. The
// stage is checked again first, because it may have waited a long time; one
// that fails is moved aside with the reason and the live database is not
// touched. A stage that cannot be applied is moved aside so the controller
// still starts.
func ApplyPendingImport(dataDir string) (*RestoreResult, error) {
	rep, err := PendingImport(dataDir)
	if err != nil || rep == nil {
		return nil, err
	}
	dir := filepath.Join(dataDir, pendingDir)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := recheckStaged(ctx, filepath.Join(dir, pendingDB), rep); err != nil {
		return nil, refuseStaged(dir, rep, err)
	}
	previous, err := swapIn(dataDir, filepath.Join(dir, pendingDB))
	if err != nil {
		aside := dir + ".failed-" + time.Now().UTC().Format("20060102T150405Z")
		_ = os.Rename(dir, aside)
		return nil, fmt.Errorf("the staged import could not be applied and was moved to %s: %w", aside, err)
	}
	_ = os.RemoveAll(dir)
	return &RestoreResult{ID: rep.ID, Previous: previous, Report: *rep}, nil
}
