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

// importFixture serves a new controller with an owner token and writes a
// password-protected copy of another controller, a token file and a password
// file for the command to read.
func importFixture(t *testing.T) (url, copyFile, tokenFile, passwordFile, dir string, st *store.Store) {
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
	ts := httptest.NewServer(api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthAccounts), api.WithBackups(svc)))
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
	url, copyFile, tokenFile, passwordFile, dir, _ := importFixture(t)
	var out bytes.Buffer
	if err := backupImport([]string{"--server", url, "--token-file", tokenFile, "--password-file", passwordFile, "--drop-tokens", copyFile}, &out); err != nil {
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
	url, copyFile, tokenFile, passwordFile, dir, st := importFixture(t)
	if _, err := st.CreateTag("already-here"); err != nil {
		t.Fatal(err)
	}
	err := backupImport([]string{"--server", url, "--token-file", tokenFile, "--password-file", passwordFile, copyFile}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--replace") || !strings.Contains(err.Error(), "changed nothing") {
		t.Fatalf("got %v", err)
	}
	if p, _ := backup.PendingImport(dir); p != nil {
		t.Fatal("a refused import was staged")
	}
	if err := backupImport([]string{"--server", url, "--token-file", tokenFile, "--password-file", passwordFile, "--replace", copyFile}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}
