package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/notify"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// These helpers exercise the actual connector entry point in separate OS
// processes, through HTTP, against the real API and a temporary SQLite store.
// Each child retains process-scoped state across controlled run --once cycles.
// Stage 1 also exercises the production foreground loop with private bootstrap
// unavailable. Native service and actual Tailcat transport have separate tests.
const syncContractToken = "stage0-disposable-owner-token"

type syncContractHarness struct {
	legacyProtocol bool
	// removals records the internal managed-removals rollout control so it
	// survives a simulated controller restart.
	removals   bool
	t          *testing.T
	st         *store.Store
	uid        int64
	root       string
	controller *httptest.Server
	fetcher    meta.Fetcher
	machines   []*syncContractMachine
}

type syncContractMachine struct {
	h                  *syncContractHarness
	name, home, config string
	id                 string
	token              string
	tokenID            int64
	endpoint           *httptest.Server
	link               *syncContractLink
	process            *exec.Cmd
	cancel             context.CancelFunc
	input              io.WriteCloser
	encode             *json.Encoder
	decode             *json.Decoder
	foreground         bool
	output             *os.File
}

type syncContractCycleResult struct{ Output, Error string }

type syncContractLink struct {
	offline            atomic.Bool
	dropNextResult     atomic.Bool
	droppedResults     atomic.Int32
	privateDown        atomic.Bool
	failedSyncRequests atomic.Int32
	mu                 sync.Mutex
	requests           map[string]int
}

func newSyncContractHarness(t *testing.T) *syncContractHarness {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := st.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken(syncContractToken)); err != nil {
		t.Fatal(err)
	}
	if err = st.ClaimPublishingUsername(uid, "contract-writer"); err != nil {
		t.Fatal(err)
	}
	// Keep the Unix control-socket path below its platform length limit.
	root, err := os.MkdirTemp("", "reet-sync-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	h := &syncContractHarness{t: t, st: st, uid: uid, root: root, fetcher: deadFetcher(t)}
	t.Cleanup(func() { _ = h.st.Close() })
	h.controller = httptest.NewServer(h.handler())
	t.Cleanup(func() { h.controller.Close() })
	t.Cleanup(h.writeEvidence)
	return h
}

// Legacy protocol fixtures deliberately simulate an older connector. Stage 3
// tests use newSyncContractHarness and exercise negotiated production defaults.
func newLegacySyncContractHarness(t *testing.T) *syncContractHarness {
	h := newSyncContractHarness(t)
	h.legacyProtocol = true
	return h
}

func (h *syncContractHarness) handler() http.Handler {
	return New(h.st, h.fetcher, "",
		WithAuthMode(AuthAccounts), WithNotify(&notify.Service{Store: h.st}),
		WithMaintenance(false), WithRefresh(false), WithTopics(false))
}

// enableManagedRemovals turns the internal rollout control on for this test
// controller only. Production controllers keep it off by default.
func (h *syncContractHarness) enableManagedRemovals() {
	h.removals = true
	h.st.SetManagedRemovals(true)
}

func (h *syncContractHarness) restartController() {
	h.t.Helper()
	address, dir := h.controller.Listener.Addr().String(), h.st.Dir()
	h.controller.Close()
	if err := h.st.Close(); err != nil {
		h.t.Fatal(err)
	}
	reopened, err := store.Open(dir)
	if err != nil {
		h.t.Fatal(err)
	}
	h.st = reopened
	h.st.SetManagedRemovals(h.removals)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		h.t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(h.handler())
	_ = server.Listener.Close()
	server.Listener = listener
	h.controller = server
	server.Start()
}

func (h *syncContractHarness) request(method, path string, body any) (int, string) {
	h.t.Helper()
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
	}
	return doClient(h.t, h.controller.Client(), h.controller, method, path,
		string(raw), "X-Reet-Token", syncContractToken)
}

func (h *syncContractHarness) mustRequest(method, path string, body any) string {
	h.t.Helper()
	code, response := h.request(method, path, body)
	if code < 200 || code >= 300 {
		h.t.Fatalf("%s %s: status %d: %s", method, path, code, response)
	}
	return response
}

func syncContractContent(name, text string) string {
	return "---\nname: " + syncContractName(name) + "\ndescription: Stage 0 fixture\n---\n" + text + "\n"
}

func syncContractName(name string) string {
	return "reet-" + strings.TrimPrefix(name, "reet-")
}

