package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func privateWrite(key, base, text string) PrivateSkillWrite {
	return PrivateSkillWrite{RequestID: key, ExpectedBase: &base, Name: "demo", Files: []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\r\nname: demo\r\ndescription: Private example\r\n---\r\n" + text + "\r\n")}, {Path: "scripts/run.sh", Data: []byte("#!/bin/sh\nprintf exact\n"), Executable: true}, {Path: "assets/empty", Data: []byte{}}}}
}
func privateCounts(t *testing.T, s *Store) []int {
	t.Helper()
	var out []int
	for _, table := range []string{"private_skill_bundles", "private_skill_revisions", "private_skill_blobs", "private_skill_revision_files"} {
		var n int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}
func requirePrivateFiles(t *testing.T, want []skillbundle.ContentFile, got PrivateSkillRevision) {
	t.Helper()
	if len(want) != len(got.Files) {
		t.Fatal("file count", got)
	}
	byPath := map[string]skillbundle.ContentFile{}
	for _, f := range got.Files {
		byPath[f.Path] = f
	}
	for _, f := range want {
		actual, ok := byPath[f.Path]
		if !ok || !bytes.Equal(actual.Data, f.Data) || actual.Executable != f.Executable {
			t.Fatal("exact file mismatch", f.Path)
		}
	}
}

func TestPrivateSkillsImmutableRetryAndOwnerScopedContents(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	other, err := s.CreateUser("private-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	in := privateWrite("first_request_0001", "", "PRIVATE_EXACT_BYTES")
	first, err := s.SavePrivateSkillRevisionFor(uid, "", in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Bundle.Head != first.Revision.ID || first.Revision.ID == first.Revision.ManifestDigest || first.Revision.Manifest.SchemaVersion != skillbundle.SchemaVersion {
		t.Fatal(first)
	}
	got, err := s.PrivateSkillRevisionFor(uid, first.Bundle.ID, first.Revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	requirePrivateFiles(t, in.Files, got)
	expected, err := skillbundle.FromContents(in.Files)
	if err != nil || !reflect.DeepEqual(*got.Manifest, expected) {
		t.Fatal("manifest mismatch", err)
	}
	for _, id := range []string{first.Revision.ID, first.Revision.ManifestDigest, strings.Repeat("f", 32)} {
		if _, err = s.PrivateSkillRevisionFor(other.ID, first.Bundle.ID, id); !errors.Is(err, ErrPrivateSkillMissing) {
			t.Fatal("foreign/guessed revision", err)
		}
	}
	if _, err = s.PrivateSkillBundleFor(other.ID, first.Bundle.ID); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("foreign bundle", err)
	}
	if rows, err := s.PrivateSkillBundlesFor(other.ID); err != nil || len(rows) != 0 {
		t.Fatal("foreign list", rows, err)
	}
	secondIn := privateWrite("second_request_0002", first.Revision.ID, "CHANGED")
	second, err := s.SavePrivateSkillRevisionFor(uid, first.Bundle.ID, secondIn)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision.Parent != first.Revision.ID || second.Revision.ID == first.Revision.ID {
		t.Fatal(second)
	}
	before := privateCounts(t, s)
	retry, err := s.SavePrivateSkillRevisionFor(uid, "", in)
	if err != nil || !reflect.DeepEqual(first, retry) || !reflect.DeepEqual(before, privateCounts(t, s)) {
		t.Fatal("retry receipt changed", first, retry, err)
	}
	current, err := s.PrivateSkillBundleFor(uid, first.Bundle.ID)
	if err != nil || current.Head != second.Revision.ID {
		t.Fatal("retry rewound head", current, err)
	}
	stale := privateWrite("stale_request_0003", first.Revision.ID, "STALE")
	if _, err = s.SavePrivateSkillRevisionFor(uid, first.Bundle.ID, stale); !errors.Is(err, ErrPrivateSkillConflict) {
		t.Fatal("stale accepted", err)
	}
	changed := privateWrite(in.RequestID, "", "DIFFERENT")
	if _, err = s.SavePrivateSkillRevisionFor(uid, "", changed); !errors.Is(err, ErrPrivateSkillConflict) {
		t.Fatal("changed retry accepted", err)
	}
	foreign := privateWrite("foreign_request_01", second.Revision.ID, "FOREIGN")
	if _, err = s.SavePrivateSkillRevisionFor(other.ID, first.Bundle.ID, foreign); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("foreign write", err)
	}
	fork := privateWrite("foreign_fork_0001", "", "FORK")
	fork.Origin = PrivateSkillOrigin{Kind: "fork", DerivedFrom: first.Revision.ID}
	if _, err = s.SavePrivateSkillRevisionFor(other.ID, "", fork); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("foreign derived reference", err)
	}
	// Identical bytes remain independently owned, including physical blob rows.
	copyResult, err := s.SavePrivateSkillRevisionFor(other.ID, "", in)
	if err != nil || copyResult.Revision.ManifestDigest != first.Revision.ManifestDigest || copyResult.Bundle.ID == first.Bundle.ID {
		t.Fatal(copyResult, err)
	}
	var owners int
	if err = s.db.QueryRow(`SELECT count(DISTINCT user_id) FROM private_skill_blobs WHERE digest=?`, expected.Files[0].Digest).Scan(&owners); err != nil || owners != 2 {
		t.Fatal("cross-owner blob reuse", owners, err)
	}
	old, err := s.PrivateSkillRevisionFor(uid, first.Bundle.ID, first.Revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	requirePrivateFiles(t, in.Files, old)
	if _, err = s.db.Exec(`UPDATE private_skill_blobs SET data=x'ff' WHERE user_id=? AND digest=?`, uid, expected.Files[0].Digest); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PrivateSkillRevisionFor(uid, first.Bundle.ID, first.Revision.ID); err == nil {
		t.Fatal("stored corruption returned as valid")
	}
	if _, err = s.PrivateSkillRevisionFor(other.ID, copyResult.Bundle.ID, copyResult.Revision.ID); err != nil {
		t.Fatal("other owner affected by corruption", err)
	}
}

func TestPrivateSkillsConcurrentHeadAndIdempotentCreate(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	second, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	in := privateWrite("concurrent_create_1", "", "BASE")
	results := make(chan PrivateSkillResult, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for _, db := range []*Store{s, second} {
		wg.Add(1)
		go func(db *Store) {
			defer wg.Done()
			r, e := db.SavePrivateSkillRevisionFor(uid, "", in)
			results <- r
			failures <- e
		}(db)
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first PrivateSkillResult
	for r := range results {
		if first.Bundle.ID == "" {
			first = r
		} else if !reflect.DeepEqual(first, r) {
			t.Fatal("duplicate create receipts", first, r)
		}
	}
	success, conflict := 0, 0
	errs := make(chan error, 2)
	for i, db := range []*Store{s, second} {
		wg.Add(1)
		go func(i int, db *Store) {
			defer wg.Done()
			_, e := db.SavePrivateSkillRevisionFor(uid, first.Bundle.ID, privateWrite(fmt.Sprintf("concurrent_edit_%02d", i), first.Revision.ID, fmt.Sprintf("EDIT%d", i)))
			errs <- e
		}(i, db)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrPrivateSkillConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	if rows, err := s.PrivateSkillRevisionsFor(uid, first.Bundle.ID); err != nil || len(rows) != 2 {
		t.Fatal("failed edit left rows", rows, err)
	}
}

func TestPrivateSkillsRollbackRolesAndBounds(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	in := privateWrite("rollback_create_01", "", "BASE")
	_, err := s.db.Exec(`CREATE TRIGGER fail_private_file BEFORE INSERT ON private_skill_revision_files WHEN NEW.path='z/fail.bin' BEGIN SELECT RAISE(ABORT,'injected interruption'); END`)
	if err != nil {
		t.Fatal(err)
	}
	in.Files = append(in.Files, skillbundle.ContentFile{Path: "z/fail.bin", Data: []byte("not committed")})
	before := privateCounts(t, s)
	if _, err = s.SavePrivateSkillRevisionFor(uid, "", in); err == nil {
		t.Fatal("injected failure missing")
	}
	if !reflect.DeepEqual(before, privateCounts(t, s)) {
		t.Fatal("interrupted create left data")
	}
	if _, err = s.db.Exec(`DROP TRIGGER fail_private_file`); err != nil {
		t.Fatal(err)
	}
	good := privateWrite("valid_create_00001", "", "BASE")
	first, err := s.SavePrivateSkillRevisionFor(uid, "", good)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER fail_private_file BEFORE INSERT ON private_skill_revision_files WHEN NEW.path='scripts/run.sh' BEGIN SELECT RAISE(ABORT,'injected interruption'); END`); err != nil {
		t.Fatal(err)
	}
	before = privateCounts(t, s)
	if _, err = s.SavePrivateSkillRevisionFor(uid, first.Bundle.ID, privateWrite("rollback_append_01", first.Revision.ID, "NEW")); err == nil {
		t.Fatal("injected failure missing")
	}
	head, _ := s.PrivateSkillBundleFor(uid, first.Bundle.ID)
	if head.Head != first.Revision.ID || !reflect.DeepEqual(before, privateCounts(t, s)) {
		t.Fatal("interrupted edit changed storage", head)
	}
	if _, err = s.db.Exec(`DROP TRIGGER fail_private_file`); err != nil {
		t.Fatal(err)
	}
	viewer, err := s.CreateUser("private-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePrivateSkillRevisionFor(viewer.ID, "", good); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("viewer write", err)
	}
	if err = s.UpdateUserRole(uid, "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PrivateSkillRevisionFor(uid, first.Bundle.ID, first.Revision.ID); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("downgraded read", err)
	}
	if err = s.UpdateUserRole(uid, "owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetUserDisabled(uid, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePrivateSkillRevisionFor(uid, "", good); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("disabled write", err)
	}
	if _, err = s.PrivateSkillBundleFor(uid, first.Bundle.ID); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("disabled read", err)
	}
}

func TestPrivateSkillsCapacityIsOwnedAndTransactional(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	// Existing account usage at the bound; SQL fixture avoids allocating 64 MiB in Go.
	if _, err := s.db.Exec(`INSERT INTO private_skill_blobs(user_id,digest,data) VALUES(?,?,zeroblob(?))`, uid, strings.Repeat("a", 64), MaxPrivateSkillBytes); err != nil {
		t.Fatal(err)
	}
	before := privateCounts(t, s)
	if _, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("quota_request_0001", "", "over quota")); !errors.Is(err, ErrPrivateSkillCapacity) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, privateCounts(t, s)) {
		t.Fatal("quota failure left partial rows")
	}
	other, err := s.CreateUser("quota-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePrivateSkillRevisionFor(other.ID, "", privateWrite("quota_request_0001", "", "own quota")); err != nil {
		t.Fatal("foreign quota affected account", err)
	}
}

func TestPrivateSkillsMigrationBackupsAndPublicationStaySeparate(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if err := s.ClaimPublishingUsername(uid, "publisher"); err != nil {
		t.Fatal(err)
	}
	published, err := s.SaveHostedSkill(uid, HostedSkill{Slug: "demo", Content: "---\nname: demo\ndescription: Public example\n---\nPublic instructions\n", Published: true})
	if err != nil {
		t.Fatal(err)
	}
	// Reopening a pre-feature schema must retain existing hosted content unchanged.
	for _, table := range []string{"private_skill_revision_files", "private_skill_revisions", "private_skill_blobs", "private_skill_bundles"} {
		if _, err = s.db.Exec("DROP TABLE " + table); err != nil {
			t.Fatal(err)
		}
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if items, err := s.PrivateSkillBundlesFor(uid); err != nil || len(items) != 0 {
		t.Fatal("invented observed content", items, err)
	}
	in := privateWrite("private_import_001", "", "PRIVATE_NOT_IN_SHELF_EXPORT")
	in.Origin = PrivateSkillOrigin{Kind: "fork", Reference: "publisher/demo"}
	receipt, err := s.SavePrivateSkillRevisionFor(uid, "", in)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.PublishedHostedSkill("publisher", "demo")
	if err != nil || !reflect.DeepEqual(published, after) {
		t.Fatal("private import changed publication", err)
	}
	snapshot, err := s.ExportFor(uid, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snapshot)
	if bytes.Contains(raw, []byte("PRIVATE_NOT_IN_SHELF_EXPORT")) {
		t.Fatal("legacy shelf export contains private payload")
	}
	backupDir := t.TempDir()
	if err = s.VacuumInto(context.Background(), filepath.Join(backupDir, "reet.db")); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.PrivateSkillRevisionFor(uid, receipt.Bundle.ID, receipt.Revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	requirePrivateFiles(t, in.Files, got)
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	head, err := again.PrivateSkillBundleFor(uid, receipt.Bundle.ID)
	if err != nil || head.Head != receipt.Revision.ID {
		t.Fatal("migration not idempotent", head, err)
	}
}

func TestPrivateSkillsValidateBeforePersistence(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	before := privateCounts(t, s)
	for _, tc := range []struct {
		name   string
		change func(*PrivateSkillWrite)
	}{
		{"request ID", func(in *PrivateSkillWrite) { in.RequestID = "short" }},
		{"explicit base", func(in *PrivateSkillWrite) { in.ExpectedBase = nil }},
		{"invalid base", func(in *PrivateSkillWrite) { base := "guessed-digest"; in.ExpectedBase = &base }},
		{"skill name", func(in *PrivateSkillWrite) { in.Name = "different" }},
		{"frontmatter", func(in *PrivateSkillWrite) { in.Files[0].Data = []byte("# No frontmatter") }},
		{"origin kind", func(in *PrivateSkillWrite) { in.Origin.Kind = "verified-publisher" }},
		{"origin controls", func(in *PrivateSkillWrite) { in.Origin.Reference = "bad\x00reference" }},
		{"origin size", func(in *PrivateSkillWrite) { in.Origin.Reference = strings.Repeat("x", 2049) }},
		{"path traversal", func(in *PrivateSkillWrite) { in.Files[1].Path = "../escape" }},
		{"markdown size", func(in *PrivateSkillWrite) {
			in.Files[0].Data = append(in.Files[0].Data, bytes.Repeat([]byte("x"), 2<<20)...)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := privateWrite("validation_req_001", "", "Instructions")
			tc.change(&in)
			if _, err := s.SavePrivateSkillRevisionFor(uid, "", in); !errors.Is(err, ErrPrivateSkillInvalid) {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, privateCounts(t, s)) {
				t.Fatal("invalid request changed storage")
			}
		})
	}
}
func TestPrivateSkillsBundleAndRevisionCountBounds(t *testing.T) {
	for _, bound := range []string{"bundles", "revisions"} {
		t.Run(bound, func(t *testing.T) {
			s := newStore(t)
			uid, _ := s.DefaultOwner()
			first, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("count_bound_first1", "", "Instructions"))
			if err != nil {
				t.Fatal(err)
			}
			if bound == "bundles" {
				_, err = s.db.Exec(`WITH RECURSIVE n(x) AS(SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<127) INSERT INTO private_skill_bundles(user_id,id,name,head,created_at) SELECT ?,printf('%032x',x),'existing','',1 FROM n`, uid)
			} else {
				_, err = s.db.Exec(`WITH RECURSIVE n(x) AS(SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<255) INSERT INTO private_skill_revisions(user_id,bundle_id,id,parent,request_id,fingerprint,manifest,manifest_digest,origin,created_at) SELECT ?,?,printf('%032x',x),'',printf('existing_request_%016x',x),'fixture','{}','fixture','{}',1 FROM n`, uid, first.Bundle.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := privateCounts(t, s)
			id, base := "", ""
			if bound == "revisions" {
				id = first.Bundle.ID
				base = first.Revision.ID
			}
			if _, err = s.SavePrivateSkillRevisionFor(uid, id, privateWrite("count_bound_next01", base, "New")); !errors.Is(err, ErrPrivateSkillCapacity) {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, privateCounts(t, s)) {
				t.Fatal("count rejection left data")
			}
			// The original nonce stays retryable even at capacity.
			if retry, e := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("count_bound_first1", "", "Instructions")); e != nil || retry.Revision.ID != first.Revision.ID {
				t.Fatal("quota prevented original receipt", e)
			}
		})
	}
}
func TestPrivateSkillsOwnedCopyAndAccountDeletion(t *testing.T) {
	s := newStore(t)
	user, err := s.CreateUser("bundle-delete@example.test", "Private owner", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	in := privateWrite("owned_copy_first01", "", "Exact")
	a, err := s.SavePrivateSkillRevisionFor(user.ID, "", in)
	if err != nil {
		t.Fatal(err)
	}
	before := privateCounts(t, s)
	in.RequestID = "owned_copy_second1"
	in.Origin = PrivateSkillOrigin{Kind: "fork", DerivedFrom: a.Revision.ID, Reference: "upstream/reference"}
	b, err := s.SavePrivateSkillRevisionFor(user.ID, "", in)
	if err != nil {
		t.Fatal(err)
	}
	if b.Bundle.ID == a.Bundle.ID || b.Revision.ID == a.Revision.ID || b.Revision.ManifestDigest != a.Revision.ManifestDigest {
		t.Fatal("copy conflates content and owned identity", a, b)
	}
	after := privateCounts(t, s)
	if after[2] != before[2] {
		t.Fatal("same-owner content not reused")
	}
	if _, err = s.db.Exec(`DELETE FROM users WHERE id=?`, user.ID); err != nil {
		t.Fatal(err)
	}
	if counts := privateCounts(t, s); !reflect.DeepEqual(counts, []int{0, 0, 0, 0}) {
		t.Fatal("account deletion retained private data", counts)
	}
}
