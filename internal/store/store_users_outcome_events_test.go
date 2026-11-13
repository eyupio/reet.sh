package store

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestOutcomeEventsAreThatOutcomesGafferRowsNewestFirst(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid := int64(1)
	if u, err := st.CreateUser("owner@example.test", "Owner", "owner", "hash"); err == nil {
		uid = u.ID
	}
	add := func(action string, detail any) {
		t.Helper()
		if err := st.AddEventFor(uid, "gaffer", action, "", detail); err != nil {
			t.Fatal(err)
		}
	}
	add("gaffer-stage-started", map[string]any{"outcome": "aaa", "pipeline": "a", "stage": "code"})
	add("gaffer-stage-started", map[string]any{"outcome": "bbb", "pipeline": "a", "stage": "code"})
	add("yolo-allow", map[string]any{"outcome": "aaa"})
	add("gaffer-stage-handed-over", map[string]any{"outcome": "aaa", "pipeline": "a", "stage": "code"})
	add("gaffer-goal-held", nil)
	got, err := st.OutcomeEventsFor(uid, "aaa", 10)
	if err != nil || len(got) != 2 || got[0].Action != "gaffer-stage-handed-over" || got[1].Action != "gaffer-stage-started" {
		t.Fatalf("events %+v %v", got, err)
	}
	if other, err := st.OutcomeEventsFor(uid+99, "aaa", 10); err != nil || len(other) != 0 {
		t.Fatalf("another account read them: %+v %v", other, err)
	}
}

func TestARepeatedEventIsRecordedOnceAndEventsCanLeaveSourcesOut(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	u, err := st.CreateUser("owner@example.test", "Owner", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	same := map[string]any{"imported": 76, "created": 0, "ignored": 12, "pruned": 0}
	for i := 0; i < 5; i++ {
		if err = st.AddEventUnlessRepeatedFor(u.ID, "scan", "scan", same, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	// Different counts are news; so is the same detail once the window has passed.
	if err = st.AddEventUnlessRepeatedFor(u.ID, "scan", "scan", map[string]any{"imported": 77, "created": 0, "ignored": 12, "pruned": 0}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err = st.AddEventUnlessRepeatedFor(u.ID, "scan", "scan", same, 0); err != nil {
		t.Fatal(err)
	}
	if err = st.AddEventFor(u.ID, "gaffer", "gaffer-goal-held", "", nil); err != nil {
		t.Fatal(err)
	}
	all, _, err := st.ListEventsPageFor(u.ID, EventQuery{Limit: 50})
	if err != nil || len(all) != 4 {
		t.Fatalf("events %d %v", len(all), err)
	}
	rest, _, err := st.ListEventsPageFor(u.ID, EventQuery{Limit: 50, Exclude: []string{"scan", "machine"}})
	if err != nil || len(rest) != 1 || rest[0].Action != "gaffer-goal-held" {
		t.Fatalf("excluding scan: %+v %v", rest, err)
	}
}

// Past the retention limit only an outcome with nothing still moving is
// dropped: one whose pipeline is still followed stays however old it is.
func TestRetentionKeepsAnOutcomeWhosePipelinesAreStillMoving(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	u, err := st.CreateUser("retention@example.test", "Owner", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	make := func(n int, state string) GafferOutcome {
		t.Helper()
		id := fmt.Sprintf("%032d", n)
		o, err := st.CreateGafferOutcomeFor(u.ID, GafferOutcome{ID: id, Objective: "Ship it.", Sessions: []GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it."}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.UpdateGafferOutcomeFor(u.ID, id, o.Revision, func(o *GafferOutcome) bool {
			o.State, o.Sessions[0].State, o.CreatedAt = "settled", state, int64(n)
			return true
		}); err != nil {
			t.Fatal(err)
		}
		if _, err = st.db.Exec(`UPDATE gaffer_outcomes SET created_at=? WHERE id=?`, n, id); err != nil {
			t.Fatal(err)
		}
		return o
	}
	live, old := make(1, "following"), make(2, "done")
	for n := 3; n <= MaxGafferOutcomes+3; n++ {
		make(n, "done")
	}
	if _, err := st.GafferOutcomeFor(u.ID, live.ID); err != nil {
		t.Fatalf("an outcome still being followed was dropped: %v", err)
	}
	if _, err := st.GafferOutcomeFor(u.ID, old.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the oldest finished outcome is the one to drop: %v", err)
	}
}