// Publishing in an isolated loopback fixture first gives an authored skill the
// existing shelf status field. Unpublishing preserves it. No public service is
// contacted, and the library still distributes owned private revisions.
func (h *syncContractHarness) hosted(name, text string, published, inUse bool) store.SyncSkill {
	h.t.Helper()
	name = syncContractName(name)
	content := syncContractContent(name, text)
	save := func(visible bool) {
		h.mustRequest(http.MethodPut, "/api/v2/hosting/skills/"+name, store.HostedSkill{
			Content: content, Published: visible,
			Files: map[string][]byte{"reference.txt": []byte("supporting fixture bytes\n")},
		})
	}
	save(published || inUse)
	item := h.candidate(name)
	if inUse {
		h.used(item.ID, true)
	}
	if inUse && !published {
		save(false)
	}
	return item
}

func (h *syncContractHarness) candidate(name string) store.SyncSkill {
	h.t.Helper()
	raw := h.mustRequest(http.MethodGet, "/api/v2/sync/candidates?collection=library", nil)
	var response struct{ Skills []store.SyncSkill }
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		h.t.Fatal(err)
	}
	for _, item := range response.Skills {
		if item.Slug == name {
			return item
		}
	}
	h.t.Fatalf("fixture skill %s is absent from candidates", name)
	return store.SyncSkill{}
}

func (h *syncContractHarness) used(id string, value bool) {
	h.t.Helper()
	h.mustRequest(http.MethodPatch, "/api/skill", map[string]any{"id": id, "used": value})
}

func (h *syncContractHarness) machine(name string) *syncContractMachine {
	h.t.Helper()
	home, err := os.MkdirTemp(h.root, "machine-")
	if err != nil {
		h.t.Fatal(err)
	}
	m := &syncContractMachine{h: h, name: name, home: home, token: "disposable-machine-token-" + name}
	credential, err := h.st.CreateAPIToken(h.uid, name, auth.HashToken(m.token))
	if err != nil {
		h.t.Fatal(err)
	}
	m.tokenID = credential.ID
	m.config = filepath.Join(m.home, ".config", "reet", "agent.json")
	m.link = &syncContractLink{requests: make(map[string]int)}
	target, err := url.Parse(h.controller.URL)
	if err != nil {
		h.t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	direct := proxy.Director
	proxy.Director = func(r *http.Request) {
		direct(r)
		// All machine-specific fault links represent the same controller.
		// Otherwise requestOrigin would manufacture different hosted identities.
		r.Host = target.Host
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxy.Transport = transport
	h.t.Cleanup(transport.CloseIdleConnections)
	proxy.ModifyResponse = func(r *http.Response) error {
		if r.Request.URL.Path == "/api/v2/sync/result" && r.StatusCode == 200 &&
			m.link.dropNextResult.CompareAndSwap(true, false) {
			m.link.droppedResults.Add(1)
			return errors.New("stage0 injected lost acknowledgement")
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "stage0 injected connection failure", http.StatusServiceUnavailable)
	}
	m.endpoint = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.link.mu.Lock()
		m.link.requests[r.Method+" "+r.URL.Path]++
		m.link.mu.Unlock()
		if m.link.offline.Load() {
			if strings.HasPrefix(r.URL.Path, "/api/v2/sync/") && r.URL.Path != "/api/v2/sync/private" {
				m.link.failedSyncRequests.Add(1)
			}
			http.Error(w, "stage0 offline machine link", http.StatusServiceUnavailable)
			return
		}
		if m.link.privateDown.Load() && r.URL.Path == "/api/v2/sync/private" {
			http.Error(w, "stage1 private monitoring unavailable", http.StatusServiceUnavailable)
			return
		}
		if h.legacyProtocol {
			r.Header.Del("X-Reet-Library-Protocol")
			if r.URL.Path == "/api/v2/library/protocol" {
				http.NotFound(w, r)
				return
			}
		}
		proxy.ServeHTTP(w, r)
	}))
	h.t.Cleanup(m.endpoint.Close)
	h.machines = append(h.machines, m)
	if output, err := m.command("enroll", "--server", m.endpoint.URL,
		"--token", m.token, "--env", name, "--config", m.config,
		"--allow-apply", "--skip-terminal-setup"); err != nil {
		h.t.Fatalf("enroll %s: %v\n%s", name, err, output)
	}
	var identity struct{ ID string }
	raw, err := os.ReadFile(m.config)
	if err != nil || json.Unmarshal(raw, &identity) != nil || identity.ID == "" {
		h.t.Fatalf("missing enrollment identity for %s: %v", name, err)
	}
	m.id = identity.ID
	// Keep test polls local and deterministic, independent of public releases.
	h.mustRequest(http.MethodPatch, "/api/v2/sync/agents",
		map[string]string{"id": m.id, "action": "auto-update-off"})
	m.start()
	h.t.Cleanup(m.stop)
	return m
}

