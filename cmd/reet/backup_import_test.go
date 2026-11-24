package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
)

const cliKey = "0123456789abcdef0123456789abcdef"
const cliPassword = "a copy password for the command"

func cliController(t *testing.T) (*store.Store, *backup.Service, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	sealer, _ := backup.NewSealer(cliKey, "test")
	svc := backup.New(st, backup.Config{Dir: filepath.Join(dir, "backups"), Sealer: sealer, Usable: true, KeySource: "test", Key: cliKey})
	return st, svc, dir
}

// importFixture serves a new controller in token mode, where the instance
// token is the owner's one credential and still sends an import, and writes a
// password-protected copy of another controller, a token file and a password
// file for the command to read.
func importFixture(t *testing.T) (url, copyFile, tokenFile, passwordFile, dir string, st *store.Store) {
	t.Helper()
	return importFixtureIn(t, api.AuthToken)
}

// importFixtureIn is importFixture in the given sign-in mode; with accounts the
// owner holds a personal token, which the import refuses.
func importFixtureIn(t *testing.T, mode api.AuthMode) (url, copyFile, tokenFile, passwordFile, dir string, st *store.Store) {
	t.Helper()
	src, ssvc, _ := cliController(t)
	if _, err := src.EnsureLocalOwner(); err != nil {
		t.Fatal(err)
	}
	pw := cliPassword
	if _, err := ssvc.SaveSettings(0, backup.SettingsInput{Password: &pw, Compression: backup.CompressionGzip, Destination: "local"}); err != nil {
		t.Fatal(err)
	}
	item, err := ssvc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ssvc.Archive(context.Background(), 0, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	st, svc, dir := cliController(t)
	owner, err := st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIToken(owner.ID, "owner", auth.HashToken("the-owner-token")); err != nil {
		t.Fatal(err)
	}
	instanceToken := ""
	if mode == api.AuthToken {
		instanceToken = "the-owner-token"
	}
	ts := httptest.NewServer(api.New(st, meta.Fetcher{}, instanceToken, api.WithAuthMode(mode), api.WithBackups(svc)))
	t.Cleanup(ts.Close)
	tmp := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(tmp, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return ts.URL, write("copy.reetbackup", raw), write("token", []byte("the-owner-token\n")), write("password", []byte(cliPassword+"\n")), dir, st
}

func TestTheImportCommandSendsACopyAndSaysWhenItTakesEffect(t *testing.T) {
	t.Parallel()
	url, copyFile, tokenFile, passwordFile, dir, _ := importFixture(t)
	var out bytes.Buffer
	if err := backupImport([]string{"--server", url, "--token-file", tokenFile, "--password-file", passwordFile, "--drop-tokens", "--allow-live-source", copyFile}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Staged ", "Revoked", "first", "restart"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), cliPassword) || strings.Contains(out.String(), "the-owner-token") {
		t.Fatal("the command printed a secret")
	}
	if p, _ := backup.PendingImport(dir); p == nil {
		t.Fatal("nothing was staged")
	}
}

func TestTheImportCommandNeverTakesASecretAsAnArgument(t *testing.T) {
	t.Parallel()
	url, copyFile, _, _, _, _ := importFixture(t)
	for _, flagName := range []string{"--token", "--password"} {
		if err := backupImport([]string{"--server", url, flagName, "secret", copyFile}, &bytes.Buffer{}); err == nil {
			t.Fatalf("%s was accepted on the command line", flagName)
		}
	}
	if err := backupImport([]string{"--server", url, copyFile}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--token-file") {
		t.Fatalf("a missing token must say how to give one, got %v", err)
	}
	if err := backupImport([]string{"--server", url, "--token-file", "-", "--password-file", "-", copyFile}, &bytes.Buffer{}); err == nil {
		t.Fatal("standard input cannot be both secrets")
	}
}

func TestTheImportCommandSaysHowToReplaceWhenTheControllerHoldsData(t *testing.T) {
	t.Parallel()
	url, copyFile, tokenFile, passwordFile, dir, st := importFixture(t)
	if _, err := st.CreateTag("already-here"); err != nil {
		t.Fatal(err)
	}
	err := backupImport([]string{"--server", url, "--token-file", tokenFile, "--password-file", passwordFile, "--allow-live-source", copyFile}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--replace") || !strings.Contains(err.Error(), "changed nothing") {
		t.Fatalf("got %v", err)
	}
	if p, _ := backup.PendingImport(dir); p != nil {
		t.Fatal("a refused import was staged")
	}
	if err := backupImport([]string{"--server", url, "--token-file", tokenFile, "--password-file", passwordFile, "--replace", "--allow-live-source", copyFile}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

// With accounts a personal token cannot stage a database replacement. The
// command must say what works instead, not only pass on a 403.
func TestTheImportCommandSaysWhatToDoWhenTheControllerWantsABrowser(t *testing.T) {
	t.Parallel()
	url, copyFile, tokenFile, passwordFile, dir, _ := importFixtureIn(t, api.AuthAccounts)
	id := "instance-20261124T070000Z-0123abcd"
	named := filepath.Join(filepath.Dir(copyFile), id+".reetbackup")
	if err := os.Rename(copyFile, named); err != nil {
		t.Fatal(err)
	}
	err := backupImport([]string{"--server", url, "--token-file", tokenFile, "--password-file", passwordFile, "--allow-live-source", named}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("a personal token staged an import")
	}
	for _, want := range []string{"changed nothing", "signed-in browser", "Admin → Move → Move a controller in", "reet backup restore --yes " + id, "backups/" + id + ".reetbackup", "with reet stopped", "--allow-live-source"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal lacks %q:\n%v", want, err)
		}
	}
	if p, _ := backup.PendingImport(dir); p != nil {
		t.Fatal("a refused import was staged")
	}
}

// The local restore the refusal names works on a downloaded copy put in the
// new controller's backups folder.
func TestADownloadedCopyPutInTheBackupsFolderRestoresLocally(t *testing.T) {
	_, copyFile, _, passwordFile, _, _ := importFixture(t)
	data := t.TempDir()
	t.Setenv("REET_DATA_DIR", data)
	t.Setenv("REET_BACKUP_KEY", cliKey)
	raw, err := os.ReadFile(copyFile)
	if err != nil {
		t.Fatal(err)
	}
	id := "instance-20261124T070000Z-0123abcd"
	if err := os.MkdirAll(filepath.Join(data, "backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "backups", id+".reetbackup"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := backupRestore([]string{"--yes", "--password-file", passwordFile, "--allow-live-source", id}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

func TestTheBackupHelpSaysImportNeedsTokenMode(t *testing.T) {
	var out bytes.Buffer
	if err := runBackup([]string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"REET_AUTH=token", "Admin → Move → Move a controller in", "restore"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help lacks %q:\n%s", want, out.String())
		}
	}
}
