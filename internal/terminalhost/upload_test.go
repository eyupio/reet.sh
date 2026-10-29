//go:build linux || darwin

package terminalhost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const uploadTestToken = "token0123456789abcdef"

func uploadPNG() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), []byte("payload")...)
}

func uploadRequest(payload []byte, mime string) terminalproto.Upload {
	sum := sha256.Sum256(payload)
	return terminalproto.Upload{
		StreamToken: uploadTestToken,
		Size:        int64(len(payload)),
		SHA256:      hex.EncodeToString(sum[:]),
		MIME:        mime,
	}
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func uploadTestLayout(t *testing.T) Layout {
	t.Helper()
	dir := filepath.Join(resolvedTempDir(t), "terminal")
	layout := Layout{
		Dir:    dir,
		Socket: filepath.Join(dir, "tmux.sock"),
		Config: filepath.Join(dir, "tmux.conf"),
		Logs:   filepath.Join(dir, "logs"),
	}
	if err := Ensure(layout); err != nil {
		t.Fatal(err)
	}
	return layout
}

// uploadTestManager builds a manager with a durable Running session without a
// tmux server; upload storage itself never needs one.
func uploadTestManager(t *testing.T, state terminalproto.SessionState) (*Manager, string, string) {
	t.Helper()
	layout := uploadTestLayout(t)
	m := &Manager{
		enrollment:  testEnrollment,
		layout:      layout,
		home:        filepath.Dir(layout.Dir),
		consent:     func() error { return nil },
		attachments: map[string]*Attachment{},
		deadSeen:    map[string]time.Time{},
		changed:     make(chan struct{}),
	}
	t.Cleanup(func() { _ = m.Close() })
	id, request := testSessionID(1), testRequestID(1)
	at := time.Now().UnixMilli()
	create := terminalproto.Create{Title: "Uploads", Cols: 80, Rows: 24}
	registry := hostRegistry{
		Version:      1,
		EnrollmentID: testEnrollment,
		Sessions: map[string]ManagedSession{
			id: {ID: id, Title: "Uploads", State: state, CreatedAt: at, UpdatedAt: at, CreateRequestID: request},
		},
		Operations: map[string]operationReceipt{
			receiptKey("create", request): {RequestID: request, SessionID: id, Kind: "create", Create: &create},
		},
	}
	if err := m.saveRegistry(registry); err != nil {
		t.Fatal(err)
	}
	return m, id, filepath.Join(layout.Dir, "uploads")
}

func writeUpload(t *testing.T, m *Manager, id string, payload []byte, mime string) string {
	t.Helper()
	path, err := m.WriteUpload(context.Background(), id, uploadRequest(payload, mime), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func uploadEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func assertNoTemps(t *testing.T, dir string) {
	t.Helper()
	for _, name := range uploadEntries(t, dir) {
		if strings.HasPrefix(name, ".tmp-") {
			t.Fatalf("temporary upload left behind: %s", name)
		}
	}
}

func TestUploadStorageIsPrivateNamedAndExact(t *testing.T) {
	m, id, root := uploadTestManager(t, terminalproto.Running)
	payload := uploadPNG()
	path := writeUpload(t, m, id, payload, "image/png")

	for _, dir := range []string{root, filepath.Join(root, id)} {
		info, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0700 || ownedByCurrentUser(info) != nil {
			t.Fatalf("upload directory is not private: %s %v", dir, info.Mode())
		}
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("upload path is not absolute: %q", path)
	}
	if rel, err := filepath.Rel(root, path); err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("upload escaped its root: %q", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || ownedByCurrentUser(info) != nil {
		t.Fatalf("upload file is not private: %v", info.Mode())
	}
	if name := filepath.Base(path); !regexp.MustCompile(`^img-[0-9a-f]{32}\.png$`).MatchString(name) {
		t.Fatalf("unexpected upload name %q", name)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("stored payload changed: %v", err)
	}
	assertNoTemps(t, filepath.Join(root, id))
}

func TestUploadExtensionFollowsMagicBytes(t *testing.T) {
	cases := []struct {
		name    string
		mime    string
		ext     string
		payload []byte
	}{
		{"png", "image/png", "png", append([]byte("\x89PNG\r\n\x1a\n"), []byte("x")...)},
		{"jpeg", "image/jpeg", "jpg", []byte{0xff, 0xd8, 0xff, 0x00}},
		{"gif", "image/gif", "gif", append([]byte("GIF89a"), []byte("x")...)},
		{"webp", "image/webp", "webp", append([]byte("RIFF\x00\x00\x00\x00WEBP"), []byte("x")...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, id, root := uploadTestManager(t, terminalproto.Running)
			path := writeUpload(t, m, id, tc.payload, tc.mime)
			if got := filepath.Ext(path); got != "."+tc.ext {
				t.Fatalf("extension %q for %s", got, tc.mime)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, tc.payload) {
				t.Fatalf("stored payload changed: %v", err)
			}
			_ = root
		})
	}
}

func TestUploadRejectsBadDeclarationsAndStreams(t *testing.T) {
	valid := uploadPNG()
	longer := append(append([]byte(nil), valid...), byte('!'))
	short := valid[:len(valid)-1]
	wrongSHA := uploadRequest(valid, "image/png")
	wrongSHA.SHA256 = strings.Repeat("0", 64)
	zero := uploadRequest(valid, "image/png")
	zero.Size = 0
	over := uploadRequest(valid, "image/png")
	over.Size = terminalproto.MaxUploadBytes + 1
	tooShort := uploadRequest(valid, "image/png")
	tooShort.Size = int64(len(valid) + 1)
	tooLong := uploadRequest(short, "image/png")
	typeMismatch := uploadRequest(valid, "image/jpeg")
	wrongMagicPayload := []byte("not-an-image-at-all")
	wrongMagic := uploadRequest(wrongMagicPayload, "image/png")
	empty := uploadRequest(valid[:1], "image/png")

	cases := []struct {
		name    string
		req     terminalproto.Upload
		payload []byte
		want    error
	}{
		{"zero declared size", zero, valid, terminalproto.ErrInvalid},
		{"over maximum declared size", over, nil, ErrUploadTooLarge},
		{"short stream", tooShort, valid, ErrUploadMismatch},
		{"extra stream byte", tooLong, longer, ErrUploadMismatch},
		{"wrong sha", wrongSHA, valid, ErrUploadMismatch},
		{"wrong magic", wrongMagic, wrongMagicPayload, ErrUploadType},
		{"mime mismatch", typeMismatch, valid, ErrUploadType},
		{"empty stream", empty, nil, ErrUploadMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, id, root := uploadTestManager(t, terminalproto.Running)
			if _, err := m.WriteUpload(context.Background(), id, tc.req, bytes.NewReader(tc.payload)); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			sessionDir := filepath.Join(root, id)
			assertNoTemps(t, sessionDir)
			for _, name := range uploadEntries(t, sessionDir) {
				if strings.HasPrefix(name, "img-") {
					t.Fatalf("failed upload published %q", name)
				}
			}
		})
	}
}

type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestUploadReadsNoMoreThanTheDeclaredSize(t *testing.T) {
	m, id, _ := uploadTestManager(t, terminalproto.Running)
	payload := []byte("x")
	req := uploadRequest(payload, "image/png")
	source := &countingReader{r: bytes.NewReader(bytes.Repeat([]byte("y"), terminalproto.MaxUploadBytes))}
	if _, err := m.WriteUpload(context.Background(), id, req, source); !errors.Is(err, ErrUploadMismatch) {
		t.Fatalf("got %v, want ErrUploadMismatch", err)
	}
	if source.n > int(req.Size)+1 {
		t.Fatalf("read %d bytes for a %d-byte declaration", source.n, req.Size)
	}
}

func TestUploadRefusesSymlinkedRootAndSession(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		m, id, root := uploadTestManager(t, terminalproto.Running)
		outside := resolvedTempDir(t)
		if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, root); err != nil {
			t.Fatal(err)
		}
		if _, err := m.WriteUpload(context.Background(), id, uploadRequest(uploadPNG(), "image/png"), bytes.NewReader(uploadPNG())); !errors.Is(err, ErrUploadFailed) {
			t.Fatalf("got %v, want ErrUploadFailed", err)
		}
		if names := uploadEntries(t, outside); len(names) != 1 || names[0] != "keep" {
			t.Fatalf("wrote outside the upload root: %v", names)
		}
	})
	t.Run("session", func(t *testing.T) {
		m, id, root := uploadTestManager(t, terminalproto.Running)
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		outside := resolvedTempDir(t)
		if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, id)); err != nil {
			t.Fatal(err)
		}
		if _, err := m.WriteUpload(context.Background(), id, uploadRequest(uploadPNG(), "image/png"), bytes.NewReader(uploadPNG())); !errors.Is(err, ErrUploadFailed) {
			t.Fatalf("got %v, want ErrUploadFailed", err)
		}
		if names := uploadEntries(t, outside); len(names) != 1 || names[0] != "keep" {
			t.Fatalf("wrote outside the upload root: %v", names)
		}
	})
}

