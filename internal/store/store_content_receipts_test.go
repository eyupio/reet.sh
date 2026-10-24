package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillplan"
	"github.com/eyupio/reet.sh/internal/skillpreview"
)

const receiptRoot = "d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0"

// receiptManaged is a valid managed subset of the fixture's private revision.
func receiptManaged(t *testing.T, e previewEnv, paths ...string) skillbundle.Manifest {
	t.Helper()
	files := []skillbundle.File{}
	for _, f := range e.bundle.Revision.Manifest.Files {
		for _, p := range paths {
			if f.Path == p {
				files = append(files, f)
			}
		}
	}
	m, err := skillbundle.FromMetadata(files)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// seedReceipt inserts an adopted receipt directly. Adoption itself is a later
// task; storage, reads and release must stand on their own.
func seedReceipt(t *testing.T, e previewEnv, revisionID string, managed skillbundle.Manifest, rootID string) SkillContentReceipt {
	t.Helper()
	raw, err := json.Marshal(managed)
	if err != nil {
		t.Fatal(err)
	}
	evidence := SkillContentReceiptEvidence{PreviewID: syncID(), ReviewHash: strings.Repeat("a", 64), CaptureHash: strings.Repeat("b", 64), ObservedDigest: strings.Repeat("c", 64), PinID: e.pin.ID, PinVersion: e.pin.Version, CapturedAt: now() - 1000, AdoptedAt: now(), AdoptedBy: e.uid}
	evidenceRaw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	r := SkillContentReceipt{ID: syncID(), EnvironmentID: e.write.EnvironmentID, AgentID: e.agent.ID, Name: e.pin.Name, BundleID: e.pin.BundleID, RevisionID: revisionID, ManagedDigest: managed.Digest, FileCount: len(managed.Files), Provenance: "adopted", Verification: "controller_attested", Evidence: evidence, Version: syncID(), CreatedAt: now(), UpdatedAt: now()}
	if _, err = e.s.db.Exec(`INSERT INTO skill_content_receipts(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,root_id,adapter,schema_version,managed,provenance,evidence,request_key,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, e.uid, r.EnvironmentID, r.AgentID, r.Name, r.BundleID, r.RevisionID, rootID, skillpreview.Adapter, 1, string(raw), "adopted", string(evidenceRaw), "seed_"+r.ID, r.Version, r.CreatedAt, r.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	return r
}

func receiptRows(t *testing.T, e previewEnv) int {
	t.Helper()
	var n int
	if err := e.s.db.QueryRow(`SELECT count(*) FROM skill_content_receipts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReceiptReadsAreMetadataOnlyAndOwnerScoped(t *testing.T) {
	e := previewFixture(t)
	if none, err := e.s.SkillContentReceiptsFor(e.uid); err != nil || none == nil || len(none) != 0 {
		t.Fatal("an account without receipts lists an empty, non-nil slice", none, err)
	}
	managed := receiptManaged(t, e, "SKILL.md", "scripts/run.sh")
	seeded := seedReceipt(t, e, e.pin.RevisionID, managed, receiptRoot)

	list, err := e.s.SkillContentReceiptsFor(e.uid)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	if list[0].Managed != nil {
		t.Fatal("a summary must not carry the managed files")
	}
	if !reflect.DeepEqual(list[0], seeded) {
		t.Fatalf("summary %+v, want %+v", list[0], seeded)
	}
	if list[0].ManagedDigest != managed.Digest || list[0].FileCount != 2 || list[0].Provenance != "adopted" || list[0].Verification != "controller_attested" {
		t.Fatalf("%+v", list[0])
	}

	item, err := e.s.SkillContentReceiptFor(e.uid, seeded.ID)
	if err != nil || item.Managed == nil {
		t.Fatal(item, err)
	}
	want := seeded
	want.Managed = &managed
	if !reflect.DeepEqual(item, want) {
		t.Fatalf("item %+v, want %+v", item, want)
	}

	for name, v := range map[string]any{"list": list, "item": item} {
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), `"data"`) {
			t.Fatalf("%s carries file bytes: %s", name, raw)
		}
		if strings.Contains(string(raw), receiptRoot) {
			t.Fatalf("%s leaks the physical root identity: %s", name, raw)
		}
	}
	raw, _ := json.Marshal(list[0])
	if strings.Contains(string(raw), `"managed"`) {
		t.Fatal("a summary has no managed key", raw)
	}

	for _, id := range []string{"", "not-an-id", strings.Repeat("0", 48)} {
		if _, err = e.s.SkillContentReceiptFor(e.uid, id); !errors.Is(err, ErrSkillContentReceiptMissing) {
			t.Fatalf("id %q: %v", id, err)
		}
	}

	other, err := e.s.CreateUser("receipt-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.s.SkillContentReceiptFor(other.ID, seeded.ID); !errors.Is(err, ErrSkillContentReceiptMissing) || got.ID != "" {
		t.Fatal("a foreign account read the receipt", got, err)
	}
	if all, err := e.s.SkillContentReceiptsFor(other.ID); err != nil || len(all) != 0 {
		t.Fatal("a foreign account listed the receipt", all, err)
	}

	for name, demote := range map[string]string{
		"viewer":   `UPDATE users SET role='viewer' WHERE id=?`,
		"disabled": `UPDATE users SET disabled_at=1 WHERE id=?`,
	} {
		t.Run(name, func(t *testing.T) {
			f := previewFixture(t)
			r := seedReceipt(t, f, f.pin.RevisionID, receiptManaged(t, f, "SKILL.md"), receiptRoot)
			if _, err := f.s.db.Exec(demote, f.uid); err != nil {
				t.Fatal(err)
			}
			if got, err := f.s.SkillContentReceiptFor(f.uid, r.ID); !errors.Is(err, ErrSkillContentReceiptMissing) || got.ID != "" {
				t.Fatal("receipt visible to an account that cannot use private skills", got, err)
			}
			if all, err := f.s.SkillContentReceiptsFor(f.uid); err != nil || len(all) != 0 {
				t.Fatal("receipts listed for an account that cannot use private skills", all, err)
			}
		})
	}
}

func TestReceiptsListInSlotOrder(t *testing.T) {
	e := previewFixture(t)
	later := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md"), receiptRoot)
	if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET name='zeta' WHERE id=?`, later.ID); err != nil {
		t.Fatal(err)
	}
	earlier := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md"), receiptRoot)
	list, err := e.s.SkillContentReceiptsFor(e.uid)
	if err != nil || len(list) != 2 || list[0].ID != earlier.ID || list[1].ID != later.ID || list[1].Name != "zeta" {
		t.Fatal("receipts must list by environment, name, then ID", list, err)
	}
}

func TestReceiptIntegrityOnRead(t *testing.T) {
	zeros := strings.Repeat("0", 64)
	rewrite := func(t *testing.T, e previewEnv, id string, managed skillbundle.Manifest) {
		t.Helper()
		raw, _ := json.Marshal(managed)
		if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET managed=? WHERE id=?`, string(raw), id); err != nil {
			t.Fatal(err)
		}
	}
	consistent := func(t *testing.T, files []skillbundle.File) skillbundle.Manifest {
		t.Helper()
		m, err := skillbundle.FromMetadata(files)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	many := []skillbundle.File{}
	for i := 0; i < 257; i++ {
		many = append(many, skillbundle.File{Path: fmt.Sprintf("f%03d", i), Bytes: 1, Digest: zeros})
	}
	many[0].Path = "SKILL.md"

	cases := map[string]func(*testing.T, previewEnv, SkillContentReceipt, skillbundle.Manifest){
		"extra path with stale identity": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			m.Files = append(append([]skillbundle.File{}, m.Files...), skillbundle.File{Path: "extra.txt", Bytes: 1, Digest: zeros})
			rewrite(t, e, r.ID, m)
		},
		"extra path the revision does not hold, otherwise consistent": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			rewrite(t, e, r.ID, consistent(t, append(append([]skillbundle.File{}, m.Files...), skillbundle.File{Path: "extra.txt", Bytes: 1, Digest: zeros})))
		},
		"file digest differs from the revision, otherwise consistent": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			files := append([]skillbundle.File{}, m.Files...)
			files[len(files)-1].Digest = zeros
			rewrite(t, e, r.ID, consistent(t, files))
		},
		"executable bit differs from the revision, otherwise consistent": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			files := append([]skillbundle.File{}, m.Files...)
			files[len(files)-1].Executable = !files[len(files)-1].Executable
			rewrite(t, e, r.ID, consistent(t, files))
		},
		"changed digest": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET managed=json_set(managed,'$.digest',?) WHERE id=?`, zeros, r.ID); err != nil {
				t.Fatal(err)
			}
		},
		"changed total": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET managed=json_set(managed,'$.totalBytes',?) WHERE id=?`, m.TotalBytes+1, r.ID); err != nil {
				t.Fatal(err)
			}
		},
		"marked incomplete": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			m.Complete = false
			rewrite(t, e, r.ID, m)
		},
		"carries issues": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			m.Issues = []skillbundle.Issue{{Path: "x", Code: "unreadable"}}
			rewrite(t, e, r.ID, m)
		},
		"unknown field": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET managed=json_set(managed,'$.note','hello') WHERE id=?`, r.ID); err != nil {
				t.Fatal(err)
			}
		},
		"empty manifest (a vacuous subset)": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			rewrite(t, e, r.ID, skillbundle.Manifest{SchemaVersion: skillbundle.SchemaVersion, Algorithm: skillbundle.Algorithm, ModeSemantics: skillbundle.ModeSemantics, Complete: true, Files: []skillbundle.File{}})
		},
		"SKILL.md not managed": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			m.Files = []skillbundle.File{m.Files[len(m.Files)-1]}
			rewrite(t, e, r.ID, m)
		},
		"257 files": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			m.Files = many
			rewrite(t, e, r.ID, m)
		},
		"not JSON": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET managed='not json' WHERE id=?`, r.ID); err != nil {
				t.Fatal(err)
			}
		},
		"trailing data": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET managed=managed||' {}' WHERE id=?`, r.ID); err != nil {
				t.Fatal(err)
			}
		},
		"revision no longer holds the files": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			next, err := e.s.SavePrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, privateWrite("receipt_other_revision1", e.bundle.Revision.ID, "DIFFERENT_BYTES"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = e.s.db.Exec(`UPDATE skill_content_receipts SET revision_id=? WHERE id=?`, next.Revision.ID, r.ID); err != nil {
				t.Fatal(err)
			}
		},
		"provenance outside the adopted vocabulary": func(t *testing.T, e previewEnv, r SkillContentReceipt, m skillbundle.Manifest) {
			if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET provenance='applied' WHERE id=?`, r.ID); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			e := previewFixture(t)
			managed := receiptManaged(t, e, "SKILL.md", "scripts/run.sh")
			r := seedReceipt(t, e, e.pin.RevisionID, managed, receiptRoot)
			if _, err := e.s.SkillContentReceiptFor(e.uid, r.ID); err != nil {
				t.Fatal("a clean receipt must read:", err)
			}
			if b, v, err := skillContentReceiptBaseline(e.s.db, e.uid, r.EnvironmentID, r.Name); err != nil || b == nil || v != r.Version {
				t.Fatal("a clean receipt must serve as a baseline:", b, v, err)
			}
			tamper(t, e, r, managed)

			got, err := e.s.SkillContentReceiptFor(e.uid, r.ID)
			if err == nil {
				t.Fatalf("damaged receipt accepted: %+v", got)
			}
			if got.Managed != nil {
				t.Fatal("a damaged receipt leaked its manifest")
			}
			for _, sentinel := range []error{ErrSkillContentReceiptInvalid, ErrSkillContentReceiptMissing, ErrSkillContentReceipt, ErrSkillContentReceiptCapacity, ErrPrivateSkillMissing} {
				if errors.Is(err, sentinel) {
					t.Fatalf("an integrity failure must not look like %q: %v", sentinel, err)
				}
			}
			if err.Error() != "stored ownership receipt failed integrity validation" {
				t.Fatal(err)
			}
			if b, v, err := skillContentReceiptBaseline(e.s.db, e.uid, r.EnvironmentID, r.Name); err == nil || b != nil || v != "" {
				t.Fatal("a damaged receipt served as a baseline", b, v, err)
			}
		})
	}
}

