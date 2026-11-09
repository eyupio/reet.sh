package api

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/scan"
)

// Promoted from the Stage 0 opt-in contract now that automatic library captures
// can recover without accepting stale process-bound previews or uploads.
func TestSimpleSyncContractImportRecoversAfterConnectorRestart(t *testing.T) {
	h := newLegacySyncContractHarness(t)
	a, b := h.machine("restart-source"), h.machine("restart-destination")
	a.localSkill("restart-import", "must survive a connector restart")
	previewed := false
	for range 8 {
		h.rounds(1, a, b)
		a.link.mu.Lock()
		previewed = a.link.requests["POST /api/v2/sync/skill-import/preview"] > 0
		a.link.mu.Unlock()
		if previewed {
			break
		}
	}
	if !previewed {
		t.Fatal("fixture did not reach a real machine-content preview")
	}
	a.stop()
	a.start()
	h.rounds(12, a, b)
	b.requireFile("restart-import", "SKILL.md", syncContractContent("restart-import", "must survive a connector restart"))
	b.requireFile("restart-import", "reference.txt", "source-only reference\n")
	agents, err := h.st.SyncAgentsFor(h.uid)
	if err != nil || len(agents) != 2 {
		t.Fatalf("restart changed enrollment count: %d: %v", len(agents), err)
	}
}

func TestSimpleSyncRecoveryControllerAndConnectorRestart(t *testing.T) {
	h := newLegacySyncContractHarness(t)
	skill := h.hosted("durable", "before restart", false, true)
	a, b := h.machine("durable-a"), h.machine("durable-b")
	h.rounds(5, a, b)
	before := map[string][]byte{}
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("durable", "SKILL.md", syncContractContent("durable", "before restart"))
		data, err := os.ReadFile(m.config)
		if err != nil {
			t.Fatal(err)
		}
		before[m.id] = data
	}
	h.hosted("durable", "after controller and connector restart", false, true)
	a.link.dropNextResult.Store(true)
	for range 6 {
		if _, err := a.tick(); err != nil {
			break
		}
	}
	if a.link.droppedResults.Load() != 1 {
		t.Fatal("fixture did not leave an uncertain result before controller restart")
	}
	if _, err := os.Stat(a.config + ".result"); err != nil {
		t.Fatalf("pending result was not durable before controller restart: %v", err)
	}
	h.restartController()
	a.stop()
	a.start()
	h.rounds(5, a, b)
	if _, err := os.Stat(a.config + ".result"); !os.IsNotExist(err) {
		t.Fatalf("controller restart did not reconcile the durable acknowledgement: %v", err)
	}
	agents, err := h.st.SyncAgentsFor(h.uid)
	if err != nil || len(agents) != 2 {
		t.Fatalf("controller restart lost enrollments: %d: %v", len(agents), err)
	}
	for _, agent := range agents {
		if _, found := before[agent.ID]; !found {
			t.Fatal("controller restart replaced an enrollment")
		}
	}
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("durable", "SKILL.md", syncContractContent("durable", "after controller and connector restart"))
		data, err := os.ReadFile(m.config)
		if err != nil || !bytes.Equal(data, before[m.id]) {
			t.Fatalf("recovery changed private enrollment config for %s: %v", m.name, err)
		}
		m.link.mu.Lock()
		enrollments := m.link.requests["POST /api/v2/sync/agents"]
		m.link.mu.Unlock()
		if enrollments != 1 {
			t.Fatalf("%s enrolled again during recovery", m.name)
		}
	}
	targets, err := h.st.SyncTargetsFor(h.uid)
	if err != nil || len(targets) != 2 {
		t.Fatalf("controller restart lost saved targets: %d: %v", len(targets), err)
	}
	for _, target := range targets {
		if len(target.Desired) != 1 || target.Desired[0].ID != skill.ID || len(target.Missing) != 0 {
			t.Fatalf("saved intent did not recover: %+v", target)
		}
	}
}

