package backup

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

const testKey = "0123456789abcdef0123456789abcdef"

func testService(t *testing.T, remote *fakeS3) (*Service, *store.Store, store.User) {
	t.Helper()
	st, err := storetest.Open(t)
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
	if len(keys) != 1 || !(strings.HasSuffix(keys[0], item.ID+".gz.enc") || strings.HasSuffix(keys[0], item.ID+".enc")) {
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
	if _, err := svc.SaveSettings(user.ID, SettingsInput{EverySeconds: 0, Keep: 2, Destination: store.DestinationBoth}); err != nil {
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
	saved, err := svc.SaveSettings(user.ID, SettingsInput{EverySeconds: 3600, Keep: 3, Destination: store.DestinationRemote})
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
	if _, err := svc.SaveSettings(user.ID, SettingsInput{EverySeconds: 0, Keep: 3, Destination: "elsewhere"}); err == nil {
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
	if _, err := svc.SaveSettings(user.ID, SettingsInput{EverySeconds: 0, Keep: 5, Destination: store.DestinationLocal}); err != nil {
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
	if _, err := svc.SaveSettings(user.ID, SettingsInput{EverySeconds: 0, Keep: 5, Destination: store.DestinationRemote}); err != nil {
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
	if _, err := svc.SaveSettings(user.ID, SettingsInput{EverySeconds: 0, Keep: 5, Destination: store.DestinationRemote}); err != nil {
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
	if _, err := svc.SaveSettings(user.ID, SettingsInput{EverySeconds: 0, Keep: 5, Destination: store.DestinationRemote}); err != nil {
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

func TestPasswordProtectedAndCompressedCopy(t *testing.T) {
	svc, st, user := testService(t, nil)
	password := "a copy password long enough"
	if _, err := svc.SaveSettings(user.ID, SettingsInput{
		Keep: 5, Destination: store.DestinationBoth, Compression: CompressionGzip, Password: &password,
	}); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/secret")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(svc.cfg.Dir, item.ID+".reetbackup"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ManifestOf(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.Protection != ProtectionPassword || m.Compression != CompressionGzip {
		t.Fatalf("manifest says %+v", m)
	}
	if _, _, err := svc.cfg.Sealer.Open(data); err == nil {
		t.Fatal("a password copy opened with the instance key alone")
	}
	if _, _, err := svc.cfg.Sealer.OpenWithPassword(data, "the wrong password here"); err == nil {
		t.Fatal("a wrong password opened the copy")
	}
	if _, payload, err := svc.cfg.Sealer.OpenWithPassword(data, password); err != nil || len(payload) == 0 {
		t.Fatalf("the password did not open the copy: %v", err)
	}
	// The stored password is what the server uses for verify, download and
	// restore until somebody removes it.
	if v := svc.Verify(context.Background(), user.ID, item.ID); !v.OK {
		t.Fatalf("verify with the stored password: %+v", v)
	}
	if _, _, err := svc.Download(context.Background(), user.ID, item.ID); err != nil {
		t.Fatalf("download with the stored password: %v", err)
	}
	if _, _, err := svc.DownloadWithPassword(context.Background(), user.ID, item.ID, password); err != nil {
		t.Fatalf("download with a typed password: %v", err)
	}
	if len(data) >= int(m.Size) && m.Size > 1024 {
		t.Fatalf("a repetitive shelf copy did not compress: %d stored, %d plain", len(data), m.Size)
	}
}

func TestCopyPasswordIsWriteOnlyAndShortOnesAreRefused(t *testing.T) {
	svc, _, user := testService(t, nil)
	short := "too short"
	if _, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationBoth, Password: &short}); err == nil {
		t.Fatal("a short copy password was accepted")
	}
	password := "a copy password long enough"
	saved, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationBoth, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.HasPassword || saved.PasswordSealed == "" {
		t.Fatalf("the password was not stored: %+v", saved)
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), password) || strings.Contains(string(raw), saved.PasswordSealed) {
		t.Fatalf("a JSON view exposed the password: %s", raw)
	}
	empty := ""
	cleared, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationBoth, Password: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.HasPassword {
		t.Fatalf("the password was not cleared: %+v", cleared)
	}
}

func TestSealArchiveRoundTrip(t *testing.T) {
	sealer, _ := NewSealer(testKey, "test")
	payload := bytes.Repeat([]byte("reet backup "), 4096)
	m := Manifest{ID: "instance-x", Kind: "instance", Target: "instance"}
	sealed, err := sealer.SealArchive(m, payload, SealOptions{Compress: true, Password: "a password long enough"})
	if err != nil {
		t.Fatal(err)
	}
	head, err := ManifestOf(sealed)
	if err != nil || head.Protection != ProtectionPassword || head.Compression != CompressionGzip || head.Size != int64(len(payload)) {
		t.Fatalf("manifest: %+v %v", head, err)
	}
	got, opened, err := sealer.OpenWithPassword(sealed, "a password long enough")
	if err != nil || !bytes.Equal(opened, payload) || got.ID != m.ID {
		t.Fatalf("open: %v", err)
	}
	if !strings.Contains(string(opened), "reet backup") {
		t.Fatal("the payload did not come back")
	}
}

func TestShipMissingThenRemoveARemoteCopy(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, nil)
	if _, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationLocal}); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/missing")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("later"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(3),
	}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.ShipMissing(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sent) != 1 || out.Sent[0].ID != item.ID || len(out.Errors) != 0 {
		t.Fatalf("shipment: %+v", out)
	}
	if keys := fake.keys(); len(keys) != 1 {
		t.Fatalf("remote keys: %v", keys)
	}
	if err := svc.DeleteRemoteCopy(context.Background(), user.ID, "later", item.ID); err != nil {
		t.Fatal(err)
	}
	if keys := fake.keys(); len(keys) != 0 {
		t.Fatalf("remote copy was not removed: %v", keys)
	}
}

