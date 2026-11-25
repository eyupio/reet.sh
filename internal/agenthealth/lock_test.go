package agenthealth

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// lockDecisions is what a locked machine does with each message kind the
// protocol decodes. Every kind Decode knows must be here, so a new kind
// cannot reach a locked machine without someone deciding.
var lockDecisions = map[string]struct {
	allowed bool
	body    terminalproto.Body
}{
	// Allowed: they only report state, or end or stop something.
	"reconcile":       {true, &terminalproto.Reconcile{}},
	"end":             {true, &terminalproto.End{}},
	"detach":          {true, &terminalproto.Detach{}},
	"acp_cancel":      {true, &terminalproto.ACPCancel{}},
	"worktree_status": {true, &terminalproto.WorktreeStatus{}},
	"native_view":     {true, &terminalproto.NativeView{}},
	"probe":           {true, &terminalproto.Probe{}},
	// Refused: each starts, attaches, types, reads, changes or uploads.
	"create":              {false, &terminalproto.Create{}},
	"attach":              {false, &terminalproto.Attach{}},
	"resize":              {false, &terminalproto.Resize{}},
	"renew_attachment":    {false, &terminalproto.RenewAttachment{}},
	"history":             {false, &terminalproto.History{}},
	"command_history":     {false, &terminalproto.CommandHistory{}},
	"worktree_remove":     {false, &terminalproto.WorktreeRemove{}},
	"inspect":             {false, &terminalproto.Inspect{}},
	"browse":              {false, &terminalproto.Browse{}},
	"branches":            {false, &terminalproto.Branches{}},
	"branch_prune_plan":   {false, &terminalproto.BranchPrunePlan{}},
	"branch_prune":        {false, &terminalproto.BranchPrune{}},
	"branch_sync":         {false, &terminalproto.BranchSync{}},
	"base_check":          {false, &terminalproto.BaseCheck{}},
	"repo_clone":          {false, &terminalproto.RepoClone{}},
	"repo_clone_status":   {false, &terminalproto.RepoCloneStatus{}},
	"repo_clone_remove":   {false, &terminalproto.RepoCloneRemove{}},
	"repo_list":           {false, &terminalproto.RepoList{}},
	"accounts":            {false, &terminalproto.Accounts{}},
	"upload":              {false, &terminalproto.Upload{}},
	"models":              {false, &terminalproto.Models{}},
	"software_apply":      {false, &terminalproto.SoftwareApply{}},
	"setup_fleet":         {false, &terminalproto.SetupFleet{}},
	"acp_prompt":          {false, &terminalproto.ACPPrompt{}},
	"acp_preview":         {false, &terminalproto.ACPPreview{}},
	"acp_control":         {false, &terminalproto.ACPControl{}},
	"acp_model":           {false, &terminalproto.ACPModel{}},
	"acp_permission":      {false, &terminalproto.ACPPermission{}},
	"acp_mode":            {false, &terminalproto.ACPMode{}},
	"acp_agent_mode":      {false, &terminalproto.ACPAgentMode{}},
	"session_mode":        {false, &terminalproto.SessionMode{}},
	"chat_reattach":       {false, &terminalproto.ChatReattach{}},
	"acp_question_answer": {false, &terminalproto.ACPQuestionAnswer{}},
	"terminal_answer":     {false, &terminalproto.TerminalAnswer{}},
	// Machine-to-controller kinds a controller never sends: refused like
	// anything the gate does not know.
	"hello":                    {false, &terminalproto.Hello{}},
	"probe_result":             {false, &terminalproto.ProbeResult{}},
	"health_report":            {false, &terminalproto.HealthReport{}},
	"worktree_state":           {false, &terminalproto.WorktreeState{}},
	"worktree_result":          {false, &terminalproto.WorktreeResult{}},
	"inspect_result":           {false, &terminalproto.InspectResult{}},
	"browse_result":            {false, &terminalproto.BrowseResult{}},
	"branches_result":          {false, &terminalproto.BranchesResult{}},
	"branch_prune_plan_result": {false, &terminalproto.BranchPrunePlanResult{}},
	"branch_prune_result":      {false, &terminalproto.BranchPruneResult{}},
	"branch_sync_result":       {false, &terminalproto.BranchSyncResult{}},
	"base_check_result":        {false, &terminalproto.BaseCheckResult{}},
	"repo_clone_result":        {false, &terminalproto.RepoCloneResult{}},
	"repo_clone_state":         {false, &terminalproto.RepoCloneState{}},
	"repo_clone_removed":       {false, &terminalproto.RepoCloneRemoved{}},
	"repo_list_result":         {false, &terminalproto.RepoListResult{}},
	"accounts_result":          {false, &terminalproto.AccountsResult{}},
	"models_result":            {false, &terminalproto.ModelsResult{}},
	"software_apply_result":    {false, &terminalproto.SoftwareApplyResult{}},
	"setup_fleet_result":       {false, &terminalproto.SetupFleetResult{}},
	"result":                   {false, &terminalproto.Result{}},
	"session_changed":          {false, &terminalproto.SessionChanged{}},
	"launch_progress":          {false, &terminalproto.LaunchProgress{}},
	"acp_preview_result":       {false, &terminalproto.ACPPreviewResult{}},
	"acp_control_result":       {false, &terminalproto.ACPControlResult{}},
	"native_view_result":       {false, &terminalproto.NativeViewResult{}},
	"acp_event":                {false, &terminalproto.ACPEvent{}},
}