func (m *syncContractMachine) connectorCommand(ctx context.Context, args ...string) *exec.Cmd {
	m.h.t.Helper()
	binary, err := os.Executable()
	if err != nil {
		m.h.t.Fatal(err)
	}
	commandArgs := append([]string{"-test.run=^TestSimpleSyncConnectorProcess$", "--"}, args...)
	cmd := exec.CommandContext(ctx, binary, commandArgs...)
	cmd.Dir = m.home
	// Give each connector its own real HOME/config. Remove ambient reet/app
	// selectors so the fixture can never scan the task's actual workspace.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME",
			"CLAUDE_CONFIG_DIR", "CODEX_HOME", "OPENCODE_CONFIG",
			"OPENCODE_CONFIG_DIR", "PI_CODING_AGENT_DIR":
			continue
		}
		if !strings.HasPrefix(key, "REET_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+m.home,
		"XDG_CONFIG_HOME="+filepath.Join(m.home, ".config"),
		"XDG_DATA_HOME="+filepath.Join(m.home, ".local", "share"),
		"XDG_STATE_HOME="+filepath.Join(m.home, ".local", "state"),
		"REET_SYNC_CONTRACT_CHILD=1",
		// Observe local deletions quickly; the production window is ten seconds.
		"REET_LIBRARY_RESTORE_DEBOUNCE=300ms")
	return cmd
}

func (m *syncContractMachine) command(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := m.connectorCommand(ctx, args...).CombinedOutput()
	return string(output), err
}

func (m *syncContractMachine) start() {
	m.h.t.Helper()
	if m.process != nil {
		m.h.t.Fatal("connector process already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := m.connectorCommand(ctx, "contract-loop", "--config", m.config)
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		m.h.t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = input.Close()
		m.h.t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		cancel()
		_ = input.Close()
		_ = output.Close()
		m.h.t.Fatal(err)
	}
	m.process, m.cancel, m.input = cmd, cancel, input
	m.encode, m.decode = json.NewEncoder(input), json.NewDecoder(output)
}

func (m *syncContractMachine) startForeground() {
	m.h.t.Helper()
	m.stop()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := m.connectorCommand(ctx, "run", "--foreground", "--config", m.config)
	output, err := os.CreateTemp(m.h.root, "foreground-*.log")
	if err != nil {
		cancel()
		m.h.t.Fatal(err)
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		_ = output.Close()
		m.h.t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		cancel()
		_ = input.Close()
		_ = output.Close()
		m.h.t.Fatal(err)
	}
	m.process, m.cancel, m.input = cmd, cancel, input
	m.foreground, m.output = true, output
}

func (m *syncContractMachine) stop() {
	if m.process != nil {
		cmd, cancel := m.process, m.cancel
		m.process = nil
		defer cancel()
		if m.foreground {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		_ = m.input.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				m.h.t.Errorf("connector %s did not exit cleanly: %v", m.name, err)
			}
		case <-time.After(2 * time.Second):
			cancel()
			<-done
			m.h.t.Errorf("connector %s did not stop after its input closed", m.name)
		}
		if m.output != nil {
			_ = m.output.Close()
			if m.h.t.Failed() {
				data, _ := os.ReadFile(m.output.Name())
				if len(data) > 6000 {
					data = data[len(data)-6000:]
				}
				m.h.t.Logf("foreground connector %s:\n%s", m.name, data)
			}
		}
		m.foreground, m.output = false, nil
	}
}

func (m *syncContractMachine) tick() (string, error) {
	if m.foreground {
		return "", errors.New("foreground connector owns its polling loop")
	}
	if err := m.encode.Encode(struct{}{}); err != nil {
		return "", err
	}
	type decodedCycle struct {
		result syncContractCycleResult
		err    error
	}
	done := make(chan decodedCycle, 1)
	go func() {
		var cycle syncContractCycleResult
		err := m.decode.Decode(&cycle)
		done <- decodedCycle{cycle, err}
	}()
	select {
	case cycle := <-done:
		if cycle.err != nil {
			return "", cycle.err
		}
		if cycle.result.Error != "" {
			return cycle.result.Output, errors.New(cycle.result.Error)
		}
		return cycle.result.Output, nil
	case <-time.After(20 * time.Second):
		m.stop()
		<-done
		return "", errors.New("connector cycle timed out")
	}
}

