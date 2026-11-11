package store

import "testing"

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
