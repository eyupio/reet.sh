package store

import (
	"strings"
	"testing"
)

func TestLibraryFlagsADifferingMachineCopyUntilAPersonChooses(t *testing.T) {
	s, uid, agent, env := observationFixture(t)
	if _, err := s.UpsertLocalFor(uid, "demo", Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(uid, "local/demo", agent.Host, "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimRemoteImportFor(uid, agent.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	if err := s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	var first string
	if err := s.db.QueryRow(`SELECT id FROM machine_import_requests WHERE user_id=?`, uid).Scan(&first); err != nil {
		t.Fatal(err)
	}
	request, err := s.RemoteImportFor(uid, first)
	if err != nil {
		t.Fatal(err)
	}
	finishMachineCopy(t, s, uid, agent, request, machineImportFixture(t).Files)
	item := func() LibraryItem {
		t.Helper()
		for _, candidate := range canonicalItems(t, s, uid) {
			if candidate.Name == "demo" {
				return candidate
			}
		}
		t.Fatal("missing library item")
		return LibraryItem{}
	}
	saved := item()
	if saved.Digest == "" {
		t.Fatalf("fixture item needs saved content: %+v", saved)
	}
	observe := func(digest string) {
		t.Helper()
		grant, err := s.PrepareContentObservationFor(uid, agent.ID)
		if err != nil {
			t.Fatal(err)
		}
		report := observationReport(grant)
		report.Scopes[0].Skills[0].Digest = digest
		if err = s.RecordContentObservationFor(uid, agent.ID, report); err != nil {
			t.Fatal(err)
		}
	}
	differences := func() []LibraryDifference {
		t.Helper()
		out, err := s.LibraryDifferencesFor(uid, canonicalItems(t, s, uid))
		if err != nil {
			t.Fatal(err)
		}
		return out[saved.ID]
	}
	// The machine holds exactly what reet saved: nothing to decide.
	observe(saved.Digest)
	if got := differences(); len(got) != 0 {
		t.Fatalf("an identical machine copy was flagged: %+v", got)
	}
	edited := strings.Repeat("b", 64)
	observe(edited)
	got := differences()
	if len(got) != 1 || got[0].EnvironmentID != env || got[0].Digest != edited || got[0].Taking {
		t.Fatalf("a differing machine copy was not flagged: %+v", got)
	}
	if err := s.ResolveLibraryDifferenceFor(uid, saved.ID, env, "keep"); err != nil {
		t.Fatal(err)
	}
	if got := differences(); len(got) != 0 || item().RevisionID != saved.RevisionID {
		t.Fatalf("keeping reet's copy must clear the flag and change nothing: %+v", got)
	}
	// A further edit on the machine is a new question.
	again := strings.Repeat("c", 64)
	observe(again)
	if got := differences(); len(got) != 1 || got[0].Digest != again {
		t.Fatalf("a later edit was not flagged after an earlier choice: %+v", got)
	}
	if err := s.ResolveLibraryDifferenceFor(uid, saved.ID, env, "take"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveLibraryDifferenceFor(uid, saved.ID, env, "take"); err != nil {
		t.Fatal("pressing twice must not fail", err)
	}
	var taking int
	if err := s.db.QueryRow(`SELECT count(*) FROM machine_import_requests WHERE user_id=? AND json_extract(data,'$.chosen')=1 AND json_extract(data,'$.bundleId')=? AND json_extract(data,'$.expectedBase')=?`, uid, saved.BundleID, saved.RevisionID).Scan(&taking); err != nil || taking != 1 {
		t.Fatalf("taking the machine's version must ask that machine once for a new version of the same item: %d %v", taking, err)
	}
	if got := differences(); len(got) != 1 || !got[0].Taking {
		t.Fatalf("a version being saved must say so: %+v", got)
	}
	if err := s.ResolveLibraryDifferenceFor(uid, saved.ID, "1/elsewhere/agents", "keep"); err == nil {
		t.Fatal("a choice about a machine that reports no difference was accepted")
	}
}