func (h *syncContractHarness) rounds(n int, machines ...*syncContractMachine) {
	h.t.Helper()
	for range n {
		for _, m := range machines {
			if output, err := m.tick(); err != nil {
				h.t.Fatalf("poll %s: %v\n%s", m.name, err, output)
			}
		}
	}
}

func (m *syncContractMachine) path(name, file string) string {
	return filepath.Join(m.home, ".agents", "skills", syncContractName(name), file)
}

func (m *syncContractMachine) localSkill(name, text string) {
	m.h.t.Helper()
	if err := os.MkdirAll(filepath.Dir(m.path(name, "SKILL.md")), 0755); err != nil {
		m.h.t.Fatal(err)
	}
	for file, body := range map[string]string{
		"SKILL.md":      syncContractContent(name, text),
		"reference.txt": "source-only reference\n",
	} {
		if err := os.WriteFile(m.path(name, file), []byte(body), 0644); err != nil {
			m.h.t.Fatal(err)
		}
	}
}

func (m *syncContractMachine) requireFile(name, file, want string) {
	m.h.t.Helper()
	got, err := os.ReadFile(m.path(name, file))
	if err != nil || string(got) != want {
		m.h.t.Fatalf("%s %s/%s: got %q, want %q: %v", m.name, name, file, got, want, err)
	}
}

// Only safe fixture metadata is exported: never enrollment keys, tokens,
// response headers, or arbitrary local file contents.
func (h *syncContractHarness) writeEvidence() {
	dir := os.Getenv("REET_SYNC_CONTRACT_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	rows := []map[string]any{}
	for _, m := range h.machines {
		m.link.mu.Lock()
		requests := make(map[string]int, len(m.link.requests))
		for path, count := range m.link.requests {
			requests[path] = count
		}
		m.link.mu.Unlock()
		files := map[string]string{}
		root := filepath.Join(m.home, ".agents", "skills")
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(relative)] = hex.EncodeToString(sum[:])
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			h.t.Errorf("evidence inventory: %v", err)
		}
		rows = append(rows, map[string]any{"machine": m.name, "id": m.id,
			"filesSha256": files, "requests": requests,
			"droppedAcknowledgements": m.link.droppedResults.Load()})
	}
	report := map[string]any{"test": h.t.Name(), "failed": h.t.Failed(), "machines": rows}
	targets, err := h.st.SyncTargetsFor(h.uid)
	if err != nil {
		h.t.Errorf("read target evidence: %v", err)
	}
	targetRows := []map[string]any{}
	for _, target := range targets {
		ids := func(items []store.SyncSkill) []string {
			out := []string{}
			for _, item := range items {
				out = append(out, item.ID)
			}
			return out
		}
		targetRows = append(targetRows, map[string]any{
			"host": target.Target.Host, "mode": target.Mode,
			"followSource": target.FollowSource, "sourceId": target.SourceID,
			"desired": ids(target.Desired), "missing": ids(target.Missing),
			"problem": target.Problem, "revision": target.Revision,
			"appliedRevision": target.AppliedRevision,
		})
	}
	report["targets"] = targetRows
	jobs, err := h.st.SyncJobsFor(h.uid)
	if err != nil {
		h.t.Errorf("read job evidence: %v", err)
	}
	jobRows := []map[string]any{}
	for _, job := range jobs {
		jobRows = append(jobRows, map[string]any{"agentId": job.AgentID,
			"kind": job.Kind, "status": job.Status, "stage": job.Stage, "revision": job.Revision})
	}
	report["jobs"] = jobRows
	imports, _, err := h.st.RemoteImportsFor(h.uid)
	if err != nil {
		h.t.Errorf("read import evidence: %v", err)
	}
	importRows := []map[string]any{}
	for _, request := range imports {
		importRows = append(importRows, map[string]any{"name": request.Name,
			"state": request.State, "error": request.Error})
	}
	report["imports"] = importRows
	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		err = os.MkdirAll(dir, 0700)
	}
	if err == nil {
		name := strings.ReplaceAll(h.t.Name(), "/", "_") + ".json"
		err = os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0600)
	}
	if err != nil {
		h.t.Errorf("write contract evidence: %v", err)
	}
}

