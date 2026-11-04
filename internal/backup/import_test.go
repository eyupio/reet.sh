package backup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/eyupio/reet.sh/internal/store"
)

const otherKey = "fedcba9876543210fedcba9876543210"
const copyPassword = "a copy password that opens anywhere"

// movable builds a source controller that holds a browser sign-in, a personal
// token, an MCP connection and a private-connection identity, and returns its
// password-protected copy, the one kind that opens on another machine.
func movable(t *testing.T) (raw []byte, user store.User) {
	t.Helper()
	svc, st, user := testService(t, nil)
	future := time.Now().Add(time.Hour).UnixMilli()
	if err := st.CreateSession("sess-1", user.ID, future, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIToken(user.ID, "laptop", "token-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateMCPConnection(user.ID, "agent", "read", "mcp-hash", future); err != nil {
		t.Fatal(err)
	}
	if err := st.SavePrivateIdentityIfAbsent(store.PrivateIdentityRecord{FormatVersion: 1, Ciphertext: base64.RawURLEncoding.EncodeToString(make([]byte, 64))}); err != nil {
		t.Fatal(err)
	}
	secret, err := svc.cfg.Sealer.SealSecret("the destination's secret key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBackupRemote(store.BackupRemote{Name: "offsite", SecretSealed: secret, PathStyle: -1}); err != nil {
		t.Fatal(err)
	}
	pw := copyPassword
	if _, err := svc.SaveSettings(0, SettingsInput{Password: &pw, Compression: CompressionGzip, Destination: "local"}); err != nil {
		t.Fatal(err)
	}
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	raw, err = svc.Archive(context.Background(), 0, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	return raw, user
}

// destination is a controller with a different backup key, so nothing sealed
// under the source's key opens there.
func destination(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sealer, err := NewSealer(otherKey, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, Config{Dir: filepath.Join(dir, "backups"), Key: otherKey, Sealer: sealer, Usable: true, Version: "test"}), dir
}

func applied(t *testing.T, dir string) *store.Store {
	t.Helper()
	res, err := ApplyPendingImport(dir)
	if err != nil || res == nil {
		t.Fatalf("apply: %v %v", res, err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestAnImportThatDropsSignInsAndTokensEndsThemAndSaysWhatCame(t *testing.T) {
	raw, user := movable(t)
	svc, dir := destination(t)
	rep, err := svc.StageImport(context.Background(), dir, raw, copyPassword, RestoreOptions{DropSessions: true, DropTokens: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SessionsEnded != 1 || rep.TokensRevoked != 1 || rep.MCPConnectionsRevoked != 1 || !rep.IdentityDropped {
		t.Fatalf("report: %+v", rep)
	}
	if rep.Users < 1 {
		t.Fatalf("the report does not say accounts came across: %+v", rep)
	}
	// The copy password and the destination's key were sealed under the
	// source's key; this controller cannot open them and must say so now
	// rather than when a backup fails.
	if len(rep.Unreadable) != 2 || !strings.Contains(rep.Unreadable[0], `"offsite"`) || !strings.Contains(rep.Unreadable[1], "copy password") {
		t.Fatalf("unreadable: %v", rep.Unreadable)
	}
	if p, _ := PendingImport(dir); p == nil || p.ID != rep.ID {
		t.Fatalf("not staged: %+v", p)
	}
	// Staging replaces nothing: the running database is untouched.
	if _, found, _ := svc.st.APITokenUser("token-hash", time.Now().UnixMilli()); found {
		t.Fatal("staging changed the live database")
	}
	st := applied(t, dir)
	now := time.Now().UnixMilli()
	if _, ok, _ := st.SessionUser("sess-1", now); ok {
		t.Fatal("a sign-in from the old machine still works")
	}
	if _, ok, _ := st.APITokenUser("token-hash", now); ok {
		t.Fatal("a personal token from the old machine still works")
	}
	if _, _, ok, _ := st.MCPUser("mcp-hash", now); ok {
		t.Fatal("an MCP connection from the old machine still works")
	}
	if _, found, _ := st.PrivateIdentity(); found {
		t.Fatal("the old controller's private identity was carried across")
	}
	if u, ok, err := st.UserByEmail(user.Email); err != nil || !ok || u.ID != user.ID {
		t.Fatalf("the account did not come across: %v", err)
	}
	if p, _ := PendingImport(dir); p != nil {
		t.Fatal("the import is still pending after it was applied")
	}
}

func TestAnImportByDefaultLeavesSignInsAndTokensWorking(t *testing.T) {
	raw, _ := movable(t)
	svc, dir := destination(t)
	rep, err := svc.StageImport(context.Background(), dir, raw, copyPassword, RestoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SessionsEnded != 0 || rep.SessionsKept != 1 || !rep.TokensKept {
		t.Fatalf("report: %+v", rep)
	}
	st := applied(t, dir)
	now := time.Now().UnixMilli()
	if _, ok, _ := st.SessionUser("sess-1", now); !ok {
		t.Fatal("a default restore ended a sign-in")
	}
	if _, ok, _ := st.APITokenUser("token-hash", now); !ok {
		t.Fatal("a default restore revoked a token")
	}
}

func TestAnUnreadableDestinationIsClearedSwitchedOffAndListed(t *testing.T) {
	raw, _ := movable(t)
	svc, dir := destination(t)
	if _, err := svc.StageImport(context.Background(), dir, raw, copyPassword, RestoreOptions{}); err != nil {
		t.Fatal(err)
	}
	st := applied(t, dir)
	remotes, err := st.BackupRemotes(0)
	if err != nil || len(remotes) != 1 {
		t.Fatalf("remotes: %v %v", remotes, err)
	}
	if remotes[0].SecretSealed != "" || !remotes[0].Disabled || remotes[0].LastError == "" {
		t.Fatalf("destination kept a secret it cannot open, or is still on: %+v", remotes[0])
	}
}

func TestACopyFromANewerSchemaIsRefusedAndNothingIsStaged(t *testing.T) {
	src := t.TempDir()
	st, err := store.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", filepath.Join(src, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version=999`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	payload, err := os.ReadFile(filepath.Join(src, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, dir := destination(t)
	raw, err := svc.cfg.Sealer.Seal(Manifest{ID: "instance-newer", Kind: "instance", Target: "instance"}, payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StageImport(context.Background(), dir, raw, "", RestoreOptions{}); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("want ErrNewerSchema, got %v", err)
	}
	if p, _ := PendingImport(dir); p != nil {
		t.Fatal("a refused copy was staged")
	}
}

func TestACancelledImportIsNeverApplied(t *testing.T) {
	raw, _ := movable(t)
	svc, dir := destination(t)
	if _, err := svc.StageImport(context.Background(), dir, raw, copyPassword, RestoreOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := CancelImport(dir); err != nil {
		t.Fatal(err)
	}
	if res, err := ApplyPendingImport(dir); res != nil || err != nil {
		t.Fatalf("applied after cancel: %v %v", res, err)
	}
}

func TestAPasswordCopyNeedsItsPasswordToBeStaged(t *testing.T) {
	raw, _ := movable(t)
	svc, dir := destination(t)
	if _, err := svc.StageImport(context.Background(), dir, raw, "", RestoreOptions{}); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("want ErrPasswordRequired, got %v", err)
	}
}