func TestSimpleSyncRecoveryRevokedCredentialPreservesOtherMachine(t *testing.T) {
	h := newLegacySyncContractHarness(t)
	h.hosted("credential", "before revocation", false, true)
	a, b := h.machine("revoked-a"), h.machine("healthy-b")
	h.rounds(5, a, b)
	before, err := os.ReadFile(a.config)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.st.RevokeAPIToken(h.uid, a.tokenID); err != nil {
		t.Fatal(err)
	}
	h.hosted("credential", "after revocation", false, true)
	if _, err = a.tick(); err == nil {
		t.Fatal("revoked credential retained controller access")
	}
	h.rounds(4, b)
	a.requireFile("credential", "SKILL.md", syncContractContent("credential", "before revocation"))
	b.requireFile("credential", "SKILL.md", syncContractContent("credential", "after revocation"))
	after, err := os.ReadFile(a.config)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("authentication failure removed or changed local enrollment: %v", err)
	}
	agents, err := h.st.SyncAgentsFor(h.uid)
	if err != nil || len(agents) != 2 {
		t.Fatalf("token revocation deleted enrollment records: %d: %v", len(agents), err)
	}
	targets, err := h.st.SyncTargetsFor(h.uid)
	if err != nil || len(targets) != 2 {
		t.Fatalf("token revocation deleted selections: %d: %v", len(targets), err)
	}
}

func syncRecoveryEventually(t *testing.T, timeout time.Duration, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal(description)
}

func TestSimpleSyncRecoveryForegroundSurvivesUnavailablePrivateConnection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Stage 1 foreground service contract targets Linux")
	}
	h := newLegacySyncContractHarness(t)
	h.hosted("foreground", "private monitoring is unavailable", false, true)
	m := h.machine("foreground")
	identity, err := os.ReadFile(m.config)
	if err != nil {
		t.Fatal(err)
	}
	m.link.privateDown.Store(true)
	m.startForeground()
	syncRecoveryEventually(t, 25*time.Second, "foreground connector did not sync with private bootstrap unavailable", func() bool {
		got, err := os.ReadFile(m.path("foreground", "SKILL.md"))
		return err == nil && string(got) == syncContractContent("foreground", "private monitoring is unavailable")
	})
	m.link.mu.Lock()
	privateAttempts := m.link.requests["POST /api/v2/sync/private"]
	m.link.mu.Unlock()
	if privateAttempts == 0 {
		t.Fatal("foreground test never exercised private monitoring")
	}
	status, err := scan.ReadAgentStatus(m.config)
	if err != nil || !status.Running || !status.LocalControl || status.Connected || status.LastCycle.IsZero() {
		t.Fatalf("local status conflated private and sync health: %+v: %v", status, err)
	}
	m.link.offline.Store(true)
	h.hosted("foreground", "queued during total outage", false, true)
	syncRecoveryEventually(t, 10*time.Second, "foreground connector did not encounter the full outage", func() bool {
		return m.link.failedSyncRequests.Load() > 0
	})
	m.link.offline.Store(false)
	syncRecoveryEventually(t, 25*time.Second, "foreground connector did not resume pending sync", func() bool {
		got, err := os.ReadFile(m.path("foreground", "SKILL.md"))
		return err == nil && string(got) == syncContractContent("foreground", "queued during total outage")
	})
	current, err := os.ReadFile(m.config)
	if err != nil || !bytes.Equal(identity, current) {
		t.Fatalf("connection failure changed the enrollment: %v", err)
	}
	var saved struct{ ID string }
	if err := json.Unmarshal(current, &saved); err != nil || saved.ID != m.id {
		t.Fatal("foreground reconnect replaced the enrolled machine", err)
	}
	m.stop()
	status, err = scan.ReadAgentStatus(m.config)
	if err != nil || status.Running {
		t.Fatalf("foreground connector did not stop cleanly: %+v: %v", status, err)
	}
}
