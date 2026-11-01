package lifecycle

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/cliui"
)

func TestApprovalRequiresTheExistingExactYes(t *testing.T) {
	for _, answer := range []string{"\n", "no\n", "y\n", "yes\n"} {
		a := New("dev", "test", strings.NewReader(answer), &bytes.Buffer{})
		if err := a.confirm(false); (err == nil) != (answer == "yes\n") {
			t.Fatalf("%q: %v", answer, err)
		}
	}
	a := New("dev", "test", strings.NewReader("yes\n"), &bytes.Buffer{})
	a.nonInteractive = true
	if a.confirm(false) == nil {
		t.Fatal("non-interactive mode consumed an approval")
	}
}

func TestJSONDoctorKeepsBothStreamsCleanAndPropagatesFailures(t *testing.T) {
	a, state, _ := testApp(t)
	t.Setenv("REET_URL", "https://fixture.invalid")
	t.Setenv("REET_TOKEN", "fixture-token")
	var progress bytes.Buffer
	a.Progress = &progress
	a.client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) { return response(401, "fixture-token"), nil })}
	err := a.Run("doctor", []string{"--state-dir", state, "--json", "--no-animation"})
	if err == nil {
		t.Fatal("authentication failure lost")
	}
	text := a.Out.(*bytes.Buffer).String()
	if !strings.HasPrefix(text, "{") || strings.Contains(text, "\x1b") || strings.Contains(text, "fixture-token") || progress.Len() != 0 {
		t.Fatalf("out=%s progress=%s", text, progress.String())
	}
}

func TestUpgradeCancellationAndRendererFailureRecoverOnceWithLiveContext(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd fixture")
	}
	for _, phase := range []string{"cancel backup", "renderer backup", "cancel new start"} {
		t.Run(phase, func(t *testing.T) {
			a, state, binary := testApp(t)
			data := t.TempDir()
			db, err := sql.Open("sqlite", filepath.Join(data, "reet.db"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("CREATE TABLE sample(value TEXT); INSERT INTO sample VALUES('old')"); err != nil {
				t.Fatal(err)
			}
			db.Close()
			d := &deployment{Mode: "native", Binary: binary, Data: data, Addr: "127.0.0.1:8080", Version: "v1.0.0"}
			if err = os.MkdirAll(state, 0700); err != nil {
				t.Fatal(err)
			}
			if err = writeJSON(filepath.Join(state, "deployment.json"), d); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			starts, stops, backups := 0, 0, 0
			uiErr := errors.New("progress renderer closed")
			a.taskRunner = func(ctx context.Context, label string, _ bool, work cliui.Work) error {
				if label == "Backing up and verifying SQLite" {
					backups++
					if phase == "cancel backup" {
						cancel()
					}
					err := work(ctx, func(string) {})
					if phase == "renderer backup" {
						return errors.Join(err, uiErr)
					}
					return err
				}
				return work(ctx, func(string) {})
			}
			a.run = func(callCtx context.Context, name string, args ...string) (string, error) {
				if name != "systemctl" {
					return "reet v2.0.0 (new)", nil
				}
				switch args[1] {
				case "show":
					return "active", nil
				case "stop":
					stops++
					if stops > 1 && callCtx.Err() != nil {
						t.Error("rollback got cancelled context")
					}
					return "", nil
				case "start":
					starts++
					if phase == "cancel new start" && starts == 1 {
						db, _ := sql.Open("sqlite", filepath.Join(data, "reet.db"))
						db.Exec("UPDATE sample SET value='migrated'")
						db.Close()
						cancel()
						return "", callCtx.Err()
					}
					if callCtx.Err() != nil {
						t.Error("recovery start got cancelled context")
					}
					return "", nil
				}
				return "", errors.New("unexpected command")
			}
			original := a.client.Transport
			a.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "/healthz") {
					if r.Context().Err() != nil {
						t.Error("recovery health check got cancelled context")
					}
					return response(200, `{"ok":true,"version":"v1.0.0"}`), nil
				}
				return original.RoundTrip(r)
			})
			err = a.RunContext(ctx, "upgrade", []string{"--state-dir", state, "--yes", "--no-animation"})
			if err == nil || backups != 1 {
				t.Fatalf("error=%v backups=%d", err, backups)
			}
			if phase == "renderer backup" && !errors.Is(err, uiErr) {
				t.Fatal("renderer failure lost", err)
			}
			if phase != "renderer backup" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			wantStarts, wantStops := 1, 1
			if phase == "cancel new start" {
				wantStarts, wantStops = 2, 2
			}
			if starts != wantStarts || stops != wantStops {
				t.Fatalf("starts=%d stops=%d", starts, stops)
			}
			if contents(t, binary) != "old binary" {
				t.Fatal("binary not restored")
			}
			current, err := load(state)
			if err != nil || current.Version != d.Version {
				t.Fatal("manifest not restored", current, err)
			}
			db, err = openReadOnly(filepath.Join(data, "reet.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var value string
			if err = db.QueryRow("SELECT value FROM sample").Scan(&value); err != nil || value != "old" {
				t.Fatal("database not restored", value, err)
			}
			if _, err = os.Stat(filepath.Join(state, "lifecycle.lock")); !os.IsNotExist(err) {
				t.Fatal("lock survived cleanup", err)
			}
			if strings.Contains(a.Out.(*bytes.Buffer).String(), "Upgraded.") {
				t.Fatal("reported success before recovery")
			}
		})
	}
}
