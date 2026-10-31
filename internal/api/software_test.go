package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
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
		entry.Version, entry.Manager = version, "reet"
		entry.Path = "/home/me/.reet/software/opencode/versions/" + version + "/opencode"
		entry.Source, entry.Evidence, entry.Managed = "verified_tarball", "verified_sha256", true
		entry.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanUpdate, terminalproto.SoftwarePlanRemove}
	case terminalproto.SoftwareMissing:
		entry.Reason, entry.Source = terminalproto.SoftwareNotInstalled, "npm"
		entry.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanInstall}
	}
	return entry
}

func softwareTestSystemEntry(version string) terminalproto.SoftwareEntry {
	e := softwareTestEntry(terminalproto.SoftwareInstalled, version)
	e.Managed, e.Manager, e.Source = false, "node_package", "package.json"
	e.Path, e.Evidence = "/usr/local/bin/opencode", "package_bin_matches_executable"
	e.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanInstall}
	return e
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
		Source    string   `json:"source"`
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
	if plan.Current != "1.18.0" || plan.Target != "1.19.5" || plan.Command != "" || plan.Source == "" || plan.Executes {
		t.Fatalf("unexpected plan body: %+v", plan)
	}
	if len(plan.Effects) != 1 || plan.Effects[0] != "1 OpenCode session is running on laptop; reet refuses to update while it runs." {
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
		t.Fatalf("install on a reet-managed copy: %d %s", code, reason)
	}
	caps = softwareTestCapabilities(softwareTestSystemEntry("1.18.0"))
	if _, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), caps); err != nil {
		t.Fatal(err)
	}
	code, _, body = call("tool=opencode&action=install")
	if code != http.StatusOK || !strings.Contains(body, "alongside") {
		t.Fatalf("install alongside a system copy: %d %s", code, body)
	}
	code, reason, _ = call("tool=opencode&action=update")
	if code != http.StatusConflict || reason != "software_action_mismatch" {
		t.Fatalf("update a system copy: %d %s", code, reason)
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

type softwareApplyAPI struct {
	srv    *Server
	st     *store.Store
	uid    int64
	agent  string
	cookie string
	asked  []terminalproto.SoftwareApply
	answer terminalproto.SoftwareApplyResult
	peer   *terminalbroker.Peer
}

func softwareApplyAPIFixture(t *testing.T, caps terminalproto.Capabilities) *softwareApplyAPI {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	if _, err := st.CreateAPIToken(uid, "owner", auth.HashToken("fixture-owner")); err != nil {
		t.Fatal(err)
	}
	srv := New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts), WithRemoteTerminals(true), WithPublicURL("https://reet.example"))
	t.Cleanup(func() { srv.Close() })
	f := &softwareApplyAPI{srv: srv, st: st, uid: uid, cookie: terminalLogin(t, st, uid)}
	agent, _, err := st.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	var peer *terminalbroker.Peer
	peer, err = srv.s.terminals.Connect(uid, agent.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.SoftwareApply:
			f.asked = append(f.asked, m.Body.(terminalproto.SoftwareApply))
			reply.Body = f.answer
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		data, err := terminalproto.Encode(reply)
		if err != nil {
			return err
		}
		decoded, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		return peer.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := peer.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.peer, f.agent = peer, agent.ID
	return f
}

func (f *softwareApplyAPI) post(t *testing.T, body string) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v2/machines/"+f.agent+"/software/apply", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://reet.example")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, w.Body.String()
}

func TestMachineSoftwareApplyInstallsAndUpdatesEvidence(t *testing.T) {
	caps := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.18.0"))
	caps.SoftwareApply = true
	caps.Launch = true
	caps.Tools = []string{"claude"}
	f := softwareApplyAPIFixture(t, caps)
	entry := softwareTestEntry(terminalproto.SoftwareInstalled, "1.19.5")
	f.answer = terminalproto.SoftwareApplyResult{
		Tool: "opencode", Action: terminalproto.SoftwareActionUpdate, State: terminalproto.SoftwareApplyApplied,
		Version: "1.19.5", Previous: "1.18.0", Entry: &entry,
	}
	code, out, raw := f.post(t, `{"tool":"opencode","action":"update"}`)
	if code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, raw)
	}
	result, _ := out["result"].(map[string]any)
	if result["state"] != "applied" || result["version"] != "1.19.5" || len(f.asked) != 1 {
		t.Fatalf("unexpected result or request: %v %+v", out, f.asked)
	}
	if f.asked[0].ExpectedState != terminalproto.SoftwareInstalled || f.asked[0].ExpectedVersion != "1.18.0" || !f.asked[0].ExpectedManaged {
		t.Fatalf("request did not carry the approved evidence: %+v", f.asked[0])
	}
	stored, err := f.st.RemoteCapabilitiesFor(f.uid, f.agent)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Capabilities.Software) != 1 || stored.Capabilities.Software[0].Version != "1.19.5" || !stored.Capabilities.Software[0].Managed {
		t.Fatalf("stored evidence was not replaced: %+v", stored.Capabilities.Software)
	}
	if !slices.Contains(stored.Capabilities.Tools, "opencode") {
		t.Fatalf("the installed tool was not added to the launch inventory: %+v", stored.Capabilities.Tools)
	}
}

