package backup

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

const testKey = "0123456789abcdef0123456789abcdef"

func testService(t *testing.T, remote *fakeS3) (*Service, *store.Store, store.User) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	user, err := st.CreateUser("sam@example.com", "Sam", "member", "x")
	if err != nil {
		t.Fatal(err)
	}
	sealer, err := NewSealer(testKey, "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Dir: t.TempDir(), Key: testKey, KeySource: "test", Sealer: sealer, Usable: true,
		UserKeep: 5, Keep: 5, Version: "test",
	}
	if remote != nil {
		cfg.S3 = S3{Endpoint: remote.server.URL, Bucket: "bucket", Prefix: "pre/", AccessKey: "key", SecretKey: "secret"}
	}
	return New(st, cfg), st, user
}

func addSkill(t *testing.T, st *store.Store, userID int64, id string) {
	t.Helper()
	r, err := ref.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertFor(userID, r, store.Patch{}); err != nil {
		t.Fatal(err)
	}
}

func TestSealRoundTripAndTamper(t *testing.T) {
	sealer, err := NewSealer(testKey, "test")
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{ID: "instance-1", Kind: "instance", Target: "instance"}
	data, err := sealer.Seal(m, []byte("hello backup"))
	if err != nil {
		t.Fatal(err)
	}
	head, err := ManifestOf(data)
	if err != nil || head.ID != "instance-1" || !head.Encrypted {
		t.Fatalf("manifest: %+v %v", head, err)
	}
	got, payload, err := sealer.Open(data)
	if err != nil || string(payload) != "hello backup" || got.ID != m.ID {
		t.Fatalf("open: %+v %q %v", got, payload, err)
	}
	other, _ := NewSealer(strings.Repeat("z", 32), "test")
	if _, _, err := other.Open(data); err == nil {
		t.Fatal("a different key opened the backup")
	}
	data[len(data)-1] ^= 0xff
	if _, _, err := sealer.Open(data); err == nil {
		t.Fatal("a damaged backup opened")
	}
}

func TestSecretsAreSealed(t *testing.T) {
	sealer, _ := NewSealer(testKey, "test")
	sealed, err := sealer.SealSecret("super-secret")
	if err != nil || strings.Contains(sealed, "super-secret") {
		t.Fatalf("seal secret: %q %v", sealed, err)
	}
	got, err := sealer.OpenSecret(sealed)
	if err != nil || got != "super-secret" {
		t.Fatalf("open secret: %q %v", got, err)
	}
	other, _ := NewSealer(strings.Repeat("z", 32), "test")
	if _, err := other.OpenSecret(sealed); err == nil {
		t.Fatal("another key opened the secret")
	}
}

