package store

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func templateStore(t *testing.T) (*Store, int64, int64) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a, err := s.CreateUser("owner@example.test", "Owner", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateUser("other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	return s, a.ID, b.ID
}

func hexID(c byte, n int) string { return strings.Repeat(string(c), 31) + string(rune('0'+n%10)) }

func templateVersion(id, request, name string) GafferTemplate {
	return GafferTemplate{ID: id, RequestID: request, Content: GafferTemplateContent{Name: name, Objective: "Ship it.", MaxReviews: 8, Hours: 24,
		Pipelines: []GafferTemplatePipeline{{Title: "Export API", Prompt: "Add the export.", Stages: []GafferTemplateStage{{Kind: "code", Tier: "standard"}}}}}}
}

// A version is a row of its own: saving on the latest makes the next one, an
// older version stays readable unchanged, and another account sees none of it.
func TestGafferTemplateVersionsAreKeptAndOwned(t *testing.T) {
	s, uid, other := templateStore(t)
	id := hexID('a', 0)
	first, err := s.SaveGafferTemplateFor(uid, templateVersion(id, hexID('1', 1), "First"), 0)
	if err != nil || first.Version != 1 || first.OwnerID != uid || first.CreatedAt == 0 {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := s.SaveGafferTemplateFor(uid, templateVersion(id, hexID('1', 2), "Second"), 1)
	if err != nil || second.Version != 2 {
		t.Fatalf("second: %+v %v", second, err)
	}
	if _, err = s.SaveGafferTemplateFor(uid, templateVersion(id, hexID('1', 3), "Stale"), 1); !errors.Is(err, ErrGafferTemplateChanged) {
		t.Fatalf("save on an older version: %v", err)
	}
	old, err := s.GafferTemplateFor(uid, id, 1)
	if err != nil || old.Content.Name != "First" || old.Version != 1 {
		t.Fatalf("old version: %+v %v", old, err)
	}
	latest, err := s.GafferTemplateFor(uid, id, 0)
	if err != nil || latest.Version != 2 || latest.Content.Name != "Second" {
		t.Fatalf("latest: %+v %v", latest, err)
	}
	if versions, err := s.GafferTemplateVersionsFor(uid, id); err != nil || len(versions) != 2 || versions[0] != 1 || versions[1] != 2 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	// Another account: not found to read, save on, list or delete.
	if _, err = s.GafferTemplateFor(other, id, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other account read: %v", err)
	}
	if _, err = s.SaveGafferTemplateFor(other, templateVersion(id, hexID('2', 1), "Theirs"), 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other account save: %v", err)
	}
	if rows, err := s.GafferTemplatesFor(other); err != nil || len(rows) != 0 {
		t.Fatalf("other account list: %v %v", rows, err)
	}
	if err = s.DeleteGafferTemplateFor(other, id, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other account delete: %v", err)
	}
	rows, err := s.GafferTemplatesFor(uid)
	if err != nil || len(rows) != 1 || rows[0].Version != 2 || rows[0].FirstCreatedAt != first.CreatedAt {
		t.Fatalf("list: %+v %v", rows, err)
	}
	// Delete names the latest version it saw, then takes every version.
	if err = s.DeleteGafferTemplateFor(uid, id, 1); !errors.Is(err, ErrGafferTemplateChanged) {
		t.Fatalf("delete on an older version: %v", err)
	}
	if err = s.DeleteGafferTemplateFor(uid, id, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GafferTemplateFor(uid, id, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

// A request answered once is answered again from its record, and a request id
// is the account's own.
func TestGafferTemplateRequestIsAnsweredFromItsRecord(t *testing.T) {
	s, uid, other := templateStore(t)
	id, request := hexID('a', 0), hexID('1', 1)
	first, err := s.SaveGafferTemplateFor(uid, templateVersion(id, request, "First"), 0)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.SaveGafferTemplateFor(uid, templateVersion(hexID('b', 0), request, "First"), 0)
	if !errors.Is(err, ErrGafferTemplateRepeat) || again.ID != id || again.Version != first.Version {
		t.Fatalf("repeat: %+v %v", again, err)
	}
	if rows, _ := s.GafferTemplatesFor(uid); len(rows) != 1 {
		t.Fatalf("a repeat made a second template: %d", len(rows))
	}
	if _, err = s.SaveGafferTemplateFor(other, templateVersion(hexID('c', 0), request, "Theirs"), 0); err != nil {
		t.Fatalf("another account's request id: %v", err)
	}
}

// Two saves on the same version: one lands and the other is told it changed.
func TestConcurrentGafferTemplateSavesLeaveOneVersion(t *testing.T) {
	s, uid, _ := templateStore(t)
	id := hexID('a', 0)
	if _, err := s.SaveGafferTemplateFor(uid, templateVersion(id, hexID('1', 0), "First"), 0); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.SaveGafferTemplateFor(uid, templateVersion(id, hexID('2', i), "Next"), 1)
		}(i)
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrGafferTemplateChanged):
			t.Fatalf("a losing save: %v", err)
		}
	}
	if versions, _ := s.GafferTemplateVersionsFor(uid, id); won != 1 || len(versions) != 2 {
		t.Fatalf("won %d, versions %v", won, versions)
	}
}

// Templates per account, versions kept and bytes per version are bounded.
func TestGafferTemplatesAreBounded(t *testing.T) {
	s, uid, _ := templateStore(t)
	for i := 0; i < MaxGafferTemplates; i++ {
		id := strings.Repeat("a", 30) + string(rune('a'+i/10)) + string(rune('0'+i%10))
		if _, err := s.SaveGafferTemplateFor(uid, templateVersion(id, strings.Repeat("1", 30)+string(rune('a'+i/10))+string(rune('0'+i%10)), "T"), 0); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := s.SaveGafferTemplateFor(uid, templateVersion(hexID('f', 0), hexID('f', 1), "One too many"), 0); !errors.Is(err, ErrGafferTemplateLimit) {
		t.Fatalf("past the account's limit: %v", err)
	}
	id := strings.Repeat("a", 30) + "a0"
	for v := 1; v <= MaxGafferTemplateVersions+2; v++ {
		if _, err := s.SaveGafferTemplateFor(uid, templateVersion(id, strings.Repeat("2", 28)+string(rune('a'+v/10))+string(rune('0'+v%10))+"00", "V"), v); err != nil {
			t.Fatal(v, err)
		}
	}
	versions, err := s.GafferTemplateVersionsFor(uid, id)
	if err != nil || len(versions) != MaxGafferTemplateVersions || versions[len(versions)-1] != MaxGafferTemplateVersions+3 || versions[0] != 4 {
		t.Fatalf("kept versions: %v %v", versions, err)
	}
	big := templateVersion(id, hexID('9', 9), "Big")
	big.Content.Description = strings.Repeat("x", MaxGafferTemplateBytes)
	if _, err = s.SaveGafferTemplateFor(uid, big, MaxGafferTemplateVersions+3); !errors.Is(err, ErrGafferTemplateTooLarge) {
		t.Fatalf("too large: %v", err)
	}
}

// An outcome saved before templates has no template fields and reads as before.
func TestAnOutcomeWithoutATemplateStillReads(t *testing.T) {
	s, uid, _ := templateStore(t)
	id := strings.Repeat("a", 32)
	old := `{"id":"` + id + `","revision":1,"objective":"Old outcome","state":"settled","sessions":[],"createdAt":1,"updatedAt":1}`
	if _, err := s.db.Exec(`INSERT INTO gaffer_outcomes(id,user_id,created_at,data) VALUES(?,?,?,?)`, id, uid, 1, old); err != nil {
		t.Fatal(err)
	}
	o, err := s.GafferOutcomeFor(uid, id)
	if err != nil || o.TemplateID != "" || o.TemplateVersion != 0 {
		t.Fatalf("old outcome: %+v %v", o, err)
	}
}

// A request id is found by the account that used it, and only by that account.
func TestGafferTemplateIsFoundByItsRequest(t *testing.T) {
	s, uid, other := templateStore(t)
	saved, err := s.SaveGafferTemplateFor(uid, templateVersion(hexID('a', 0), hexID('1', 1), "First"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := s.GafferTemplateByRequestFor(uid, hexID('1', 1)); err != nil || found.ID != saved.ID || found.Version != 1 || found.Content.Name != "First" {
		t.Fatalf("by request: %+v %v", found, err)
	}
	if _, err := s.GafferTemplateByRequestFor(other, hexID('1', 1)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another account: %v", err)
	}
	if _, err := s.GafferTemplateByRequestFor(uid, hexID('1', 2)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unused id: %v", err)
	}
}