func TestMachineSoftwareApplyRefusesWhileASessionRuns(t *testing.T) {
	caps := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.18.0"))
	caps.SoftwareApply = true
	f := softwareApplyAPIFixture(t, caps)
	record, err := f.st.RemoteCapabilitiesFor(f.uid, f.agent)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := f.st.ReserveTerminalCreateFor(f.uid, f.agent, terminalbroker.ID(), record.Generation, terminalproto.Create{Title: "Agent run", Cols: 80, Rows: 24, Tool: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.ObserveTerminalStateFor(f.uid, f.agent, session.ID, record.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.post(t, `{"tool":"opencode","action":"update"}`)
	if code != http.StatusConflict || out["code"] != "software_sessions_running" {
		t.Fatalf("session refusal: %d %s", code, raw)
	}
	if len(f.asked) != 0 {
		t.Fatal("the controller sent an apply while a session was running")
	}
}

func TestMachineSoftwareApplyRefusals(t *testing.T) {
	caps := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.18.0"))
	caps.SoftwareApply = true
	f := softwareApplyAPIFixture(t, caps)
	f.answer = terminalproto.SoftwareApplyResult{
		Tool: "opencode", Action: terminalproto.SoftwareActionUpdate, State: terminalproto.SoftwareApplyRefused,
		Reason: terminalproto.SoftwareApplyChanged,
	}
	code, out, raw := f.post(t, `{"tool":"opencode","action":"update"}`)
	if code != http.StatusConflict || out["code"] != "software_changed" {
		t.Fatalf("changed refusal: %d %s", code, raw)
	}

	code, out, raw = f.post(t, `{"tool":"opencode","action":"run"}`)
	if code != http.StatusBadRequest || out["code"] != "software_action_invalid" {
		t.Fatalf("unknown action refusal: %d %s", code, raw)
	}

	withoutApply := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.18.0"))
	withoutApply.SoftwareApply = false
	g := softwareApplyAPIFixture(t, withoutApply)
	code, out, raw = g.post(t, `{"tool":"opencode","action":"update"}`)
	if code != http.StatusConflict || out["code"] != "software_apply_unsupported" {
		t.Fatalf("capability refusal: %d %s", code, raw)
	}
}

func TestMachineSoftwarePlanRollbackAndRemove(t *testing.T) {
	srv, st, a, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	entry := softwareTestEntry(terminalproto.SoftwareInstalled, "1.19.5")
	entry.Previous = "1.18.0"
	caps := softwareTestCapabilities(entry)
	if _, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), caps); err != nil {
		t.Fatal(err)
	}
	get := func(query string) (int, map[string]any, string) {
		r := httptest.NewRequest(http.MethodGet, "/api/v2/machines/"+a.ID+"/software/plan?"+query, nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		out := map[string]any{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out, w.Body.String()
	}
	code, out, raw := get("tool=opencode&action=rollback")
	if code != http.StatusOK || out["target"] != "1.18.0" || out["action"] != "rollback" {
		t.Fatalf("rollback plan: %d %s", code, raw)
	}
	code, out, raw = get("tool=opencode&action=remove")
	if code != http.StatusOK || out["action"] != "remove" || out["target"] != "" {
		t.Fatalf("remove plan: %d %s", code, raw)
	}

	withoutPrevious := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.19.5"))
	if _, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), withoutPrevious); err != nil {
		t.Fatal(err)
	}
	code, out, raw = get("tool=opencode&action=rollback")
	if code != http.StatusConflict || out["code"] != "software_no_rollback" {
		t.Fatalf("rollback without a previous version: %d %s", code, raw)
	}
}

func TestMachineSoftwareApplyRollsBackAndRemoves(t *testing.T) {
	caps := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.19.5"))
	caps.Software[0].Previous = "1.18.0"
	caps.SoftwareApply = true
	f := softwareApplyAPIFixture(t, caps)

	rolledEntry := softwareTestEntry(terminalproto.SoftwareInstalled, "1.18.0")
	rolledEntry.Previous = "1.19.5"
	f.answer = terminalproto.SoftwareApplyResult{
		Tool: "opencode", Action: terminalproto.SoftwareActionRollback, State: terminalproto.SoftwareApplyApplied,
		Version: "1.18.0", Previous: "1.19.5", Entry: &rolledEntry,
	}
	code, out, raw := f.post(t, `{"tool":"opencode","action":"rollback"}`)
	if code != http.StatusOK || out["result"].(map[string]any)["state"] != "applied" {
		t.Fatalf("rollback apply: %d %s", code, raw)
	}
	if len(f.asked) != 1 || f.asked[0].ExpectedPrevious != "1.18.0" || f.asked[0].ExpectedVersion != "1.19.5" {
		t.Fatalf("rollback request evidence: %+v", f.asked)
	}
	stored, err := f.st.RemoteCapabilitiesFor(f.uid, f.agent)
	if err != nil || len(stored.Capabilities.Software) != 1 || stored.Capabilities.Software[0].Version != "1.18.0" {
		t.Fatalf("stored rollback evidence: %+v %v", stored.Capabilities.Software, err)
	}

	missingEntry := softwareTestEntry(terminalproto.SoftwareMissing, "")
	f.answer = terminalproto.SoftwareApplyResult{
		Tool: "opencode", Action: terminalproto.SoftwareActionRemove, State: terminalproto.SoftwareApplyApplied,
		Entry: &missingEntry,
	}
	code, out, raw = f.post(t, `{"tool":"opencode","action":"remove"}`)
	if code != http.StatusOK || out["result"].(map[string]any)["state"] != "applied" {
		t.Fatalf("remove apply: %d %s", code, raw)
	}
	stored, err = f.st.RemoteCapabilitiesFor(f.uid, f.agent)
	if err != nil || len(stored.Capabilities.Software) != 1 || stored.Capabilities.Software[0].State != terminalproto.SoftwareMissing {
		t.Fatalf("stored remove evidence: %+v %v", stored.Capabilities.Software, err)
	}
}

func TestFleetSoftwarePlansReadStoredEvidence(t *testing.T) {
	srv, st, a, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)

	updateCaps := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.18.0"))
	updateCaps.SoftwareApply = true
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), updateCaps)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := st.ReserveTerminalCreateFor(uid, a.ID, terminalbroker.ID(), record.Generation, terminalproto.Create{Title: "Agent run", Cols: 80, Rows: 24, Tool: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ObserveTerminalStateFor(uid, a.ID, session.ID, record.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}

	noneEntry := softwareTestEntry(terminalproto.SoftwareInstalled, "1.19.5")
	noneEntry.Previous = "1.18.0"
	add := func(host string, caps terminalproto.Capabilities) {
		agent, _, err := st.EnrollSyncAgent(uid, host, "test")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), caps); err != nil {
			t.Fatal(err)
		}
	}
	add("current", softwareTestCapabilities(noneEntry))
	add("bare", softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareMissing, "")))
	add("system", softwareTestCapabilities(softwareTestSystemEntry("1.18.0")))
	add("older", terminalproto.Capabilities{ProtocolVersion: terminalproto.Version, OS: "linux", OSUser: "me", TerminalEnabled: true, TerminalControl: true, HealthEnabled: true, Readiness: terminalproto.Ready})

	r := httptest.NewRequest(http.MethodGet, "/api/v2/software/plans", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("plans: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Machines []fleetSoftwareRow `json:"machines"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Machines) != 5 {
		t.Fatalf("machines = %+v", out.Machines)
	}
	byHost := map[string]fleetSoftwareRow{}
	for _, row := range out.Machines {
		byHost[row.Host] = row
	}
	if row := byHost["laptop"]; row.State != "update" || row.Version != "1.18.0" || row.SessionsRunning != 1 || !row.CanApply || row.Target != "1.19.5" {
		t.Fatalf("laptop row = %+v", row)
	}
	if row := byHost["current"]; row.State != "none" || !strings.Contains(row.Sentence, "rollback") {
		t.Fatalf("current row = %+v", row)
	}
	if row := byHost["bare"]; row.State != "install" || row.Target != "1.19.5" {
		t.Fatalf("bare row = %+v", row)
	}
	if row := byHost["system"]; row.State != "install" || !strings.Contains(row.Sentence, "alongside") {
		t.Fatalf("system row = %+v", row)
	}
	if row := byHost["older"]; row.State != "unknown" || !strings.Contains(row.Sentence, "Update reet") {
		t.Fatalf("older row = %+v", row)
	}
}
