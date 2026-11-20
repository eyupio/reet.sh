package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The effort chosen for an outcome stage is carried through the controller's
// launch path into the structured create the machine receives, even when the
// agent's descriptor names no thought levels.
func TestOutcomeStartCarriesTheSelectedEffortToTheMachine(t *testing.T) {
	caps := chatCaps()
	caps.AgentsV2, caps.ACPLiveControls, caps.ACPStartOptions = true, true, true
	caps.Tools = []string{"opencode"}
	entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5"}}
	caps.ACPAgents = &entries
	f := launchAPIFixture(t, caps)
	_, _, err := outcomeStarter{f.srv.s}.StartOutcomeSession(t.Context(), f.uid, gaffer.OutcomeLaunch{
		AgentID: f.agent.ID, RequestID: terminalbroker.ID(), Title: "Export API", Tool: "opencode",
		Effort: "medium", Dir: "/tmp", Trust: true, Prompt: "Add the CSV export endpoint with tests.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent := f.sent(); len(sent) != 1 || !sent[0].Structured || sent[0].Effort != "medium" || !sent[0].Trust {
		t.Fatalf("sent: %+v", sent)
	}
}

// Every image the outcome holds reaches the machine on the opening prompt,
// not only the first; a path that needs quoting keeps its own reference, and
// a start with no images sends exactly the prompt it was given.
func TestOutcomeStartCarriesEveryImageToTheMachine(t *testing.T) {
	caps := chatCaps()
	caps.AgentsV2, caps.ACPLiveControls, caps.ACPStartOptions = true, true, true
	caps.Tools = []string{"opencode"}
	entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5"}}
	caps.ACPAgents = &entries
	f := launchAPIFixture(t, caps)
	_, _, err := outcomeStarter{f.srv.s}.StartOutcomeSession(t.Context(), f.uid, gaffer.OutcomeLaunch{
		AgentID: f.agent.ID, RequestID: terminalbroker.ID(), Title: "Export API", Tool: "opencode",
		Dir: "/tmp", Trust: true, Prompt: "Add the CSV export endpoint with tests.",
		Images: []string{"/home/dev/launch-uploads/one.png", "/home/dev/launch-uploads/two.webp", "/home/dev/launch-uploads/a b.png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Add the CSV export endpoint with tests.\n\nImages: /home/dev/launch-uploads/one.png /home/dev/launch-uploads/two.webp '/home/dev/launch-uploads/a b.png'"
	if sent := f.sent(); len(sent) != 1 || sent[0].Prompt != want {
		t.Fatalf("sent: %+v", sent)
	}
}

func TestOutcomePromptImagesLeavesAPromptWithNoImagesAlone(t *testing.T) {
	for _, prompt := range []string{"Do the work.", "", "Do it.\n"} {
		if got := outcomePromptImages(prompt, nil); got != prompt {
			t.Fatalf("no images changed %q to %q", prompt, got)
		}
	}
	prompt := "Do the work.\n\nImages: /home/dev/launch-uploads/one.png /home/dev/launch-uploads/two.webp"
	if got := outcomePromptImages(prompt, []string{"/home/dev/launch-uploads/one.png", "/home/dev/launch-uploads/two.webp"}); got != prompt {
		t.Fatalf("an approval prompt was doubled: %q", got)
	}
}

// A machine that refuses a start because the agent did not confirm the chosen
// effort says so to the Factory as that refusal, with the machine's sentence
// for the person; a refusal for the model is an ordinary one. A start asked
// for afresh, without the effort, on a request of its own starts the session;
// the refused request stays refused and starts nothing.
func TestOutcomeStartSaysWhenTheAgentDidNotConfirmTheEffort(t *testing.T) {
	caps := chatCaps()
	caps.AgentsV2, caps.ACPLiveControls, caps.ACPStartOptions = true, true, true
	caps.Tools = []string{"opencode"}
	entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5"}}
	caps.ACPAgents = &entries
	f := launchAPIFixture(t, caps)
	f.mu.Lock()
	f.reply = func(op terminalproto.Create) terminalproto.Body {
		switch {
		case op.Effort != "" || op.Model == "replayed/model":
			return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: terminalproto.ACPEffortUnconfirmedDetail}
		case op.Model == "shortened/model":
			return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The agent did not confirm the chosen effort for this model."}
		case op.Model == "wrong/model":
			return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The agent did not use the chosen model, so the chat was not started."}
		}
		return terminalproto.SessionChanged{State: terminalproto.Running}
	}
	f.mu.Unlock()
	starter := outcomeStarter{f.srv.s}
	launch := gaffer.OutcomeLaunch{AgentID: f.agent.ID, RequestID: terminalbroker.ID(), Title: "Export API", Tool: "opencode", Model: "asked/model", Effort: "high", Dir: "/tmp", Trust: true, Prompt: "Add the CSV export endpoint with tests."}
	_, _, err := starter.StartOutcomeSession(t.Context(), f.uid, launch)
	if !errors.Is(err, gaffer.ErrEffortNotOffered) || errors.Is(err, gaffer.ErrLaunchUnknown) || err.Error() != terminalproto.ACPEffortUnconfirmedDetail {
		t.Fatalf("effort refusal: %v", err)
	}
	// The refused request is answered from its record: nothing is started for it.
	if _, _, err = starter.StartOutcomeSession(t.Context(), f.uid, launch); err == nil {
		t.Fatal("a refused request started a session when asked again")
	}
	again := launch
	again.RequestID, again.Effort = terminalbroker.ID(), ""
	session, _, err := starter.StartOutcomeSession(t.Context(), f.uid, again)
	if err != nil || session == "" {
		t.Fatalf("without the effort: %q %v", session, err)
	}
	if sent := f.sent(); len(sent) != 2 || sent[0].Effort != "high" || sent[1].Effort != "" || sent[1].Model != "asked/model" {
		t.Fatalf("sent: %+v", sent)
	}
	wrong := again
	wrong.RequestID, wrong.Model = terminalbroker.ID(), "wrong/model"
	if _, _, err = starter.StartOutcomeSession(t.Context(), f.uid, wrong); err == nil || errors.Is(err, gaffer.ErrEffortNotOffered) || !strings.Contains(err.Error(), "chosen model") {
		t.Fatalf("model refusal: %v", err)
	}
	// The same refusal for a launch that carried no effort, as a refusal
	// recorded for an earlier request is answered again, is still that refusal.
	replayed := again
	replayed.RequestID, replayed.Model = terminalbroker.ID(), "replayed/model"
	if _, _, err = starter.StartOutcomeSession(t.Context(), f.uid, replayed); !errors.Is(err, gaffer.ErrEffortNotOffered) {
		t.Fatalf("effort refusal without an effort: %v", err)
	}
	// Any other sentence under the same code is an ordinary refusal: the
	// Factory stops the stage on it and does not ask again. (This is where a
	// refusal for another reason is told apart; the Factory sees only the
	// typed error.)
	shortened := launch
	shortened.RequestID, shortened.Model = terminalbroker.ID(), "shortened/model"
	shortened.Effort = ""
	if _, _, err = starter.StartOutcomeSession(t.Context(), f.uid, shortened); err == nil || errors.Is(err, gaffer.ErrEffortNotOffered) {
		t.Fatalf("another sentence: %v", err)
	}
}

// What a session is on is read back from the machine, not from what its
// launch asked for: the model and effort the agent reports now.
func TestOutcomeSessionChoiceIsReadBackFromTheMachine(t *testing.T) {
	caps := chatCaps()
	caps.AgentsV2, caps.ACPLiveControls, caps.ACPStartOptions = true, true, true
	caps.Tools = []string{"opencode"}
	entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5"}}
	caps.ACPAgents = &entries
	f := launchAPIFixture(t, caps)
	starter := outcomeStarter{f.srv.s}
	session, _, err := starter.StartOutcomeSession(t.Context(), f.uid, gaffer.OutcomeLaunch{
		AgentID: f.agent.ID, RequestID: terminalbroker.ID(), Title: "Export API", Tool: "opencode",
		Model: "asked/model", Effort: "high", Dir: "/tmp", Trust: true, Prompt: "Add the CSV export endpoint with tests.",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.controls = func(*terminalproto.ACPControl) terminalproto.ACPControlResult {
		return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Model: "actual/model", Options: []terminalproto.ACPSetting{{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Current: "medium", Values: []string{"low", "medium", "high"}}}}}
	}
	f.mu.Unlock()
	got, err := starter.OutcomeSessionChoice(t.Context(), f.uid, session)
	if err != nil || got.Tool != "opencode" || got.Model != "actual/model" || got.Effort != "medium" {
		t.Fatalf("read back: %+v %v", got, err)
	}
	// The read is kept on the session, so the Factory page compares against it afterwards.
	if row, err := f.st.TerminalSessionFor(f.uid, session); err != nil || row.Model != "actual/model" || row.Effort != "medium" {
		t.Fatalf("session after the read: %+v %v", row, err)
	}
	// An agent in the middle of a settings change is not taken at its word.
	f.mu.Lock()
	f.controls = func(*terminalproto.ACPControl) terminalproto.ACPControlResult {
		return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Configuring: true, Options: []terminalproto.ACPSetting{}}}
	}
	f.mu.Unlock()
	if _, err := starter.OutcomeSessionChoice(t.Context(), f.uid, session); err == nil {
		t.Fatal("a session that was changing settings was read as settled")
	}
}

// Approving an outcome starts agents on a machine, so it needs the owning
// writer's browser, the canonical origin and remote sessions switched on.
func TestGafferOutcomeRoutesNeedTheOwningBrowserAndRemoteSessions(t *testing.T) {
	t.Parallel()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	plan := `{"objective":"Ship CSV export.","projectId":"missing","tool":"opencode","maxReviews":8,"hours":24,"confirmed":true,"sessions":[{"title":"Export","prompt":"Add CSV export. Do not merge."}]}`
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", plan, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign origin: %d", code)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "POST", "/api/v2/gaffer/outcomes", plan); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
	if _, err := st.CreateAPIToken(1, "Outcome refusal test", auth.HashToken("synthetic-api-token")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v2/gaffer/outcomes", "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32), "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32) + "/history"} {
		if code, _ := gafferClient(t, &http.Client{}, srv, "GET", path, "", "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token read of %s: %d", path, code)
		}
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "POST", "/api/v2/gaffer/outcomes", plan, "Authorization", "Bearer synthetic-api-token"); code != 403 {
		t.Fatalf("API token create: %d", code)
	}
	// A fractional limit or an unknown field never reaches the plan: the body is strict.
	for name, bad := range map[string]string{
		"fractional reviews": strings.Replace(plan, `"maxReviews":8`, `"maxReviews":1.5`, 1),
		"session yolo":       strings.Replace(plan, `"title":"Export"`, `"title":"Export","yolo":true`, 1),
	} {
		if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", bad); code != 400 {
			t.Fatalf("%s: %d", name, code)
		}
	}
	// YOLO is the person's to choose for an outcome, never a session's or a plan's, and only where the account allows it.
	if code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", strings.Replace(plan, `"confirmed":true`, `"confirmed":true,"yolo":true`, 1)); code != 403 || !strings.Contains(body, "Remote sessions are disabled") && !strings.Contains(body, "allow YOLO") {
		t.Fatalf("yolo without the account setting: %d %s", code, body)
	}
	// Remote sessions are off on this controller, so nothing can be approved.
	if code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", plan); code != 403 || !strings.Contains(body, "Remote sessions are disabled") {
		t.Fatalf("remote sessions off: %d %s", code, body)
	}
	code, body := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/outcomes", "")
	if code != 200 || strings.TrimSpace(body) != `{"outcomes":[]}` {
		t.Fatalf("list: %d %s", code, body)
	}
	if code, _ := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/outcomes/"+strings.Repeat("a", 32), ""); code != 404 {
		t.Fatalf("unknown outcome: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes/"+strings.Repeat("a", 32)+"/stop", `{"revision":1}`); code != 404 {
		t.Fatalf("stop unknown outcome: %d", code)
	}
	// A merge is approved by the owning browser, explicitly, for a pipeline that exists.
	merge := "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32) + "/sessions/a/merge"
	if code, _ := doClient(t, owner, srv, "POST", merge, `{"revision":1,"confirmed":true}`, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign-origin merge: %d", code)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "POST", merge, `{"revision":1,"confirmed":true}`, "Authorization", "Bearer synthetic-api-token"); code != 403 {
		t.Fatalf("API token merge: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", merge, `{"revision":1}`); code != 400 {
		t.Fatalf("unconfirmed merge: %d", code)
	}
	// An approval starts the merge stage, so with remote sessions off it is refused like the other controls that start work.
	if code, body := gafferClient(t, owner, srv, "POST", merge, `{"revision":1,"confirmed":true}`); code != 409 || !strings.Contains(body, "Remote sessions are disabled") {
		t.Fatalf("merge with remote sessions off: %d %s", code, body)
	}
	// Trying again, adding pipelines and asking for a report are the owning browser's requests too.
	base := "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32)
	for path, body := range map[string]string{base + "/sessions/a/retry": `{"revision":1}`, base + "/sessions": `{"revision":1,"confirmed":true,"sessions":[{"title":"More","prompt":"Do it. Do not merge."}]}`, base + "/review": `{"revision":1}`} {
		if code, _ := doClient(t, owner, srv, "POST", path, body, "Origin", "https://different.example"); code != 403 {
			t.Fatalf("foreign origin %s: %d", path, code)
		}
		if code, _ := gafferClient(t, &http.Client{}, srv, "POST", path, body, "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token %s: %d", path, code)
		}
		if code, _ := gafferClient(t, owner, srv, "POST", path, `{}`); code != 400 {
			t.Fatalf("no revision %s: %d", path, code)
		}
	}
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/review", `{"revision":1}`); code != 404 {
		t.Fatalf("report on an unknown outcome: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/sessions", `{"revision":1,"confirmed":true,"yolo":true,"sessions":[]}`); code != 400 {
		t.Fatalf("unknown field on added pipelines: %d", code)
	}
	// Giving Gaffer the decisions names the choice and the revision it was made on.
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/automation", `{"revision":1}`); code != 400 {
		t.Fatalf("automation without a choice: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/automation", `{"revision":1,"on":true}`); code != 404 {
		t.Fatalf("automation on an unknown outcome: %d", code)
	}
	// Putting a started outcome on YOLO needs the person's confirmation and the account's Allow YOLO setting.
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/yolo", `{"revision":1,"on":true}`); code != 400 {
		t.Fatalf("YOLO without confirmation: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/yolo", `{"revision":1,"on":true,"confirmed":true}`); code != 403 {
		t.Fatalf("YOLO while the account does not allow it: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/yolo", `{"revision":1,"on":false}`); code != 404 {
		t.Fatalf("taking an unknown outcome off YOLO: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes/"+strings.Repeat("a", 32)+"/stop", `{}`); code != 400 {
		t.Fatalf("stop without a revision: %d", code)
	}
	// Moving on, sending back, the merge setting, telling the agent and stopping following are the owning browser's too.
	for path, body := range map[string]string{
		base + "/sessions/a/advance":   `{"confirmed":true,"stage":"code"}`,
		base + "/sessions/a/back":      `{"stage":"code","reason":"Again.","confirmed":true}`,
		base + "/sessions/a/automerge": `{"on":false}`,
		base + "/sessions/a/tell":      `{"requestId":"` + strings.Repeat("c", 32) + `","text":"Carry on."}`,
		base + "/stop-following":       `{"revision":1}`,
		base + "/automation":           `{"revision":1,"on":true}`,
		base + "/yolo":                 `{"revision":1,"on":false}`,
	} {
		if code, _ := doClient(t, owner, srv, "POST", path, body, "Origin", "https://different.example"); code != 403 {
			t.Fatalf("foreign origin %s: %d", path, code)
		}
		if code, _ := gafferClient(t, &http.Client{}, srv, "POST", path, body, "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token %s: %d", path, code)
		}
		if code, _ := gafferClient(t, &http.Client{}, srv, "POST", path, body); code != 401 {
			t.Fatalf("signed out %s: %d", path, code)
		}
		if code, _ := gafferClient(t, owner, srv, "POST", path, strings.Replace(body, "{", `{"yolo":true,`, 1)); code != 400 {
			t.Fatalf("unknown field %s: %d", path, code)
		}
		// With everything in order, an outcome that is not this account's changes nothing.
		if code, text := gafferClient(t, owner, srv, "POST", path, body); code != 404 && code != 403 {
			t.Fatalf("unknown outcome %s: %d %s", path, code, text)
		}
	}
	// Archiving, restoring and deleting are the owning browser's requests, and name the revision they mean.
	for _, action := range []string{"archive", "restore"} {
		path := base + "/" + action
		if code, _ := doClient(t, owner, srv, "POST", path, `{"revision":1}`, "Origin", "https://different.example"); code != 403 {
			t.Fatalf("foreign origin %s: %d", action, code)
		}
		if code, _ := gafferClient(t, owner, srv, "POST", path, `{}`); code != 400 {
			t.Fatalf("%s without a revision: %d", action, code)
		}
		if code, _ := gafferClient(t, owner, srv, "POST", path, `{"revision":1}`); code != 404 {
			t.Fatalf("%s of an unknown outcome: %d", action, code)
		}
	}
	if code, _ := gafferClient(t, owner, srv, "DELETE", base, ""); code != 400 {
		t.Fatalf("delete without a revision: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "DELETE", base+"?revision=1", ""); code != 404 {
		t.Fatalf("delete of an unknown outcome: %d", code)
	}
}
