package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
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
	// The comparison with the revision belongs to item reads and baseline use, so a
	// manifest that parses but no longer matches its revision is still summarised
	// here, and is not flagged: the list never claims to have verified anything.
	if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET managed=json_remove(managed,'$.files[1]') WHERE id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	list, err := e.s.SkillContentReceiptsFor(e.uid)
	if err != nil || len(list) != 1 || list[0].FileCount != 1 || list[0].Damaged {
		t.Fatal(list, err)
	}
	if err = e.s.ReleaseSkillContentReceiptFor(e.uid, r.ID, list[0].Version); err != nil {
		t.Fatal(err)
	}
}

func TestReceiptListKeepsDamagedRowsReleasable(t *testing.T) {
	for name, damage := range map[string]string{
		"unreadable managed":                 `UPDATE skill_content_receipts SET managed='not json' WHERE id=?`,
		"managed with an unknown field":      `UPDATE skill_content_receipts SET managed=json_set(managed,'$.note','x') WHERE id=?`,
		"unreadable evidence":                `UPDATE skill_content_receipts SET evidence='not json' WHERE id=?`,
		"provenance outside the adopted set": `UPDATE skill_content_receipts SET provenance='applied' WHERE id=?`,
	} {
		t.Run(name, func(t *testing.T) {
			e := previewFixture(t)
			broken := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md"), receiptRoot)
			if _, err := e.s.db.Exec(`UPDATE skill_content_receipts SET name='zeta' WHERE id=?`, broken.ID); err != nil {
				t.Fatal(err)
			}
			healthy := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md", "scripts/run.sh"), receiptRoot)
			if _, err := e.s.db.Exec(damage, broken.ID); err != nil {
				t.Fatal(err)
			}

			// A damaged row is listed with only what identifies it and what releases it.
			list, err := e.s.SkillContentReceiptsFor(e.uid)
			if err != nil || len(list) != 2 {
				t.Fatal("one damaged row must not hide the others:", list, err)
			}
			wantBroken := SkillContentReceipt{ID: broken.ID, EnvironmentID: broken.EnvironmentID, AgentID: broken.AgentID, Name: "zeta", BundleID: broken.BundleID, RevisionID: broken.RevisionID, Version: broken.Version, CreatedAt: broken.CreatedAt, UpdatedAt: broken.UpdatedAt, Damaged: true}
			if !reflect.DeepEqual(list[0], healthy) || list[0].Damaged {
				t.Fatalf("healthy row %+v, want %+v", list[0], healthy)
			}
			if !reflect.DeepEqual(list[1], wantBroken) {
				t.Fatalf("damaged row %+v, want %+v", list[1], wantBroken)
			}
			for want, v := range map[bool]SkillContentReceipt{true: list[1], false: list[0]} {
				raw, _ := json.Marshal(v)
				if strings.Contains(string(raw), `"damaged":true`) != want {
					t.Fatalf("damaged flag on the wire: %s", raw)
				}
			}

			// Item reads and baseline use stay strict.
			if got, err := e.s.SkillContentReceiptFor(e.uid, broken.ID); err == nil || err.Error() != "stored ownership receipt failed integrity validation" || got.ID != "" {
				t.Fatal("a damaged receipt must still fail item reads:", got, err)
			}
			if b, v, err := skillContentReceiptBaseline(e.s.db, e.uid, broken.EnvironmentID, "zeta"); err == nil || b != nil || v != "" {
				t.Fatal("a damaged receipt must still fail as a baseline:", b, v, err)
			}

			// The listed version is enough to release it; a stale one is still refused.
			if err = e.s.ReleaseSkillContentReceiptFor(e.uid, broken.ID, strings.Repeat("e", 48)); !errors.Is(err, ErrSkillContentReceipt) {
				t.Fatal("a stale version released a damaged receipt:", err)
			}
			if receiptRows(t, e) != 2 {
				t.Fatal("a refused release removed a receipt")
			}
			if err = e.s.ReleaseSkillContentReceiptFor(e.uid, broken.ID, list[1].Version); err != nil {
				t.Fatal("a damaged receipt could not be released:", err)
			}
			if b, v, err := skillContentReceiptBaseline(e.s.db, e.uid, broken.EnvironmentID, "zeta"); err != nil || b != nil || v != "" {
				t.Fatal("the slot must be free after release", b, v, err)
			}
			if after, err := e.s.SkillContentReceiptsFor(e.uid); err != nil || len(after) != 1 || after[0].ID != healthy.ID {
				t.Fatal(after, err)
			}
		})
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
	other, err := e.s.CreateUser("receipt-baseline-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if v, err := currentReceiptVersion(e.s.db, other.ID, r.EnvironmentID, r.Name); err != nil || v != "" {
		t.Fatal("another account has no receipt in this slot", v, err)
	}
	if b, v, err := skillContentReceiptBaseline(e.s.db, other.ID, r.EnvironmentID, r.Name); err != nil || b != nil || v != "" {
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

func TestReceiptBaselineWithoutReadableRevisionIsAnIntegrityFailureNotMissing(t *testing.T) {
	// The receipt row exists, so a revision that cannot be read is damage or a
	// caller that skipped the account guard, never "no receipt". A foreign key
	// stops the revision row itself from vanishing, so the state reachable through
	// the real schema is an account that can no longer read private skills.
	e := previewFixture(t)
	r := seedReceipt(t, e, e.pin.RevisionID, receiptManaged(t, e, "SKILL.md"), receiptRoot)
	if _, err := e.s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, e.uid); err != nil {
		t.Fatal(err)
	}
	b, v, err := skillContentReceiptBaseline(e.s.db, e.uid, r.EnvironmentID, r.Name)
	if err == nil || b != nil || v != "" {
		t.Fatal(b, v, err)
	}
	for _, sentinel := range []error{ErrSkillContentReceiptInvalid, ErrSkillContentReceiptMissing, ErrSkillContentReceipt, ErrSkillContentReceiptCapacity, ErrPrivateSkillMissing} {
		if errors.Is(err, sentinel) {
			t.Fatalf("a baseline failure must not look like %q: %v", sentinel, err)
		}
	}
	if err.Error() != "stored ownership receipt failed integrity validation" {
		t.Fatal(err)
	}
	// An item read, which has already passed the account guard, still reports it as unavailable.
	if _, err = e.s.SkillContentReceiptFor(e.uid, r.ID); !errors.Is(err, ErrSkillContentReceiptMissing) {
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

// capturedPreview runs one request for the pinned slot and requires a current,
// captured review of it: the only kind of preview adoption accepts.
func capturedPreview(t *testing.T, e previewEnv, key string, c skillpreview.Capture) SkillContentPreview {
	t.Helper()
	p := e.preview(t, key, c)
	if p.State != "captured" || len(p.Reasons) != 0 || p.ReviewHash == "" || p.Plan == nil {
		t.Fatalf("expected a current captured review: %+v", p)
	}
	return p
}

// matching is a present, exclusive capture of exactly the fixture's first revision.
func matching(e previewEnv) skillpreview.Capture {
	return presentOf(*e.bundle.Revision.Manifest, "exclusive")
}

// adoption is the whole request: the preview, the review hash it showed and the
// revision to adopt the folder as. Nothing else is accepted from a caller.
func adoption(key string, p SkillContentPreview, revisionID string) SkillContentAdopt {
	return SkillContentAdopt{RequestKey: key, PreviewID: p.ID, ReviewHash: p.ReviewHash, AdoptAsRevisionID: revisionID}
}

// rowsOf reads one text column, so a test can compare a table before and after.
func rowsOf(t *testing.T, e previewEnv, query string) []string {
	t.Helper()
	rows, err := e.s.db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// fillReceipts records n receipts for other skill slots on the same machine.
func fillReceipts(t *testing.T, e previewEnv, n int) {
	t.Helper()
	managed, err := json.Marshal(receiptManaged(t, e, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	err = e.s.withTx(func(tx *sql.Tx) error {
		for i := 0; i < n; i++ {
			if _, err := tx.Exec(`INSERT INTO skill_content_receipts(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,root_id,adapter,schema_version,managed,provenance,evidence,request_key,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				syncID(), e.uid, e.write.EnvironmentID, e.agent.ID, fmt.Sprintf("filler-%04d", i), e.bundle.Bundle.ID, e.bundle.Revision.ID, receiptRoot, skillpreview.Adapter, 1, string(managed), "adopted", "{}", fmt.Sprintf("filler_request_%04d", i), syncID(), now(), now()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// secondBundle is another bundle with the same skill name and the same bytes as
// the fixture's revision, so only its bundle tells the two apart.
func secondBundle(t *testing.T, e previewEnv) PrivateSkillResult {
	t.Helper()
	second, err := e.s.SavePrivateSkillRevisionFor(e.uid, "", privateWrite("adopt_second_bundle_0001", "", "PRIVATE_PIN_BYTES"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Bundle.ID == e.bundle.Bundle.ID || second.Bundle.Name != e.pin.Name || second.Revision.ManifestDigest != e.bundle.Revision.ManifestDigest {
		t.Fatal("fixture needs a distinct bundle with the same name and bytes", second)
	}
	return second
}

func TestAdoptOlderRevisionThenUpgradePlan(t *testing.T) {
	e := previewFixture(t)
	r1 := e.bundle.Revision
	r2, err := e.s.SavePrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, privateWrite("adopt_upgrade_revision_2", r1.ID, "NEWER"))
	if err != nil {
		t.Fatal(err)
	}
	e = e.repin(t, func(in *SkillContentTargetWrite) { in.RevisionID = r2.Revision.ID })
	p := capturedPreview(t, e, "adopt_upgrade_preview_01", presentOf(*r1.Manifest, "exclusive"))
	if !reflect.DeepEqual(conflictCodes(p), []string{"adoption_required"}) {
		t.Fatalf("%v", conflictCodes(p))
	}
	if p.Adoption == nil || len(p.Adoption.Candidates) == 0 || p.Adoption.Candidates[0] != (AdoptionCandidate{RevisionID: r1.ID, Matched: 3}) {
		t.Fatalf("the revision the folder came from must be offered first: %+v", p.Adoption)
	}

	r, err := e.s.AdoptSkillContentPreviewFor(e.uid, adoption("adopt_upgrade_request_01", p, r1.ID))
	if err != nil {
		t.Fatal(err)
	}
	if r.Provenance != "adopted" || r.Verification != "controller_attested" || r.Damaged || r.FileCount != 3 || r.Managed == nil || !reflect.DeepEqual(*r.Managed, *r1.Manifest) || r.ManagedDigest != r1.ManifestDigest {
		t.Fatalf("%+v", r)
	}
	if r.EnvironmentID != e.pin.EnvironmentID || r.AgentID != e.pin.AgentID || r.Name != e.pin.Name || r.BundleID != e.pin.BundleID || r.RevisionID != r1.ID {
		t.Fatalf("the receipt must name the reviewed slot and the adopted revision: %+v", r)
	}
	if len(r.ID) != 48 || len(r.Version) != 48 || r.ID == r.Version {
		t.Fatalf("identity and version must be independent random IDs: %q %q", r.ID, r.Version)
	}
	wantEvidence := SkillContentReceiptEvidence{PreviewID: p.ID, ReviewHash: p.ReviewHash, CaptureHash: p.CaptureHash, ObservedDigest: r1.ManifestDigest, PinID: e.pin.ID, PinVersion: e.pin.Version, CapturedAt: p.CapturedAt, AdoptedAt: r.Evidence.AdoptedAt, AdoptedBy: e.uid}
	if r.Evidence != wantEvidence || r.Evidence.AdoptedAt < p.CapturedAt || r.CreatedAt != r.Evidence.AdoptedAt || r.UpdatedAt != r.CreatedAt {
		t.Fatalf("evidence %+v (created %d, updated %d), want %+v", r.Evidence, r.CreatedAt, r.UpdatedAt, wantEvidence)
	}
	if item, err := e.s.SkillContentReceiptFor(e.uid, r.ID); err != nil || !reflect.DeepEqual(item, r) {
		t.Fatalf("adoption must return what an item read returns: %+v %v", item, err)
	}
	baseline, version, err := skillContentReceiptBaseline(e.s.db, e.uid, r.EnvironmentID, r.Name)
	if err != nil || baseline == nil || version != r.Version || baseline.Target != p.Plan.Preconditions.Target {
		t.Fatalf("the receipt must rebuild exactly the reviewed target: %+v %q %v", baseline, version, err)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), `"data"`) || strings.Contains(string(raw), p.Capture.RootID) {
		t.Fatalf("a receipt carries metadata only: %s", raw)
	}

	old := e.state(t, p.ID)
	if old.State != "superseded" || !reflect.DeepEqual(old.Reasons, []string{"receipt_changed"}) || old.Adoption != nil {
		t.Fatalf("the adopted preview must be consumed: %+v", old)
	}
	next := capturedPreview(t, e, "adopt_upgrade_preview_02", presentOf(*r1.Manifest, "exclusive"))
	if !next.Plan.Ready || !reflect.DeepEqual(actionList(next), []string{"update SKILL.md"}) || next.ReceiptVersion != r.Version || next.Adoption != nil {
		t.Fatalf("adopting the older revision must yield an upgrade to the pinned one: %+v", next.Plan)
	}
}

func TestAdoptCurrentRevisionIsANoOpPlan(t *testing.T) {
	e := previewFixture(t)
	p := capturedPreview(t, e, "adopt_current_preview_01", matching(e))
	if p.Adoption == nil || !reflect.DeepEqual(p.Adoption.Candidates, []AdoptionCandidate{{RevisionID: e.pin.RevisionID, Matched: 3}}) {
		t.Fatalf("%+v", p.Adoption)
	}
	r, err := e.s.AdoptSkillContentPreviewFor(e.uid, adoption("adopt_current_request_01", p, e.pin.RevisionID))
	if err != nil {
		t.Fatal(err)
	}
	next := capturedPreview(t, e, "adopt_current_preview_02", matching(e))
	if !next.Plan.Ready || len(next.Plan.Actions) != 0 || len(next.Plan.Conflicts) != 0 || len(next.Plan.Preserved) != 0 || next.ReceiptVersion != r.Version {
		t.Fatalf("a folder adopted at the pinned revision has nothing to do: %+v", next.Plan)
	}
	if next.Plan.ExpectedManaged == nil || next.Plan.ExpectedManaged.Digest != e.pin.ManifestDigest {
		t.Fatalf("%+v", next.Plan.ExpectedManaged)
	}
}

func TestAdoptClaimsOnlyProvableFiles(t *testing.T) {
	e := previewFixture(t)
	files := []skillbundle.File{}
	for _, f := range e.bundle.Revision.Manifest.Files {
		switch f.Path {
		case "scripts/run.sh":
			f.Bytes, f.Digest = f.Bytes+1, strings.Repeat("e", 64) // edited on the machine
		case "assets/empty":
			f.Executable = !f.Executable // identical bytes, different mode
		}
		files = append(files, f)
	}
	observed, err := skillbundle.FromMetadata(append(files, skillbundle.File{Path: "notes/mine.txt", Bytes: 3, Digest: strings.Repeat("f", 64)}))
	if err != nil {
		t.Fatal(err)
	}
	p := capturedPreview(t, e, "adopt_provable_preview_1", presentOf(observed, "exclusive"))
	if p.Adoption == nil || !reflect.DeepEqual(p.Adoption.Candidates, []AdoptionCandidate{{RevisionID: e.pin.RevisionID, Matched: 1, ObservedOnly: 3, RevisionOnly: 2}}) {
		t.Fatalf("%+v", p.Adoption)
	}
	r, err := e.s.AdoptSkillContentPreviewFor(e.uid, adoption("adopt_provable_request_1", p, e.pin.RevisionID))
	if err != nil {
		t.Fatal(err)
	}
	if r.FileCount != 1 || r.Managed == nil || !reflect.DeepEqual(*r.Managed, receiptManaged(t, e, "SKILL.md")) {
		t.Fatalf("only the file that equals the revision may be claimed: %+v", r.Managed)
	}
	raw, _ := json.Marshal(r)
	for _, unclaimed := range []string{"notes/mine.txt", "scripts/run.sh", "assets/empty"} {
		if strings.Contains(string(raw), unclaimed) {
			t.Fatalf("the receipt mentions unclaimed %s: %s", unclaimed, raw)
		}
	}
	next := capturedPreview(t, e, "adopt_provable_preview_2", presentOf(observed, "exclusive"))
	want := []skillplan.Conflict{{Path: "assets/empty", Code: "unmanaged_path"}, {Path: "scripts/run.sh", Code: "unmanaged_path"}}
	if next.Plan.Ready || len(next.Plan.Actions) != 0 || !reflect.DeepEqual(next.Plan.Conflicts, want) {
		t.Fatalf("a desired path that was not claimed must never be overwritten: %+v", next.Plan)
	}
}

func TestAdoptRefusals(t *testing.T) {
	type attempt struct {
		uid int64
		in  SkillContentAdopt
	}
	const key = "adopt_refusal_request_1"
	someHash := strings.Repeat("a", 64)
	current := func(t *testing.T, e previewEnv) SkillContentPreview {
		return capturedPreview(t, e, "adopt_refusal_preview_1", matching(e))
	}
	mine := func(e previewEnv, p SkillContentPreview) attempt {
		return attempt{e.uid, adoption(key, p, e.bundle.Revision.ID)}
	}
	with := func(t *testing.T, e previewEnv, change func(*SkillContentAdopt)) attempt {
		a := mine(e, current(t, e))
		change(&a.in)
		return a
	}
	unreviewed := func(e previewEnv, p SkillContentPreview) attempt {
		a := mine(e, p)
		a.in.ReviewHash = someHash
		return a
	}
	exec := func(t *testing.T, e previewEnv, query string, args ...any) {
		t.Helper()
		if _, err := e.s.db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	// missed makes a review look current although the slot gained a receipt after
	// it, so the in-transaction slot check is exercised on its own.
	missed := func(t *testing.T, e previewEnv, p SkillContentPreview, r SkillContentReceipt) {
		e.edit(t, p.ID, func(p *SkillContentPreview) { p.ReceiptVersion = r.Version })
		if got := e.state(t, p.ID); got.State != "captured" || len(got.Reasons) != 0 {
			t.Fatalf("fixture must make the review look current: %+v", got)
		}
	}
	otherBundleReceipt := func(t *testing.T, e previewEnv) SkillContentReceipt {
		second := secondBundle(t, e)
		other := e
		other.pin.BundleID = second.Bundle.ID
		return seedReceipt(t, other, second.Revision.ID, *second.Revision.Manifest, exclusiveAbsent().RootID)
	}

	cases := []struct {
		name  string
		setup func(*testing.T, previewEnv) attempt
		want  error
	}{
		{"short request key", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.RequestKey = "short" })
		}, ErrSkillContentReceiptInvalid},
		{"path-like request key", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.RequestKey = "../adopt_refusal_request" })
		}, ErrSkillContentReceiptInvalid},
		{"preview ID too short", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.PreviewID = in.PreviewID[:47] })
		}, ErrSkillContentReceiptInvalid},
		{"preview ID in capitals", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.PreviewID = strings.Repeat("A", 48) })
		}, ErrSkillContentReceiptInvalid},
		{"review hash too short", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.ReviewHash = in.ReviewHash[:63] })
		}, ErrSkillContentReceiptInvalid},
		{"review hash in capitals", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.ReviewHash = strings.Repeat("A", 64) })
		}, ErrSkillContentReceiptInvalid},
		{"revision ID too short", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.AdoptAsRevisionID = in.AdoptAsRevisionID[:31] })
		}, ErrSkillContentReceiptInvalid},
		{"digest as revision ID", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.AdoptAsRevisionID = e.pin.ManifestDigest })
		}, ErrSkillContentReceiptInvalid},
		{"wrong review hash", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.ReviewHash = strings.Repeat("0", 64) })
		}, ErrSkillContentReceipt},
		{"review hash of another preview", func(t *testing.T, e previewEnv) attempt {
			first := current(t, e)
			second := capturedPreview(t, e, "adopt_refusal_preview_2", matching(e))
			a := mine(e, second)
			a.in.ReviewHash = first.ReviewHash
			return a
		}, ErrSkillContentReceipt},
		{"stale", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			e.edit(t, p.ID, func(p *SkillContentPreview) { p.CapturedAt = now() - skillplan.MaxAgeMillis - 1000 })
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"superseded", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			if err := e.s.RecordInventoryFor(e.uid, e.write.EnvironmentID, true, MachineInfo{}); err != nil {
				t.Fatal(err)
			}
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"pin changed after the preview", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			e.repin(t, func(in *SkillContentTargetWrite) { in.Mode = "managed" })
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"pin removed after the preview", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			if err := e.s.DeleteSkillContentTargetFor(e.uid, e.pin.ID, e.pin.Version); err != nil {
				t.Fatal(err)
			}
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"queued", func(t *testing.T, e previewEnv) attempt {
			return unreviewed(e, e.create(t, "adopt_refusal_preview_1"))
		}, ErrSkillContentReceipt},
		{"capturing", func(t *testing.T, e previewEnv) attempt {
			p := e.create(t, "adopt_refusal_preview_1")
			e.claim(t)
			return unreviewed(e, p)
		}, ErrSkillContentReceipt},
		{"expired", func(t *testing.T, e previewEnv) attempt {
			p := e.create(t, "adopt_refusal_preview_1")
			e.edit(t, p.ID, func(p *SkillContentPreview) { p.ExpiresAt = now() - 1 })
			return unreviewed(e, p)
		}, ErrSkillContentReceipt},
		{"failed", func(t *testing.T, e previewEnv) attempt {
			p := e.create(t, "adopt_refusal_preview_1")
			e.claim(t)
			if err := e.s.FailSkillContentPreviewFor(e.uid, e.agent.ID, p.ID, previewInstance, "capture_failed"); err != nil {
				t.Fatal(err)
			}
			return unreviewed(e, p)
		}, ErrSkillContentReceipt},
		{"cancelled", func(t *testing.T, e previewEnv) attempt {
			p := e.create(t, "adopt_refusal_preview_1")
			e.claim(t)
			if err := e.s.StopSkillContentPreviewFor(e.uid, p.ID); err != nil {
				t.Fatal(err)
			}
			return unreviewed(e, p)
		}, ErrSkillContentReceipt},
		{"shared root", func(t *testing.T, e previewEnv) attempt {
			return mine(e, capturedPreview(t, e, "adopt_refusal_preview_1", presentOf(*e.bundle.Revision.Manifest, "shared")))
		}, ErrSkillContentReceipt},
		{"root ownership unknown", func(t *testing.T, e previewEnv) attempt {
			return mine(e, capturedPreview(t, e, "adopt_refusal_preview_1", presentOf(*e.bundle.Revision.Manifest, "unknown")))
		}, ErrSkillContentReceipt},
		{"unknown capture", func(t *testing.T, e previewEnv) attempt {
			return mine(e, capturedPreview(t, e, "adopt_refusal_preview_1", unknownCapture("exclusive", skillpreview.Issue{Path: "demo", Code: "skill_symlink"})))
		}, ErrSkillContentReceipt},
		{"absent capture", func(t *testing.T, e previewEnv) attempt {
			return mine(e, capturedPreview(t, e, "adopt_refusal_preview_1", exclusiveAbsent()))
		}, ErrSkillContentReceipt},
		{"revision from another bundle", func(t *testing.T, e previewEnv) attempt {
			second := secondBundle(t, e)
			return with(t, e, func(in *SkillContentAdopt) { in.AdoptAsRevisionID = second.Revision.ID })
		}, ErrSkillContentReceipt},
		{"revision from another account", func(t *testing.T, e previewEnv) attempt {
			other, err := e.s.CreateUser("adopt-revision-other@example.test", "Other", "member", "hash")
			if err != nil {
				t.Fatal(err)
			}
			theirs, err := e.s.SavePrivateSkillRevisionFor(other.ID, "", privateWrite("adopt_foreign_revision01", "", "PRIVATE_PIN_BYTES"))
			if err != nil {
				t.Fatal(err)
			}
			return with(t, e, func(in *SkillContentAdopt) { in.AdoptAsRevisionID = theirs.Revision.ID })
		}, ErrPrivateSkillMissing},
		{"SKILL.md differs from the revision", func(t *testing.T, e previewEnv) attempt {
			differs, err := e.s.SavePrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, privateWrite("adopt_skill_md_differs01", e.bundle.Revision.ID, "DIFFERENT"))
			if err != nil {
				t.Fatal(err)
			}
			return with(t, e, func(in *SkillContentAdopt) { in.AdoptAsRevisionID = differs.Revision.ID })
		}, skillplan.ErrNotClaimable},
		{"unknown revision", func(t *testing.T, e previewEnv) attempt {
			return with(t, e, func(in *SkillContentAdopt) { in.AdoptAsRevisionID = strings.Repeat("0", 32) })
		}, ErrPrivateSkillMissing},
		{"paused connector", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			if err := e.s.ControlSyncAgent(e.uid, e.agent.ID, "pause"); err != nil {
				t.Fatal(err)
			}
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"viewer", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			exec(t, e, `UPDATE users SET role='viewer' WHERE id=?`, e.uid)
			return mine(e, p)
		}, ErrSkillContentReceiptMissing},
		{"disabled account", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			exec(t, e, `UPDATE users SET disabled_at=1 WHERE id=?`, e.uid)
			return mine(e, p)
		}, ErrSkillContentReceiptMissing},
		{"foreign account's preview", func(t *testing.T, e previewEnv) attempt {
			other, err := e.s.CreateUser("adopt-preview-other@example.test", "Other", "member", "hash")
			if err != nil {
				t.Fatal(err)
			}
			a := mine(e, current(t, e))
			a.uid = other.ID
			return a
		}, ErrSkillContentPreviewMissing},
		{"1,024 existing receipts", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			fillReceipts(t, e, MaxSkillContentReceipts)
			return mine(e, p)
		}, ErrSkillContentReceiptCapacity},
		{"existing receipt for the slot, same bundle", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			e.adopted(t)
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"existing receipt for the slot, another bundle", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			otherBundleReceipt(t, e)
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"existing receipt the review missed, same bundle", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			missed(t, e, p, e.adopted(t))
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"existing receipt the review missed, another bundle", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			missed(t, e, p, otherBundleReceipt(t, e))
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"damaged receipt the review missed", func(t *testing.T, e previewEnv) attempt {
			p := current(t, e)
			r := e.adopted(t)
			exec(t, e, `UPDATE skill_content_receipts SET managed='not json' WHERE id=?`, r.ID)
			missed(t, e, p, r)
			return mine(e, p)
		}, ErrSkillContentReceipt},
		{"existing receipt the review missed, account full", func(t *testing.T, e previewEnv) attempt {
			// Releasing the slot's receipt is the one step that resolves both, so
			// the slot conflict is reported before the limit.
			p := current(t, e)
			fillReceipts(t, e, MaxSkillContentReceipts-1)
			missed(t, e, p, e.adopted(t))
			return mine(e, p)
		}, ErrSkillContentReceipt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := previewFixture(t)
			a := tc.setup(t, e)
			receiptsQuery := `SELECT id||' '||version||' '||name||' '||managed||' '||evidence FROM skill_content_receipts ORDER BY id`
			previewsQuery := `SELECT id||' '||state||' '||data FROM skill_content_previews ORDER BY id`
			receipts, previews := rowsOf(t, e, receiptsQuery), rowsOf(t, e, previewsQuery)
			got, err := e.s.AdoptSkillContentPreviewFor(a.uid, a.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if !reflect.DeepEqual(got, SkillContentReceipt{}) {
				t.Fatalf("a refusal returned a receipt: %+v", got)
			}
			if !reflect.DeepEqual(rowsOf(t, e, receiptsQuery), receipts) {
				t.Fatal("a refused adoption created or changed a receipt")
			}
			if !reflect.DeepEqual(rowsOf(t, e, previewsQuery), previews) {
				t.Fatal("a refused adoption changed a preview")
			}
		})
	}

	t.Run("the last free place can be used", func(t *testing.T) {
		e := previewFixture(t)
		p := current(t, e)
		fillReceipts(t, e, MaxSkillContentReceipts-1)
		if _, err := e.s.AdoptSkillContentPreviewFor(e.uid, adoption(key, p, e.bundle.Revision.ID)); err != nil {
			t.Fatal(err)
		}
		if receiptRows(t, e) != MaxSkillContentReceipts {
			t.Fatal(receiptRows(t, e))
		}
	})
}

func TestAdoptIsIdempotentAndConsumesThePreview(t *testing.T) {
	e := previewFixture(t)
	p := capturedPreview(t, e, "adopt_idempotent_preview1", matching(e))
	in := adoption("adopt_idempotent_request1", p, e.pin.RevisionID)
	first, err := e.s.AdoptSkillContentPreviewFor(e.uid, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.s.AdoptSkillContentPreviewFor(e.uid, in)
	if err != nil || !reflect.DeepEqual(again, first) {
		t.Fatalf("an identical retry must return the original receipt: %+v %v", again, err)
	}

	other, err := e.s.SavePrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, privateWrite("adopt_idempotent_revision", e.pin.RevisionID, "OTHER"))
	if err != nil {
		t.Fatal(err)
	}
	later := capturedPreview(t, e, "adopt_idempotent_preview2", matching(e))
	for name, change := range map[string]func(*SkillContentAdopt){
		"revision":    func(a *SkillContentAdopt) { a.AdoptAsRevisionID = other.Revision.ID },
		"review hash": func(a *SkillContentAdopt) { a.ReviewHash = strings.Repeat("0", 64) },
		"preview":     func(a *SkillContentAdopt) { a.PreviewID, a.ReviewHash = later.ID, later.ReviewHash },
	} {
		changed := in
		change(&changed)
		if got, err := e.s.AdoptSkillContentPreviewFor(e.uid, changed); !errors.Is(err, ErrSkillContentReceipt) || got.ID != "" {
			t.Fatalf("a changed %s reused a request key: %+v %v", name, got, err)
		}
	}
	if receiptRows(t, e) != 1 {
		t.Fatal(receiptRows(t, e))
	}

	if err = e.s.ReleaseSkillContentReceiptFor(e.uid, first.ID, first.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = e.s.AdoptSkillContentPreviewFor(e.uid, in); !errors.Is(err, ErrSkillContentReceipt) {
		t.Fatal("a replay after release must not claim the folder again:", err)
	}
	if _, err = e.s.AdoptSkillContentPreviewFor(e.uid, adoption("adopt_idempotent_request2", p, e.pin.RevisionID)); !errors.Is(err, ErrSkillContentReceipt) {
		t.Fatal("a consumed preview must not be adopted again under a new key:", err)
	}
	if receiptRows(t, e) != 0 {
		t.Fatal("a refused replay created a second claim")
	}
	if old := e.state(t, p.ID); old.State != "superseded" || !reflect.DeepEqual(old.Reasons, []string{"receipt_changed"}) {
		t.Fatalf("a consumed preview must stay superseded after release: %+v", old)
	}

	fresh := capturedPreview(t, e, "adopt_idempotent_preview3", matching(e))
	second, err := e.s.AdoptSkillContentPreviewFor(e.uid, adoption("adopt_idempotent_request3", fresh, e.pin.RevisionID))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.Version == first.Version || second.ManagedDigest != first.ManagedDigest {
		t.Fatalf("a new adoption is a new decision with a new version: %+v %+v", first, second)
	}
	if old := e.state(t, p.ID); old.State != "superseded" || !reflect.DeepEqual(old.Reasons, []string{"receipt_changed"}) {
		t.Fatalf("the first preview must stay superseded: %+v", old)
	}
}

func TestAdoptConcurrentRequestsCreateOneReceipt(t *testing.T) {
	e := previewFixture(t)
	first := capturedPreview(t, e, "adopt_concurrent_preview1", matching(e))
	second := capturedPreview(t, e, "adopt_concurrent_preview2", matching(e))
	if got := e.state(t, first.ID); got.State != "captured" || len(got.Reasons) != 0 {
		t.Fatalf("both reviews must be current before the race: %+v", got)
	}
	other, err := Open(e.s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	barrier := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, run := range []struct {
		st *Store
		p  SkillContentPreview
	}{{e.s, first}, {other, second}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-barrier
			_, err := run.st.AdoptSkillContentPreviewFor(e.uid, adoption(fmt.Sprintf("adopt_concurrent_request_%d", i), run.p, e.pin.RevisionID))
			results <- err
		}()
	}
	close(barrier)
	wg.Wait()
	close(results)
	ok, refused := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrSkillContentReceipt):
			refused++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || refused != 1 || receiptRows(t, e) != 1 {
		t.Fatal("concurrent adoptions of one slot", ok, refused, receiptRows(t, e))
	}
}

func TestAdoptDispatchesNothingToTheMachine(t *testing.T) {
	e := previewFixture(t)
	p := capturedPreview(t, e, "adopt_dispatch_preview_01", matching(e))
	before, err := e.s.SyncTargetsFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.s.AdoptSkillContentPreviewFor(e.uid, adoption("adopt_dispatch_request_01", p, e.pin.RevisionID)); err != nil {
		t.Fatal(err)
	}
	var jobs, pending int
	if err = e.s.db.QueryRow(`SELECT count(*) FROM sync_jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatal("adoption queued a job", jobs, err)
	}
	if err = e.s.db.QueryRow(`SELECT count(*) FROM skill_content_previews WHERE state IN ('queued','capturing')`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("adoption queued preview work", pending, err)
	}
	if job, err := e.s.ClaimSyncJob(e.uid, e.agent); err != nil || job != nil {
		t.Fatal("a legacy poll received work after adoption", job, err)
	}
	if work, err := e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, previewInstance); err != nil || work != nil {
		t.Fatal("a preview poll received work after adoption", work, err)
	}
	after, err := e.s.SyncTargetsFor(e.uid)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("adoption changed legacy desired state", after, err)
	}
	if pin, err := e.s.SkillContentTargetFor(e.uid, e.pin.ID); err != nil || !reflect.DeepEqual(pin, e.pin) {
		t.Fatal("adoption changed the pin", pin, err)
	}
}

func TestAdoptionOfferOnlyWhereAdoptable(t *testing.T) {
	t.Run("best match first, then newest, at most 16, and SKILL.md must match", func(t *testing.T) {
		e := previewFixture(t)
		r1 := e.bundle.Revision
		differs, err := e.s.SavePrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, privateWrite("adopt_offer_skill_differs", r1.ID, "DIFFERENT"))
		if err != nil {
			t.Fatal(err)
		}
		head, partial := differs.Revision.ID, []string{}
		for i := 0; i < 17; i++ {
			w := privateWrite(fmt.Sprintf("adopt_offer_revision_%04d", i), head, "PRIVATE_PIN_BYTES")
			w.Files[1].Data = []byte(fmt.Sprintf("#!/bin/sh\nprintf v%d\n", i))
			rev, err := e.s.SavePrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, w)
			if err != nil {
				t.Fatal(err)
			}
			// Creation times are set apart so "newest" does not hinge on the clock's resolution.
			if _, err = e.s.db.Exec(`UPDATE private_skill_revisions SET created_at=? WHERE id=?`, r1.CreatedAt+int64(i+1)*1000, rev.Revision.ID); err != nil {
				t.Fatal(err)
			}
			head, partial = rev.Revision.ID, append(partial, rev.Revision.ID)
		}
		// The newest revision of all differs only in SKILL.md, so nothing but that rule can exclude it.
		if _, err = e.s.db.Exec(`UPDATE private_skill_revisions SET created_at=? WHERE id=?`, r1.CreatedAt+100000, differs.Revision.ID); err != nil {
			t.Fatal(err)
		}
		p := capturedPreview(t, e, "adopt_offer_ranked_000001", matching(e))
		want := []AdoptionCandidate{{RevisionID: r1.ID, Matched: 3}}
		for i := len(partial) - 1; len(want) < 16; i-- {
			want = append(want, AdoptionCandidate{RevisionID: partial[i], Matched: 2, ObservedOnly: 1, RevisionOnly: 1})
		}
		if p.Adoption == nil || !reflect.DeepEqual(p.Adoption.Candidates, want) {
			t.Fatalf("offer %+v, want %+v", p.Adoption, want)
		}
		raw, _ := json.Marshal(p)
		if !strings.Contains(string(raw), `"adoption":{"candidates":[{"revisionId":"`+r1.ID+`","matched":3,"observedOnly":0,"revisionOnly":0},`) {
			t.Fatalf("offer on the wire: %s", raw)
		}
	})

	t.Run("an adoptable folder no revision can prove gets an empty offer", func(t *testing.T) {
		e := previewFixture(t)
		files := append([]skillbundle.File{}, e.bundle.Revision.Manifest.Files...)
		for i := range files {
			if files[i].Path == "SKILL.md" {
				files[i].Digest = strings.Repeat("e", 64)
			}
		}
		observed, err := skillbundle.FromMetadata(files)
		if err != nil {
			t.Fatal(err)
		}
		p := capturedPreview(t, e, "adopt_offer_unprovable_01", presentOf(observed, "exclusive"))
		if !reflect.DeepEqual(conflictCodes(p), []string{"adoption_required"}) || p.Adoption == nil || p.Adoption.Candidates == nil || len(p.Adoption.Candidates) != 0 {
			t.Fatalf("%v %+v", conflictCodes(p), p.Adoption)
		}
	})

	t.Run("never on list reads", func(t *testing.T) {
		e := previewFixture(t)
		p := capturedPreview(t, e, "adopt_offer_list_0000001", matching(e))
		if p.Adoption == nil {
			t.Fatal("the item read should carry the offer")
		}
		list, _, err := e.s.SkillContentPreviewsFor(e.uid)
		if err != nil || len(list) != 1 || list[0].ID != p.ID || list[0].State != "captured" {
			t.Fatal(list, err)
		}
		raw, _ := json.Marshal(list)
		if list[0].Adoption != nil || strings.Contains(string(raw), `"adoption"`) {
			t.Fatalf("a list summary carries an offer: %s", raw)
		}
	})

	for name, c := range map[string]func(previewEnv) skillpreview.Capture{
		"not for a shared root":          func(e previewEnv) skillpreview.Capture { return presentOf(*e.bundle.Revision.Manifest, "shared") },
		"not for a root of unknown kind": func(e previewEnv) skillpreview.Capture { return presentOf(*e.bundle.Revision.Manifest, "unknown") },
		"not for a ready plan":           func(previewEnv) skillpreview.Capture { return exclusiveAbsent() },
	} {
		t.Run(name, func(t *testing.T) {
			e := previewFixture(t)
			if p := capturedPreview(t, e, "adopt_offer_not_adoptable", c(e)); p.Adoption != nil {
				t.Fatalf("%v %+v", conflictCodes(p), p.Adoption)
			}
		})
	}

	t.Run("not for a stale review", func(t *testing.T) {
		e := previewFixture(t)
		p := capturedPreview(t, e, "adopt_offer_stale_000001", matching(e))
		e.edit(t, p.ID, func(p *SkillContentPreview) { p.CapturedAt = now() - skillplan.MaxAgeMillis - 1000 })
		if got := e.state(t, p.ID); got.State != "stale" || got.Adoption != nil {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("not once a receipt exists", func(t *testing.T) {
		e := previewFixture(t)
		p := capturedPreview(t, e, "adopt_offer_receipt_00001", matching(e))
		e.adopted(t)
		if got := e.state(t, p.ID); got.State != "superseded" || got.Adoption != nil {
			t.Fatalf("%+v", got)
		}
		if q := capturedPreview(t, e, "adopt_offer_receipt_00002", matching(e)); q.Adoption != nil || !q.Plan.Ready {
			t.Fatalf("%+v", q)
		}
	})
}
