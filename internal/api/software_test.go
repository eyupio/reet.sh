package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func softwareTestCapabilities(entry terminalproto.SoftwareEntry) terminalproto.Capabilities {
	return terminalproto.Capabilities{
		ProtocolVersion: terminalproto.Version, OS: "linux", OSUser: "tester",
		TerminalEnabled: true, TerminalControl: true, HealthEnabled: true, Readiness: terminalproto.Ready,
		Software: []terminalproto.SoftwareEntry{entry},
	}
}

func softwareTestEntry(state terminalproto.SoftwareState, version string) terminalproto.SoftwareEntry {
	entry := terminalproto.SoftwareEntry{
		Tool: "opencode", State: state, Platform: "linux", Arch: "amd64", Package: "opencode-ai", CheckedAt: 1_700_000_000_000,
	}
	switch state {
	case terminalproto.SoftwareInstalled:
		entry.Version, entry.Manager, entry.Path = version, "node_package", "/usr/local/bin/opencode"
		entry.Source, entry.Evidence = "package.json", "package_bin_matches_executable"
		entry.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanUpdate, terminalproto.SoftwarePlanRemove}
	case terminalproto.SoftwareMissing:
		entry.Reason, entry.Source = terminalproto.SoftwareNotInstalled, "npm"
		entry.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanInstall}
	}
	return entry
}

func TestMachineSoftwarePlanFromStoredEvidence(t *testing.T) {
	srv, st, a, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	caps := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.18.0"))
	gen, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err := st.ReserveTerminalCreateFor(uid, a.ID, terminalbroker.ID(), gen.Generation, terminalproto.Create{Title: "Agent run", Cols: 80, Rows: 24, Tool: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ObserveTerminalStateFor(uid, a.ID, sess.ID, gen.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v2/machines/"+a.ID+"/software/plan?tool=opencode&action=update", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", w.Code, w.Body.String())
	}
	var plan struct {
		Host      string   `json:"host"`
		Tool      string   `json:"tool"`
		Action    string   `json:"action"`
		State     string   `json:"state"`
		Current   string   `json:"current"`
		Target    string   `json:"target"`
		Command   string   `json:"command"`
		Effects   []string `json:"effects"`
		Executes  bool     `json:"executes"`
		CheckedAt int64    `json:"checkedAt"`
		Sentence  string   `json:"sentence"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Host != a.Host || plan.Tool != "opencode" || plan.Action != "update" || plan.State != "ready" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Current != "1.18.0" || plan.Target != "1.19.5" || plan.Command != "npm install -g opencode-ai@1.19.5" || plan.Executes {
		t.Fatalf("unexpected plan body: %+v", plan)
	}
	if len(plan.Effects) != 1 || plan.Effects[0] != "1 OpenCode session is running on laptop; finish or close it before you update." {
		t.Fatalf("unexpected effects: %+v", plan.Effects)
	}
	if plan.CheckedAt != 1_700_000_000_000 || plan.Sentence == "" {
		t.Fatalf("checked time or sentence wrong: %+v", plan)
	}
}

func TestMachineSoftwarePlanRefusals(t *testing.T) {
	srv, st, a, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	call := func(query string) (int, string, string) {
		r := httptest.NewRequest(http.MethodGet, "/api/v2/machines/"+a.ID+"/software/plan?"+query, nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body.Code, w.Body.String()
	}

	code, reason, _ := call("tool=opencode&action=update")
	if code != http.StatusConflict || reason != "software_unsupported" {
		t.Fatalf("without capabilities: %d %s", code, reason)
	}

	caps := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareMissing, ""))
	if _, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), caps); err != nil {
		t.Fatal(err)
	}
	code, reason, _ = call("tool=opencode&action=update")
	if code != http.StatusConflict || reason != "software_not_installed" {
		t.Fatalf("update when missing: %d %s", code, reason)
	}
	code, _, body := call("tool=opencode&action=install")
	if code != http.StatusOK {
		t.Fatalf("install when missing: %d %s", code, body)
	}
	code, reason, _ = call("tool=opencode&action=remove")
	if code != http.StatusConflict || reason != "software_not_installed" {
		t.Fatalf("remove when missing: %d %s", code, reason)
	}
	code, reason, _ = call("tool=opencode&action=run")
	if code != http.StatusBadRequest || reason != "software_action_invalid" {
		t.Fatalf("unknown action: %d %s", code, reason)
	}
	caps = softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.19.5"))
	if _, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), caps); err != nil {
		t.Fatal(err)
	}
	code, reason, _ = call("tool=opencode&action=install")
	if code != http.StatusConflict || reason != "software_action_mismatch" {
		t.Fatalf("install when installed: %d %s", code, reason)
	}
	code, reason, _ = call("tool=claude&action=update")
	if code != http.StatusConflict || reason != "software_unsupported" {
		t.Fatalf("known tool without evidence: %d %s", code, reason)
	}
	code, reason, _ = call("tool=vim&action=update")
	if code != http.StatusBadRequest || reason != "software_tool_invalid" {
		t.Fatalf("unknown tool: %d %s", code, reason)
	}

	other := httptest.NewRequest(http.MethodGet, "/api/v2/machines/missing/software/plan?action=update", nil)
	other.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, other)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown machine: %d %s", w.Code, w.Body.String())
	}
}
