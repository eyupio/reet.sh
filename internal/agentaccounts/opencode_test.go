package agentaccounts

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const openCodeUsage = `{"plan":"Go","usage":{"rolling":{"percent":12.4,"resetsAt":"2026-11-14T13:00:00Z"},"weekly":{"percent":55,"resetsAt":1795046400000},"monthly":{"percent":3,"resetsAt":null}}}`

func openCodeKey(name string) string { return "sk-" + name + "-" + secret }

// openCodeStore writes an OpenCode 2 store at path holding the credentials
// given as integration, key, active and time created, in that order.
func openCodeStore(t *testing.T, path string, wal bool, rows ...[4]any) *sql.DB {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	statements := []string{
		`create table session_v2 (id text primary key)`,
		`create table credential (id text primary key, integration_id text not null, value text not null, active integer not null, time_created integer not null)`,
	}
	if wal {
		statements = append([]string{`pragma journal_mode = wal`}, statements...)
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for i, row := range rows {
		value := fmt.Sprintf(`{"type":"key","key":%q}`, row[1])
		if raw, ok := row[1].(rawValue); ok {
			value = string(raw)
		}
		if _, err := db.Exec(`insert into credential values (?, ?, ?, ?, ?)`, fmt.Sprintf("c%d", i), row[0], value, row[2], row[3]); err != nil {
			t.Fatal(err)
		}
	}
	if !wal {
		db.Close()
		return nil
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// rawValue is a credential value written as it is, not as a key.
type rawValue string

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func TestOpenCodeKeyIsReadFromItsStoreWithoutChangingIt(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.home, ".local/share/opencode")
	openCodeStore(t, filepath.Join(dir, "opencode.db"), false,
		[4]any{"opencode", openCodeKey("zen"), 1, 300},
		[4]any{"opencode-go", openCodeKey("old"), 0, 200},
		[4]any{"opencode-go", openCodeKey("go"), 1, 100},
	)
	// An auth.json beside a store is out of date and is not read.
	f.write(".local/share/opencode/auth.json", fmt.Sprintf(`{"opencode-go":{"type":"api","key":%q}}`, openCodeKey("stale")))
	before := listDir(t, dir)
	stat, _ := os.Stat(filepath.Join(dir, "opencode.db"))

	a := f.one("opencode", false, false)
	want := terminalproto.AgentAccount{Tool: "opencode", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, LimitsState: terminalproto.AccountLimitsNotAsked}
	if fmt.Sprint(a) != fmt.Sprint(want) {
		t.Fatalf("opencode = %+v, want %+v", a, want)
	}
	if f.asked() != 0 {
		t.Fatal("a provider was asked though the allowance was not")
	}

	f.openCode.answer(200, openCodeUsage)
	a = f.one("opencode", true, false)
	wantLimits := []terminalproto.AccountLimit{
		{Kind: terminalproto.AccountLimitSession, UsedPercent: 12, ResetsAt: time.Date(2026, 11, 14, 13, 0, 0, 0, time.UTC).UnixMilli()},
		{Kind: terminalproto.AccountLimitWeek, UsedPercent: 55, ResetsAt: 1795046400000},
		{Kind: terminalproto.AccountLimitMonth, UsedPercent: 3},
	}
	if a.LimitsState != terminalproto.AccountLimitsRead || a.Plan != "go" || a.Account != "" || fmt.Sprint(a.Limits) != fmt.Sprint(wantLimits) {
		t.Fatalf("opencode limits = %+v, want %+v on the go plan", a, wantLimits)
	}
	request := f.openCode.last()
	if request.Method != "GET" || request.URL.Path != "/zen/go/v1/usage" || request.Header.Get("Authorization") != "Bearer "+openCodeKey("go") {
		t.Fatalf("opencode request: %s %s %v", request.Method, request.URL, request.Header)
	}
	if after := listDir(t, dir); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("reading the store left files behind: %v, was %v", after, before)
	}
	if again, _ := os.Stat(filepath.Join(dir, "opencode.db")); !again.ModTime().Equal(stat.ModTime()) || again.Size() != stat.Size() {
		t.Fatal("the store was written")
	}
}

func TestOpenCodeStoreInUseIsReadBesideOpenCode(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.home, ".local/share/opencode/opencode.db")
	// OpenCode holds the store open in WAL mode, so its -wal and -shm are there.
	openCodeStore(t, path, true, [4]any{"opencode", openCodeKey("zen"), 1, 1})
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); err != nil {
			t.Fatalf("the test store is not in WAL mode: %v", err)
		}
	}
	f.openCode.answer(200, openCodeUsage)
	if a := f.one("opencode", true, false); a.LimitsState != terminalproto.AccountLimitsRead {
		t.Fatalf("a store in use: %+v", a)
	}
	if got := f.openCode.last().Header.Get("Authorization"); got != "Bearer "+openCodeKey("zen") {
		t.Fatalf("the Zen key was not used when there is no Go key: %q", got)
	}
}

