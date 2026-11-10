package store

import (
	"strings"
	"testing"
)

func TestMachineCheckInSavesSharedSkillIntoItsLibraryEntry(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	if _, err := s.UpsertLocalFor(uid, "demo", Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(uid, "local/demo", agent.Host, "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimRemoteImportFor(uid, agent.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	entries := func() []LibraryItem {
		t.Helper()
		var out []LibraryItem
		for _, item := range canonicalItems(t, s, uid) {
			if item.Name == "demo" {
				out = append(out, item)
			}
		}
		return out
	}
	before := entries()
	if len(before) != 1 || before[0].RevisionID != "" {
		t.Fatalf("fixture needs one library entry without content: %+v", before)
	}
	requests := func() []RemoteImport {
		t.Helper()
		rows, err := s.db.Query(`SELECT id FROM machine_import_requests WHERE user_id=? ORDER BY created_at,id`, uid)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		rows.Close()
		var out []RemoteImport
		for _, id := range ids {
			r, err := s.RemoteImportFor(uid, id)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, r)
		}
		return out
	}
	for range 2 {
		if err := s.MaintainSync(uid, "owner", agent); err != nil {
			t.Fatal(err)
		}
	}
	queued := requests()
	if len(queued) != 1 || queued[0].SyncSkillID != "local/demo" || queued[0].DestinationID != "" || queued[0].AgentID != agent.ID {
		t.Fatalf("a check-in must ask the reporting machine for the skill once, installing nothing: %+v", queued)
	}
	done := finishMachineCopy(t, s, uid, agent, queued[0], machineImportFixture(t).Files)
	if done.State != "completed" {
		t.Fatalf("capture did not complete: %+v", done)
	}
	after := entries()
	if len(after) != 1 || after[0].ID != before[0].ID || after[0].RevisionID == "" || after[0].Status != before[0].Status {
		t.Fatalf("the copy must fill the existing entry and keep its status, not add a second one: %+v", after)
	}
	if err := s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	if got := requests(); len(got) != 1 {
		t.Fatalf("a saved skill was asked for again: %d requests", len(got))
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if len(target.Desired) != 0 {
			t.Fatalf("saving a copy must not select the skill for any machine: %+v", target.Desired)
		}
	}
}
