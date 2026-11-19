//go:build linux || darwin

package scan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
)

// fakeCaptures stands for the controller's side of the machine capture: it
// hands out one piece of work and records what the machine sends back.
type fakeCaptures struct {
	mu        sync.Mutex
	work      *store.RemoteImport
	previews  []skillimport.Preview
	failures  []string
	transfers [][]skillbundle.ContentFile
}

func (f *fakeCaptures) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var in struct {
		Preview skillimport.Preview       `json:"preview"`
		Code    string                    `json:"code"`
		Files   []skillbundle.ContentFile `json:"files"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	reply := any(map[string]bool{"ok": true})
	switch r.URL.Path {
	case "/api/v2/sync/skill-import/work":
		// Work that ended is no longer handed out.
		var work *store.RemoteImport
		if f.work.State == "previewing" || f.work.State == "uploading" {
			work = f.work
		}
		reply = map[string]any{"schemaVersion": skillimport.RemoteVersion, "work": work}
	case "/api/v2/sync/skill-import/preview":
		f.previews = append(f.previews, in.Preview)
		// Every file is allowed, so the controller approves it at once.
		m, err := in.Preview.Manifest(nil)
		if err == nil {
			p := in.Preview
			f.work.State, f.work.Preview, f.work.Digest = "uploading", &p, m.Digest
		}
	case "/api/v2/sync/skill-import/failure":
		f.failures = append(f.failures, in.Code)
		f.work.State = "failed"
	case "/api/v2/sync/skill-import/transfer":
		f.transfers = append(f.transfers, in.Files)
		f.work.State = "completed"
		reply = store.PrivateSkillResult{}
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reply)
}

const captureInstance = "cccccccccccccccccccccccccccccccccccccccccccccccc"

// useVersionMachine is a home with a live copy of demo in ~/.agents/skills,
// reet's, and the backup of the copy edited there, and a controller asking
// for backup.
func useVersionMachine(t *testing.T, backup string) (agentConfig, *fakeCaptures, string) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	writeSkill := func(dir, body string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo\n---\n"+body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill(filepath.Join(home, ".agents", "skills", "demo"), "reet's copy")
	writeSkill(filepath.Join(home, ".agents", ".reet-backups", "2026-11-18", "agents-demo"), "edited on this machine")
	f := &fakeCaptures{work: &store.RemoteImport{ID: "use-version-1", Instance: captureInstance, State: "previewing", Agent: "agents", Name: "demo", BackupPath: backup, UseVersionItem: "item"}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return agentConfig{ID: "agent", Secret: "secret", Home: home, Host: "studio", Server: srv.URL, Token: "token"}, f, home
}

func runCapture(t *testing.T, c agentConfig) {
	t.Helper()
	if handled, err := processRemoteImportWith(context.Background(), c, "test", captureInstance); err != nil || !handled {
		t.Fatalf("capture: handled=%t %v", handled, err)
	}
}

func sentText(files []skillbundle.ContentFile) string {
	var b strings.Builder
	for _, f := range files {
		b.Write(f.Data)
	}
	return b.String()
}

// Use this version reads the backup the controller names, not the copy in
// the skills folder, which is reet's own.
func TestUseVersionCaptureReadsTheBackup(t *testing.T) {
	c, f, home := useVersionMachine(t, "~/.agents/.reet-backups/2026-11-18/agents-demo")
	runCapture(t, c)
	backup, err := skillbundle.Scan(filepath.Join(home, ".agents", ".reet-backups", "2026-11-18", "agents-demo"), skillbundle.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.failures) != 0 || len(f.previews) != 1 || f.previews[0].SourceDigest != backup.Digest {
		t.Fatalf("preview: %+v failures %v", f.previews, f.failures)
	}
	runCapture(t, c)
	if len(f.failures) != 0 || len(f.transfers) != 1 {
		t.Fatalf("transfer: %d failures %v", len(f.transfers), f.failures)
	}
	if text := sentText(f.transfers[0]); !strings.Contains(text, "edited on this machine") || strings.Contains(text, "reet's copy") {
		t.Fatalf("sent %q", text)
	}
}

// Anything but one backup beside the agent's own managed skills folder is
// refused, and nothing is read or sent.
func TestUseVersionCaptureRefusesPathsOutsideBackups(t *testing.T) {
	for _, tc := range []struct{ backup, agent, project string }{
		{backup: "~/.agents/skills/demo"},
		{backup: "~/.agents/.reet-backups/2026-11-18/../../skills/demo"},
		{backup: "~/.agents/.reet-backups/2026-11-18/./agents-demo"},
		{backup: "~/.agents/.reet-backups/2026-11-18"},
		{backup: "~/.agents/.reet-backups/2026-11-18/agents-demo/SKILL.md"},
		{backup: "~/.agents/.reet-backups/not-a-date/agents-demo"},
		{backup: "~/.agents/.reet-sync-backups/kept/agents-demo"},
		{backup: "~/.codex/.reet-backups/2026-11-18/agents-demo"},
		{backup: "~/.claude/.reet-backups/2026-11-18/agents-demo"},
		{backup: "~/.agents/.reet-backups/2026-11-18/agents-demo", agent: "codex"},
		{backup: "~/.agents/.reet-backups/2026-11-18/agents-demo", project: "/tmp/project"},
		{backup: "/etc/.reet-backups/2026-11-18/agents-demo"},
		{backup: "~/../.agents/.reet-backups/2026-11-18/agents-demo"},
		{backup: "~/.agents//.reet-backups/2026-11-18/agents-demo"},
		{backup: "~\\.agents\\.reet-backups\\2026-11-18\\agents-demo"},
	} {
		t.Run(tc.backup+" "+tc.agent+tc.project, func(t *testing.T) {
			c, f, _ := useVersionMachine(t, tc.backup)
			if tc.agent != "" {
				f.work.Agent = tc.agent
			}
			f.work.Project = tc.project
			runCapture(t, c)
			if len(f.failures) != 1 || f.failures[0] != "scope_unavailable" || len(f.previews) != 0 || len(f.transfers) != 0 {
				t.Fatalf("failures %v previews %d transfers %d", f.failures, len(f.previews), len(f.transfers))
			}
		})
	}
}

// A backups folder, date folder or backup that is a link is refused, and a
// link inside the backup is never followed: nothing outside is read or sent.
func TestUseVersionCaptureNeverFollowsALink(t *testing.T) {
	const backup = "~/.agents/.reet-backups/2026-11-18/agents-demo"
	outside := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		skill := filepath.Join(dir, "2026-11-18", "agents-demo")
		if err := os.MkdirAll(skill, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: demo\ndescription: Outside\n---\nOUTSIDE SECRET\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	for name, swap := range map[string]func(t *testing.T, home string){
		"backups folder": func(t *testing.T, home string) {
			root := filepath.Join(home, ".agents", ".reet-backups")
			must(t, os.RemoveAll(root))
			must(t, os.Symlink(outside(t), root))
		},
		"date folder": func(t *testing.T, home string) {
			date := filepath.Join(home, ".agents", ".reet-backups", "2026-11-18")
			must(t, os.RemoveAll(date))
			must(t, os.Symlink(filepath.Join(outside(t), "2026-11-18"), date))
		},
		"backup": func(t *testing.T, home string) {
			entry := filepath.Join(home, ".agents", ".reet-backups", "2026-11-18", "agents-demo")
			must(t, os.RemoveAll(entry))
			must(t, os.Symlink(filepath.Join(outside(t), "2026-11-18", "agents-demo"), entry))
		},
		"file in the backup": func(t *testing.T, home string) {
			skill := filepath.Join(home, ".agents", ".reet-backups", "2026-11-18", "agents-demo", "notes.md")
			must(t, os.Symlink(filepath.Join(outside(t), "2026-11-18", "agents-demo", "SKILL.md"), skill))
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, f, home := useVersionMachine(t, backup)
			swap(t, home)
			runCapture(t, c)
			if len(f.failures) != 1 || len(f.transfers) != 0 {
				t.Fatalf("failures %v transfers %d", f.failures, len(f.transfers))
			}
			raw, _ := json.Marshal(f.previews)
			if strings.Contains(string(raw), "Outside") {
				t.Fatal("read outside the backups")
			}
		})
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
