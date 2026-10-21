package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// Service takes, lists, verifies and restores copies. One instance's copies
// belong to owner 0; a person's belong to that person's account id.
type Service struct {
	st  *store.Store
	cfg Config
	log *slog.Logger
	mu  sync.Mutex
}

func New(st *store.Store, cfg Config) *Service {
	return &Service{st: st, cfg: cfg, log: slog.Default()}
}

func (s *Service) Config() Config { return s.cfg }

// Settings returns the effective schedule and retention for one owner.
func (s *Service) Settings(ctx context.Context, ownerID int64) (store.BackupSettings, error) {
	return s.settingsFor(ownerID)
}

func (s *Service) Usable() bool { return s.cfg.Usable && s.cfg.Sealer != nil }

// Item is one copy, here or at a destination.
type Item struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	OwnerID int64    `json:"-"`
	Target  string   `json:"target"`
	TakenAt int64    `json:"takenAt"`
	Bytes   int64    `json:"bytes"`
	Version string   `json:"version,omitempty"`
	Local   bool     `json:"local"`
	Remotes []string `json:"remotes"`
}

// RemoteView is one destination as a page draws it. A secret never leaves the
// server, only whether one is stored.
type RemoteView struct {
	Name         string `json:"name"`
	Source       string `json:"source"`
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	Prefix       string `json:"prefix"`
	AccessKey    string `json:"accessKey"`
	PathStyle    *bool  `json:"pathStyle"`
	Keep         int    `json:"keep"`
	Disabled     bool   `json:"disabled"`
	HasSecret    bool   `json:"hasSecret"`
	Where        string `json:"where"`
	Problem      string `json:"problem,omitempty"`
	Uploading    bool   `json:"uploading"`
	LastUploadAt int64  `json:"lastUploadAt,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	LastErrorAt  int64  `json:"lastErrorAt,omitempty"`
}

// StatusView is everything the Backups page needs in one answer.
type StatusView struct {
	Scope       string               `json:"scope"`
	Enabled     bool                 `json:"enabled"`
	Dir         string               `json:"dir"`
	Key         KeyView              `json:"key"`
	Settings    store.BackupSettings `json:"settings"`
	Defaults    Defaults             `json:"defaults"`
	Remotes     []RemoteView         `json:"remotes"`
	Runs        []store.BackupRun    `json:"runs"`
	Items       []Item               `json:"items"`
	Unreachable []string             `json:"unreachable,omitempty"`
	Findings    []Finding            `json:"findings"`
}

type KeyView struct {
	Fingerprint string `json:"fingerprint"`
	Source      string `json:"source"`
	Path        string `json:"path,omitempty"`
}

type Defaults struct {
	EverySeconds int64  `json:"everySeconds"`
	Keep         int    `json:"keep"`
	UserEvery    int64  `json:"userEverySeconds"`
	UserKeep     int    `json:"userKeep"`
	Destination  string `json:"destination"`
	Compression  string `json:"compression"`
}

type runtimeRemote struct {
	store.BackupRemote
	Source  string
	Problem string
	S3      S3
}

func anyUsableRemote(remotes []RemoteView) bool {
	for _, r := range remotes {
		if !r.Disabled && r.Problem == "" && r.HasSecret && r.Endpoint != "" {
			return true
		}
	}
	return false
}

func effectiveCompression(compression string) string {
	if compression == CompressionGzip || compression == CompressionNone {
		return compression
	}
	return DefaultCompression
}

func effectiveDestination(destination string) string {
	if store.ValidBackupDestination(destination) {
		return destination
	}
	return store.DestinationBoth
}

func scopeOf(ownerID int64) string {
	if ownerID == 0 {
		return "instance"
	}
	return "user"
}

func (s *Service) settingsFor(ownerID int64) (store.BackupSettings, error) {
	every, keep := int64(0), 0
	if ownerID == 0 {
		every, keep = int64(s.cfg.Every/time.Second), s.cfg.Keep
	} else {
		every, keep = int64(s.cfg.UserEvery/time.Second), s.cfg.UserKeep
	}
	destination := s.cfg.Destination
	if !store.ValidBackupDestination(destination) {
		destination = store.DestinationBoth
	}
	compression := s.cfg.Compression
	if compression != CompressionGzip && compression != CompressionNone {
		compression = DefaultCompression
	}
	return s.st.EnsureBackupSettings(ownerID, every, keep, destination, compression)
}

func (s *Service) loadRemotes(ownerID int64) ([]runtimeRemote, error) {
	var out []runtimeRemote
	if ownerID == 0 && s.cfg.S3.Configured() {
		problem := ""
		if s.cfg.S3.Bucket == "" || s.cfg.S3.AccessKey == "" || s.cfg.S3.SecretKey == "" {
			problem = "the environment names an endpoint but not all of its bucket, access key id and secret access key"
		}
		out = append(out, runtimeRemote{
			BackupRemote: store.BackupRemote{Name: s.cfg.RemoteName, PathStyle: pathStyleValue(s.cfg.S3.PathStyle), Keep: s.cfg.Keep},
			Source:       "environment", Problem: problem, S3: s.cfg.S3,
		})
	}
	stored, err := s.st.BackupRemotes(ownerID)
	if err != nil {
		return nil, err
	}
	for _, r := range stored {
		rm := runtimeRemote{BackupRemote: r, Source: "database", S3: S3{
			Endpoint: r.Endpoint, Region: r.Region, Bucket: r.Bucket, Prefix: r.Prefix,
			AccessKey: r.AccessKey, PathStyle: pathStylePtr(r.PathStyle),
		}}
		if r.SecretSealed != "" {
			if s.cfg.Sealer == nil {
				rm.Problem = "the backup key is missing or unusable, so the stored secret cannot be opened"
			} else if secret, err := s.cfg.Sealer.OpenSecret(r.SecretSealed); err != nil {
				rm.Problem = err.Error()
			} else {
				rm.S3.SecretKey = secret
			}
		} else {
			rm.Problem = "no secret access key is stored"
		}
		out = append(out, rm)
	}
	return out, nil
}

func pathStyleValue(p *bool) int {
	if p == nil {
		return -1
	}
	if *p {
		return 1
	}
	return 0
}

func pathStylePtr(v int) *bool {
	if v < 0 {
		return nil
	}
	b := v != 0
	return &b
}

func (s *Service) remoteViews(ownerID int64) ([]RemoteView, error) {
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]RemoteView, 0, len(remotes))
	for _, r := range remotes {
		out = append(out, RemoteView{
			Name: r.Name, Source: r.Source, Endpoint: r.Endpoint, Region: r.Region,
			Bucket: r.Bucket, Prefix: r.Prefix, AccessKey: r.AccessKey,
			PathStyle: r.S3.PathStyle, Keep: r.Keep, Disabled: r.Disabled,
			HasSecret: r.S3.SecretKey != "", Where: r.S3.Where(), Problem: r.Problem,
			Uploading: r.Uploading, LastUploadAt: r.LastUploadAt,
			LastError: r.LastError, LastErrorAt: r.LastErrorAt,
		})
	}
	return out, nil
}

func (s *Service) Status(ctx context.Context, ownerID int64) (StatusView, error) {
	settings, err := s.settingsFor(ownerID)
	if err != nil {
		return StatusView{}, err
	}
	remotes, err := s.remoteViews(ownerID)
	if err != nil {
		return StatusView{}, err
	}
	runs, err := s.st.ListBackupRuns(ownerID, 30)
	if err != nil {
		return StatusView{}, err
	}
	items, unreachable, err := s.Items(ctx, ownerID)
	if err != nil {
		return StatusView{}, err
	}
	view := StatusView{
		Scope: scopeOf(ownerID), Enabled: s.Usable(), Dir: s.cfg.Dir,
		Key:      KeyView{Source: s.cfg.KeySource, Path: s.cfg.KeyPath},
		Settings: settings, Remotes: remotes, Runs: runs, Items: items, Unreachable: unreachable,
		Defaults: Defaults{
			EverySeconds: int64(s.cfg.Every / time.Second), Keep: s.cfg.Keep,
			UserEvery: int64(s.cfg.UserEvery / time.Second), UserKeep: s.cfg.UserKeep,
			Destination: effectiveDestination(s.cfg.Destination),
			Compression: effectiveCompression(s.cfg.Compression),
		},
		Findings: append([]Finding(nil), s.cfg.Findings...),
	}
	if s.cfg.Sealer != nil {
		view.Key.Fingerprint = s.cfg.Sealer.Fingerprint()
	}
	for _, r := range remotes {
		if r.Problem != "" {
			view.Findings = append(view.Findings, Finding{Severity: "error", Code: "backup.remote_unusable",
				Message: r.Name + ": " + r.Problem})
		}
		if r.LastError != "" {
			view.Findings = append(view.Findings, Finding{Severity: "error", Code: "backup.remote_upload_failed",
				Message: r.Name + ": " + r.LastError})
		}
	}
	if settings.Destination == store.DestinationRemote && !anyUsableRemote(remotes) {
		view.Findings = append(view.Findings, Finding{Severity: "error", Code: "backup.no_offsite",
			Message: "Copies are set to go off-site only, but no destination is enabled and usable; every copy will fail until one is added and tested."})
	}
	if ownerID != 0 {
		// A person's page is not the place to disclose the server's key path or
		// the instance's configuration findings; it only needs to know whether
		// their own copies leave this machine.
		view.Dir = ""
		view.Key = KeyView{}
		enabled := 0
		for _, r := range remotes {
			if !r.Disabled && r.Problem == "" {
				enabled++
			}
		}
		view.Findings = view.Findings[:0]
		for _, r := range remotes {
			if r.Problem != "" {
				view.Findings = append(view.Findings, Finding{Severity: "error", Code: "backup.remote_unusable", Message: r.Name + ": " + r.Problem})
			}
			if r.LastError != "" {
				view.Findings = append(view.Findings, Finding{Severity: "error", Code: "backup.remote_upload_failed", Message: r.Name + ": " + r.LastError})
			}
		}
		if enabled == 0 {
			view.Findings = append(view.Findings, Finding{Severity: "warning", Code: "backup.no_offsite",
				Message: "Your copies are on this machine only. Add an S3-compatible destination here to keep them somewhere else too."})
		}
	}
	return view, nil
}

// SettingsInput is a change to one owner's backup settings. Password is
// nil to leave the stored copy password alone, a pointer to "" to remove it,
// and a pointer to a value to replace it.
type SettingsInput struct {
	EverySeconds int64
	Keep         int
	Destination  string
	Compression  string
	Password     *string
}

// SaveSettings stores a schedule, retention, where copies go and how they
// are stored for one owner.
func (s *Service) SaveSettings(ownerID int64, in SettingsInput) (store.BackupSettings, error) {
	if in.EverySeconds < 0 || (in.EverySeconds > 0 && in.EverySeconds < 60) {
		return store.BackupSettings{}, errors.New("the shortest schedule is a minute; 0 turns it off")
	}
	if in.Keep < 0 || in.Keep > MaxKeep {
		return store.BackupSettings{}, fmt.Errorf("keep a number of copies from 0 to %d; 0 keeps every one", MaxKeep)
	}
	destination := strings.ToLower(strings.TrimSpace(in.Destination))
	if destination == "" {
		destination = store.DestinationBoth
	}
	if !store.ValidBackupDestination(destination) {
		return store.BackupSettings{}, errors.New("copies go to local, remote or both")
	}
	compression := strings.ToLower(strings.TrimSpace(in.Compression))
	if compression == "" {
		compression = effectiveCompression(s.cfg.Compression)
	}
	if compression != CompressionGzip && compression != CompressionNone {
		return store.BackupSettings{}, errors.New("compression is gzip or none")
	}
	current, err := s.settingsFor(ownerID)
	if err != nil {
		return store.BackupSettings{}, err
	}
	if in.Password != nil {
		password := *in.Password
		if strings.TrimSpace(password) == "" {
			current.PasswordSealed = ""
		} else {
			if len(password) < MinPassword {
				return store.BackupSettings{}, fmt.Errorf("a copy password needs at least %d characters", MinPassword)
			}
			if s.cfg.Sealer == nil {
				return store.BackupSettings{}, errors.New("backups are not enabled: no usable backup key")
			}
			sealed, err := s.cfg.Sealer.SealSecret(password)
			if err != nil {
				return store.BackupSettings{}, err
			}
			current.PasswordSealed = sealed
		}
	}
	current.EverySeconds = in.EverySeconds
	current.Keep = in.Keep
	current.Destination = destination
	current.Compression = compression
	current.NextAt = 0
	if in.EverySeconds > 0 {
		current.NextAt = time.Now().Add(time.Duration(in.EverySeconds) * time.Second).UnixMilli()
	}
	if err := s.st.SaveBackupSettingsFor(current); err != nil {
		return store.BackupSettings{}, err
	}
	return s.st.BackupSettingsFor(ownerID)
}

// copyPassword opens the stored copy password for one owner, or "" when
// none is set.
func (s *Service) copyPassword(ownerID int64) (string, error) {
	settings, err := s.settingsFor(ownerID)
	if err != nil {
		return "", err
	}
	if settings.PasswordSealed == "" {
		return "", nil
	}
	if s.cfg.Sealer == nil {
		return "", errors.New("the backup key is missing, so the stored copy password cannot be opened")
	}
	return s.cfg.Sealer.OpenSecret(settings.PasswordSealed)
}

// openArchive opens a sealed copy. A password-protected one uses password,
// or the stored one when password is empty.
func (s *Service) openArchive(ownerID int64, data []byte, password string) (Manifest, []byte, error) {
	m, err := ManifestOf(data)
	if err != nil {
		return m, nil, err
	}
	if m.Protection != ProtectionPassword {
		if s.cfg.Sealer == nil {
			return m, nil, errors.New("the backup key is missing, so this copy cannot be opened")
		}
		return s.cfg.Sealer.Open(data)
	}
	if s.cfg.Sealer == nil {
		return m, nil, errors.New("backups are not enabled: no usable backup key")
	}
	if password == "" {
		password, err = s.copyPassword(ownerID)
		if err != nil {
			return m, nil, err
		}
	}
	if password == "" {
		return m, nil, errors.New("this copy is protected with a password; enter it to open it")
	}
	return s.cfg.Sealer.OpenWithPassword(data, password)
}

func (s *Service) newID(ownerID int64) string {
	ts := time.Now().UTC().Format("20060102T150405Z")
	nonce, _ := randomHex(4)
	if ownerID == 0 {
		return "instance-" + ts + "-" + nonce
	}
	return "user-" + strconv.FormatInt(ownerID, 10) + "-" + ts + "-" + nonce
}

func (s *Service) localPath(id string) string {
	if !validID(id) {
		return ""
	}
	return filepath.Join(s.cfg.Dir, id+".reetbackup")
}

func (s *Service) localManifest(id string) (Manifest, error) {
	if !validID(id) {
		return Manifest{}, os.ErrNotExist
	}
	data, err := os.ReadFile(s.localPath(id))
	if err != nil {
		return Manifest{}, err
	}
	return ManifestOf(data)
}

func (s *Service) localItems(ownerID int64) ([]Item, error) {
	entries, err := os.ReadDir(s.cfg.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Item{}, nil
		}
		return nil, err
	}
	scope := scopeOf(ownerID)
	out := []Item{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".reetbackup") {
			continue
		}
		f, err := os.Open(filepath.Join(s.cfg.Dir, name))
		if err != nil {
			continue
		}
		buf := make([]byte, 1<<20)
		n, _ := io.ReadFull(f, buf)
		f.Close()
		m, err := ManifestOf(buf[:n])
		if err != nil {
			continue
		}
		if !validID(m.ID) || m.Kind != scope || (ownerID != 0 && m.OwnerID != ownerID) {
			continue
		}
		out = append(out, Item{ID: m.ID, Kind: m.Kind, OwnerID: m.OwnerID, Target: m.Target,
			TakenAt: m.TakenAt, Bytes: m.Size, Version: m.Version, Local: true, Remotes: []string{}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TakenAt > out[j].TakenAt })
	return out, nil
}

func listingPrefix(prefix, scope string, ownerID int64) string {
	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	if scope == "instance" {
		return prefix + "instance/"
	}
	return prefix + "user-" + strconv.FormatInt(ownerID, 10) + "/"
}

// Items lists copies here and at every destination, merging the same copy
// into one row. A destination that does not answer is named, not fatal.
func (s *Service) Items(ctx context.Context, ownerID int64) ([]Item, []string, error) {
	items, err := s.localItems(ownerID)
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]int{}
	for i, it := range items {
		byID[it.ID] = i
	}
	scope := scopeOf(ownerID)
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return nil, nil, err
	}
	var unreachable []string
	for _, r := range remotes {
		if r.Disabled || r.Problem != "" || !r.S3.Configured() {
			continue
		}
		cl, err := newS3Client(r.S3)
		if err != nil {
			unreachable = append(unreachable, r.Name)
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		objects, err := cl.list(rctx, listingPrefix(r.S3.Prefix, scope, ownerID))
		cancel()
		if err != nil {
			unreachable = append(unreachable, r.Name)
			continue
		}
		for _, o := range objects {
			if !strings.HasSuffix(o.Key, ".reetbackup") {
				continue
			}
			base := filepath.Base(o.Key)
			id := strings.TrimSuffix(base, ".reetbackup")
			if !validID(id) {
				continue
			}
			if i, ok := byID[id]; ok {
				items[i].Remotes = append(items[i].Remotes, r.Name)
				continue
			}
			byID[id] = len(items)
			items = append(items, Item{ID: id, Kind: scope, OwnerID: ownerID, Target: targetOf(ownerID), TakenAt: o.LastModified.UnixMilli(), Bytes: o.Size, Local: false, Remotes: []string{r.Name}})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].TakenAt > items[j].TakenAt })
	return items, unreachable, nil
}

func targetOf(ownerID int64) string {
	if ownerID == 0 {
		return "instance"
	}
	return "user-" + strconv.FormatInt(ownerID, 10)
}

// RunInstance takes a copy of the whole database and ships it.
func (s *Service) RunInstance(ctx context.Context, trigger string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runLocked(ctx, 0, trigger)
}

// RunUser takes a copy of one person's shelf and ships it.
func (s *Service) RunUser(ctx context.Context, userID int64, trigger string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runLocked(ctx, userID, trigger)
}

func (s *Service) runLocked(ctx context.Context, ownerID int64, trigger string) (Item, error) {
	if !s.Usable() {
		return Item{}, errors.New("backups are not enabled: no usable backup key")
	}
	settings, err := s.settingsFor(ownerID)
	if err != nil {
		return Item{}, err
	}
	destination := effectiveDestination(settings.Destination)
	scope := scopeOf(ownerID)
	started := time.Now().UnixMilli()
	run, err := s.st.AddBackupRun(store.BackupRun{OwnerID: ownerID, Scope: scope, Trigger: trigger, State: "running", StartedAt: started})
	if err != nil {
		return Item{}, err
	}
	fail := func(cause error) (Item, error) {
		finished := time.Now().UnixMilli()
		_ = s.st.FinishBackupRun(run.ID, "failed", finished, 0, "", "", cause.Error())
		s.finish(ownerID, started, "failed", cause.Error())
		return Item{}, cause
	}
	if destination == store.DestinationRemote {
		remotes, err := s.enabledRemotes(ownerID)
		if err != nil {
			return fail(err)
		}
		if len(remotes) == 0 {
			return fail(errors.New("copies go to a destination only, but no enabled destination is configured and usable; add one, test it, and try again"))
		}
	}
	if err := os.MkdirAll(s.cfg.Dir, 0o700); err != nil {
		return fail(err)
	}
	id := s.newID(ownerID)
	var payload []byte
	if ownerID == 0 {
		temp, err := os.CreateTemp(s.cfg.Dir, ".reet-snapshot-*")
		if err != nil {
			return fail(err)
		}
		tempName := temp.Name()
		temp.Close()
		_ = os.Remove(tempName)
		defer os.Remove(tempName)
		vctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err = s.st.VacuumInto(vctx, tempName)
		cancel()
		if err != nil {
			return fail(fmt.Errorf("taking the database snapshot: %w", err))
		}
		if info, statErr := os.Stat(tempName); statErr == nil && info.Size() > maxObjectBytes {
			return fail(fmt.Errorf("the database snapshot is %d MiB, larger than this build's %d MiB limit; prune the event history or back it up as a file on this machine", info.Size()>>20, maxObjectBytes>>20))
		}
		payload, err = os.ReadFile(tempName)
		if err != nil {
			return fail(err)
		}
	} else {
		snap, err := s.st.ExportFor(ownerID, true)
		if err != nil {
			return fail(err)
		}
		payload, err = json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return fail(err)
		}
	}
	if len(payload) > maxObjectBytes {
		return fail(fmt.Errorf("this copy would be %d MiB, larger than this build's %d MiB limit; reduce the event history or keep the copy on this machine only", len(payload)>>20, maxObjectBytes>>20))
	}
	password, err := s.copyPassword(ownerID)
	if err != nil {
		return fail(err)
	}
	m := Manifest{ID: id, Kind: scope, OwnerID: ownerID, Target: targetOf(ownerID), TakenAt: started, Version: s.cfg.Version}
	sealed, err := s.cfg.Sealer.SealArchive(m, payload, SealOptions{
		Compress: settings.Compression != CompressionNone,
		Password: password,
	})
	if err != nil {
		return fail(err)
	}
	// Open what was just written, as a restore would, before it counts as a
	// copy: a bad seal or a bad compression must not reach a destination or
	// push a good copy out of retention.
	written, opened, err := s.openArchive(ownerID, sealed, password)
	if err != nil {
		return fail(fmt.Errorf("checking the copy just taken: %w", err))
	}
	if err := verifyContent(ctx, written, opened); err != nil {
		return fail(fmt.Errorf("checking the copy just taken: %w", err))
	}
	path := s.localPath(id)
	if err := writeAtomic(path, sealed, 0o600); err != nil {
		return fail(err)
	}
	offsite, shipErr := "skipped", error(nil)
	if destination != store.DestinationLocal {
		offsite, shipErr = s.ship(ctx, ownerID, id, sealed)
	}
	finished := time.Now().UnixMilli()
	item := Item{ID: id, Kind: scope, OwnerID: ownerID, Target: m.Target, TakenAt: started,
		Bytes: int64(len(sealed)), Version: s.cfg.Version, Local: true, Remotes: []string{}}
	var shipped map[string]string
	if json.Unmarshal([]byte(offsite), &shipped) == nil {
		for name, why := range shipped {
			if why == "" {
				item.Remotes = append(item.Remotes, name)
			}
		}
		sort.Strings(item.Remotes)
	}
	if destination == store.DestinationRemote && len(item.Remotes) > 0 {
		// The destination has it; keep this machine holding only what no
		// destination took. A copy kept solely at a destination is still
		// listed, downloaded and restored through the destination.
		if err := os.Remove(path); err == nil {
			path = ""
			item.Local = false
		}
	}
	if destination == store.DestinationRemote && len(item.Remotes) == 0 && shipErr == nil {
		shipErr = errors.New("no enabled destination took the copy, so it was kept on this machine")
	}
	if shipErr != nil {
		s.log.Error("backup shipped nowhere", "owner", ownerID, "id", id, "error", shipErr)
	}
	_ = s.st.FinishBackupRun(run.ID, "succeeded", finished, item.Bytes, path, offsite, "")
	s.finish(ownerID, started, "succeeded", "")
	if settings, err := s.settingsFor(ownerID); err == nil {
		_, _ = s.pruneLocal(ownerID, settings.Keep)
		_ = s.pruneRemotes(ctx, ownerID, settings.Keep)
	}
	_ = s.st.PruneBackupRuns(ownerID, 200)
	return item, nil
}

// enabledRemotes is every destination that could take a copy now: enabled,
// with a secret, and naming a service the client accepts.
func (s *Service) enabledRemotes(ownerID int64) ([]runtimeRemote, error) {
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]runtimeRemote, 0, len(remotes))
	for _, r := range remotes {
		if r.Disabled || r.Problem != "" || !r.S3.Configured() {
			continue
		}
		if _, err := newS3Client(r.S3); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// ship copies one archive to every enabled destination. It returns the
// per-destination outcome as JSON and an error only when nothing arrived and
// something was meant to.
func (s *Service) ship(ctx context.Context, ownerID int64, id string, data []byte) (string, error) {
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return "", err
	}
	outcome := map[string]string{}
	sent, failed := 0, 0
	var firstErr error
	for _, r := range remotes {
		if r.Disabled || r.Problem != "" {
			continue
		}
		cl, err := newS3Client(r.S3)
		if err != nil {
			outcome[nameOr(r.Name)] = err.Error()
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_ = s.st.SetBackupRemoteUploading(ownerID, r.Name, true)
		key := remotePath(r.S3.Prefix, scopeOf(ownerID), ownerID, id)
		putErr := cl.putRetry(ctx, key, data)
		at := time.Now().UnixMilli()
		msg := ""
		if putErr != nil {
			msg = putErr.Error()
			failed++
			if firstErr == nil {
				firstErr = putErr
			}
		} else {
			sent++
		}
		_ = s.st.SetBackupRemoteResult(ownerID, r.Name, at, msg)
		outcome[r.Name] = msg
	}
	if len(outcome) == 0 {
		return "skipped", nil
	}
	raw, _ := json.Marshal(outcome)
	if sent == 0 && failed > 0 {
		return string(raw), firstErr
	}
	return string(raw), nil
}

func nameOr(name string) string {
	if name == "" {
		return DefaultName
	}
	return name
}

func (s *Service) finish(ownerID, started int64, state, errText string) {
	settings, err := s.settingsFor(ownerID)
	if err != nil {
		return
	}
	next := int64(0)
	if settings.EverySeconds > 0 {
		next = time.Now().Add(time.Duration(settings.EverySeconds) * time.Second).UnixMilli()
	}
	_ = s.st.FinishBackup(ownerID, started, next, state, errText)
}

func (s *Service) pruneLocal(ownerID int64, keep int) ([]string, error) {
	if keep <= 0 {
		return nil, nil
	}
	items, err := s.localItems(ownerID)
	if err != nil {
		return nil, err
	}
	var removed []string
	for i, it := range items {
		if i < keep {
			continue
		}
		if err := os.Remove(s.localPath(it.ID)); err == nil {
			removed = append(removed, it.ID)
		}
	}
	return removed, nil
}

func (s *Service) pruneRemotes(ctx context.Context, ownerID int64, keep int) error {
	if keep <= 0 {
		return nil
	}
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return err
	}
	for _, r := range remotes {
		_, _ = s.pruneOneRemote(ctx, ownerID, r)
	}
	return nil
}

// pruneOneRemote removes what one destination holds beyond its own count.
func (s *Service) pruneOneRemote(ctx context.Context, ownerID int64, r runtimeRemote) ([]string, error) {
	if r.Disabled || r.Problem != "" || r.Keep <= 0 {
		return nil, nil
	}
	cl, err := newS3Client(r.S3)
	if err != nil {
		return nil, err
	}
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	objects, err := cl.list(rctx, listingPrefix(r.S3.Prefix, scopeOf(ownerID), ownerID))
	if err != nil {
		return nil, err
	}
	copies := objects[:0]
	for _, o := range objects {
		if strings.HasSuffix(o.Key, ".reetbackup") {
			copies = append(copies, o)
		}
	}
	sort.Slice(copies, func(i, j int) bool { return copies[i].LastModified.After(copies[j].LastModified) })
	var removed []string
	for i, o := range copies {
		if i < r.Keep {
			continue
		}
		if err := cl.remove(rctx, o.Key); err != nil {
			return removed, err
		}
		removed = append(removed, strings.TrimSuffix(filepath.Base(o.Key), ".reetbackup"))
	}
	return removed, nil
}

// SentCopy is one copy a copy-offsite pass sent to one destination.
type SentCopy struct {
	Remote string `json:"remote"`
	ID     string `json:"id"`
	Bytes  int64  `json:"bytes"`
}

// Shipment is what a copy-offsite pass did.
type Shipment struct {
	Sent   []SentCopy        `json:"sent"`
	Errors map[string]string `json:"errors"`
}

// ShipMissing makes each destination hold every copy this machine or
// another destination holds, then applies that destination's own retention.
// It is what to run after fixing a credential or adding a destination, and
// it works when a copy is no longer on this machine because copies go
// off-site only.
func (s *Service) ShipMissing(ctx context.Context, ownerID int64) (Shipment, error) {
	out := Shipment{Sent: []SentCopy{}, Errors: map[string]string{}}
	local, err := s.localItems(ownerID)
	if err != nil {
		return out, err
	}
	remotes, err := s.enabledRemotes(ownerID)
	if err != nil {
		return out, err
	}
	type source struct {
		r  runtimeRemote
		cl *s3Client
	}
	var sources []source
	held := map[string]map[string]bool{}
	ids := map[string]bool{}
	localIDs := map[string]bool{}
	for _, it := range local {
		ids[it.ID] = true
		localIDs[it.ID] = true
	}
	for _, r := range remotes {
		cl, err := newS3Client(r.S3)
		if err != nil {
			out.Errors[r.Name] = err.Error()
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		objects, err := cl.list(rctx, listingPrefix(r.S3.Prefix, scopeOf(ownerID), ownerID))
		cancel()
		if err != nil {
			out.Errors[r.Name] = err.Error()
			continue
		}
		held[r.Name] = map[string]bool{}
		for _, o := range objects {
			if !strings.HasSuffix(o.Key, ".reetbackup") {
				continue
			}
			id := strings.TrimSuffix(filepath.Base(o.Key), ".reetbackup")
			if validID(id) {
				held[r.Name][id] = true
				ids[id] = true
			}
		}
		sources = append(sources, source{r: r, cl: cl})
	}
	for _, src := range sources {
		r := src.r
		for id := range ids {
			if held[r.Name][id] {
				continue
			}
			var data []byte
			if localIDs[id] {
				data, err = os.ReadFile(s.localPath(id))
			}
			if data == nil || err != nil {
				data = nil
				for _, other := range sources {
					if other.r.Name == r.Name || !held[other.r.Name][id] {
						continue
					}
					rctx, cancel := context.WithTimeout(ctx, 60*time.Second)
					data, err = other.cl.get(rctx, remotePath(other.r.S3.Prefix, scopeOf(ownerID), ownerID, id))
					cancel()
					if err == nil {
						break
					}
					data = nil
				}
			}
			if data == nil {
				continue
			}
			if err := src.cl.putRetry(ctx, remotePath(r.S3.Prefix, scopeOf(ownerID), ownerID, id), data); err != nil {
				out.Errors[r.Name] = err.Error()
				_ = s.st.SetBackupRemoteResult(ownerID, r.Name, time.Now().UnixMilli(), err.Error())
				break
			}
			held[r.Name][id] = true
			_ = s.st.SetBackupRemoteResult(ownerID, r.Name, time.Now().UnixMilli(), "")
			out.Sent = append(out.Sent, SentCopy{Remote: r.Name, ID: id, Bytes: int64(len(data))})
		}
		if _, err := s.pruneOneRemote(ctx, ownerID, r); err != nil && out.Errors[r.Name] == "" {
			out.Errors[r.Name] = err.Error()
		}
	}
	return out, nil
}

// RemotePrune is what retention removed from one destination.
type RemotePrune struct {
	Name    string   `json:"name"`
	Removed []string `json:"removed"`
	Error   string   `json:"error,omitempty"`
}

// PruneResult is what applying retention now removed, here and per
// destination.
type PruneResult struct {
	Removed []string      `json:"removed"`
	Remotes []RemotePrune `json:"remotes"`
}

// PruneNow applies retention to this machine and every destination by its
// own count, without taking a new copy.
func (s *Service) PruneNow(ctx context.Context, ownerID int64) (PruneResult, error) {
	settings, err := s.settingsFor(ownerID)
	if err != nil {
		return PruneResult{}, err
	}
	removed, err := s.pruneLocal(ownerID, settings.Keep)
	if err != nil {
		return PruneResult{}, err
	}
	out := PruneResult{Removed: removed, Remotes: []RemotePrune{}}
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return out, err
	}
	for _, r := range remotes {
		gone, err := s.pruneOneRemote(ctx, ownerID, r)
		rp := RemotePrune{Name: r.Name, Removed: gone, Error: ""}
		if err != nil {
			rp.Error = err.Error()
		}
		out.Remotes = append(out.Remotes, rp)
	}
	return out, nil
}

// DeleteRemoteCopy removes one copy from one destination.
func (s *Service) DeleteRemoteCopy(ctx context.Context, ownerID int64, name, id string) error {
	if !validID(id) {
		return os.ErrNotExist
	}
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return err
	}
	for _, r := range remotes {
		if r.Name != name {
			continue
		}
		if r.Problem != "" {
			return errors.New(r.Problem)
		}
		cl, err := newS3Client(r.S3)
		if err != nil {
			return err
		}
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return cl.remove(rctx, remotePath(r.S3.Prefix, scopeOf(ownerID), ownerID, id))
	}
	return store.ErrNotFound
}

// Download opens one copy, from disk or from a destination that still has
// it. A copy protected with a password uses the one stored for its owner.
func (s *Service) Download(ctx context.Context, ownerID int64, id string) (Manifest, []byte, error) {
	return s.download(ctx, ownerID, id, "")
}

// DownloadWithPassword opens a copy with a password typed by a person,
// which is what a machine rebuilt from nothing has and a stored setting it
// does not.
func (s *Service) DownloadWithPassword(ctx context.Context, ownerID int64, id, password string) (Manifest, []byte, error) {
	return s.download(ctx, ownerID, id, password)
}

func (s *Service) download(ctx context.Context, ownerID int64, id, password string) (Manifest, []byte, error) {
	if !validID(id) {
		return Manifest{}, nil, os.ErrNotExist
	}
	if data, err := os.ReadFile(s.localPath(id)); err == nil {
		m, err := ManifestOf(data)
		if err != nil {
			return Manifest{}, nil, err
		}
		if err := ownedBy(m, ownerID); err != nil {
			return Manifest{}, nil, err
		}
		return s.openArchive(ownerID, data, password)
	}
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return Manifest{}, nil, err
	}
	for _, r := range remotes {
		if r.Disabled || r.Problem != "" {
			continue
		}
		cl, err := newS3Client(r.S3)
		if err != nil {
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		data, err := cl.get(rctx, remotePath(r.S3.Prefix, scopeOf(ownerID), ownerID, id))
		cancel()
		if err != nil {
			continue
		}
		m, err := ManifestOf(data)
		if err != nil {
			return Manifest{}, nil, err
		}
		if err := ownedBy(m, ownerID); err != nil {
			return Manifest{}, nil, err
		}
		return s.openArchive(ownerID, data, password)
	}
	return Manifest{}, nil, os.ErrNotExist
}

func ownedBy(m Manifest, ownerID int64) error {
	if ownerID == 0 && m.Kind != "instance" {
		return errors.New("that copy belongs to a person, not the instance")
	}
	if ownerID != 0 && (m.Kind != "user" || m.OwnerID != ownerID) {
		return errors.New("that copy belongs to somebody else")
	}
	return nil
}

// Archive returns the sealed bytes as they are stored, for a download that
// keeps the encrypted copy rather than the plaintext inside it.
func (s *Service) Archive(ctx context.Context, ownerID int64, id string) ([]byte, error) {
	if !validID(id) {
		return nil, os.ErrNotExist
	}
	if data, err := os.ReadFile(s.localPath(id)); err == nil {
		m, err := ManifestOf(data)
		if err != nil {
			return nil, err
		}
		if err := ownedBy(m, ownerID); err != nil {
			return nil, err
		}
		return data, nil
	}
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return nil, err
	}
	for _, r := range remotes {
		if r.Disabled || r.Problem != "" {
			continue
		}
		cl, err := newS3Client(r.S3)
		if err != nil {
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		data, err := cl.get(rctx, remotePath(r.S3.Prefix, scopeOf(ownerID), ownerID, id))
		cancel()
		if err != nil {
			continue
		}
		m, err := ManifestOf(data)
		if err != nil {
			return nil, err
		}
		if err := ownedBy(m, ownerID); err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, os.ErrNotExist
}

// VerifyResult says whether a copy opens and what is wrong when it does not.
type VerifyResult struct {
	OK       bool     `json:"ok"`
	Problems []string `json:"problems"`
}

// Verify opens a copy as a restore would.
func (s *Service) Verify(ctx context.Context, ownerID int64, id string) VerifyResult {
	return s.VerifyWithPassword(ctx, ownerID, id, "")
}

// VerifyWithPassword verifies a copy with a password typed by a person,
// which an old password-protected copy needs after its stored password has
// been removed.
func (s *Service) VerifyWithPassword(ctx context.Context, ownerID int64, id, password string) VerifyResult {
	m, payload, err := s.DownloadWithPassword(ctx, ownerID, id, password)
	if err != nil {
		return VerifyResult{Problems: []string{err.Error()}}
	}
	if err := verifyContent(ctx, m, payload); err != nil {
		return VerifyResult{Problems: []string{err.Error()}}
	}
	return VerifyResult{OK: true, Problems: []string{}}
}

// verifyContent is what a restore checks in an opened copy: SQLite's own
// check for the instance's database, and a shelf that is valid JSON with
// skills in it.
func verifyContent(ctx context.Context, m Manifest, payload []byte) error {
	if m.Kind == "instance" {
		temp, err := os.CreateTemp("", "reet-verify-*.db")
		if err != nil {
			return err
		}
		name := temp.Name()
		temp.Close()
		defer os.Remove(name)
		if err := os.WriteFile(name, payload, 0o600); err != nil {
			return err
		}
		vctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = store.CheckDatabaseFile(vctx, name)
		cancel()
		return err
	}
	var snap store.Snapshot
	if err := json.Unmarshal(payload, &snap); err != nil {
		return errors.New("the shelf backup is not valid JSON: " + err.Error())
	}
	if snap.Skills == nil {
		return errors.New("the shelf backup has no skills")
	}
	return nil
}

// RestoreUser puts one person's shelf back. It never touches another person's.
func (s *Service) RestoreUser(ctx context.Context, userID int64, id string) (int, error) {
	return s.RestoreUserWithPassword(ctx, userID, id, "")
}

// RestoreUserWithPassword restores a shelf from a copy whose password is
// typed by the person.
func (s *Service) RestoreUserWithPassword(ctx context.Context, userID int64, id, password string) (int, error) {
	m, payload, err := s.DownloadWithPassword(ctx, userID, id, password)
	if err != nil {
		return 0, err
	}
	if m.Kind != "user" || m.OwnerID != userID {
		return 0, errors.New("that copy belongs to somebody else")
	}
	var snap store.Snapshot
	if err := json.Unmarshal(payload, &snap); err != nil {
		return 0, err
	}
	if snap.Skills == nil {
		return 0, errors.New("the backup has no skills")
	}
	if err := s.st.RestoreSnapshotFor(userID, snap); err != nil {
		return 0, err
	}
	return len(snap.Skills), nil
}

// DeleteLocal removes one copy from this machine only.
func (s *Service) DeleteLocal(ownerID int64, id string) error {
	if !validID(id) {
		return os.ErrNotExist
	}
	m, err := s.localManifest(id)
	if err != nil {
		return err
	}
	if err := ownedBy(m, ownerID); err != nil {
		return err
	}
	return os.Remove(s.localPath(id))
}

// RemoteInput is a destination as the page sends it. Every field is optional
// so a change carries only what somebody changed. OriginalName is the name
// an edit started from, so a draft test can find the stored secret across a
// rename; saving ignores it.
type RemoteInput struct {
	Name         *string         `json:"name"`
	OriginalName *string         `json:"originalName,omitempty"`
	Endpoint     *string         `json:"endpoint"`
	Region       *string         `json:"region"`
	Bucket       *string         `json:"bucket"`
	Prefix       *string         `json:"prefix"`
	AccessKey    *string         `json:"accessKey"`
	SecretKey    *string         `json:"secretKey"`
	PathStyle    json.RawMessage `json:"pathStyle"`
	Keep         *int            `json:"keep"`
	Enabled      *bool           `json:"enabled"`
}

var remoteNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var backupIDPattern = regexp.MustCompile(`^(instance|user-[0-9]+)-[0-9]{8}T[0-9]{6}Z-[a-f0-9]{8}$`)

func validID(id string) bool { return backupIDPattern.MatchString(id) }

func (s *Service) applyRemote(in RemoteInput, r store.BackupRemote, creating bool) (store.BackupRemote, error) {
	if s.cfg.Sealer == nil {
		return r, errors.New("backups are not enabled: no usable backup key")
	}
	oldEndpoint := r.Endpoint
	r, err := s.mergeRemote(in, r, creating, true)
	if err != nil {
		return r, err
	}
	secretGiven := in.SecretKey != nil && strings.TrimSpace(*in.SecretKey) != ""
	if in.SecretKey != nil && !secretGiven {
		return r, errors.New("enter the secret access key; a destination without one is refused")
	}
	if secretGiven {
		sealed, err := s.cfg.Sealer.SealSecret(strings.TrimSpace(*in.SecretKey))
		if err != nil {
			return r, err
		}
		r.SecretSealed = sealed
	} else if creating {
		return r, errors.New("enter the secret access key")
	} else if !strings.EqualFold(strings.TrimRight(oldEndpoint, "/"), r.Endpoint) && r.SecretSealed != "" {
		return r, fmt.Errorf("the stored secret is for %s; enter it again to use it with %s", oldEndpoint, r.Endpoint)
	}
	return r, nil
}

// mergeRemote applies the fields of a form to a destination and checks them.
// The secret is handled separately: saving seals it; testing uses it as it
// was typed, or the one already stored. checkEnv refuses a stored
// destination that would share the environment's name.
func (s *Service) mergeRemote(in RemoteInput, r store.BackupRemote, creating, checkEnv bool) (store.BackupRemote, error) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	set(&r.Name, in.Name)
	set(&r.Endpoint, in.Endpoint)
	set(&r.Region, in.Region)
	set(&r.Bucket, in.Bucket)
	set(&r.Prefix, in.Prefix)
	set(&r.AccessKey, in.AccessKey)
	r.Name = strings.ToLower(strings.TrimSpace(r.Name))
	r.Endpoint = strings.TrimRight(strings.TrimSpace(r.Endpoint), "/")
	if len(in.PathStyle) > 0 {
		var p *bool
		if err := json.Unmarshal(in.PathStyle, &p); err != nil {
			return r, errors.New("pathStyle is true, false or null")
		}
		r.PathStyle = pathStyleValue(p)
	}
	if in.Keep != nil {
		r.Keep = *in.Keep
	} else if creating {
		if r.OwnerID == 0 {
			r.Keep = s.cfg.Keep
		} else {
			r.Keep = s.cfg.UserKeep
		}
	}
	if in.Enabled != nil {
		r.Disabled = !*in.Enabled
	}
	if r.Name == "" {
		r.Name = DefaultName
	}
	if !remoteNamePattern.MatchString(r.Name) {
		return r, fmt.Errorf("the name %q is not one a destination can have; use lower-case letters, digits and dashes", r.Name)
	}
	if checkEnv && s.cfg.S3.Configured() && r.Name == s.cfg.RemoteName {
		return r, errors.New("the environment already describes a destination with that name; choose another")
	}
	u, err := url.Parse(r.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return r, fmt.Errorf("the endpoint %q is not the service's URL; write one like https://s3.eu-west-2.amazonaws.com or http://minio:9000", r.Endpoint)
	}
	if u.Scheme != "https" && !s.cfg.AllowLocal && !onThisMachine(u.Hostname()) {
		return r, errors.New("use https:// for a destination on another machine; an http:// endpoint sends the access key id in the clear")
	}
	switch {
	case r.Bucket == "":
		return r, errors.New("name the bucket; reet never creates one")
	case strings.ContainsAny(r.Bucket, "/ \t"):
		return r, errors.New("the bucket name has no slash or space; put a path in the prefix")
	case r.AccessKey == "":
		return r, errors.New("enter the access key id")
	}
	r.Prefix = strings.Trim(r.Prefix, "/")
	if r.Prefix != "" {
		r.Prefix += "/"
	}
	if r.Keep < 0 || r.Keep > MaxKeep {
		return r, fmt.Errorf("keep a number of copies from 0 to %d; 0 keeps every one", MaxKeep)
	}
	return r, nil
}

// TestRemoteDraft checks a destination as it stands in the form, before it
// is saved. With no secret typed, the one already stored for that name is
// used, but only while the endpoint is unchanged.
func (s *Service) TestRemoteDraft(ctx context.Context, ownerID int64, in RemoteInput) ([]CheckStep, error) {
	if s.cfg.Sealer == nil {
		return nil, errors.New("backups are not enabled: no usable backup key")
	}
	base, storedSecret, found, err := s.draftRemote(ownerID, in)
	if err != nil {
		return nil, err
	}
	merged, err := s.mergeRemote(in, base, !found, false)
	if err != nil {
		return nil, err
	}
	secret := ""
	if in.SecretKey != nil {
		secret = strings.TrimSpace(*in.SecretKey)
	}
	if secret == "" {
		if !found {
			return nil, errors.New("enter the secret access key")
		}
		if !strings.EqualFold(strings.TrimRight(base.Endpoint, "/"), merged.Endpoint) {
			return nil, fmt.Errorf("the stored secret is for %s; enter it again to use it with %s", base.Endpoint, merged.Endpoint)
		}
		secret = storedSecret
		if secret == "" {
			return nil, errors.New("no secret access key is stored; enter it")
		}
	}
	return Test(ctx, S3{
		Endpoint: merged.Endpoint, Region: merged.Region, Bucket: merged.Bucket, Prefix: merged.Prefix,
		AccessKey: merged.AccessKey, SecretKey: secret, PathStyle: pathStylePtr(merged.PathStyle),
	})
}

// draftRemote finds the destination a form is editing by name, and the
// secret already stored for it, whether it comes from the environment or the
// database.
func (s *Service) draftRemote(ownerID int64, in RemoteInput) (store.BackupRemote, string, bool, error) {
	name := DefaultName
	if in.OriginalName != nil && strings.TrimSpace(*in.OriginalName) != "" {
		name = strings.ToLower(strings.TrimSpace(*in.OriginalName))
	} else if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		name = strings.ToLower(strings.TrimSpace(*in.Name))
	}
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return store.BackupRemote{}, "", false, err
	}
	for _, r := range remotes {
		if r.Name != name {
			continue
		}
		if r.Problem != "" {
			return store.BackupRemote{}, "", true, errors.New(r.Problem)
		}
		return r.BackupRemote, r.S3.SecretKey, true, nil
	}
	return store.BackupRemote{OwnerID: ownerID}, "", false, nil
}

// CreateRemote stores a destination for one owner.
func (s *Service) CreateRemote(ownerID int64, in RemoteInput) (RemoteView, error) {
	r, err := s.applyRemote(in, store.BackupRemote{OwnerID: ownerID}, true)
	if err != nil {
		return RemoteView{}, err
	}
	r.OwnerID = ownerID
	r, err = s.st.CreateBackupRemote(r)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return RemoteView{}, errors.New("a destination with that name already exists")
		}
		return RemoteView{}, err
	}
	views, err := s.remoteViews(ownerID)
	if err != nil {
		return RemoteView{}, err
	}
	for _, v := range views {
		if v.Name == r.Name {
			return v, nil
		}
	}
	return RemoteView{}, nil
}

// UpdateRemote changes a stored destination. The environment's is changed
// where it is described, not here.
func (s *Service) UpdateRemote(ownerID int64, name string, in RemoteInput) (RemoteView, error) {
	before, ok, err := s.st.BackupRemoteByName(ownerID, name)
	if err != nil {
		return RemoteView{}, err
	}
	if !ok {
		if ownerID == 0 && name == s.cfg.RemoteName && s.cfg.S3.Configured() {
			return RemoteView{}, errors.New("that destination is described by the environment; change it there")
		}
		return RemoteView{}, store.ErrNotFound
	}
	r, err := s.applyRemote(in, before, false)
	if err != nil {
		return RemoteView{}, err
	}
	r.ID, r.OwnerID = before.ID, ownerID
	if in.Name != nil {
		r.Name = before.Name
	}
	if in.Name != nil && strings.ToLower(strings.TrimSpace(*in.Name)) != before.Name {
		// Renaming is a new name for the same row, and the unique index must
		// not see both at once.
		if err := s.st.DeleteBackupRemote(ownerID, before.Name); err != nil {
			return RemoteView{}, err
		}
		r.Name = strings.ToLower(strings.TrimSpace(*in.Name))
		if _, err := s.st.CreateBackupRemote(r); err != nil {
			_, _ = s.st.CreateBackupRemote(before)
			return RemoteView{}, err
		}
	} else if err := s.st.UpdateBackupRemote(r); err != nil {
		return RemoteView{}, err
	}
	views, err := s.remoteViews(ownerID)
	if err != nil {
		return RemoteView{}, err
	}
	for _, v := range views {
		if v.Name == r.Name {
			return v, nil
		}
	}
	return RemoteView{}, nil
}

func (s *Service) DeleteRemote(ownerID int64, name string) error {
	if ownerID == 0 && name == s.cfg.RemoteName && s.cfg.S3.Configured() {
		return errors.New("that destination is described by the environment; remove it there")
	}
	return s.st.DeleteBackupRemote(ownerID, name)
}

// TestRemote writes, reads and deletes a small object at one destination.
func (s *Service) TestRemote(ctx context.Context, ownerID int64, name string) ([]CheckStep, error) {
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return nil, err
	}
	for _, r := range remotes {
		if r.Name == name {
			if r.Problem != "" {
				return nil, errors.New(r.Problem)
			}
			return Test(ctx, r.S3)
		}
	}
	return nil, store.ErrNotFound
}

// RemoteCopies lists what one destination holds for one owner.
func (s *Service) RemoteCopies(ctx context.Context, ownerID int64, name string) ([]Item, error) {
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return nil, err
	}
	for _, r := range remotes {
		if r.Name != name {
			continue
		}
		if r.Problem != "" {
			return nil, errors.New(r.Problem)
		}
		cl, err := newS3Client(r.S3)
		if err != nil {
			return nil, err
		}
		objects, err := cl.list(ctx, listingPrefix(r.S3.Prefix, scopeOf(ownerID), ownerID))
		if err != nil {
			return nil, err
		}
		out := []Item{}
		for _, o := range objects {
			if !strings.HasSuffix(o.Key, ".reetbackup") {
				continue
			}
			id := strings.TrimSuffix(filepath.Base(o.Key), ".reetbackup")
			if !validID(id) {
				continue
			}
			out = append(out, Item{ID: id, Kind: scopeOf(ownerID), OwnerID: ownerID, Target: targetOf(ownerID),
				TakenAt: o.LastModified.UnixMilli(), Bytes: o.Size, Remotes: []string{name}})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].TakenAt > out[j].TakenAt })
		return out, nil
	}
	return nil, store.ErrNotFound
}

// Start runs the schedule. It is safe to call once; every copy is serialized,
// so a slow destination delays the next one rather than overlapping it.
func (s *Service) Start(ctx context.Context) {
	if !s.Usable() {
		return
	}
	go func() {
		s.runDue(ctx)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runDue(ctx)
			}
		}
	}()
}

func (s *Service) runDue(ctx context.Context) {
	now := time.Now().UnixMilli()
	if settings, err := s.settingsFor(0); err == nil && settings.EverySeconds > 0 && (settings.NextAt == 0 || settings.NextAt <= now) {
		if _, err := s.RunInstance(ctx, "schedule"); err != nil {
			s.log.Error("scheduled instance backup failed", "error", err)
		}
	}
	ids, err := s.st.BackupDueUsers(now)
	if err != nil {
		s.log.Error("reading due user backups", "error", err)
		return
	}
	for _, id := range ids {
		if _, err := s.RunUser(ctx, id, "schedule"); err != nil {
			s.log.Error("scheduled user backup failed", "user", id, "error", err)
		}
	}
}

// LocalItems lists only the copies on this machine, without touching a
// destination.
func (s *Service) LocalItems(ownerID int64) ([]Item, error) { return s.localItems(ownerID) }

// RunNow is the page's button: it picks the scope from the owner id.
func (s *Service) RunNow(ctx context.Context, ownerID int64, trigger string) (Item, error) {
	if ownerID == 0 {
		return s.RunInstance(ctx, trigger)
	}
	return s.RunUser(ctx, ownerID, trigger)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".reet-backup-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func onThisMachine(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}