func TestUserBackupRestoreWithoutRemote(t *testing.T) {
	svc, st, user := testService(t, nil)
	addSkill(t, st, user.ID, "owner/repo/skill")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Local || item.Kind != "user" || item.ID == "" {
		t.Fatalf("item: %+v", item)
	}
	if v := svc.Verify(context.Background(), user.ID, item.ID); !v.OK {
		t.Fatalf("verify: %+v", v)
	}
	if err := st.RemoveFor(user.ID, "owner/repo/skill"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.GetFor(user.ID, "owner/repo/skill"); ok {
		t.Fatal("skill still there")
	}
	n, err := svc.RestoreUser(context.Background(), user.ID, item.ID)
	if err != nil || n != 1 {
		t.Fatalf("restore: %d %v", n, err)
	}
	if _, ok, _ := st.GetFor(user.ID, "owner/repo/skill"); !ok {
		t.Fatal("skill was not restored")
	}
}

func TestUserBackupShipsToRemote(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, fake)
	_, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/other")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	keys := fake.keys()
	if len(keys) != 1 || !strings.HasSuffix(keys[0], item.ID+".reetbackup") {
		t.Fatalf("remote keys: %v", keys)
	}
	items, unreachable, err := svc.Items(context.Background(), user.ID)
	if err != nil || len(unreachable) != 0 {
		t.Fatalf("items: %v %v", unreachable, err)
	}
	if len(items) != 1 || len(items[0].Remotes) != 1 || items[0].Remotes[0] != "mine" {
		t.Fatalf("items: %+v", items)
	}
	// Remove the local copy and restore from the destination.
	if err := svc.DeleteLocal(user.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RestoreUser(context.Background(), user.ID, item.ID); err != nil {
		t.Fatalf("restore from remote: %v", err)
	}
}

func TestInstanceBackupAndVerify(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, _ := testService(t, fake)
	addSkill(t, st, 1, "owner/repo/instance")
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if v := svc.Verify(context.Background(), 0, item.ID); !v.OK {
		t.Fatalf("verify: %+v", v)
	}
	if keys := fake.keys(); len(keys) != 1 || !strings.Contains(keys[0], "/instance/") {
		t.Fatalf("remote keys: %v", keys)
	}
}

func TestRetentionKeepsNewest(t *testing.T) {
	svc, st, user := testService(t, nil)
	if _, err := svc.SaveSettings(user.ID, 0, 2, store.DestinationBoth); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/one")
	var ids []string
	for i := 0; i < 4; i++ {
		item, err := svc.RunUser(context.Background(), user.ID, "test")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
		time.Sleep(2 * time.Millisecond)
	}
	items, err := svc.LocalItems(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("kept %d copies, want 2: %+v", len(items), items)
	}
	if items[0].ID != ids[3] || items[1].ID != ids[2] {
		t.Fatalf("kept the wrong copies: %+v", items)
	}
}

type fakeS3 struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex
	store  map[string][]byte
}

func newFakeS3(t *testing.T) *fakeS3 {
	f := &fakeS3{t: t, store: map[string][]byte{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeS3) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.store))
	for k := range f.store {
		out = append(out, k)
	}
	return out
}

func (f *fakeS3) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		http.Error(w, "unsigned", http.StatusForbidden)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	if strings.HasSuffix(r.URL.Path, "/bucket") || strings.HasSuffix(r.URL.Path, "/bucket/") {
		key = ""
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.store[key] = body
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		delete(f.store, key)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		if r.URL.Query().Get("list-type") == "2" {
			prefix := r.URL.Query().Get("prefix")
			fmt.Fprint(w, "<ListBucketResult><IsTruncated>false</IsTruncated>")
			for k, v := range f.store {
				if strings.HasPrefix(k, prefix) {
					fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size><LastModified>%s</LastModified></Contents>",
						k, len(v), time.Now().UTC().Format(time.RFC3339))
				}
			}
			fmt.Fprint(w, "</ListBucketResult>")
			return
		}
		body, ok := f.store[key]
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		w.Write(body)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type listXML struct {
	XMLName xml.Name `xml:"ListBucketResult"`
}

func ptr(s string) *string { return &s }
func ptrInt(n int) *int    { return &n }
func rawBool(b bool) []byte {
	if b {
		return []byte("true")
	}
	return []byte("false")
}

func TestMissingKeyWithCopiesDoesNotGenerateAnother(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backups", "instance-20260101T000000Z-aaaaaaaa.reetbackup"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := FromEnv(func(key string) string {
		if key == "REET_DATA_DIR" {
			return dir
		}
		return ""
	})
	if err == nil {
		t.Fatal("a missing key beside existing copies was not refused")
	}
	if cfg.Usable {
		t.Fatal("backups were usable without the key that opens existing copies")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "backup.key")); !os.IsNotExist(statErr) {
		t.Fatal("a replacement key was written beside copies it cannot open")
	}
}

func TestInvalidBackupIDsCannotEscape(t *testing.T) {
	svc, _, user := testService(t, nil)
	for _, id := range []string{"../../etc/passwd", "instance-x", "user-1-../../x", ""} {
		if _, _, err := svc.Download(context.Background(), user.ID, id); err == nil {
			t.Fatalf("%q was accepted", id)
		}
	}
}