func TestUploadQuotas(t *testing.T) {
	t.Run("session file count", func(t *testing.T) {
		m, id, root := uploadTestManager(t, terminalproto.Running)
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < terminalproto.MaxUploadsPerSession; i++ {
			name := fmt.Sprintf("img-%032x.png", i)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := m.WriteUpload(context.Background(), id, uploadRequest(uploadPNG(), "image/png"), bytes.NewReader(uploadPNG())); !errors.Is(err, ErrUploadQuota) {
			t.Fatalf("got %v, want ErrUploadQuota", err)
		}
	})
	t.Run("session byte limit", func(t *testing.T) {
		m, id, root := uploadTestManager(t, terminalproto.Running)
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, fmt.Sprintf("img-%032x.png", 1))
		if err := os.WriteFile(name, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(name, terminalproto.MaxUploadSessionBytes); err != nil {
			t.Fatal(err)
		}
		if _, err := m.WriteUpload(context.Background(), id, uploadRequest(uploadPNG(), "image/png"), bytes.NewReader(uploadPNG())); !errors.Is(err, ErrUploadQuota) {
			t.Fatalf("got %v, want ErrUploadQuota", err)
		}
	})
	t.Run("machine byte limit", func(t *testing.T) {
		m, id, root := uploadTestManager(t, terminalproto.Running)
		for i := 0; i < 4; i++ {
			dir := filepath.Join(root, testSessionID(10+i))
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(dir, fmt.Sprintf("img-%032x.png", i))
			if err := os.WriteFile(name, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(name, terminalproto.MaxUploadSessionBytes); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := m.WriteUpload(context.Background(), id, uploadRequest(uploadPNG(), "image/png"), bytes.NewReader(uploadPNG())); !errors.Is(err, ErrUploadQuota) {
			t.Fatalf("got %v, want ErrUploadQuota", err)
		}
	})
	t.Run("temporary files do not count", func(t *testing.T) {
		m, id, root := uploadTestManager(t, terminalproto.Running)
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		temp := filepath.Join(dir, ".tmp-"+strings.Repeat("a", 32))
		if err := os.WriteFile(temp, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(temp, terminalproto.MaxUploadSessionBytes); err != nil {
			t.Fatal(err)
		}
		writeUpload(t, m, id, uploadPNG(), "image/png")
	})
}

func TestUploadChecksConsentSessionAndRequest(t *testing.T) {
	m, id, _ := uploadTestManager(t, terminalproto.Running)
	payload := uploadPNG()
	req := uploadRequest(payload, "image/png")

	if _, err := m.WriteUpload(context.Background(), "../../outside-session", req, bytes.NewReader(payload)); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatalf("path-like session id: %v", err)
	}
	for name, mutate := range map[string]func(*terminalproto.Upload){
		"token": func(u *terminalproto.Upload) { u.StreamToken = "../bad" },
		"sha":   func(u *terminalproto.Upload) { u.SHA256 = strings.Repeat("A", 64) },
		"mime":  func(u *terminalproto.Upload) { u.MIME = "image/svg+xml" },
		"size":  func(u *terminalproto.Upload) { u.Size = -1 },
	} {
		bad := req
		mutate(&bad)
		if _, err := m.WriteUpload(context.Background(), id, bad, bytes.NewReader(payload)); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%s: got %v, want ErrInvalid", name, err)
		}
	}

	m.uploadConsent = func() error { return errors.New("off") }
	if _, err := m.WriteUpload(context.Background(), id, req, bytes.NewReader(payload)); !errors.Is(err, ErrUploadDisabled) {
		t.Fatalf("upload consent: %v", err)
	}
	m.uploadConsent = nil
	if _, err := m.WriteUpload(context.Background(), id, req, bytes.NewReader(payload)); err != nil {
		t.Fatalf("nil upload consent refused: %v", err)
	}
	m.consent = func() error { return errors.New("disabled") }
	if _, err := m.WriteUpload(context.Background(), id, req, bytes.NewReader(payload)); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatalf("terminal consent: %v", err)
	}

	ended, endedID, _ := uploadTestManager(t, terminalproto.Ended)
	if _, err := ended.WriteUpload(context.Background(), endedID, req, bytes.NewReader(payload)); !errors.Is(err, ErrSessionMissing) {
		t.Fatalf("ended session: %v", err)
	}
}

func TestSweepUploadRootRemovesOrphansAndOldFiles(t *testing.T) {
	root := filepath.Join(resolvedTempDir(t), "uploads")
	active, empty, orphan, link := testSessionID(1), testSessionID(4), testSessionID(2), testSessionID(3)
	activeDir := filepath.Join(root, active)
	if err := os.MkdirAll(activeDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(root, empty), filepath.Join(root, orphan)} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, orphan, "img-"+strings.Repeat("a", 32)+".png"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-25 * time.Hour)
	fresh := now.Add(-time.Hour)
	files := map[string]time.Time{
		"img-" + strings.Repeat("b", 32) + ".png": fresh,
		"img-" + strings.Repeat("c", 32) + ".png": old,
		".tmp-" + strings.Repeat("d", 32):         old,
		".tmp-" + strings.Repeat("e", 32):         fresh,
	}
	for name, at := range files {
		path := filepath.Join(activeDir, name)
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	outside := resolvedTempDir(t)
	sentinel := filepath.Join(outside, "keep")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, link)); err != nil {
		t.Fatal(err)
	}

	activeSet := map[string]bool{active: true, empty: true, link: true}
	if err := sweepUploadRoot(root, activeSet, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, orphan)); !os.IsNotExist(err) {
		t.Fatalf("orphan folder survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, empty)); !os.IsNotExist(err) {
		t.Fatalf("empty folder survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(activeDir, "img-"+strings.Repeat("b", 32)+".png")); err != nil {
		t.Fatalf("fresh file removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(activeDir, ".tmp-"+strings.Repeat("e", 32))); err != nil {
		t.Fatalf("fresh temp removed: %v", err)
	}
	for _, name := range []string{"img-" + strings.Repeat("c", 32) + ".png", ".tmp-" + strings.Repeat("d", 32)} {
		if _, err := os.Stat(filepath.Join(activeDir, name)); !os.IsNotExist(err) {
			t.Fatalf("old file survived: %s", name)
		}
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("symlink target touched: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, link)); err != nil {
		t.Fatalf("sweep followed or removed a symlink: %v", err)
	}
}

func TestNewManagerSweepsUploadsOnStart(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	root := filepath.Join(opts.Layout.Dir, "uploads")
	sessionDir := filepath.Join(root, s.ID)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(sessionDir, "img-"+strings.Repeat("a", 32)+".png")
	old := filepath.Join(sessionDir, "img-"+strings.Repeat("b", 32)+".png")
	for _, path := range []string{fresh, old} {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, testSessionID(9))
	if err := os.MkdirAll(orphan, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "img-"+strings.Repeat("c", 32)+".png"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	next, err := NewManager(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh upload removed on start: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old upload survived start: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan folder survived start: %v", err)
	}
}

func TestManagerEndAndLostRemoveSessionUploads(t *testing.T) {
	t.Run("end", func(t *testing.T) {
		m, opts := managerFixture(t)
		s := createManaged(t, m, 1)
		writeUpload(t, m, s.ID, uploadPNG(), "image/png")
		dir := filepath.Join(opts.Layout.Dir, "uploads", s.ID)
		if _, err := os.Stat(dir); err != nil {
			t.Fatal(err)
		}
		if _, err := m.End(context.Background(), testRequestID(9), s.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("uploads survived End: %v", err)
		}
	})
	t.Run("lost", func(t *testing.T) {
		m, opts := managerFixture(t)
		s := createManaged(t, m, 1)
		writeUpload(t, m, s.ID, uploadPNG(), "image/png")
		dir := filepath.Join(opts.Layout.Dir, "uploads", s.ID)
		if _, err := m.command(context.Background(), 0, "kill-session", "-t", "="+m.name(s.ID)); err != nil {
			t.Fatal(err)
		}
		if err := m.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("uploads survived Lost: %v", err)
		}
	})
}

func TestExitedSessionUploadsAreRemoved(t *testing.T) {
	m, id, root := uploadTestManager(t, terminalproto.Exited)
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "img-"+strings.Repeat("a", 32)+".png"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := hostRegistry{
		Version:      1,
		EnrollmentID: testEnrollment,
		Sessions: map[string]ManagedSession{
			id: {ID: id, Title: "Uploads", State: terminalproto.Exited, CreatedAt: 1, UpdatedAt: 1, CreateRequestID: testRequestID(1)},
		},
		Operations: map[string]operationReceipt{},
	}
	m.cleanupClosedSessions(&registry)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("uploads survived Exited cleanup: %v", err)
	}
}

func TestRunningManagerSweepsOldUploads(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	path := writeUpload(t, m, s.ID, uploadPNG(), "image/png")
	past := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	previous := uploadSweepInterval
	uploadSweepInterval = 25 * time.Millisecond
	t.Cleanup(func() { uploadSweepInterval = previous })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	deadline := time.Now().Add(6 * time.Second)
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old upload survived the running sweep")
		}
		time.Sleep(25 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	_ = opts
}

func TestNewManagerIgnoresUnsweepableUploadRoot(t *testing.T) {
	m, opts := managerFixture(t)
	_ = m
	root := filepath.Join(opts.Layout.Dir, "uploads")
	outside := resolvedTempDir(t)
	victim := filepath.Join(outside, testSessionID(1))
	if err := os.MkdirAll(victim, 0700); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(victim, "img-"+strings.Repeat("a", 32)+".png")
	if err := os.WriteFile(image, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	next, err := NewManager(context.Background(), opts)
	if err != nil {
		t.Fatalf("NewManager failed on an unsweepable upload root: %v", err)
	}
	defer next.Close()
	if _, err := os.Stat(image); err != nil {
		t.Fatalf("sweep followed a symlink outside the upload root: %v", err)
	}
}