func TestPruneNowAppliesTheCurrentCount(t *testing.T) {
	svc, st, user := testService(t, nil)
	if _, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationBoth}); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/prune")
	for i := 0; i < 3; i++ {
		if _, err := svc.RunUser(context.Background(), user.ID, "test"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if _, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 1, Destination: store.DestinationBoth}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.PruneNow(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Removed) != 2 {
		t.Fatalf("pruned %v, want 2", out.Removed)
	}
	local, err := svc.LocalItems(user.ID)
	if err != nil || len(local) != 1 {
		t.Fatalf("local copies after pruning: %+v %v", local, err)
	}
}

func TestEscrowRoundTrip(t *testing.T) {
	svc, _, _ := testService(t, nil)
	if _, err := svc.CreateRemote(0, RemoteInput{
		Name: ptr("offsite"), Endpoint: ptr("http://127.0.0.1:9000"), Bucket: ptr("bucket"),
		AccessKey: ptr("key"), SecretKey: ptr("the destination secret"), PathStyle: rawBool(true), Keep: ptrInt(7),
	}); err != nil {
		t.Fatal(err)
	}
	e, err := svc.Escrow(0)
	if err != nil {
		t.Fatal(err)
	}
	if e.Key != testKey || len(e.Destinations) != 1 || e.Destinations[0].SecretKey != "the destination secret" {
		t.Fatalf("escrow: %+v", e)
	}
	passphrase := "an escrow passphrase long enough"
	sealed, err := SealEscrow(e, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	got, err := OpenEscrow(sealed, passphrase)
	if err != nil || got.Key != testKey || len(got.Destinations) != 1 {
		t.Fatalf("open escrow: %+v %v", got, err)
	}
	if _, err := OpenEscrow(sealed, "the wrong escrow passphrase"); err == nil {
		t.Fatal("a wrong escrow passphrase opened it")
	}
	if _, err := SealEscrow(e, "short"); err == nil {
		t.Fatal("a short escrow passphrase was accepted")
	}
}

func TestShipMissingBackfillsANewDestination(t *testing.T) {
	a, b, c := newFakeS3(t), newFakeS3(t), newFakeS3(t)
	svc, st, user := testService(t, nil)
	for _, r := range []struct {
		name string
		f    *fakeS3
	}{{"a", a}, {"b", b}} {
		if _, err := svc.CreateRemote(user.ID, RemoteInput{
			Name: ptr(r.name), Endpoint: ptr(r.f.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
			SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(5),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationLocal}); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/backfill")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ShipMissing(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.keys()) != 1 || len(b.keys()) != 1 {
		t.Fatalf("the first two destinations: %v %v", a.keys(), b.keys())
	}
	if err := svc.DeleteLocal(user.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("c"), Endpoint: ptr(c.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(5),
	}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.ShipMissing(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sent) == 0 || len(c.keys()) != 1 {
		t.Fatalf("the new destination was not backfilled: %+v %v", out, c.keys())
	}
}

func (f *fakeS3) object(key string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.store[key]...)
}

func TestANewRemoteCopyIsExactlyOneObject(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, fake)
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(5),
	}); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/object")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	keys := fake.keys()
	if len(keys) != 1 || !strings.HasSuffix(keys[0], item.ID+".gz.enc") {
		t.Fatalf("remote keys = %v, want one .gz.enc object", keys)
	}
	object := fake.object(keys[0])
	if !bytes.HasPrefix(object, []byte(ObjectMagic)) {
		t.Fatalf("object does not start with %q", ObjectMagic)
	}
	m, payload, err := DecodeObject(object)
	if err != nil {
		t.Fatal(err)
	}
	if m.CopyID != item.ID || m.Compression != CompressionGzip || m.Target != targetOf(user.ID) {
		t.Fatalf("manifest = %+v", m)
	}
	if m.PayloadSize != int64(len(payload)) || m.KeyFingerprint == "" {
		t.Fatalf("manifest sizes = %+v", m)
	}
	if strings.Contains(string(object[:HeaderLength(object)]), "secret") {
		t.Fatal("the clear manifest carries a secret")
	}
}

