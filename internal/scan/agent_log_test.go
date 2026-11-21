//go:build linux || darwin

package scan

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPeriodicScanPrintsFindingsOnlyWhenTheyChange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Agent":{"Host":"studio"}}`))
	}))
	defer srv.Close()
	config := enrolledMachine(t, srv.URL)
	c := loaded(t, config)
	writeSkill(t, filepath.Join(c.Home, ".agents", "skills", "alpha"))
	periodicScans = &scanEcho{}
	t.Cleanup(func() { periodicScans = &scanEcho{} })
	cycle := func(periodic bool) string {
		t.Helper()
		var out bytes.Buffer
		last := time.Time{}
		if err := agentCycle(context.Background(), c, config, &out, "test", &last, periodic); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	first := cycle(true)
	if !strings.Contains(first, "alpha") {
		t.Fatalf("the first scan did not print its findings: %q", first)
	}
	if again := cycle(true); again != "" {
		t.Fatalf("an unchanged scan printed again: %q", again)
	}
	writeSkill(t, filepath.Join(c.Home, ".agents", "skills", "beta"))
	if changed := cycle(true); !strings.Contains(changed, "beta") {
		t.Fatalf("a changed scan was kept quiet: %q", changed)
	}
	if again := cycle(true); again != "" {
		t.Fatalf("an unchanged scan printed again: %q", again)
	}
	// A single run always says what it found.
	if once := cycle(false); !strings.Contains(once, "beta") {
		t.Fatalf("a single run printed %q", once)
	}
}

func TestARepeatedCycleErrorIsSaidOnChangeAndThenEveryFewMinutes(t *testing.T) {
	at := time.Unix(1700000000, 0)
	echo := &movedEcho{now: func() time.Time { return at }}
	refused := func(port string) error {
		return errors.New("dial tcp 10.0.0.1:" + port + "->10.0.0.2:443: connect: connection refused")
	}
	if !echo.worthSaying(refused("50001")) {
		t.Fatal("the first failure was kept quiet")
	}
	for i := 0; i < 20; i++ {
		at = at.Add(5 * time.Second)
		if echo.worthSaying(refused("5000" + string(rune('0'+i%10)))) {
			t.Fatalf("the same failure was said again after %s", time.Duration(i+1)*5*time.Second)
		}
	}
	if !echo.worthSaying(controllerError{Status: 503}) {
		t.Fatal("a different failure was kept quiet")
	}
	// A good cycle in between does not make the same failure news.
	echo.worthSaying(nil)
	at = at.Add(time.Minute)
	if echo.worthSaying(controllerError{Status: 503}) {
		t.Fatal("the same failure was said again a minute later")
	}
	at = at.Add(errorRepeat)
	if !echo.worthSaying(controllerError{Status: 503}) {
		t.Fatal("a failure that lasted was never said again")
	}
}

func TestAgentLogOverTheLimitKeepsOnlyItsTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	var lines bytes.Buffer
	for i := 0; lines.Len() < 4096; i++ {
		lines.WriteString(strings.Repeat("x", 30) + " line\n")
	}
	lines.WriteString("the last line\n")
	if err := os.WriteFile(path, lines.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	// Under the limit nothing changes.
	if err := trimAgentLog(path, int64(lines.Len()), 512); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, lines.Bytes()) {
		t.Fatal("a log within the limit was changed")
	}
	// A writer that already holds the log open carries on at the new end.
	writer, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err = trimAgentLog(path, 1024, 512); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.WriteString("written afterwards\n"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(b) > 700 || !strings.HasPrefix(got[0], "(log passed") {
		t.Fatalf("trimmed log is %d bytes, first line %q", len(b), got[0])
	}
	if got[1] != strings.Repeat("x", 30)+" line" {
		t.Fatalf("the tail starts mid-line: %q", got[1])
	}
	if got[len(got)-2] != "the last line" || got[len(got)-1] != "written afterwards" {
		t.Fatalf("the tail ends %q", got[len(got)-2:])
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
}

func TestAgentLogTrimRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(real, bytes.Repeat([]byte("line\n"), 1000), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "agent.log")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := trimAgentLog(link, 1024, 512); err == nil {
		t.Fatal("a symlinked log was followed")
	}
	if info, _ := os.Stat(real); info.Size() != 5000 {
		t.Fatal("the file behind the symlink was changed")
	}
}