// decodedKinds reads the message kinds terminalproto.Decode accepts from its
// source, so the table above cannot fall behind the protocol.
func decodedKinds(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../terminalproto/protocol.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Decode" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					kind, _ := strconv.Unquote(lit.Value)
					kinds = append(kinds, kind)
				}
			}
			return true
		})
	}
	if len(kinds) < 40 {
		t.Fatalf("read only %d kinds from Decode", len(kinds))
	}
	return kinds
}

func lockGate(t *testing.T, locked, negotiated bool) (func(context.Context, terminalproto.Message) error, *[]terminalproto.Message, *[]terminalproto.Message) {
	t.Helper()
	cfg := AgentConfig{Locked: func() bool { return locked }, machineLock: negotiated}
	var forwarded, sent []terminalproto.Message
	gate := gateMachineRequests(cfg, func(_ context.Context, m terminalproto.Message) error { forwarded = append(forwarded, m); return nil },
		func(_ context.Context, m terminalproto.Message) error { sent = append(sent, m); return nil })
	return gate, &forwarded, &sent
}

func TestALockedMachineServesOnlyItsAllowList(t *testing.T) {
	kinds := decodedKinds(t)
	for _, kind := range kinds {
		if _, ok := lockDecisions[kind]; !ok {
			t.Errorf("message kind %q has no decision for a locked machine", kind)
		}
	}
	for kind := range lockDecisions {
		if !slices.Contains(kinds, kind) {
			t.Errorf("decision for %q, which Decode does not know", kind)
		}
	}
	for kind, decision := range lockDecisions {
		for _, negotiated := range []bool{true, false} {
			gate, forwarded, sent := lockGate(t, true, negotiated)
			ask := terminalproto.Message{Version: terminalproto.Version, RequestID: "lock-request-000001", SessionID: "session-0000000001", Generation: 1, Body: decision.body}
			if err := gate(t.Context(), ask); err != nil {
				t.Fatal(kind, err)
			}
			if decision.allowed {
				if len(*forwarded) != 1 || len(*sent) != 0 {
					t.Errorf("%s was not served while locked: %v %v", kind, *forwarded, *sent)
				}
				continue
			}
			want := terminalproto.Locked
			if !negotiated {
				// An older controller's strict decoder does not know locked.
				want = terminalproto.Denied
			}
			if len(*forwarded) != 0 || len(*sent) != 1 {
				t.Errorf("%s was not refused while locked: %v %v", kind, *forwarded, *sent)
				continue
			}
			reply := (*sent)[0]
			if body, ok := reply.Body.(terminalproto.Result); !ok || body.Code != want || reply.RequestID != ask.RequestID || reply.SessionID != ask.SessionID {
				t.Errorf("%s refused as %+v, want %s", kind, reply, want)
			}
		}
	}
}

// A kind the gate has never heard of is refused, not served.
type unknownBody struct{ terminalproto.Body }

func TestALockedMachineRefusesAKindItDoesNotKnow(t *testing.T) {
	gate, forwarded, sent := lockGate(t, true, true)
	if err := gate(t.Context(), terminalproto.Message{RequestID: "lock-request-000002", Body: unknownBody{}}); err != nil {
		t.Fatal(err)
	}
	if len(*forwarded) != 0 || len(*sent) != 1 {
		t.Fatalf("served: %v %v", *forwarded, *sent)
	}
}