func TestOldPairAndLegacyCopiesStillListFetchRestoreAndDelete(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, nil)
	if _, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationLocal}); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/old")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := os.ReadFile(filepath.Join(svc.cfg.Dir, item.ID+".reetbackup"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ManifestOf(sealed)
	if err != nil {
		t.Fatal(err)
	}
	rawManifest, _ := json.Marshal(m)
	if err := svc.DeleteLocal(user.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), Prefix: ptr("pre"),
		AccessKey: ptr("key"), SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(5),
	}); err != nil {
		t.Fatal(err)
	}
	dir := listingPrefix("pre/", "user", user.ID)
	fake.mu.Lock()
	fake.store[dir+item.ID+".sealed"] = sealed
	fake.store[dir+item.ID+".json"] = rawManifest
	fake.store[dir+"instance-20260910T030004Z-aaaaaaaa.reetbackup"] = sealed
	fake.mu.Unlock()
	items, unreachable, err := svc.Items(context.Background(), user.ID)
	if err != nil || len(unreachable) != 0 {
		t.Fatalf("items: %v %v", unreachable, err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v, want the old pair and the legacy copy", items)
	}
	if err := st.RemoveFor(user.ID, "owner/repo/old"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RestoreUser(context.Background(), user.ID, item.ID); err != nil {
		t.Fatalf("restore from the old pair: %v", err)
	}
	if _, ok, _ := st.GetFor(user.ID, "owner/repo/old"); !ok {
		t.Fatal("the old pair did not restore the shelf")
	}
	if err := svc.DeleteRemoteCopy(context.Background(), user.ID, "mine", item.ID); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	_, sealedThere := fake.store[dir+item.ID+".sealed"]
	_, jsonThere := fake.store[dir+item.ID+".json"]
	fake.mu.Unlock()
	if sealedThere || jsonThere {
		t.Fatal("deleting the old copy left one of its two objects behind")
	}
}

func TestARemoteOnlyCopyIsFetchedAndRestored(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, fake)
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), AccessKey: ptr("key"),
		SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(5),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationRemote}); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/remote")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if item.Local || len(item.Remotes) != 1 {
		t.Fatalf("remote-only item = %+v", item)
	}
	if err := st.RemoveFor(user.ID, "owner/repo/remote"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RestoreUser(context.Background(), user.ID, item.ID); err != nil {
		t.Fatalf("restore only from the destination: %v", err)
	}
	if _, ok, _ := st.GetFor(user.ID, "owner/repo/remote"); !ok {
		t.Fatal("the remote copy did not restore the shelf")
	}
}