func TestReceiptListParsesManagedOnlyToSummarise(t *testing.T) {
	e := previewFixture(t)
	r := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md", "scripts/run.sh"), receiptRoot)
	// The comparison with the revision belongs to item reads and baseline use.
	if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET managed=json_remove(managed,'$.files[1]') WHERE id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	list, err := e.s.SkillContentReceiptsFor(e.uid)
	if err != nil || len(list) != 1 || list[0].FileCount != 1 {
		t.Fatal(list, err)
	}
	if _, err = e.s.db.Exec(`UPDATE skill_content_receipts SET managed='not json' WHERE id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if list, err = e.s.SkillContentReceiptsFor(e.uid); err == nil {
		t.Fatal("an unreadable receipt must not be summarised quietly", list)
	}
}

func TestReleaseRequiresCurrentVersion(t *testing.T) {
	e := previewFixture(t)
	r := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md"), receiptRoot)

	if err := e.s.ReleaseSkillContentReceiptFor(e.uid, r.ID, strings.Repeat("e", 48)); !errors.Is(err, ErrSkillContentReceipt) {
		t.Fatal("a stale version released the receipt:", err)
	}
	if receiptRows(t, e) != 1 {
		t.Fatal("a refused release removed the receipt")
	}
	for _, bad := range []struct{ id, version string }{{r.ID, ""}, {r.ID, "latest"}, {"nope", r.Version}} {
		if err := e.s.ReleaseSkillContentReceiptFor(e.uid, bad.id, bad.version); !errors.Is(err, ErrSkillContentReceiptInvalid) {
			t.Fatalf("%+v: %v", bad, err)
		}
	}

	other, err := e.s.CreateUser("receipt-release-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.s.ReleaseSkillContentReceiptFor(other.ID, r.ID, r.Version); !errors.Is(err, ErrSkillContentReceiptMissing) {
		t.Fatal("a foreign account released the receipt:", err)
	}
	if receiptRows(t, e) != 1 {
		t.Fatal("a foreign release removed the receipt")
	}

	if err = e.s.ReleaseSkillContentReceiptFor(e.uid, r.ID, r.Version); err != nil {
		t.Fatal(err)
	}
	if receiptRows(t, e) != 0 {
		t.Fatal("release left the receipt behind")
	}
	if err = e.s.ReleaseSkillContentReceiptFor(e.uid, r.ID, r.Version); !errors.Is(err, ErrSkillContentReceiptMissing) {
		t.Fatal("a repeated release must report the receipt as gone:", err)
	}
	got, err := e.s.SkillContentTargetFor(e.uid, e.pin.ID)
	if err != nil || !reflect.DeepEqual(got, e.pin) {
		t.Fatal("release changed the pin", got, err)
	}
	if _, err = e.s.PrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, e.bundle.Revision.ID); err != nil {
		t.Fatal("release removed private bytes", err)
	}
}

func TestReceiptReleaseIsRefusedForAccountsThatCannotWrite(t *testing.T) {
	for name, demote := range map[string]string{
		"viewer":   `UPDATE users SET role='viewer' WHERE id=?`,
		"disabled": `UPDATE users SET disabled_at=1 WHERE id=?`,
	} {
		t.Run(name, func(t *testing.T) {
			e := previewFixture(t)
			r := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md"), receiptRoot)
			if _, err := e.s.db.Exec(demote, e.uid); err != nil {
				t.Fatal(err)
			}
			if err := e.s.ReleaseSkillContentReceiptFor(e.uid, r.ID, r.Version); !errors.Is(err, ErrSkillContentReceiptMissing) {
				t.Fatal(err)
			}
			if receiptRows(t, e) != 1 {
				t.Fatal("an account that cannot write released ownership")
			}
		})
	}
}

func TestReceiptCascades(t *testing.T) {
	for name, remove := range map[string]struct {
		change func(*testing.T, previewEnv)
		gone   bool
	}{
		"revoked enrolment": {func(t *testing.T, e previewEnv) {
			if err := e.s.ControlSyncAgent(e.uid, e.agent.ID, "revoke"); err != nil {
				t.Fatal(err)
			}
		}, true},
		"forgotten environment": {func(t *testing.T, e previewEnv) {
			if err := e.s.ForgetEnvironmentFor(e.uid, e.write.EnvironmentID); err != nil {
				t.Fatal(err)
			}
		}, true},
		"removed pin": {func(t *testing.T, e previewEnv) {
			if err := e.s.DeleteSkillContentTargetFor(e.uid, e.pin.ID, e.pin.Version); err != nil {
				t.Fatal(err)
			}
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			e := previewFixture(t)
			r := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md"), receiptRoot)
			remove.change(t, e)
			n := receiptRows(t, e)
			if remove.gone && n != 0 {
				t.Fatal("ownership survived", name)
			}
			if !remove.gone {
				if n != 1 {
					t.Fatal("ownership must not depend on the pin", name)
				}
				if _, err := e.s.SkillContentReceiptFor(e.uid, r.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.s.PrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, e.bundle.Revision.ID); err != nil {
				t.Fatal("removal deleted private bytes", err)
			}
		})
	}
}

func TestReceiptBaselineLookup(t *testing.T) {
	e := previewFixture(t)
	if b, v, err := skillContentReceiptBaseline(e.s.db, e.uid, e.write.EnvironmentID, "demo"); err != nil || b != nil || v != "" {
		t.Fatal("no receipt means no baseline", b, v, err)
	}
	if v, err := currentReceiptVersion(e.s.db, e.uid, e.write.EnvironmentID, "demo"); err != nil || v != "" {
		t.Fatal("no receipt means an empty version", v, err)
	}

	managed := receiptManaged(t, e, "SKILL.md", "assets/empty")
	r := seedReceipt(t, e, e.pin.RevisionID, managed, receiptRoot)

	b, version, err := skillContentReceiptBaseline(e.s.db, e.uid, r.EnvironmentID, r.Name)
	if err != nil || b == nil {
		t.Fatal(b, err)
	}
	wantTarget := skillplan.Target{OwnerID: e.uid, AgentID: r.AgentID, EnvironmentID: r.EnvironmentID, Generation: r.AgentID, RootID: receiptRoot, SkillName: r.Name, Adapter: skillpreview.Adapter, Schema: 1}
	if b.Target != wantTarget || b.Target.Generation != b.Target.AgentID {
		t.Fatalf("target %+v, want %+v", b.Target, wantTarget)
	}
	if b.BundleID != r.BundleID || b.RevisionID != r.RevisionID || !reflect.DeepEqual(b.Manifest, managed) || version != r.Version {
		t.Fatalf("%+v %q", b, version)
	}
	if v, err := currentReceiptVersion(e.s.db, e.uid, r.EnvironmentID, r.Name); err != nil || v != r.Version {
		t.Fatal(v, err)
	}
	if v, err := currentReceiptVersion(e.s.db, e.uid, r.EnvironmentID, "another-skill"); err != nil || v != "" {
		t.Fatal("another slot has no receipt", v, err)
	}
	if v, err := currentReceiptVersion(e.s.db, e.uid+1, r.EnvironmentID, r.Name); err != nil || v != "" {
		t.Fatal("another account has no receipt in this slot", v, err)
	}
	if b, v, err := skillContentReceiptBaseline(e.s.db, e.uid+1, r.EnvironmentID, r.Name); err != nil || b != nil || v != "" {
		t.Fatal("another account has no baseline in this slot", b, v, err)
	}

	// The same lookups work inside the write transaction that builds a plan.
	err = e.s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, e.uid); err != nil {
			return err
		}
		inTx, v, err := skillContentReceiptBaseline(tx, e.uid, r.EnvironmentID, r.Name)
		if err != nil || inTx == nil || v != r.Version || !reflect.DeepEqual(inTx, b) {
			t.Fatal(inTx, v, err)
		}
		if v, err = currentReceiptVersion(tx, e.uid, r.EnvironmentID, r.Name); err != nil || v != r.Version {
			t.Fatal(v, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReceiptMigrationIsIdempotentAcrossReopen(t *testing.T) {
	e := previewFixture(t)
	r := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md"), receiptRoot)

	columns := func(s *Store) []string {
		rows, err := s.db.Query(`SELECT name FROM pragma_table_info('skill_content_receipts') ORDER BY cid`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var c string
			if err = rows.Scan(&c); err != nil {
				t.Fatal(err)
			}
			out = append(out, c)
		}
		return out
	}
	before := columns(e.s)
	wantColumns := []string{"id", "user_id", "environment_id", "agent_id", "name", "bundle_id", "revision_id", "root_id", "adapter", "schema_version", "managed", "provenance", "evidence", "request_key", "version", "created_at", "updated_at"}
	if !reflect.DeepEqual(before, wantColumns) {
		t.Fatalf("columns %v, want %v", before, wantColumns)
	}

	dir := e.s.dir
	if err := e.s.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		reopened, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := columns(reopened); !reflect.DeepEqual(got, wantColumns) {
			t.Fatal("reopen changed the table", got)
		}
		got, err := reopened.SkillContentReceiptFor(e.uid, r.ID)
		if err != nil || got.Version != r.Version || got.Managed == nil {
			t.Fatal("reopen lost the receipt", got, err)
		}
		if i == 1 {
			// One slot holds one receipt, and a request key names one adoption.
			dup := func(key, name string) error {
				_, err := reopened.db.Exec(`INSERT INTO skill_content_receipts SELECT ?,user_id,environment_id,agent_id,?,bundle_id,revision_id,root_id,adapter,schema_version,managed,provenance,evidence,?,version,created_at,updated_at FROM skill_content_receipts WHERE id=?`, syncID(), name, key, r.ID)
				return err
			}
			if err = dup("another_request_key", r.Name); err == nil {
				t.Fatal("a slot may hold only one receipt")
			}
			if err = dup("seed_"+r.ID, "another-skill"); err == nil {
				t.Fatal("a request key may name only one receipt per account")
			}
			if err = dup("fresh_request_key", "another-skill"); err != nil {
				t.Fatal(err)
			}
			var onDelete int
			if err = reopened.db.QueryRow(`SELECT count(DISTINCT id) FROM pragma_foreign_key_list('skill_content_receipts') WHERE on_delete='CASCADE'`).Scan(&onDelete); err != nil || onDelete != 4 {
				t.Fatalf("expected the user, environment, enrolment and revision keys to cascade, got %d (%v)", onDelete, err)
			}
		}
		if err = reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