func TestAnUnlockedMachineServesEveryKind(t *testing.T) {
	for kind, decision := range lockDecisions {
		gate, forwarded, sent := lockGate(t, false, true)
		if err := gate(t.Context(), terminalproto.Message{RequestID: "lock-request-000003", Body: decision.body}); err != nil {
			t.Fatal(kind, err)
		}
		if len(*forwarded) != 1 || len(*sent) != 0 {
			t.Errorf("%s was not passed on while unlocked: %v %v", kind, *forwarded, *sent)
		}
	}
}

// The lock is read for every request, so a lock taken a moment ago holds
// at once, before the next capability update.
func TestTheGateReadsTheLockForEveryRequest(t *testing.T) {
	var locked atomic.Bool
	cfg := AgentConfig{Locked: locked.Load, machineLock: true}
	var forwarded int
	gate := gateMachineRequests(cfg, func(context.Context, terminalproto.Message) error { forwarded++; return nil },
		func(context.Context, terminalproto.Message) error { return nil })
	ask := terminalproto.Message{RequestID: "lock-request-000004", Body: &terminalproto.Create{}}
	_ = gate(t.Context(), ask)
	locked.Store(true)
	_ = gate(t.Context(), ask)
	locked.Store(false)
	_ = gate(t.Context(), ask)
	if forwarded != 2 {
		t.Fatalf("forwarded %d of 3", forwarded)
	}
}

func TestLockedIsSentOnlyWhenNegotiated(t *testing.T) {
	cfg := AgentConfig{TerminalSession: func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}, locked: true}
	older := capabilitiesFor(cfg, true, terminalproto.Ready)
	if older.Locked || older.TerminalEnabled || older.Readiness == terminalproto.Ready {
		t.Fatalf("an older controller was told %+v", older)
	}
	if err := older.ValidateFor(terminalFeatures(cfg)); err != nil {
		t.Fatalf("older capabilities invalid: %v", err)
	}
	cfg.machineLock = true
	newer := capabilitiesFor(cfg, true, terminalproto.Ready)
	if !newer.Locked || newer.TerminalEnabled || newer.Readiness != terminalproto.DisabledLocally {
		t.Fatalf("a newer controller was told %+v", newer)
	}
	if err := newer.ValidateFor(terminalFeatures(cfg)); err != nil {
		t.Fatalf("newer capabilities invalid: %v", err)
	}
	cfg.locked = false
	if open := capabilitiesFor(cfg, true, terminalproto.Ready); open.Locked || !open.TerminalEnabled || open.Readiness != terminalproto.Ready {
		t.Fatalf("an unlocked machine said %+v", open)
	}
	var negotiated AgentConfig
	negotiateFeatures(&negotiated, terminalproto.AttentionFeature+","+terminalproto.MachineLockFeature)
	if !negotiated.machineLock {
		t.Fatal("machine-lock-v1 was not negotiated")
	}
	negotiateFeatures(&negotiated, terminalproto.AttentionFeature)
	if negotiated.machineLock {
		t.Fatal("machine-lock-v1 was claimed without the controller advertising it")
	}
}

// Taking and lifting the lock each reach the controller as a new hello
// within one consent poll.
func TestConsentControlSendsTheLock(t *testing.T) {
	var locked atomic.Bool
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 10 * time.Millisecond, machineLock: true, Locked: locked.Load,
		TerminalSession: func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
			return nil, func() {}
		},
		Consent: func() (bool, terminalproto.Readiness, error) { return true, terminalproto.Ready, nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan terminalproto.Message, 4)
	errs := make(chan error, 2)
	go consentControl(ctx, cfg, 3, true, terminalproto.Ready, func(_ context.Context, m terminalproto.Message) error { sent <- m; return nil }, errs)
	next := func() terminalproto.Capabilities {
		t.Helper()
		select {
		case m := <-sent:
			return m.Body.(terminalproto.Hello).Capabilities
		case err := <-errs:
			t.Fatal(err)
		case <-time.After(2 * time.Second):
			t.Fatal("no hello")
		}
		return terminalproto.Capabilities{}
	}
	locked.Store(true)
	if caps := next(); !caps.Locked || caps.TerminalEnabled || caps.Readiness == terminalproto.Ready {
		t.Fatalf("lock: %+v", caps)
	}
	locked.Store(false)
	if caps := next(); caps.Locked || !caps.TerminalEnabled || caps.Readiness != terminalproto.Ready {
		t.Fatalf("unlock: %+v", caps)
	}
}