func TestSettingsPersistDestination(t *testing.T) {
	svc, _, user := testService(t, nil)
	saved, err := svc.SaveSettings(user.ID, 3600, 3, store.DestinationRemote)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Destination != store.DestinationRemote || saved.EverySeconds != 3600 || saved.Keep != 3 {
		t.Fatalf("saved: %+v", saved)
	}
	got, err := svc.Settings(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Destination != store.DestinationRemote {
		t.Fatalf("destination was not persisted: %+v", got)
	}
	if _, err := svc.SaveSettings(user.ID, 0, 3, "elsewhere"); err == nil {
		t.Fatal("an unknown destination was accepted")
	}
}

func TestLocalOnlyDoesNotShip(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, nil)
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(3),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveSettings(user.ID, 0, 5, store.DestinationLocal); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/local")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Local || len(item.Remotes) != 0 {
		t.Fatalf("local-only item: %+v", item)
	}
	if keys := fake.keys(); len(keys) != 0 {
		t.Fatalf("local-only copied off-site: %v", keys)
	}
}

func TestRemoteOnlyKeepsNothingHereWhenShipped(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, nil)
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(3),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveSettings(user.ID, 0, 5, store.DestinationRemote); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/remote")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if item.Local || len(item.Remotes) != 1 || item.Remotes[0] != "mine" {
		t.Fatalf("remote-only item: %+v", item)
	}
	if keys := fake.keys(); len(keys) != 1 {
		t.Fatalf("remote keys: %v", keys)
	}
	local, err := svc.LocalItems(user.ID)
	if err != nil || len(local) != 0 {
		t.Fatalf("remote-only kept a local copy: %+v %v", local, err)
	}
	if v := svc.Verify(context.Background(), user.ID, item.ID); !v.OK {
		t.Fatalf("verify from the destination: %+v", v)
	}
}

func TestRemoteOnlyWithoutADestinationFails(t *testing.T) {
	svc, _, user := testService(t, nil)
	if _, err := svc.SaveSettings(user.ID, 0, 5, store.DestinationRemote); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunUser(context.Background(), user.ID, "test"); err == nil || !strings.Contains(err.Error(), "no enabled destination") {
		t.Fatalf("remote-only with no destination: %v", err)
	}
}

func TestRemoteOnlyKeepsTheCopyWhenShippingFails(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<Error><Code>AccessDenied</Code></Error>", http.StatusForbidden)
	}))
	defer broken.Close()
	svc, st, user := testService(t, nil)
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(broken.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(3),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveSettings(user.ID, 0, 5, store.DestinationRemote); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/fallback")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Local || len(item.Remotes) != 0 {
		t.Fatalf("a failed ship should keep the local copy: %+v", item)
	}
}

func TestTestRemoteDraftBeforeSaving(t *testing.T) {
	fake := newFakeS3(t)
	svc, _, user := testService(t, nil)
	in := RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true),
	}
	steps, err := svc.TestRemoteDraft(context.Background(), user.ID, in)
	if err != nil || len(steps) != 3 {
		t.Fatalf("draft test: %+v %v", steps, err)
	}
	if _, err := svc.CreateRemote(user.ID, in); err != nil {
		t.Fatal(err)
	}
	// The secret already stored is used when the form leaves it blank.
	again := RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		PathStyle: rawBool(true),
	}
	if _, err := svc.TestRemoteDraft(context.Background(), user.ID, again); err != nil {
		t.Fatalf("draft test with the stored secret: %v", err)
	}
	// Renaming it keeps the stored secret for a draft test.
	renamed := again
	renamed.Name = ptr("renamed")
	renamed.OriginalName = ptr("mine")
	if _, err := svc.TestRemoteDraft(context.Background(), user.ID, renamed); err != nil {
		t.Fatalf("draft test across a rename: %v", err)
	}
	// Pointing it somewhere else without the secret is refused.
	moved := again
	moved.Endpoint = ptr(strings.Replace(fake.server.URL, "127.0.0.1", "localhost", 1))
	if _, err := svc.TestRemoteDraft(context.Background(), user.ID, moved); err == nil || !strings.Contains(err.Error(), "enter it again") {
		t.Fatalf("a moved destination without its secret: %v", err)
	}
}
