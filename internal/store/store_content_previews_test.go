package store

import (
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

const previewInstance = "c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0"

type previewEnv struct {
	s      *Store
	uid    int64
	agent  SyncAgent
	write  SkillContentTargetWrite
	bundle PrivateSkillResult
	pin    SkillContentTarget
}

// previewFixture pins the private revision and lets a connector negotiate, as a
// real one does by asking for work before it reads any file.
func previewFixture(t *testing.T) previewEnv {
	t.Helper()
	s, uid, a, in, bundle := contentTargetFixture(t)
	pin, err := s.SaveSkillContentTargetFor(uid, in)
	if err != nil {
		t.Fatal(err)
	}
	if work, err := s.ClaimSkillContentPreviewFor(uid, a.ID, previewInstance); err != nil || work != nil {
		t.Fatal("negotiation must not hand out work or touch files", work, err)
	}
	return previewEnv{s, uid, a, in, bundle, pin}
}

func (e previewEnv) create(t *testing.T, key string) SkillContentPreview {
	t.Helper()
	p, err := e.s.CreateSkillContentPreviewFor(e.uid, SkillContentPreviewCreate{RequestKey: key, TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (e previewEnv) claim(t *testing.T) *SkillContentPreviewWork {
	t.Helper()
	w, err := e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, previewInstance)
	if err != nil || w == nil {
		t.Fatal("expected work", w, err)
	}
	return w
}

func exclusiveAbsent() skillpreview.Capture {
	return skillpreview.Capture{SchemaVersion: 1, Adapter: skillpreview.Adapter, RootID: strings.Repeat("d", 64), RootOwnership: "exclusive", Presence: "absent", Complete: true, Issues: []skillpreview.Issue{}}
}

func presentOf(m skillbundle.Manifest, ownership string) skillpreview.Capture {
	c := exclusiveAbsent()
	c.Presence, c.Manifest, c.RootOwnership = "present", &m, ownership
	return c
}

func unknownCapture(ownership string, issues ...skillpreview.Issue) skillpreview.Capture {
	c := exclusiveAbsent()
	c.Presence, c.Complete, c.RootOwnership, c.Issues = "unknown", false, ownership, issues
	return c
}

func (e previewEnv) report(t *testing.T, id string, c skillpreview.Capture) error {
	t.Helper()
	return e.s.RecordSkillContentCaptureFor(e.uid, e.agent.ID, id, previewInstance, c)
}

func (e previewEnv) edit(t *testing.T, id string, change func(*SkillContentPreview)) {
	t.Helper()
	var raw string
	if err := e.s.db.QueryRow(`SELECT data FROM skill_content_previews WHERE id=?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var p SkillContentPreview
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	change(&p)
	out, _ := json.Marshal(p)
	if _, err := e.s.db.Exec(`UPDATE skill_content_previews SET data=?,state=? WHERE id=?`, string(out), p.State, id); err != nil {
		t.Fatal(err)
	}
}

func (e previewEnv) state(t *testing.T, id string) SkillContentPreview {
	t.Helper()
	p, err := e.s.SkillContentPreviewFor(e.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func conflictCodes(p SkillContentPreview) []string {
	out := []string{}
	if p.Plan != nil {
		for _, c := range p.Plan.Conflicts {
			out = append(out, c.Code)
		}
	}
	return out
}

func TestSkillContentPreviewLifecycleBindsPinMachineProcessAndInventory(t *testing.T) {
	e := previewFixture(t)
	before, _ := e.s.SyncTargetsFor(e.uid)

	p := e.create(t, "preview_lifecycle_0001")
	if p.State != "queued" || p.TargetID != e.pin.ID || p.TargetVersion != e.pin.Version || p.AgentID != e.agent.ID || p.Instance != previewInstance || p.Authority != "preview_only" {
		t.Fatalf("%+v", p)
	}
	if p.BundleID != e.pin.BundleID || p.RevisionID != e.pin.RevisionID || p.ManifestDigest != e.pin.ManifestDigest || p.Mode != "additive" || p.Name != "demo" || p.Agent != "agents" || p.Project != "" {
		t.Fatalf("not selected server-side from the owned pin: %+v", p)
	}
	if p.InventoryRevision != 0 || p.Capture != nil || p.Plan != nil || p.ReviewHash != "" {
		t.Fatal("nothing is bound or observed before capture starts", p)
	}

	work := e.claim(t)
	if work.ID != p.ID || work.Instance != previewInstance || work.Agent != "agents" || work.Project != "" || work.Name != "demo" || work.ExpiresAt != p.ExpiresAt {
		t.Fatalf("%+v", work)
	}
	raw, _ := json.Marshal(work)
	for _, secret := range []string{e.pin.BundleID, e.pin.RevisionID, e.pin.ManifestDigest, e.pin.ID, "additive"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("connector work leaks %q: %s", secret, raw)
		}
	}
	capturing := e.state(t, p.ID)
	var current int64
	if err := e.s.db.QueryRow(`SELECT inventory_revision FROM environment_scans WHERE env_id=?`, e.write.EnvironmentID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if capturing.State != "capturing" || capturing.InventoryRevision != current || current <= 0 {
		t.Fatalf("inventory revision must be bound when capture begins: %+v (inventory %d)", capturing, current)
	}
	if again := e.claim(t); again.ID != p.ID {
		t.Fatal("a retried claim from the same process must return the same work", again)
	}

	if err := e.report(t, p.ID, exclusiveAbsent()); err != nil {
		t.Fatal(err)
	}
	done := e.state(t, p.ID)
	if done.State != "captured" || done.Plan == nil || !done.Plan.Ready || done.ReviewHash == "" || done.CaptureHash == "" || done.CapturedAt == 0 || len(done.Reasons) != 0 {
		t.Fatalf("%+v", done)
	}
	if got := len(done.Plan.Actions); got != 3 {
		t.Fatalf("a new install creates the three desired files, got %d", got)
	}
	for _, a := range done.Plan.Actions {
		if a.Kind != "create" {
			t.Fatal(a)
		}
	}
	if done.Plan.Preconditions.Target.RootID != strings.Repeat("d", 64) || done.Plan.Preconditions.Baseline != nil || done.Plan.Preconditions.Mode != "additive" {
		t.Fatalf("planner input was not built from trusted storage: %+v", done.Plan.Preconditions)
	}
	if done.Plan.Preconditions.Desired.RevisionID != e.pin.RevisionID || done.Plan.Preconditions.Desired.Manifest.Digest != e.pin.ManifestDigest {
		t.Fatal("desired revision is not the pinned immutable revision")
	}
	// A preview creates no job, changes no pin and touches no legacy desired state.
	var jobs int
	_ = e.s.db.QueryRow(`SELECT count(*) FROM sync_jobs`).Scan(&jobs)
	after, _ := e.s.SyncTargetsFor(e.uid)
	same, _ := e.s.SkillContentTargetFor(e.uid, e.pin.ID)
	if jobs != 0 || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(same, e.pin) {
		t.Fatal("preview had a side effect", jobs, same)
	}
	if job, err := e.s.ClaimSyncJob(e.uid, e.agent); err != nil || job != nil {
		t.Fatal("a legacy poll received preview work", job, err)
	}
}

func TestSkillContentPreviewNeverTreatsMatchingBytesAsOwnership(t *testing.T) {
	e := previewFixture(t)
	exact := *e.bundle.Revision.Manifest

	t.Run("identical bytes still need adoption", func(t *testing.T) {
		p := e.create(t, "preview_equal_bytes_0001")
		e.claim(t)
		if err := e.report(t, p.ID, presentOf(exact, "exclusive")); err != nil {
			t.Fatal(err)
		}
		got := e.state(t, p.ID)
		if got.State != "captured" || got.Plan.Ready || len(got.Plan.Actions) != 0 || !reflect.DeepEqual(conflictCodes(got), []string{"adoption_required"}) {
			t.Fatalf("%+v %v", got.Plan, conflictCodes(got))
		}
		if err := e.s.StopSkillContentPreviewFor(e.uid, p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.s.DismissSkillContentPreviewFor(e.uid, p.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an unmanaged extra file is preserved, not claimed", func(t *testing.T) {
		p := e.create(t, "preview_extra_file_000001")
		e.claim(t)
		files := append([]skillbundle.File{}, exact.Files...)
		files = append(files, skillbundle.File{Path: "notes/mine.txt", Bytes: 3, Digest: strings.Repeat("e", 64)})
		m, err := skillbundle.FromMetadata(files)
		if err != nil {
			t.Fatal(err)
		}
		if err = e.report(t, p.ID, presentOf(m, "exclusive")); err != nil {
			t.Fatal(err)
		}
		got := e.state(t, p.ID)
		if got.Plan.Ready || len(got.Plan.Actions) != 0 || got.Plan.ExpectedManaged != nil || !reflect.DeepEqual(conflictCodes(got), []string{"adoption_required"}) {
			t.Fatalf("%+v", got.Plan)
		}
		if _, err = e.s.DismissSkillContentPreviewFor(e.uid, p.ID); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSkillContentPreviewBlocksSharedUnknownAndUnreadableRoots(t *testing.T) {
	e := previewFixture(t)
	for _, tc := range []struct {
		name string
		c    skillpreview.Capture
		want string
	}{
		{"shared root", unknownCapture("shared", skillpreview.Issue{Path: ".", Code: "root_aliased"}), "root_shared"},
		{"unknown ownership", unknownCapture("unknown", skillpreview.Issue{Path: ".", Code: "alias_check_incomplete"}), "root_unknown"},
		{"exclusive but unreadable skill", unknownCapture("exclusive", skillpreview.Issue{Path: "demo", Code: "skill_symlink"}), "observation_unknown"},
		{"unavailable root", func() skillpreview.Capture {
			c := unknownCapture("unknown", skillpreview.Issue{Path: ".", Code: "scope_root_missing"})
			c.RootID = skillpreview.Unavailable
			return c
		}(), "root_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := e.create(t, "preview_blocked_"+strings.ReplaceAll(tc.name, " ", "_")+"_01")
			e.claim(t)
			if err := e.report(t, p.ID, tc.c); err != nil {
				t.Fatal(err)
			}
			got := e.state(t, p.ID)
			if got.State != "captured" || got.Plan.Ready || len(got.Plan.Actions) != 0 || !reflect.DeepEqual(conflictCodes(got), []string{tc.want}) {
				t.Fatalf("%s: %+v", tc.name, got.Plan)
			}
			if got.Capture == nil || got.Capture.Presence != "unknown" {
				t.Fatal("the unknown state must stay visible rather than being rewritten", got.Capture)
			}
			if _, err := e.s.DismissSkillContentPreviewFor(e.uid, p.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSkillContentPreviewRejectsInvalidAndConflictingReports(t *testing.T) {
	e := previewFixture(t)
	p := e.create(t, "preview_reports_000000001")
	e.claim(t)

	forged := presentOf(*e.bundle.Revision.Manifest, "exclusive")
	m := *forged.Manifest
	m.Digest = strings.Repeat("0", 64)
	forged.Manifest = &m
	if err := e.report(t, p.ID, forged); !errors.Is(err, ErrSkillContentPreviewInvalid) {
		t.Fatal("accepted a forged digest", err)
	}
	partial := exclusiveAbsent()
	partial.Complete = false
	if err := e.report(t, p.ID, partial); !errors.Is(err, ErrSkillContentPreviewInvalid) {
		t.Fatal("accepted absence without a complete check", err)
	}
	if got := e.state(t, p.ID); got.State != "capturing" {
		t.Fatal("an invalid report changed the request", got.State)
	}

	if err := e.report(t, p.ID, exclusiveAbsent()); err != nil {
		t.Fatal(err)
	}
	first := e.state(t, p.ID)
	if err := e.report(t, p.ID, exclusiveAbsent()); err != nil {
		t.Fatal("a lost acknowledgement must be safe to retry", err)
	}
	if again := e.state(t, p.ID); again.ReviewHash != first.ReviewHash || again.CapturedAt != first.CapturedAt {
		t.Fatal("a retry replaced the reviewed capture")
	}
	other := exclusiveAbsent()
	other.RootID = strings.Repeat("9", 64)
	if err := e.report(t, p.ID, other); !errors.Is(err, ErrSkillContentPreview) {
		t.Fatal("a different capture replaced a recorded one", err)
	}
	if err := e.s.RecordSkillContentCaptureFor(e.uid, e.agent.ID, strings.Repeat("a", 48), previewInstance, exclusiveAbsent()); !errors.Is(err, ErrSkillContentPreviewMissing) {
		t.Fatal("reported against an unknown request", err)
	}
}

func TestSkillContentPreviewReviewIdentityBindsThePinVersion(t *testing.T) {
	base := skillContentReviewIdentity{OwnerID: 1, RequestID: "r", TargetID: "t", TargetVersion: "v1", BundleID: "b", RevisionID: "rev", ManifestDigest: "m", Mode: "additive", AgentID: "a", Instance: "i", InventoryRevision: 7, CaptureHash: "c", PlanHash: "p"}
	h1 := base.Hash()
	// A pin can change away and back to identical content; the version differs.
	back := base
	back.TargetVersion = "v3"
	if h1 == back.Hash() {
		t.Fatal("review identity ignores the pin version")
	}
	for name, change := range map[string]func(*skillContentReviewIdentity){
		"account":   func(i *skillContentReviewIdentity) { i.OwnerID = 2 },
		"request":   func(i *skillContentReviewIdentity) { i.RequestID = "x" },
		"target":    func(i *skillContentReviewIdentity) { i.TargetID = "x" },
		"revision":  func(i *skillContentReviewIdentity) { i.RevisionID = "x" },
		"mode":      func(i *skillContentReviewIdentity) { i.Mode = "managed" },
		"machine":   func(i *skillContentReviewIdentity) { i.AgentID = "x" },
		"process":   func(i *skillContentReviewIdentity) { i.Instance = "x" },
		"inventory": func(i *skillContentReviewIdentity) { i.InventoryRevision = 8 },
		"capture":   func(i *skillContentReviewIdentity) { i.CaptureHash = "x" },
		"plan":      func(i *skillContentReviewIdentity) { i.PlanHash = "x" },
	} {
		changed := base
		change(&changed)
		if changed.Hash() == h1 {
			t.Fatalf("review identity ignores %s", name)
		}
	}
	if base.Hash() != h1 {
		t.Fatal("not deterministic")
	}
}

func TestSkillContentPreviewPinChangedAwayAndBackInvalidatesOldReview(t *testing.T) {
	e := previewFixture(t)
	p := e.create(t, "preview_aba_000000000001")
	e.claim(t)
	if err := e.report(t, p.ID, exclusiveAbsent()); err != nil {
		t.Fatal(err)
	}
	reviewed := e.state(t, p.ID)
	if reviewed.State != "captured" {
		t.Fatal(reviewed.State)
	}

	next, err := e.s.SavePrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, privateWrite("preview_aba_new_revision", e.bundle.Revision.ID, "NEWER"))
	if err != nil {
		t.Fatal(err)
	}
	away := e.write
	away.RevisionID, away.ExpectedVersion = next.Revision.ID, &e.pin.Version
	moved, err := e.s.SaveSkillContentTargetFor(e.uid, away)
	if err != nil {
		t.Fatal(err)
	}
	back := e.write
	back.ExpectedVersion = &moved.Version
	restored, err := e.s.SaveSkillContentTargetFor(e.uid, back)
	if err != nil {
		t.Fatal(err)
	}
	if restored.RevisionID != e.pin.RevisionID || restored.Mode != e.pin.Mode || restored.Version == e.pin.Version {
		t.Fatalf("fixture did not reproduce an away-and-back change: %+v", restored)
	}
	old := e.state(t, p.ID)
	if old.State != "superseded" || !reflect.DeepEqual(old.Reasons, []string{"pin_changed"}) {
		t.Fatalf("old review survived an intervening pin change: %+v", old)
	}
	if old.Plan == nil {
		t.Fatal("the superseded review should stay inspectable")
	}
	if _, err = e.s.CreateSkillContentPreviewFor(e.uid, SkillContentPreviewCreate{RequestKey: "preview_aba_stale_version", TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version}); !errors.Is(err, ErrSkillContentTargetConflict) {
		t.Fatal("a preview was created from a stale pin version", err)
	}
}

func TestSkillContentPreviewFencesEveryChangeBeforeAndDuringCapture(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, previewEnv, SkillContentPreview)
		reason string
	}{
		{"pin removed", func(t *testing.T, e previewEnv, p SkillContentPreview) {
			if err := e.s.DeleteSkillContentTargetFor(e.uid, e.pin.ID, e.pin.Version); err != nil {
				t.Fatal(err)
			}
		}, "pin_removed"},
		{"pin changed", func(t *testing.T, e previewEnv, p SkillContentPreview) {
			in := e.write
			in.Mode, in.ExpectedVersion = "managed", &e.pin.Version
			if _, err := e.s.SaveSkillContentTargetFor(e.uid, in); err != nil {
				t.Fatal(err)
			}
		}, "pin_changed"},
		{"connector paused", func(t *testing.T, e previewEnv, p SkillContentPreview) {
			if err := e.s.ControlSyncAgent(e.uid, e.agent.ID, "pause"); err != nil {
				t.Fatal(err)
			}
		}, "connector_paused"},
		{"inventory report", func(t *testing.T, e previewEnv, p SkillContentPreview) {
			if err := e.s.RecordInventoryFor(e.uid, e.write.EnvironmentID, true, MachineInfo{}); err != nil {
				t.Fatal(err)
			}
		}, "inventory_changed"},
		{"new connector process", func(t *testing.T, e previewEnv, p SkillContentPreview) {
			if _, err := e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, strings.Repeat("f", 48)); err != nil {
				t.Fatal(err)
			}
		}, "process_changed"},
		{"account became a viewer", func(t *testing.T, e previewEnv, p SkillContentPreview) {
			if _, err := e.s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, e.uid); err != nil {
				t.Fatal(err)
			}
		}, "permission_changed"},
		{"machine renamed", func(t *testing.T, e previewEnv, p SkillContentPreview) {
			if err := e.s.MergeMachinesFor(e.uid, e.agent.Host, "renamed-laptop"); err != nil {
				t.Fatal(err)
			}
		}, "pin_changed"},
	}
	for _, claimed := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s (claimed=%t)", tc.name, claimed), func(t *testing.T) {
				e := previewFixture(t)
				p := e.create(t, "preview_fence_00000000001")
				if claimed {
					e.claim(t)
				}
				tc.change(t, e, p)
				if tc.reason == "inventory_changed" && !claimed {
					// Nothing is bound yet; the claim binds whatever is current.
					e.claim(t)
					if got := e.state(t, p.ID); got.State != "capturing" {
						t.Fatal("an inventory report before capture began should not block it", got.State)
					}
					return
				}
				err := e.report(t, p.ID, exclusiveAbsent())
				if err == nil {
					t.Fatal("a capture was recorded after", tc.name)
				}
				got, readErr := e.s.SkillContentPreviewFor(e.uid, p.ID)
				if tc.reason == "permission_changed" {
					if readErr == nil {
						t.Fatal("a viewer read a private review", got)
					}
					return
				}
				if readErr != nil {
					t.Fatal(readErr)
				}
				if got.State != "superseded" || got.Plan != nil || got.Capture != nil {
					t.Fatalf("%+v", got)
				}
				found := false
				for _, r := range got.Reasons {
					found = found || r == tc.reason
				}
				if !found {
					t.Fatalf("want reason %s, got %v", tc.reason, got.Reasons)
				}
				if work, err := e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, previewInstance); err == nil && work != nil {
					t.Fatal("handed superseded work to a connector", work)
				}
			})
		}
	}
}

func TestSkillContentPreviewExpiryAndStaleness(t *testing.T) {
	e := previewFixture(t)
	p := e.create(t, "preview_expiry_000000001")
	e.edit(t, p.ID, func(p *SkillContentPreview) { p.ExpiresAt = now() - 1 })
	if got := e.state(t, p.ID); got.State != "expired" {
		t.Fatal(got.State)
	}
	if work, err := e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, previewInstance); err != nil || work != nil {
		t.Fatal("claimed expired work", work, err)
	}
	if err := e.report(t, p.ID, exclusiveAbsent()); err == nil {
		t.Fatal("reported against an expired request")
	}

	fresh := e.create(t, "preview_staleness_0000001")
	e.claim(t)
	if err := e.report(t, fresh.ID, exclusiveAbsent()); err != nil {
		t.Fatal(err)
	}
	e.edit(t, fresh.ID, func(p *SkillContentPreview) { p.CapturedAt = now() - skillplan.MaxAgeMillis - 1000 })
	stale := e.state(t, fresh.ID)
	if stale.State != "stale" || stale.Plan == nil {
		t.Fatalf("an old capture must be labelled stale, not current: %s", stale.State)
	}
}

func TestSkillContentPreviewCreateRequiresNegotiationOwnershipAndAFreshPin(t *testing.T) {
	e := previewFixture(t)
	attempt := func(key string) error {
		_, err := e.s.CreateSkillContentPreviewFor(e.uid, SkillContentPreviewCreate{RequestKey: key, TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version})
		return err
	}
	for name, in := range map[string]SkillContentPreviewCreate{
		"short request key": {RequestKey: "short", TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version},
		"missing target":    {RequestKey: "preview_create_00000001", ExpectedTargetVersion: e.pin.Version},
		"missing version":   {RequestKey: "preview_create_00000001", TargetID: e.pin.ID},
		"digest as version": {RequestKey: "preview_create_00000001", TargetID: e.pin.ID, ExpectedTargetVersion: strings.Repeat("a", 64)},
		"path-like target":  {RequestKey: "preview_create_00000001", TargetID: "../" + e.pin.ID, ExpectedTargetVersion: e.pin.Version},
		"unknown pin":       {RequestKey: "preview_create_00000001", TargetID: strings.Repeat("0", 48), ExpectedTargetVersion: e.pin.Version},
		"stale version":     {RequestKey: "preview_create_00000001", TargetID: e.pin.ID, ExpectedTargetVersion: strings.Repeat("1", 48)},
	} {
		if _, err := e.s.CreateSkillContentPreviewFor(e.uid, in); err == nil {
			t.Fatal("accepted", name)
		}
	}
	other, err := e.s.CreateUser("preview-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.s.CreateSkillContentPreviewFor(other.ID, SkillContentPreviewCreate{RequestKey: "preview_foreign_000001", TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version}); !errors.Is(err, ErrSkillContentTargetMissing) {
		t.Fatal("a foreign pin was previewable", err)
	}

	// The connector must have negotiated recently; an old or unseen process cannot be asked.
	if _, err = e.s.db.Exec(`UPDATE skill_content_preview_capabilities SET observed_at=? WHERE agent_id=?`, now()-91000, e.agent.ID); err != nil {
		t.Fatal(err)
	}
	if err = attempt("preview_not_ready_00001"); !errors.Is(err, ErrSkillContentPreview) {
		t.Fatal("created work for a connector that has not negotiated", err)
	}
	if _, err = e.s.db.Exec(`DELETE FROM skill_content_preview_capabilities`); err != nil {
		t.Fatal(err)
	}
	if err = attempt("preview_no_negotiation_1"); !errors.Is(err, ErrSkillContentPreview) {
		t.Fatal("created work for an old connector", err)
	}
	if _, err = e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, previewInstance); err != nil {
		t.Fatal(err)
	}

	if err = e.s.ControlSyncAgent(e.uid, e.agent.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	if err = attempt("preview_paused_0000001"); err == nil {
		t.Fatal("previewed through a paused connector")
	}
	if _, err = e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, previewInstance); err == nil {
		t.Fatal("a paused connector may not be handed work")
	}
	if err = e.s.ControlSyncAgent(e.uid, e.agent.ID, "resume"); err != nil {
		t.Fatal(err)
	}

	if _, err = e.s.db.Exec(`UPDATE environment_scans SET complete=0 WHERE env_id=?`, e.write.EnvironmentID); err != nil {
		t.Fatal(err)
	}
	if err = attempt("preview_incomplete_0001"); !errors.Is(err, ErrSkillContentTargetScope) {
		t.Fatal("previewed a scope the pin API would not accept", err)
	}
	if _, err = e.s.db.Exec(`UPDATE environment_scans SET complete=1 WHERE env_id=?`, e.write.EnvironmentID); err != nil {
		t.Fatal(err)
	}
	if err = attempt("preview_all_good_00001"); err != nil {
		t.Fatal(err)
	}
	if err = attempt("preview_second_active_1"); !errors.Is(err, ErrSkillContentPreview) {
		t.Fatal("two active previews on one machine", err)
	}
}

func TestSkillContentPreviewRetriesReturnTheOriginalRequest(t *testing.T) {
	e := previewFixture(t)
	first := e.create(t, "preview_retry_00000001")
	again, err := e.s.CreateSkillContentPreviewFor(e.uid, SkillContentPreviewCreate{RequestKey: "preview_retry_00000001", TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version})
	if err != nil || again.ID != first.ID {
		t.Fatal("a lost response must be retryable", again, err)
	}
	if _, err = e.s.CreateSkillContentPreviewFor(e.uid, SkillContentPreviewCreate{RequestKey: "preview_retry_00000001", TargetID: strings.Repeat("a", 48), ExpectedTargetVersion: e.pin.Version}); !errors.Is(err, ErrSkillContentPreview) {
		t.Fatal("a changed payload reused a request key", err)
	}
	in := e.write
	in.Mode, in.ExpectedVersion = "managed", &e.pin.Version
	if _, err = e.s.SaveSkillContentTargetFor(e.uid, in); err != nil {
		t.Fatal(err)
	}
	after, err := e.s.CreateSkillContentPreviewFor(e.uid, SkillContentPreviewCreate{RequestKey: "preview_retry_00000001", TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version})
	if err != nil || after.ID != first.ID || after.State != "superseded" {
		t.Fatal("a retry after the pin moved must report the original as superseded, not start a new review", after.State, err)
	}
}

func TestSkillContentPreviewClaimsOnlyForTheBoundProcessAndIdleMachines(t *testing.T) {
	e := previewFixture(t)
	p := e.create(t, "preview_process_0000001")
	if _, err := e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, "not-a-valid-instance"); err == nil {
		t.Fatal("accepted a malformed process identity")
	}
	// A running legacy sync job occupies the machine; no content work is handed out.
	if _, err := e.s.db.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,kind,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, strings.Repeat("7", 48), e.uid, e.agent.ID, "apply", "running", now(), now()); err != nil {
		t.Fatal(err)
	}
	if w, err := e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, previewInstance); err != nil || w != nil {
		t.Fatal("claimed while a legacy job was running", w, err)
	}
	if _, err := e.s.db.Exec(`DELETE FROM sync_jobs`); err != nil {
		t.Fatal(err)
	}
	e.claim(t)
	// Another process replaces the first one: the old request is no longer claimable by anyone.
	replacement := strings.Repeat("e", 48)
	if w, err := e.s.ClaimSkillContentPreviewFor(e.uid, e.agent.ID, replacement); err != nil || w != nil {
		t.Fatal("work bound to an old process was handed to a new one", w, err)
	}
	got := e.state(t, p.ID)
	if got.State == "capturing" || got.State == "captured" {
		t.Fatal(got.State)
	}
	if err := e.report(t, p.ID, exclusiveAbsent()); err == nil {
		t.Fatal("the old process recorded a capture after being replaced")
	}
}

func TestSkillContentPreviewStopFailureAndDismissal(t *testing.T) {
	e := previewFixture(t)
	p := e.create(t, "preview_stop_000000001")
	e.claim(t)
	if err := e.s.FailSkillContentPreviewFor(e.uid, e.agent.ID, p.ID, previewInstance, "raw error: /home/someone/.claude"); err == nil {
		t.Fatal("arbitrary connector text was accepted")
	}
	if err := e.s.FailSkillContentPreviewFor(e.uid, e.agent.ID, p.ID, "f"+previewInstance[1:], "capture_failed"); err == nil {
		t.Fatal("another process failed the request")
	}
	if err := e.s.StopSkillContentPreviewFor(e.uid, p.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.state(t, p.ID); got.State != "cancelled" {
		t.Fatal(got.State)
	}
	if err := e.report(t, p.ID, exclusiveAbsent()); err == nil {
		t.Fatal("recorded a capture after cancellation")
	}

	q := e.create(t, "preview_failure_0000001")
	e.claim(t)
	if err := e.s.FailSkillContentPreviewFor(e.uid, e.agent.ID, q.ID, previewInstance, "scope_unavailable"); err != nil {
		t.Fatal(err)
	}
	if got := e.state(t, q.ID); got.State != "failed" || got.Error != "scope_unavailable" || got.Plan != nil {
		t.Fatalf("%+v", got)
	}

	other, err := e.s.CreateUser("preview-stop-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.s.SkillContentPreviewFor(other.ID, q.ID); err == nil {
		t.Fatal("foreign read")
	}
	if err = e.s.StopSkillContentPreviewFor(other.ID, q.ID); err == nil {
		t.Fatal("foreign cancel")
	}
	if _, err = e.s.DismissSkillContentPreviewFor(other.ID, q.ID); err == nil {
		t.Fatal("foreign dismissal")
	}
	for _, id := range []string{p.ID, q.ID} {
		if _, err = e.s.DismissSkillContentPreviewFor(e.uid, id); err != nil {
			t.Fatal(err)
		}
		if _, err = e.s.SkillContentPreviewFor(e.uid, id); !errors.Is(err, ErrSkillContentPreviewMissing) {
			t.Fatal("dismissed request still readable", err)
		}
	}
	// Dismissing history never touches the pin or the private bytes.
	if _, err = e.s.SkillContentTargetFor(e.uid, e.pin.ID); err != nil {
		t.Fatal(err)
	}
	if rev, err := e.s.PrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, e.bundle.Revision.ID); err != nil || len(rev.Files) == 0 {
		t.Fatal(err)
	}
}

func TestSkillContentPreviewDismissingAnActiveRequestCancelsIt(t *testing.T) {
	e := previewFixture(t)
	p := e.create(t, "preview_dismiss_active_1")
	e.claim(t)
	cancelled, err := e.s.DismissSkillContentPreviewFor(e.uid, p.ID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatal("dismissing live work should revoke it first", cancelled, err)
	}
	if err = e.report(t, p.ID, exclusiveAbsent()); err == nil {
		t.Fatal("recorded after cancellation")
	}
}

func TestSkillContentPreviewListIsMetadataOnlyAndReportsCapability(t *testing.T) {
	e := previewFixture(t)
	p := e.create(t, "preview_list_00000001")
	e.claim(t)
	if err := e.report(t, p.ID, exclusiveAbsent()); err != nil {
		t.Fatal(err)
	}
	list, caps, err := e.s.SkillContentPreviewsFor(e.uid)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	if list[0].Capture != nil || list[0].Plan != nil || list[0].State != "captured" || list[0].ReviewHash == "" {
		t.Fatalf("list responses are bounded summaries: %+v", list[0])
	}
	if len(caps) != 1 || caps[0].AgentID != e.agent.ID || !caps[0].Ready {
		t.Fatal(caps)
	}
	if _, err = e.s.db.Exec(`UPDATE skill_content_preview_capabilities SET observed_at=?`, now()-91000); err != nil {
		t.Fatal(err)
	}
	if _, caps, _ = e.s.SkillContentPreviewsFor(e.uid); caps[0].Ready {
		t.Fatal("an old negotiation is not readiness")
	}
}

func TestSkillContentPreviewCapacityAndOneActiveRequestPerMachine(t *testing.T) {
	e := previewFixture(t)
	for i := 0; i < MaxSkillContentPreviews; i++ {
		id := fmt.Sprintf("%048x", i+1)
		if _, err := e.s.db.Exec(`INSERT INTO skill_content_previews(id,user_id,agent_id,request_key,state,data,created_at) VALUES(?,?,?,?,?,?,?)`, id, e.uid, e.agent.ID, fmt.Sprintf("filler_request_%04d_xx", i), "cancelled", `{}`, now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.s.CreateSkillContentPreviewFor(e.uid, SkillContentPreviewCreate{RequestKey: "preview_over_capacity_1", TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version}); !errors.Is(err, ErrSkillContentPreviewCapacity) {
		t.Fatal(err)
	}
}

func TestSkillContentPreviewConcurrentConnectionsCreateOneActiveRequest(t *testing.T) {
	e := previewFixture(t)
	second, err := Open(e.s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	barrier := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, st := range []*Store{e.s, second} {
		wg.Add(1)
		go func(i int, st *Store) {
			defer wg.Done()
			<-barrier
			_, err := st.CreateSkillContentPreviewFor(e.uid, SkillContentPreviewCreate{RequestKey: fmt.Sprintf("preview_concurrent_%04d", i), TargetID: e.pin.ID, ExpectedTargetVersion: e.pin.Version})
			results <- err
		}(i, st)
	}
	close(barrier)
	wg.Wait()
	close(results)
	ok, busy := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrSkillContentPreview):
			busy++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || busy != 1 {
		t.Fatal("concurrent creates", ok, busy)
	}
}

func TestSkillContentPreviewRevocationAndReopenPreserveOrRemoveCorrectly(t *testing.T) {
	e := previewFixture(t)
	p := e.create(t, "preview_reopen_0000001")
	e.claim(t)
	if err := e.report(t, p.ID, exclusiveAbsent()); err != nil {
		t.Fatal(err)
	}
	dir := e.s.dir
	if err := e.s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.SkillContentPreviewFor(e.uid, p.ID)
	if err != nil || got.State != "captured" || got.ReviewHash == "" {
		t.Fatal("migration or reopen lost the review", got, err)
	}
	if err = reopened.ControlSyncAgent(e.uid, e.agent.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.SkillContentPreviewFor(e.uid, p.ID); !errors.Is(err, ErrSkillContentPreviewMissing) {
		t.Fatal("revoking a machine left its previews behind", err)
	}
	var n int
	_ = reopened.db.QueryRow(`SELECT count(*) FROM skill_content_preview_capabilities`).Scan(&n)
	if n != 0 {
		t.Fatal("revoked machine kept its negotiation")
	}
	if _, err = reopened.PrivateSkillRevisionFor(e.uid, e.bundle.Bundle.ID, e.bundle.Revision.ID); err != nil {
		t.Fatal("revocation removed private bytes", err)
	}
}

func TestSkillContentPreviewNeverPlansFromARevisionThatNoLongerMatchesItsPin(t *testing.T) {
	// Private revisions are immutable through the API. If stored metadata ever
	// stopped matching the identity the pin recorded, no plan may be offered, and
	// the request ends there instead of asking the connector to retry.
	other, err := skillbundle.FromContents([]skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("a different, self-consistent skill")}})
	if err != nil {
		t.Fatal(err)
	}
	otherJSON, _ := json.Marshal(other)
	for name, corrupt := range map[string]func(*testing.T, previewEnv){
		"digest field altered (caught by the planner)": func(t *testing.T, e previewEnv) {
			if _, err := e.s.db.Exec(`UPDATE private_skill_revisions SET manifest=json_set(manifest,'$.digest',?) WHERE id=?`, strings.Repeat("0", 64), e.pin.RevisionID); err != nil {
				t.Fatal(err)
			}
		},
		"valid manifest swapped in (caught by the store)": func(t *testing.T, e previewEnv) {
			if _, err := e.s.db.Exec(`UPDATE private_skill_revisions SET manifest=? WHERE id=?`, string(otherJSON), e.pin.RevisionID); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := previewFixture(t)
			p := e.create(t, "preview_corrupt_revision1")
			e.claim(t)
			corrupt(t, e)
			if err := e.report(t, p.ID, exclusiveAbsent()); err != nil {
				t.Fatal("a request the planner cannot serve is a terminal result, not a connector error to retry:", err)
			}
			got := e.state(t, p.ID)
			if got.State != "failed" || got.Error != "plan_unavailable" || got.Plan != nil || got.Capture != nil || got.ReviewHash != "" {
				t.Fatalf("%+v", got)
			}
		})
	}
}