// While locked the machine holds its chats' events back instead of
// streaming them to the controller; once unlocked it reconnects so the
// held events are delivered from the chats' own buffer.
func TestALockedMachineHoldsChatEvents(t *testing.T) {
	var locked atomic.Bool
	cfg := AgentConfig{Locked: locked.Load}
	var delivered int
	var held atomic.Bool
	send := holdChatEvents(cfg, &held, func(context.Context, terminalproto.Message) error { delivered++; return nil })
	event := terminalproto.Message{Body: &terminalproto.ACPEvent{Kind: "agent"}}
	state := terminalproto.Message{Body: terminalproto.SessionChanged{State: terminalproto.Running}}
	if err := send(t.Context(), event); err != nil || delivered != 1 || held.Load() {
		t.Fatal("an unlocked machine held an event")
	}
	locked.Store(true)
	if err := send(t.Context(), event); err == nil || delivered != 1 || !held.Load() {
		t.Fatal("a locked machine streamed an event")
	}
	if err := send(t.Context(), state); err != nil || delivered != 2 {
		t.Fatal("a locked machine stopped reporting state")
	}
}

// Lifting the lock ends the connection once chat events were held, so the
// agent reconnects and the chats replay what they kept.
func TestUnlockReconnectsWhenChatEventsWereHeld(t *testing.T) {
	var locked atomic.Bool
	locked.Store(true)
	held := &atomic.Bool{}
	held.Store(true)
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 10 * time.Millisecond, machineLock: true, Locked: locked.Load, locked: true, chatsHeld: held,
		Consent: func() (bool, terminalproto.Readiness, error) { return true, terminalproto.Ready, nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 2)
	go consentControl(ctx, cfg, 3, true, terminalproto.Ready, func(context.Context, terminalproto.Message) error { return nil }, errs)
	time.Sleep(30 * time.Millisecond)
	select {
	case err := <-errs:
		t.Fatalf("ended while still locked: %v", err)
	default:
	}
	locked.Store(false)
	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("no reason given")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unlock did not reconnect to deliver held chat events")
	}
}

// When the lock went on is sent with Locked to a controller that negotiated
// it, and never to an older one.
func TestLockedAtIsSentOnlyWhenNegotiatedAndLocked(t *testing.T) {
	cfg := AgentConfig{TerminalSession: func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}, locked: true, lockedAt: 1795000000}
	if older := capabilitiesFor(cfg, true, terminalproto.Ready); older.LockedAt != 0 {
		t.Fatalf("an older controller was told %d", older.LockedAt)
	}
	cfg.machineLock = true
	newer := capabilitiesFor(cfg, true, terminalproto.Ready)
	if newer.LockedAt != 1795000000 || newer.ValidateFor(terminalFeatures(cfg)) != nil {
		t.Fatalf("a newer controller was told %+v", newer)
	}
	cfg.locked = false
	if open := capabilitiesFor(cfg, true, terminalproto.Ready); open.LockedAt != 0 {
		t.Fatalf("an unlocked machine sent %d", open.LockedAt)
	}
}

// The consent poll sends a new hello when the lock's time changes.
func TestConsentControlSendsWhenTheLockWentOn(t *testing.T) {
	var at atomic.Int64
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 10 * time.Millisecond, machineLock: true,
		Locked: func() bool { return true }, LockedAt: at.Load, locked: true,
		Consent: func() (bool, terminalproto.Readiness, error) { return true, terminalproto.Ready, nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan terminalproto.Message, 4)
	go consentControl(ctx, cfg, 3, true, terminalproto.Ready, func(_ context.Context, m terminalproto.Message) error { sent <- m; return nil }, make(chan error, 2))
	at.Store(1795000000)
	select {
	case m := <-sent:
		if got := m.Body.(terminalproto.Hello).Capabilities.LockedAt; got != 1795000000 {
			t.Fatalf("lockedAt %d", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no hello")
	}
}
