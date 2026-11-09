//go:build sync_contract

package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
)

// Stage 5 browser acceptance: the real web app, in a real browser, signed in with a real session, against the real
// controller and two separately enrolled connector processes with their own homes. The browser script drives what a person
// does; this test is only the controller, the connectors and a small control channel the script uses to run connector
// cycles and change files on a machine, because that is what the world does while a browser is open or closed.
//
// Opt in with REET_STAGE5_BROWSER=1. It needs the web app built (npm --prefix web run build), Chromium, and the Stage 5 UI
// script web/tests/stage5-acceptance.e2e.mjs; without the script it skips.
func TestStage5BrowserAcceptance(t *testing.T) {
	if os.Getenv("REET_STAGE5_BROWSER") != "1" {
		t.Skip("set REET_STAGE5_BROWSER=1 to run the browser acceptance")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "web", "tests", "stage5-acceptance.e2e.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(script); err != nil {
		t.Skip("the Stage 5 browser script is not in this checkout: ", err)
	}
	h := newSyncContractHarness(t)
	h.serveWebUI()
	h.enableManagedRemovals()
	for _, name := range []string{"review-helper", "api-design", "release-notes"} {
		h.hosted(name, "canonical "+name, false, true)
	}
	machines := map[string]*syncContractMachine{}
	for _, name := range []string{"workstation", "laptop"} {
		machines[name] = h.machine(name)
	}
	h.rounds(2, machines["workstation"], machines["laptop"])

	// A real account with a real password: the browser signs in through the sign-in form, so the session it holds is the
	// one the server issues, not one placed in the store for it. (The instance owner is the account the library belongs to.)
	const email, password = "local@reet.invalid", "correct-horse-battery-staple"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.st.SetUserPassword(h.uid, hash); err != nil {
		t.Fatal(err)
	}
	// A second, read-only account: its library is its own and empty, and it must not reach the owner's items or machines.
	const viewerEmail = "viewer@example.test"
	viewerHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.st.CreateUser(viewerEmail, "Viewer", "viewer", viewerHash); err != nil {
		t.Fatal(err)
	}

	// A second writer with nothing of its own: no machines and, if the library is per account, no items.
	const memberEmail = "member@example.test"
	memberHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.st.CreateUser(memberEmail, "Member", "member", memberHash); err != nil {
		t.Fatal(err)
	}

	pick := func(r *http.Request) (*syncContractMachine, string) {
		m := machines[r.URL.Query().Get("m")]
		return m, r.URL.Query().Get("name")
	}
	fail := func(w http.ResponseWriter, err error) { http.Error(w, err.Error(), http.StatusInternalServerError) }
	mux := http.NewServeMux()
	// Run connector cycles on the named machines, optionally pausing before them so a time-based deletion window can pass.
	mux.HandleFunc("/rounds", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if pause, _ := strconv.Atoi(r.URL.Query().Get("pause")); pause > 0 {
			time.Sleep(time.Duration(pause) * time.Millisecond)
		}
		for range n {
			for _, name := range strings.Split(r.URL.Query().Get("m"), ",") {
				m := machines[name]
				if m == nil || m.link.offline.Load() {
					continue
				}
				if output, err := m.tick(); err != nil {
					fail(w, fmt.Errorf("%s: %w\n%s", name, err, output))
					return
				}
			}
		}
		fmt.Fprint(w, "ok")
	})
	// Take a machine's connector off the network (it simply stops being reachable) or bring it back.
	mux.HandleFunc("/offline", func(w http.ResponseWriter, r *http.Request) {
		m, _ := pick(r)
		m.link.offline.Store(r.URL.Query().Get("value") == "true")
		fmt.Fprint(w, "ok")
	})
	// A skill that exists only on the machine: not in the library, so the connect step can offer to import it.
	mux.HandleFunc("/localskill", func(w http.ResponseWriter, r *http.Request) {
		m, name := pick(r)
		m.localSkill(name, "written on the machine "+name)
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/present", func(w http.ResponseWriter, r *http.Request) {
		m, name := pick(r)
		fmt.Fprint(w, m.present(name))
	})
	mux.HandleFunc("/backups", func(w http.ResponseWriter, r *http.Request) {
		m, name := pick(r)
		fmt.Fprint(w, len(m.backupsOf(name)))
	})
	mux.HandleFunc("/used", func(w http.ResponseWriter, r *http.Request) {
		_, name := pick(r)
		// The shelf status is keyed by the skill's own id, not the library item id.
		h.used(h.candidate(syncContractName(name)).ID, r.URL.Query().Get("value") == "true")
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/deletefile", func(w http.ResponseWriter, r *http.Request) {
		m, name := pick(r)
		if err := os.Remove(m.path(name, r.URL.Query().Get("file"))); err != nil {
			fail(w, err)
			return
		}
		fmt.Fprint(w, "ok")
	})
	// Replace a managed folder by a plain file: a local entry reet.sh cannot treat as an install.
	mux.HandleFunc("/breakdir", func(w http.ResponseWriter, r *http.Request) {
		m, name := pick(r)
		dir := filepath.Dir(m.path(name, "SKILL.md"))
		if err := os.RemoveAll(dir); err != nil {
			fail(w, err)
			return
		}
		if err := os.WriteFile(dir, []byte("not a folder"), 0644); err != nil {
			fail(w, err)
			return
		}
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/healdir", func(w http.ResponseWriter, r *http.Request) {
		m, name := pick(r)
		if err := os.Remove(filepath.Dir(m.path(name, "SKILL.md"))); err != nil {
			fail(w, err)
			return
		}
		fmt.Fprint(w, "ok")
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	control := &http.Server{Handler: mux}
	go func() { _ = control.Serve(listener) }()
	t.Cleanup(func() { _ = control.Close() })

	cmd := exec.Command("node", script)
	cmd.Dir = filepath.Dir(filepath.Dir(script))
	cmd.Env = append(os.Environ(),
		"REET_UI_URL="+h.controller.URL,
		"REET_ACCEPT_EMAIL="+email,
		"REET_ACCEPT_PASSWORD="+password,
		"REET_ACCEPT_VIEWER_EMAIL="+viewerEmail,
		"REET_ACCEPT_MEMBER_EMAIL="+memberEmail,
		"REET_ACCEPT_CONTROL=http://"+listener.Addr().String(),
		"PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH="+envOr("PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH", "/opt/pw-browsers/chromium"),
	)
	output, err := cmd.CombinedOutput()
	t.Logf("browser acceptance output:\n%s", output)
	if err != nil {
		t.Fatalf("browser acceptance failed: %v", err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
