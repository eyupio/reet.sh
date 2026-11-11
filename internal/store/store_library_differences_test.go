package store

import (
	"github.com/eyupio/reet.sh/internal/skillbundle"
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
	// Asking what differs has the machine list its files, and nothing more.
	for range 2 {
		if err := s.ResolveLibraryDifferenceFor(uid, saved.ID, env, "compare"); err != nil {
			t.Fatal(err)
		}
	}
	if got := differences(); len(got) != 1 || !got[0].Comparing || got[0].Taking || got[0].Compared {
		t.Fatalf("a comparison in progress must say so and take nothing: %+v", got)
	}
	var comparisons int
	var comparison string
	if err := s.db.QueryRow(`SELECT count(*),COALESCE(max(id),'') FROM machine_import_requests WHERE user_id=? AND request_key LIKE 'compare_%'`, uid).Scan(&comparisons, &comparison); err != nil || comparisons != 1 {
		t.Fatalf("pressing twice must ask the machine once: %d %v", comparisons, err)
	}
	pending, err := s.RemoteImportFor(uid, comparison)
	if err != nil {
		t.Fatal(err)
	}
	// The machine answers with the saved files, one of them edited, and one new file.
	onMachine := append([]skillbundle.ContentFile{}, machineImportFixture(t).Files...)
	onMachine[0].Data = append(append([]byte{}, onMachine[0].Data...), []byte("\nedited\n")...)
	onMachine = append(onMachine, skillbundle.ContentFile{Path: "extra/notes.md", Data: []byte("new")})
	reviewed := reviewRemote(t, s, uid, agent, pending, onMachine)
	if reviewed.State != "reviewing" {
		t.Fatalf("a comparison must wait for the person, not save anything: %+v", reviewed.State)
	}
	got = differences()
	if len(got) != 1 || !got[0].Compared || got[0].Comparing || got[0].Taking || item().RevisionID != saved.RevisionID {
		t.Fatalf("an answered comparison must be shown and change nothing: %+v", got)
	}
	changes := map[string]string{}
	for _, file := range got[0].Files {
		changes[file.Path] = file.Change
	}
	if len(changes) != 2 || changes[onMachine[0].Path] != "changed" || changes["extra/notes.md"] != "machine" {
		t.Fatalf("the comparison must list exactly the edited and the new file: %+v", got[0].Files)
	}
	// Taking the version uses the files the machine already listed.
	if err := s.ResolveLibraryDifferenceFor(uid, saved.ID, env, "take"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveLibraryDifferenceFor(uid, saved.ID, env, "take"); err != nil {
		t.Fatal("pressing twice must not fail", err)
	}
	if approved, err := s.RemoteImportFor(uid, comparison); err != nil || approved.State != "approved" {
		t.Fatalf("taking after a comparison must approve that comparison: %+v %v", approved.State, err)
	}
	var asked int
	if err := s.db.QueryRow(`SELECT count(*) FROM machine_import_requests WHERE user_id=? AND json_extract(data,'$.bundleId')=? AND json_extract(data,'$.expectedBase')=?`, uid, saved.BundleID, saved.RevisionID).Scan(&asked); err != nil || asked != 1 {
		t.Fatalf("comparing and then taking must ask the machine for its files once: %d %v", asked, err)
	}
	if got := differences(); len(got) != 1 || !got[0].Taking {
		t.Fatalf("a version being saved must say so: %+v", got)
	}
	if err := s.ResolveLibraryDifferenceFor(uid, saved.ID, "1/elsewhere/agents", "keep"); err == nil {
		t.Fatal("a choice about a machine that reports no difference was accepted")
	}
}