func TestOpenCodeFallsBackToAuthFileThenEnvironment(t *testing.T) {
	// No store: auth.json, the Go key before the Zen key.
	f := newFixture(t)
	f.write(".local/share/opencode/auth.json", fmt.Sprintf(`{"anthropic":{"type":"oauth","access":"x"},"opencode":{"type":"api","key":%q},"opencode-go":{"type":"api","key":%q}}`, openCodeKey("zen"), openCodeKey("go")))
	f.openCode.answer(200, openCodeUsage)
	f.one("opencode", true, false)
	if got := f.openCode.last().Header.Get("Authorization"); got != "Bearer "+openCodeKey("go") {
		t.Fatalf("auth.json: %q", got)
	}
	// An older OpenCode's database is not OpenCode 2's store: auth.json still counts.
	g := newFixture(t)
	old := filepath.Join(g.home, ".local/share/opencode/opencode.db")
	if err := os.MkdirAll(filepath.Dir(old), 0o700); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", "file:"+old)
	if _, err := db.Exec(`create table session (id text)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	g.write(".local/share/opencode/auth.json", fmt.Sprintf(`{"opencode":{"type":"api","key":%q}}`, openCodeKey("zen")))
	if a := g.one("opencode", false, false); a.State != terminalproto.AccountSignedIn {
		t.Fatalf("auth.json beside an older database: %+v", a)
	}
	// Neither: the connector's own environment.
	h := newFixture(t, "OPENCODE_API_KEY="+openCodeKey("env"))
	h.openCode.answer(200, openCodeUsage)
	h.one("opencode", true, false)
	if got := h.openCode.last().Header.Get("Authorization"); got != "Bearer "+openCodeKey("env") {
		t.Fatalf("environment: %q", got)
	}
	// A store with no OpenCode key, and no key anywhere else, is no sign-in,
	// whatever an out-of-date auth.json says.
	k := newFixture(t)
	openCodeStore(t, filepath.Join(k.home, ".local/share/opencode/opencode.db"), false, [4]any{"anthropic", openCodeKey("other"), 1, 1})
	k.write(".local/share/opencode/auth.json", fmt.Sprintf(`{"opencode":{"type":"api","key":%q}}`, openCodeKey("stale")))
	if a := k.one("opencode", true, false); a.State != terminalproto.AccountNone || k.asked() != 0 {
		t.Fatalf("a store without an OpenCode key: %+v", a)
	}
}

func TestOpenCodeMovedFoldersAreFollowed(t *testing.T) {
	data := t.TempDir()
	f := newFixture(t, "XDG_DATA_HOME="+data)
	f.write(".local/share/opencode/auth.json", fmt.Sprintf(`{"opencode":{"type":"api","key":%q}}`, openCodeKey("wrong")))
	if a := f.one("opencode", false, false); a.State != terminalproto.AccountNone {
		t.Fatalf("the default folder was read though XDG_DATA_HOME moved it: %+v", a)
	}
	elsewhere := filepath.Join(t.TempDir(), "store.db")
	g := newFixture(t, "XDG_DATA_HOME="+data, "OPENCODE_DB="+elsewhere)
	openCodeStore(t, elsewhere, false, [4]any{"opencode-go", openCodeKey("go"), 1, 1})
	if a := g.one("opencode", false, false); a.State != terminalproto.AccountSignedIn {
		t.Fatalf("the store OPENCODE_DB names was not read: %+v", a)
	}
	for _, env := range []string{"XDG_DATA_HOME=share", "OPENCODE_DB=opencode.db"} {
		if a := newFixture(t, env).one("opencode", false, false); a.State != terminalproto.AccountUnreadable {
			t.Fatalf("%s: %+v", env, a)
		}
	}
}

func TestOpenCodeKeyWithoutAGoPlanHasNothingToShow(t *testing.T) {
	f := newFixture(t, "OPENCODE_API_KEY="+openCodeKey("zen"))
	f.openCode.answer(403, `{"error":"no subscription"}`)
	a := f.one("opencode", true, false)
	if a.State != terminalproto.AccountSignedIn || a.LimitsState != terminalproto.AccountLimitsUnsupported || len(a.Limits) != 0 || a.LimitsAt != 0 {
		t.Fatalf("a key with no Go plan: %+v", a)
	}
	// It stands as figures do, and is not asked again every minute.
	f.now = f.now.Add(2 * time.Minute)
	f.one("opencode", true, false)
	if f.openCode.count() != 1 {
		t.Fatalf("asked again %d times inside five minutes", f.openCode.count()-1)
	}
	// A key that is refused outright is refused.
	g := newFixture(t, "OPENCODE_API_KEY="+openCodeKey("bad"))
	g.openCode.answer(401, `{}`)
	if a := g.one("opencode", true, false); a.LimitsState != terminalproto.AccountLimitsRefused {
		t.Fatalf("a refused key: %+v", a)
	}
}

func TestOpenCodeAnotherKeyDoesNotInheritFigures(t *testing.T) {
	f := newFixture(t)
	f.write(".local/share/opencode/auth.json", fmt.Sprintf(`{"opencode":{"type":"api","key":%q}}`, openCodeKey("one")))
	f.openCode.answer(200, openCodeUsage)
	f.one("opencode", true, false)
	f.write(".local/share/opencode/auth.json", fmt.Sprintf(`{"opencode":{"type":"api","key":%q}}`, openCodeKey("two")))
	f.now = f.now.Add(time.Minute)
	f.openCode.answer(500, `{}`)
	if a := f.one("opencode", true, false); len(a.Limits) != 0 || a.Plan != "" || a.LimitsState != terminalproto.AccountLimitsFailed {
		t.Fatalf("a second key was shown the first one's figures: %+v", a)
	}
}

func TestOpenCodeWhatCannotBeReadIsSaid(t *testing.T) {
	for name, setup := range map[string]func(f *fixture){
		"broken auth.json": func(f *fixture) { f.write(".local/share/opencode/auth.json", `{"opencode":`) },
		"oversized auth.json": func(f *fixture) {
			f.write(".local/share/opencode/auth.json", strings.Repeat(" ", maxCredentialBytes+1))
		},
		"not a database": func(f *fixture) { f.write(".local/share/opencode/opencode.db", "not a database at all, just text") },
		"broken credential": func(f *fixture) {
			openCodeStore(f.t, filepath.Join(f.home, ".local/share/opencode/opencode.db"), false, [4]any{"opencode-go", rawValue(`{"type":`), 1, 1})
		},
		"a folder for a store": func(f *fixture) {
			if err := os.MkdirAll(filepath.Join(f.home, ".local/share/opencode/opencode.db"), 0o700); err != nil {
				f.t.Fatal(err)
			}
		},
	} {
		f := newFixture(t)
		setup(f)
		if a := f.one("opencode", true, false); a.State != terminalproto.AccountUnreadable || f.asked() != 0 {
			t.Fatalf("%s: %+v", name, a)
		}
	}
}

func TestOpenCodeStrangeAnswerIsAFailure(t *testing.T) {
	for name, body := range map[string]string{
		"not json": "<html>", "no usage": `{"plan":"Go"}`, "no windows": `{"usage":{"rolling":null}}`, "another shape": `{"usage":[1]}`,
	} {
		f := newFixture(t, "OPENCODE_API_KEY="+openCodeKey("zen"))
		f.openCode.answer(200, body)
		if a := f.one("opencode", true, false); a.LimitsState != terminalproto.AccountLimitsFailed || len(a.Limits) != 0 {
			t.Fatalf("%s: %+v", name, a)
		}
	}
}

func TestResetTimeReadsTheShapesProvidersUse(t *testing.T) {
	for raw, want := range map[string]int64{
		`"2026-11-14T13:00:00Z"`: time.Date(2026, 11, 14, 13, 0, 0, 0, time.UTC).UnixMilli(),
		`1795046400`:             1795046400000, `1795046400000`: 1795046400000,
		`null`: 0, `"soon"`: 0, `-4`: 0, `1e20`: 0, ``: 0,
	} {
		if got := resetTime([]byte(raw)); got != want {
			t.Errorf("resetTime(%s) = %d, want %d", raw, got, want)
		}
	}
}