func TestSimpleSyncConnectorProcess(t *testing.T) {
	if os.Getenv("REET_SYNC_CONTRACT_CHILD") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			args := os.Args[i+1:]
			if len(args) == 3 && args[0] == "contract-loop" && args[1] == "--config" {
				decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
				for {
					var request struct{}
					if err := decoder.Decode(&request); err != nil {
						if errors.Is(err, io.EOF) {
							os.Exit(0)
						}
						t.Fatal(err)
					}
					var output bytes.Buffer
					err := scan.RunAgentInput([]string{"run", "--once", "--config", args[2]},
						strings.NewReader(""), &output, "stage0")
					cycle := syncContractCycleResult{Output: output.String()}
					if err != nil {
						cycle.Error = err.Error()
					}
					if err := encoder.Encode(cycle); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := scan.RunAgentInput(args, strings.NewReader(""), os.Stdout, "stage0"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	t.Fatal("connector helper requires an explicit argument separator")
}

// A passing baseline for the harness, not an assertion that the current
// whole-library policy satisfies the new selected-only contract.
func TestSimpleSyncHarnessOfflineRecovery(t *testing.T) {
	h := newLegacySyncContractHarness(t)
	h.hosted("fixture", "first", false, true)
	a, b := h.machine("fixture-a"), h.machine("fixture-b")
	if a.id == b.id || a.home == b.home || a.config == b.config {
		t.Fatal("connectors did not receive separate enrollment/state")
	}
	h.rounds(6, a, b)
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("fixture", "SKILL.md", syncContractContent("fixture", "first"))
		m.requireFile("fixture", "reference.txt", "supporting fixture bytes\n")
	}
	b.link.offline.Store(true)
	if _, err := b.tick(); err == nil {
		t.Fatal("offline fault did not reach the connector")
	}
	h.hosted("fixture", "changed while B offline", false, true)
	h.rounds(4, a)
	a.requireFile("fixture", "SKILL.md", syncContractContent("fixture", "changed while B offline"))
	b.requireFile("fixture", "SKILL.md", syncContractContent("fixture", "first"))
	b.link.offline.Store(false)
	h.rounds(4, b)
	b.requireFile("fixture", "SKILL.md", syncContractContent("fixture", "changed while B offline"))
	agents, err := h.st.SyncAgentsFor(h.uid)
	if err != nil || len(agents) != 2 {
		t.Fatalf("reconnect changed enrollment count: %d: %v", len(agents), err)
	}
}

func TestSimpleSyncHarnessLostAcknowledgement(t *testing.T) {
	h := newLegacySyncContractHarness(t)
	h.hosted("receipt", "first", false, true)
	m := h.machine("receipt-machine")
	h.rounds(5, m)
	h.hosted("receipt", "second", false, true)
	m.link.dropNextResult.Store(true)
	for range 6 {
		if _, err := m.tick(); err != nil {
			break
		}
	}
	if m.link.droppedResults.Load() != 1 {
		t.Fatal("did not drop a controller-accepted acknowledgement")
	}
	if _, err := os.Stat(m.config + ".result"); err != nil {
		t.Fatalf("connector did not persist uncertain result: %v", err)
	}
	h.rounds(6, m)
	m.requireFile("receipt", "SKILL.md", syncContractContent("receipt", "second"))
	if _, err := os.Stat(m.config + ".result"); !os.IsNotExist(err) {
		t.Fatalf("result replay did not settle: %v", err)
	}
	jobs, err := h.st.SyncJobsFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	h.rounds(2, m)
	after, err := h.st.SyncJobsFor(h.uid)
	if err != nil || len(after) != len(jobs) {
		t.Fatalf("idle replay queued duplicate work: %d -> %d: %v", len(jobs), len(after), err)
	}
}

func TestSimpleSyncHarnessLocalContentImport(t *testing.T) {
	h := newLegacySyncContractHarness(t)
	a, b := h.machine("import-source"), h.machine("import-destination")
	name := "machine-only"
	a.localSkill(name, "original local instructions")
	h.rounds(12, a, b)
	b.requireFile(name, "SKILL.md", syncContractContent(name, "original local instructions"))
	b.requireFile(name, "reference.txt", "source-only reference\n")
	a.link.offline.Store(true)
	bundles, err := h.st.PrivateSkillBundlesFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	captured := false
	for _, bundle := range bundles {
		if bundle.Name != syncContractName(name) {
			continue
		}
		revision, err := h.st.PrivateSkillRevisionFor(h.uid, bundle.ID, bundle.Head)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range revision.Files {
			if file.Path == "reference.txt" && string(file.Data) == "source-only reference\n" {
				captured = true
			}
		}
	}
	if !captured {
		t.Fatal("controller has no complete private copy while source is offline")
	}
}