func TestRetentionRemovesTheOldestByParsedNameAndTime(t *testing.T) {
	fake := newFakeS3(t)
	svc, _, user := testService(t, nil)
	if _, err := svc.CreateRemote(user.ID, RemoteInput{
		Name: ptr("mine"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"), Prefix: ptr("pre"),
		AccessKey: ptr("key"), SecretKey: ptr("secret"), PathStyle: rawBool(true), Keep: ptrInt(2),
	}); err != nil {
		t.Fatal(err)
	}
	prefix := listingPrefix("pre/", "user", user.ID)
	fake.mu.Lock()
	for _, id := range []string{
		"user-1-20260910T030004Z-aaaaaaaa",
		"user-1-20260911T030004Z-bbbbbbbb",
		"user-1-20260912T030004Z-cccccccc",
	} {
		fake.store[prefix+id+".gz.enc"] = []byte("x")
	}
	fake.mu.Unlock()
	if _, err := svc.PruneNow(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if _, ok := fake.store[prefix+"user-1-20260910T030004Z-aaaaaaaa.gz.enc"]; ok {
		t.Fatal("the oldest copy survived retention")
	}
	if len(fake.store) != 2 {
		t.Fatalf("objects left = %v", fake.store)
	}
}

func TestADestinationPassphraseSealsAgainAndAFetchUnwrapsIt(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, nil)
	if _, err := svc.SaveSettings(user.ID, SettingsInput{Keep: 5, Destination: store.DestinationLocal}); err != nil {
		t.Fatal(err)
	}
	addSkill(t, st, user.ID, "owner/repo/passphrase")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := os.ReadFile(filepath.Join(svc.cfg.Dir, item.ID+".reetbackup"))
	if err != nil {
		t.Fatal(err)
	}
	sm, err := ManifestOf(sealed)
	if err != nil {
		t.Fatal(err)
	}
	om, err := svc.objectManifestOf(sm, sealed)
	if err != nil {
		t.Fatal(err)
	}
	object, err := sealObject(om, sealed, "the destination passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := DecodeObject(object); err != nil {
		t.Fatal(err)
	}
	r := runtimeRemote{BackupRemote: store.BackupRemote{Name: "mine", Keep: 5},
		S3: S3{Endpoint: fake.server.URL, Bucket: "bucket", AccessKey: "key", SecretKey: "secret", Passphrase: "the destination passphrase"}}
	cl, err := newS3Client(r.S3)
	if err != nil {
		t.Fatal(err)
	}
	key := ObjectKey("", om.Target, om.CopyID, om.Compression)
	if err := cl.PutObjectRetry(context.Background(), key, object); err != nil {
		t.Fatal(err)
	}
	ref := CopyRef{Key: key, Target: om.Target, Name: item.ID, Layout: LayoutObject}
	got, err := svc.fetchRemotePayload(context.Background(), r, ref)
	if err != nil || !bytes.Equal(got, sealed) {
		t.Fatalf("fetch with the destination passphrase: %d bytes, %v", len(got), err)
	}
	r.S3.Passphrase = "the wrong destination passphrase"
	if _, err := svc.fetchRemotePayload(context.Background(), r, ref); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("a wrong destination passphrase was not named: %v", err)
	}
}
