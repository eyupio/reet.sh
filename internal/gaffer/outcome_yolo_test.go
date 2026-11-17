package gaffer

import (
	"context"
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
	o, left, err := r.s.SetOutcomeYolo(context.Background(), r.p.UserID, created.ID, mustOutcome(t, r, created.ID).Revision, true)
	if err != nil || !o.Yolo || len(left) != 0 {
		t.Fatalf("on: %v yolo %v left %v", err, o.Yolo, left)
	}
	if on, ok := y.sets[code.SessionID]; !ok || !on {
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
	o, _, err = r.s.SetOutcomeYolo(context.Background(), r.p.UserID, created.ID, mustOutcome(t, r, created.ID).Revision, false)
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
	o, left, err := r.s.SetOutcomeYolo(context.Background(), r.p.UserID, created.ID, mustOutcome(t, r, created.ID).Revision, true)
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
