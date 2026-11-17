package gaffer

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

type yoloStarter struct {
	*outcomeStarterFake
	mu   sync.Mutex
	sets map[string]bool
	fail bool
	// during runs inside a launch, before the session exists.
	during func()
}

func (y *yoloStarter) StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (string, string, error) {
	if hook := y.during; hook != nil {
		y.during = nil
		hook()
	}
	return y.outcomeStarterFake.StartOutcomeSession(ctx, uid, in)
}

func (y *yoloStarter) set(session string) (bool, bool) {
	y.mu.Lock()
	defer y.mu.Unlock()
	on, ok := y.sets[session]
	return on, ok
}

func (y *yoloStarter) SetOutcomeSessionYolo(_ context.Context, _ int64, session string, on bool) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.fail {
		return LaunchUnknown("offline")
	}
	if y.sets == nil {
		y.sets = map[string]bool{}
	}
	y.sets[session] = on
	return nil
}

// Found on a real run: an outcome started with ordinary approvals stopped at
// a permission prompt in every new session, and nothing could put the whole
// outcome on YOLO once it had started.
func TestAStartedOutcomeCanBePutOnYoloAndTakenOff(t *testing.T) {
	r := outcomeFixture(t)
	y := &yoloStarter{outcomeStarterFake: r.starter}
	r.s.SetSessionStarter(y)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, true))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	o, left, err := switchYolo(t, r, created.ID, true)
	if err != nil || !o.Yolo || len(left) != 0 {
		t.Fatalf("on: %v yolo %v left %v", err, o.Yolo, left)
	}
	if on, ok := y.set(code.SessionID); !ok || !on {
		t.Fatalf("the session in hand was not switched: %v", y.sets)
	}
	// A pipeline that merges by itself needs the person's separate yes on YOLO.
	if MergesByItself(o, o.Sessions[0]) {
		t.Fatal("a pipeline set to merge by itself kept doing so on YOLO without the separate confirmation")
	}
	// The next stage starts on YOLO.
	if _, err = r.s.MovePipelineOn(r.p.UserID, created.ID, code.Key, ""); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 1 && p.State == "following" })
	if last := y.asked()[len(y.asked())-1]; !last.Yolo {
		t.Fatalf("the next stage did not start on YOLO: %+v", last)
	}
	o, _, err = switchYolo(t, r, created.ID, false)
	if err != nil || o.Yolo {
		t.Fatalf("off: %v yolo %v", err, o.Yolo)
	}
}

// A session that could not be switched is named, and the outcome is still
// on YOLO for the stages to come.
func TestASessionThatCouldNotBeSwitchedToYoloIsNamed(t *testing.T) {
	r := outcomeFixture(t)
	y := &yoloStarter{outcomeStarterFake: r.starter, fail: true}
	r.s.SetSessionStarter(y)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	o, left, err := switchYolo(t, r, created.ID, true)
	if err != nil || !o.Yolo || len(left) != 1 || left[0] != code.Title {
		t.Fatalf("%v yolo %v left %v", err, o.Yolo, left)
	}
}

// Found on the same run: a session the controller ended because its pipeline
// moved on showed as newly finished, as though the person had something to do.
func TestASessionThePipelineLeftBehindDoesNotAskForThePerson(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	before := time.Now().UnixMilli()
	if _, err = r.s.MovePipelineOn(r.p.UserID, created.ID, code.Key, ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(r.starter.closed()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := r.starter.closed(); len(got) != 1 || got[0] != code.SessionID {
		t.Fatalf("ended: %v", got)
	}
	var seen int64
	for time.Now().Before(deadline) {
		if seen, _ = r.st.SessionSeenFor(r.p.UserID, code.SessionID); seen > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if seen < before {
		t.Fatalf("the ended session was not marked seen: %d", seen)
	}
}

// A launch in flight when the person changes the outcome's approvals started
// on the old choice: its session is put right when it is taken up.
func TestALaunchInFlightTakesTheOutcomesNewApprovals(t *testing.T) {
	r := outcomeFixture(t)
	y := &yoloStarter{outcomeStarterFake: r.starter}
	var created store.GafferOutcome
	ready := make(chan struct{})
	y.during = func() {
		<-ready
		if _, _, err := switchYolo(t, r, created.ID, true); err != nil {
			t.Error(err)
		}
	}
	r.s.SetSessionStarter(y)
	r.s.StartGoalSupervisor()
	var err error
	if created, err = r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false)); err != nil {
		t.Fatal(err)
	}
	close(ready)
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if on, ok := y.set(code.SessionID); ok && on {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("a session that started while the outcome was put on YOLO was left asking: launches %+v", y.asked())
}

// A pipeline waiting at Ready to merge keeps a live session the person can
// still give work to, so it is switched with the rest.
func TestASessionWaitingAtTheMergeGateIsSwitchedToo(t *testing.T) {
	r := outcomeFixture(t)
	y := &yoloStarter{outcomeStarterFake: r.starter}
	r.s.SetSessionStarter(y)
	r.s.StartGoalSupervisor()
	in := pipelineInput(r, false)
	in.Sessions[0].Stages = in.Sessions[0].Stages[2:] // test, merge
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	test := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if _, err = r.s.MovePipelineOn(r.p.UserID, created.ID, test.Key, ""); err != nil {
		t.Fatal(err)
	}
	gate := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	if gate.SessionID == "" {
		t.Fatalf("no session kept at the merge gate: %+v", gate)
	}
	if _, left, err := switchYolo(t, r, created.ID, true); err != nil || len(left) != 0 {
		t.Fatalf("%v left %v", err, left)
	}
	if on, ok := y.set(gate.SessionID); !ok || !on {
		t.Fatal("the session waiting at the merge gate was not switched")
	}
}

// switchYolo is the card's switch. The controller keeps moving the outcome,
// so, as the page does, a revision that has moved on is read again.
func switchYolo(t *testing.T, r *outcomeRig, id string, on bool) (store.GafferOutcome, []string, error) {
	t.Helper()
	var o store.GafferOutcome
	var left []string
	var err error
	for attempt := 0; attempt < 50; attempt++ {
		o, left, err = r.s.SetOutcomeYolo(context.Background(), r.p.UserID, id, mustOutcome(t, r, id).Revision, on)
		if err == nil || !strings.Contains(err.Error(), "changed") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return o, left, err
}
