package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
)

const importKey = "0123456789abcdef0123456789abcdef"

// source is a controller that has taken a copy of itself, optionally sealed
// under a password.
func source(t *testing.T, password string) []byte {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.EnsureLocalOwner(); err != nil {
		t.Fatal(err)
	}
	sealer, _ := backup.NewSealer(importKey, "test")
	svc := backup.New(st, backup.Config{Dir: filepath.Join(dir, "backups"), Sealer: sealer, Usable: true, KeySource: "test", Key: importKey})
	if password != "" {
		if _, err := svc.SaveSettings(0, backup.SettingsInput{Password: &password, Compression: backup.CompressionGzip, Destination: "local"}); err != nil {
			t.Fatal(err)
		}
	}
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := svc.Archive(context.Background(), 0, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// target is a controller an import lands on, served over HTTP in accounts
// mode with an owner token and a member token.
type target struct {
	st  *store.Store
	dir string
	url string
}

func newTarget(t *testing.T, populated bool) target {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	owner, err := st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIToken(owner.ID, "owner", auth.HashToken("owner-token")); err != nil {
		t.Fatal(err)
	}
	if populated {
		// A second account is data; a new controller has only its first owner.
		member, err := st.CreateUser("member@example.com", "Member", "member", "x")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateAPIToken(member.ID, "member", auth.HashToken("member-token")); err != nil {
			t.Fatal(err)
		}
	}
	sealer, _ := backup.NewSealer(importKey, "test")
	svc := backup.New(st, backup.Config{Dir: filepath.Join(dir, "backups"), Sealer: sealer, Usable: true, KeySource: "test", Key: importKey})
	ts := httptest.NewServer(New(st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts), WithBackups(svc)))
	t.Cleanup(ts.Close)
	return target{st: st, dir: dir, url: ts.URL}
}

func form(t *testing.T, fields [][2]string, file []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range fields {
		if err := mw.WriteField(f[0], f[1]); err != nil {
			t.Fatal(err)
		}
	}
	if file != nil {
		w, err := mw.CreateFormFile("file", "copy.reetbackup")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(file); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

type reply struct {
	code int
	text string
	v    map[string]any
}

// post sends an import. These fixtures are copies of controllers that were
// not fenced, so it says allowLiveSource unless the test sets it itself.
func (tg target) post(t *testing.T, token string, fields [][2]string, file []byte) reply {
	t.Helper()
	set := false
	for _, f := range fields {
		set = set || f[0] == "allowLiveSource"
	}
	if !set {
		fields = append(append([][2]string{}, fields...), [2]string{"allowLiveSource", "1"})
	}
	body, ctype := form(t, fields, file)
	req, _ := http.NewRequest("POST", tg.url+"/api/v2/admin/backups/import", body)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("X-Reet-Import", "1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doImport(t, req)
}

func (tg target) call(t *testing.T, method, token string) reply {
	t.Helper()
	req, _ := http.NewRequest(method, tg.url+"/api/v2/admin/backups/import", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return doImport(t, req)
}

func doImport(t *testing.T, req *http.Request) reply {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	return reply{code: resp.StatusCode, text: string(b), v: v}
}

func (tg target) untouched(t *testing.T) {
	t.Helper()
	if p, _ := backup.PendingImport(tg.dir); p != nil {
		t.Fatal("something was staged")
	}
	for _, pattern := range []string{"reet.db.before-import-*", ".reet-upload-*", ".reet-import-*"} {
		if m, _ := filepath.Glob(filepath.Join(tg.dir, pattern)); len(m) != 0 {
			t.Fatalf("a refused import left %v behind", m)
		}
	}
}

func TestAnImportIsStagedNotAppliedAndCanBeCancelled(t *testing.T) {
	t.Parallel()
	raw := source(t, "")
	tg := newTarget(t, false)
	if r := tg.call(t, "GET", "owner-token"); r.code != 200 || r.v["pending"] != nil {
		t.Fatalf("nothing should be pending: %d %s", r.code, r.text)
	}
	r := tg.post(t, "owner-token", nil, raw)
	if r.code != 200 || r.v["staged"] != true || r.v["restartRequired"] != true {
		t.Fatalf("stage: %d %s", r.code, r.text)
	}
	// A running controller keeps serving the database it has; the copy waits
	// for the next start.
	if _, err := os.Stat(filepath.Join(tg.dir, "pending-import", "reet.db")); err != nil {
		t.Fatalf("not staged on disk: %v", err)
	}
	if r := tg.call(t, "GET", "owner-token"); r.code != 200 || r.v["pending"] == nil {
		t.Fatalf("pending: %d %s", r.code, r.text)
	}
	if r := tg.post(t, "owner-token", nil, []byte("not a sealed copy")); r.code != 400 {
		t.Fatalf("garbage should be refused, got %d", r.code)
	}
	if r := tg.call(t, "DELETE", "owner-token"); r.code != 200 {
		t.Fatalf("cancel: %d", r.code)
	}
	if p, _ := backup.PendingImport(tg.dir); p != nil {
		t.Fatal("still pending after cancel")
	}
}

// The password travels in the body, so it can be any text at all, spaces and
// accents included, and no header or address ever holds it.
func TestAnImportOpensACopyWhosePasswordIsAnyText(t *testing.T) {
	t.Parallel()
	password := "  pässwörd ✓ with edges  "
	raw := source(t, password)
	tg := newTarget(t, false)
	if r := tg.post(t, "owner-token", nil, raw); r.code != 400 || !strings.Contains(r.text, "password") {
		t.Fatalf("without the password the copy must be refused with a message that names it, got %d %s", r.code, r.text)
	}
	if r := tg.post(t, "owner-token", [][2]string{{"password", "the wrong one entirely"}}, raw); r.code != 400 {
		t.Fatalf("a wrong password must be refused, got %d", r.code)
	}
	tg.untouched(t)
	if r := tg.post(t, "owner-token", [][2]string{{"password", password}}, raw); r.code != 200 {
		t.Fatalf("the exact password must open the copy, got %d %s", r.code, r.text)
	}
}

func TestAnImportNeverEchoesOrLogsThePassword(t *testing.T) {
	t.Parallel()
	const password = "a password nobody should ever read back"
	raw := source(t, password)
	tg := newTarget(t, false)
	r := tg.post(t, "owner-token", [][2]string{{"password", password}, {"ownerEmail", "local@reet.invalid"}}, raw)
	if r.code != 200 {
		t.Fatalf("stage: %d %s", r.code, r.text)
	}
	if strings.Contains(r.text, password) {
		t.Fatal("the response repeats the password")
	}
	evs, err := tg.st.ListEvents(100, "")
	if err != nil {
		t.Fatal(err)
	}
	var logged bool
	for _, e := range evs {
		if strings.Contains(e.Detail, password) || strings.Contains(e.Action, password) {
			t.Fatalf("the activity log holds the password: %+v", e)
		}
		if e.Action == "backup-import-staged" {
			logged = true
			for _, want := range []string{`"id"`, `"replace":false`, `"safetyCopy"`, `"ownerChecked":true`} {
				if !strings.Contains(e.Detail, want) {
					t.Fatalf("event %s lacks %s", e.Detail, want)
				}
			}
		}
	}
	if !logged {
		t.Fatal("the import left no line in the activity log")
	}
}

func TestAnImportIntoAControllerThatHoldsDataIsRefusedUnlessToldToReplace(t *testing.T) {
	t.Parallel()
	raw := source(t, "")
	tg := newTarget(t, true)
	if _, err := tg.st.CreateTag("existing"); err != nil {
		t.Fatal(err)
	}
	r := tg.post(t, "owner-token", nil, raw)
	if r.code != http.StatusConflict || !strings.Contains(r.text, "1 tags") {
		t.Fatalf("a controller with data must be refused, naming it: %d %s", r.code, r.text)
	}
	tg.untouched(t)
	r = tg.post(t, "owner-token", [][2]string{{"replace", "1"}}, raw)
	if r.code != 200 {
		t.Fatalf("replace must be accepted when asked for: %d %s", r.code, r.text)
	}
}

func TestTheLiveDatabaseIsCopiedAsideBeforeAnImportIsStaged(t *testing.T) {
	t.Parallel()
	raw := source(t, "")
	tg := newTarget(t, true)
	if _, err := tg.st.CreateTag("kept-in-the-safety-copy"); err != nil {
		t.Fatal(err)
	}
	r := tg.post(t, "owner-token", [][2]string{{"replace", "1"}}, raw)
	if r.code != 200 {
		t.Fatalf("stage: %d %s", r.code, r.text)
	}
	report, _ := r.v["report"].(map[string]any)
	safety, _ := report["safetyCopy"].(string)
	if safety == "" || filepath.Dir(safety) != tg.dir {
		t.Fatalf("the response does not say where the safety copy is: %v", report)
	}
	copied, err := store.InspectDatabaseFile(context.Background(), safety)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Tags != 1 || copied.Users != 2 {
		t.Fatalf("the safety copy is not the database that was live: %+v", copied)
	}
	if info, err := os.Stat(safety); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the safety copy must be private: %v %v", info, err)
	}
}

func TestADamagedCopyChangesNothing(t *testing.T) {
	t.Parallel()
	raw := source(t, "")
	tg := newTarget(t, false)
	damaged := append([]byte(nil), raw...)
	damaged[len(damaged)-5] ^= 0xff
	if r := tg.post(t, "owner-token", nil, damaged); r.code != 400 {
		t.Fatalf("a damaged copy must be refused, got %d %s", r.code, r.text)
	}
	tg.untouched(t)
}

func TestOnlyAnOwnerMayImport(t *testing.T) {
	t.Parallel()
	raw := source(t, "")
	tg := newTarget(t, true)
	for name, token := range map[string]string{"a member": "member-token", "nobody": ""} {
		if r := tg.post(t, token, nil, raw); r.code != 401 && r.code != 403 {
			t.Fatalf("%s must not import: %d %s", name, r.code, r.text)
		}
	}
	tg.untouched(t)
}

// A form can be posted from any website, but it cannot add a header, so an
// upload without the marker is treated as the forgery it may be.
func TestAnImportFormWithoutTheMarkerHeaderIsRefused(t *testing.T) {
	t.Parallel()
	raw := source(t, "")
	tg := newTarget(t, false)
	body, ctype := form(t, nil, raw)
	req, _ := http.NewRequest("POST", tg.url+"/api/v2/admin/backups/import", body)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Authorization", "Bearer owner-token")
	if r := doImport(t, req); r.code != http.StatusUnsupportedMediaType {
		t.Fatalf("got %d %s", r.code, r.text)
	}
	tg.untouched(t)
}

func TestAnUploadLargerThanTheLimitIsRefusedAndLeavesNothingOnDisk(t *testing.T) {
	old := importBodyLimit
	importBodyLimit = 4096
	defer func() { importBodyLimit = old }()
	tg := newTarget(t, false)
	r := tg.post(t, "owner-token", nil, bytes.Repeat([]byte("x"), 64<<10))
	if r.code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d %s", r.code, r.text)
	}
	tg.untouched(t)
}

func TestACopyAfterTheOptionsIsRequired(t *testing.T) {
	tg := newTarget(t, false)
	if r := tg.post(t, "owner-token", [][2]string{{"replace", "1"}}, nil); r.code != 400 || !strings.Contains(r.text, "no copy") {
		t.Fatalf("got %d %s", r.code, r.text)
	}
	tg.untouched(t)
}

// A copy of a controller that was not fenced is refused at the API with a 409
// that says why, and is accepted when the owner says the source is gone.
func TestAnImportOfAnUnfencedCopyIsRefusedUnlessTheOwnerAllowsALiveSource(t *testing.T) {
	t.Parallel()
	raw := source(t, "")
	tg := newTarget(t, false)
	r := tg.post(t, "owner-token", [][2]string{{"allowLiveSource", "0"}}, raw)
	if r.code != http.StatusConflict || !strings.Contains(r.text, "not stopped") || !strings.Contains(r.text, "--allow-live-source") {
		t.Fatalf("an unfenced copy: %d %s", r.code, r.text)
	}
	// The page offers its "the old controller is stopped or gone" tick on this
	// code, not on the wording of the sentence.
	if r.v["code"] != "copy_source_live" {
		t.Fatalf("the refusal carries the code %v", r.v["code"])
	}
	tg.untouched(t)
	r = tg.post(t, "owner-token", [][2]string{{"allowLiveSource", "1"}}, raw)
	if r.code != 200 {
		t.Fatalf("with allowLiveSource: %d %s", r.code, r.text)
	}
	// What a copy cannot carry is sent with the report, so the page can list
	// what to set by hand without a second source of the same list.
	names, _ := r.v["notInCopy"].([]any)
	if len(names) != len(backup.Checklist()) || !strings.Contains(r.text, "REET_PUBLIC_URL") {
		t.Fatalf("the staged answer lists %d things not in the copy: %s", len(names), r.text)
	}
	// A page opened later reads the same list beside the staged report.
	req, _ := http.NewRequest("GET", tg.url+"/api/v2/admin/backups/import", nil)
	req.Header.Set("Authorization", "Bearer owner-token")
	pending := doImport(t, req)
	if again, _ := pending.v["notInCopy"].([]any); pending.code != 200 || pending.v["pending"] == nil || len(again) != len(names) {
		t.Fatalf("the pending answer: %d %s", pending.code, pending.text)
	}
}

// publicUrl is validated before anything is staged and, when good, is reported.
func TestAnImportTakesAPublicURLAndRefusesABadOne(t *testing.T) {
	t.Parallel()
	raw := source(t, "")
	tg := newTarget(t, false)
	if r := tg.post(t, "owner-token", [][2]string{{"publicUrl", "https://new.example.com/path"}}, raw); r.code != 400 || !strings.Contains(r.text, "plain http or https origin") {
		t.Fatalf("a bad address: %d %s", r.code, r.text)
	}
	tg.untouched(t)
	r := tg.post(t, "owner-token", [][2]string{{"publicUrl", "https://new.example.com"}}, raw)
	if r.code != 200 || !strings.Contains(r.text, `"publicUrl":"https://new.example.com"`) {
		t.Fatalf("a good address: %d %s", r.code, r.text)
	}
}
