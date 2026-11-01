package store

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const installOrigin = "https://reet.example"

type installEnv struct {
	s     *Store
	uid   int64
	env   string
	agent SyncAgent
	hosts map[string]SyncSkill
}

// installFixture enrols a sync agent for "laptop", records a complete scan of
// its claude skills folder and publishes the hosted skills "pdf" and "docx"
// (plus the unpublished "draft").
func installFixture(t *testing.T) *installEnv {
	t.Helper()
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if err := s.ClaimPublishingUsername(uid, "publisher"); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"pdf", "docx", "draft"} {
		if _, err := s.SaveHostedSkill(uid, HostedSkill{Slug: slug, Description: "Fixture", Content: "---\nname: " + slug + "\ndescription: Fixture\n---\n\nBody", Published: slug != "draft"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.TouchEnvironmentFor(uid, "laptop", "claude", "", "/home/u/.claude/skills"); err != nil {
		t.Fatal(err)
	}
	env := EnvironmentIDFor(uid, "laptop", "claude", "")
	if err := s.RecordInventoryFor(uid, env, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.SyncHostedFor(uid, installOrigin)
	if err != nil {
		t.Fatal(err)
	}
	hosts := map[string]SyncSkill{}
	for _, sk := range list {
		hosts[sk.Slug] = sk
	}
	return &installEnv{s: s, uid: uid, env: env, agent: a, hosts: hosts}
}

func (e *installEnv) req(slug string) InstallRequest {
	return InstallRequest{Tool: "claude", Host: "laptop", AgentID: e.agent.ID, Slug: slug, Origin: installOrigin}
}

// dump is every table an install may write, so a refusal can prove it wrote nothing.
func (e *installEnv) dump(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT user_id,target_id,source_id,source_name,desired,revision FROM sync_targets ORDER BY target_id`,
		`SELECT target_id,follow_source,problem FROM sync_policies ORDER BY target_id`,
		`SELECT target_id,mode,baseline,applied_revision FROM sync_options ORDER BY target_id`,
		`SELECT id,agent_id,target_id,kind,status,revision,message FROM sync_jobs ORDER BY id`,
		`SELECT job_id,user_id,target_id,slug,created_target,revision FROM skill_installs ORDER BY job_id`,
	} {
		rows, err := e.s.db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintln(&b, vals...)
		}
		rows.Close()
		b.WriteString("--\n")
	}
	return b.String()
}

func (e *installEnv) target(t *testing.T) SyncTarget {
	t.Helper()
	list, err := e.s.SyncTargetsFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range list {
		if x.Target.ID == e.env {
			return x
		}
	}
	t.Fatal("no saved set")
	return SyncTarget{}
}

func (e *installEnv) hasTarget() bool {
	var n int
	e.s.db.QueryRow(`SELECT COUNT(*) FROM sync_targets WHERE target_id=?`, e.env).Scan(&n)
	return n > 0
}

func slugs(list []SyncSkill) []string {
	out := []string{}
	for _, sk := range list {
		out = append(out, sk.Slug)
	}
	return out
}

func wantInstallError(t *testing.T, err error, code, message string) {
	t.Helper()
	var ie *InstallError
	if !errors.As(err, &ie) {
		t.Fatalf("want InstallError %s, got %v", code, err)
	}
	if ie.Code != code {
		t.Fatalf("code %q (%s), want %q", ie.Code, ie.Message, code)
	}
	if message != "" && ie.Message != message {
		t.Fatalf("message %q, want %q", ie.Message, message)
	}
}

func TestMergeDesiredKeepsOrderAddsOnceAndDoesNotMutate(t *testing.T) {
	a := SyncSkill{ID: "x/a", Slug: "a"}
	b := SyncSkill{ID: "x/b", Slug: "b"}
	c := SyncSkill{ID: "x/c", Slug: "c"}
	for name, tc := range map[string]struct {
		in   []SyncSkill
		add  SyncSkill
		want []string
	}{
		"empty":         {nil, a, []string{"a"}},
		"appends":       {[]SyncSkill{b, a}, c, []string{"b", "a", "c"}},
		"already there": {[]SyncSkill{a, b}, a, []string{"a", "b"}},
	} {
		t.Run(name, func(t *testing.T) {
			before := append([]SyncSkill(nil), tc.in...)
			got := MergeDesired(tc.in, tc.add)
			if !reflect.DeepEqual(slugs(got), tc.want) {
				t.Fatalf("%v, want %v", slugs(got), tc.want)
			}
			if !reflect.DeepEqual(tc.in, before) {
				t.Fatal("argument mutated")
			}
		})
	}
	// An append never writes into the caller's spare capacity.
	spare := make([]SyncSkill, 1, 4)
	spare[0] = a
	got := MergeDesired(spare, b)
	got[0].Slug = "changed"
	if spare[0].Slug != "a" || len(spare) != 1 || spare[:2][1].Slug != "" {
		t.Fatal("shared backing array")
	}
}

func TestRemoveDesiredDropsOneSlug(t *testing.T) {
	a := SyncSkill{ID: "x/a", Slug: "a"}
	b := SyncSkill{ID: "x/b", Slug: "b"}
	c := SyncSkill{ID: "x/c", Slug: "c"}
	for name, tc := range map[string]struct {
		in   []SyncSkill
		slug string
		want []string
	}{
		"middle":  {[]SyncSkill{a, b, c}, "b", []string{"a", "c"}},
		"missing": {[]SyncSkill{a, b}, "z", []string{"a", "b"}},
		"only":    {[]SyncSkill{a}, "a", []string{}},
		"nil":     {nil, "a", []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			before := append([]SyncSkill(nil), tc.in...)
			got := RemoveDesired(tc.in, tc.slug)
			if got == nil || !reflect.DeepEqual(slugs(got), tc.want) {
				t.Fatalf("%v, want %v", slugs(got), tc.want)
			}
			if !reflect.DeepEqual(tc.in, before) {
				t.Fatal("argument mutated")
			}
		})
	}
}

func TestInstallCreatesAdditiveSetAndQueuesPreview(t *testing.T) {
	e := installFixture(t)
	if e.hasTarget() {
		t.Fatal("fixture already has a saved set")
	}
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if res.JobID == "" || res.Reused || res.Host != "laptop" || res.Directory != "/home/u/.claude/skills" {
		t.Fatalf("%+v", res)
	}
	tg := e.target(t)
	if tg.Mode != "additive" || tg.FollowSource || tg.SourceID != "hosted" || tg.SourceName != "My published reet.sh skills" {
		t.Fatalf("%+v", tg)
	}
	if !reflect.DeepEqual(slugs(tg.Desired), []string{"pdf"}) || tg.Desired[0].ID != e.hosts["pdf"].ID {
		t.Fatalf("%+v", tg.Desired)
	}
	jobs, _ := e.s.SyncJobsFor(e.uid)
	if len(jobs) != 1 || jobs[0].ID != res.JobID || jobs[0].Kind != "preview" || jobs[0].Status != "queued" || jobs[0].Revision != tg.Revision || jobs[0].TargetID != e.env || jobs[0].AgentID != e.agent.ID {
		t.Fatalf("%+v", jobs)
	}
	var created int
	var rev, slug string
	if err := e.s.db.QueryRow(`SELECT created_target,revision,slug FROM skill_installs WHERE job_id=?`, res.JobID).Scan(&created, &rev, &slug); err != nil || created != 1 || rev != tg.Revision || slug != "pdf" {
		t.Fatal(created, rev, slug, err)
	}
}

func TestInstallKeepsAnExistingSetAndMode(t *testing.T) {
	e := installFixture(t)
	for _, id := range []string{"a/b/one", "a/b/two"} {
		e.s.UpsertFor(e.uid, mustRef(t, id), Patch{})
	}
	if err := e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "mirror", SkillIDs: []string{"a/b/one", "a/b/two"}}); err != nil {
		t.Fatal(err)
	}
	before := e.target(t)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	tg := e.target(t)
	if tg.Mode != "mirror" || tg.SourceID != before.SourceID || tg.Revision == before.Revision {
		t.Fatalf("%+v", tg)
	}
	if got := slugs(tg.Desired); !reflect.DeepEqual(got, []string{slugs(before.Desired)[0], slugs(before.Desired)[1], "pdf"}) {
		t.Fatalf("%v", got)
	}
	var created int
	e.s.db.QueryRow(`SELECT created_target FROM skill_installs WHERE job_id=?`, res.JobID).Scan(&created)
	if created != 0 {
		t.Fatal("created_target set for an existing set")
	}
}

func TestInstallRefusals(t *testing.T) {
	cases := []struct {
		name, code, message string
		prepare             func(t *testing.T, e *installEnv, in *InstallRequest)
	}{
		{"unpublished", "unpublished", "Publish this skill to install it.", func(t *testing.T, e *installEnv, in *InstallRequest) { in.Slug = "draft" }},
		{"unknown skill", "unpublished", "", func(t *testing.T, e *installEnv, in *InstallRequest) { in.Slug = "nope" }},
		{"offline", "connector-offline", "laptop isn't connected to reet right now, so it can't install this skill.", func(t *testing.T, e *installEnv, in *InstallRequest) {
			e.s.db.Exec(`UPDATE sync_agents SET last_seen=? WHERE id=?`, time.Now().Add(-time.Hour).UnixMilli(), e.agent.ID)
		}},
		{"paused", "connector-offline", "", func(t *testing.T, e *installEnv, in *InstallRequest) {
			e.s.db.Exec(`UPDATE sync_agents SET paused=1 WHERE id=?`, e.agent.ID)
		}},
		{"unknown agent", "connector-offline", "", func(t *testing.T, e *installEnv, in *InstallRequest) { in.AgentID = "missing" }},
		{"no scan", "no-scan", "reet hasn't read laptop's claude skills folder recently. Run a scan on laptop, then try again.", func(t *testing.T, e *installEnv, in *InstallRequest) {
			e.s.db.Exec(`DELETE FROM environment_scans`)
		}},
		{"no environment", "no-scan", "", func(t *testing.T, e *installEnv, in *InstallRequest) { in.Host = "elsewhere" }},
		{"incomplete scan", "no-scan", "", func(t *testing.T, e *installEnv, in *InstallRequest) {
			e.s.db.Exec(`UPDATE environment_scans SET complete=0`)
		}},
		{"old scan", "no-scan", "", func(t *testing.T, e *installEnv, in *InstallRequest) {
			e.s.db.Exec(`UPDATE environment_scans SET scanned_at=?`, time.Now().Add(-15*24*time.Hour).UnixMilli())
		}},
		{"unsupported agent", "unsupported-agent", "Skills can't be installed for codex from reet yet.", func(t *testing.T, e *installEnv, in *InstallRequest) { in.Tool = "codex" }},
		{"follows source", "follows-source", "laptop's skills follow another machine, so this skill would be removed the next time it syncs.", func(t *testing.T, e *installEnv, in *InstallRequest) {
			e.s.RecordEnvironmentFor(e.uid, "a/b/one", "other", "claude", "", "", "")
			if err := e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "mirror", FollowSource: true}); err != nil {
				t.Fatal(err)
			}
		}},
		{"already installed", "already-installed", "This skill is already on laptop for claude.", func(t *testing.T, e *installEnv, in *InstallRequest) {
			e.s.UpsertFor(e.uid, mustRef(t, "a/b/pdf"), Patch{})
			if err := e.s.RecordEnvironmentFor(e.uid, "a/b/pdf", "laptop", "claude", "", "/home/u/.claude/skills", ""); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := installFixture(t)
			in := e.req("pdf")
			tc.prepare(t, e, &in)
			before := e.dump(t)
			_, err := e.s.InstallHostedSkillFor(e.uid, in)
			wantInstallError(t, err, tc.code, tc.message)
			if after := e.dump(t); after != before {
				t.Fatalf("refusal wrote:\n%s\nvs\n%s", before, after)
			}
		})
	}
}

func TestInstallSameSkillTwiceReusesTheJob(t *testing.T) {
	e := installFixture(t)
	first, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	before := e.dump(t)
	second, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil || !second.Reused || second.JobID != first.JobID || second.Directory != first.Directory || second.Host != "laptop" {
		t.Fatalf("%+v %v", second, err)
	}
	if e.dump(t) != before {
		t.Fatal("reuse wrote")
	}
}

func TestInstallDifferentSkillWhilePendingIsRefused(t *testing.T) {
	e := installFixture(t)
	if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
		t.Fatal(err)
	}
	before := e.dump(t)
	_, err := e.s.InstallHostedSkillFor(e.uid, e.req("docx"))
	wantInstallError(t, err, "install-in-progress", "Another install is waiting on laptop. Finish or cancel it first.")
	if e.dump(t) != before {
		t.Fatal("refusal wrote")
	}
}

func TestInstallRefusedWhenTheMachineIsBusyWithAnotherJob(t *testing.T) {
	e := installFixture(t)
	if _, err := e.s.QueueSyncJob(e.uid, e.agent.ID, "", "scan"); err != nil {
		t.Fatal(err)
	}
	before := e.dump(t)
	_, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	wantInstallError(t, err, "install-in-progress", "laptop is busy with another sync job. Finish or cancel it, then try again.")
	if e.dump(t) != before {
		t.Fatal("refusal wrote")
	}
	// An apply job is the same.
	e.s.db.Exec(`UPDATE sync_jobs SET kind='apply'`)
	before = e.dump(t)
	_, err = e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	wantInstallError(t, err, "install-in-progress", "laptop is busy with another sync job. Finish or cancel it, then try again.")
	if e.dump(t) != before {
		t.Fatal("refusal wrote")
	}
}

func TestInstallResolvesAMergedMachineAlias(t *testing.T) {
	e := installFixture(t)
	if err := e.s.MergeMachinesFor(e.uid, "laptop", "work-laptop"); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"laptop", "work-laptop"} {
		in := e.req("pdf")
		in.Host = host
		res, err := e.s.InstallHostedSkillFor(e.uid, in)
		if err != nil {
			t.Fatal(host, err)
		}
		if res.Host != "work-laptop" {
			t.Fatalf("%+v", res)
		}
	}
}

func TestCancelRevertsTheMerge(t *testing.T) {
	e := installFixture(t)
	e.s.UpsertFor(e.uid, mustRef(t, "a/b/one"), Patch{})
	if err := e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "mirror", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	was := slugs(e.target(t).Desired)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := e.s.CancelInstallFor(e.uid, res.JobID)
	if err != nil || !reverted {
		t.Fatal(reverted, err)
	}
	tg := e.target(t)
	if !reflect.DeepEqual(slugs(tg.Desired), was) || tg.Mode != "mirror" {
		t.Fatalf("%+v", tg)
	}
	jobs, _ := e.s.SyncJobsFor(e.uid)
	if len(jobs) != 1 || jobs[0].Status != "cancelled" {
		t.Fatalf("%+v", jobs)
	}
}

func TestCancelRemovesASetTheInstallCreated(t *testing.T) {
	e := installFixture(t)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := e.s.CancelInstallFor(e.uid, res.JobID)
	if err != nil || !reverted {
		t.Fatal(reverted, err)
	}
	for _, table := range []string{"sync_targets", "sync_policies", "sync_options"} {
		var n int
		e.s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE target_id=?`, e.env).Scan(&n)
		if n != 0 {
			t.Fatal(table, "row left behind")
		}
	}
	// With the set gone the same skill can be installed again.
	if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
		t.Fatal(err)
	}
}

func TestCancelLeavesASetSomeoneElseChanged(t *testing.T) {
	e := installFixture(t)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	e.s.db.Exec(`UPDATE sync_targets SET revision='changed-by-someone' WHERE target_id=?`, e.env)
	reverted, err := e.s.CancelInstallFor(e.uid, res.JobID)
	if err != nil || reverted {
		t.Fatal(reverted, err)
	}
	if got := slugs(e.target(t).Desired); !reflect.DeepEqual(got, []string{"pdf"}) {
		t.Fatalf("%v", got)
	}
	jobs, _ := e.s.SyncJobsFor(e.uid)
	if jobs[0].Status != "cancelled" {
		t.Fatalf("%+v", jobs)
	}
}

func TestCancelRefusesARunningJob(t *testing.T) {
	e := installFixture(t)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if j, err := e.s.ClaimSyncJob(e.uid, e.agent); err != nil || j == nil {
		t.Fatal(j, err)
	}
	before := e.dump(t)
	reverted, err := e.s.CancelInstallFor(e.uid, res.JobID)
	wantInstallError(t, err, "cannot-cancel", "This install is already running and can't be cancelled.")
	if reverted || e.dump(t) != before {
		t.Fatal("running job changed")
	}
	// A finished job and a job that is not an install are refused too.
	e.s.db.Exec(`UPDATE sync_jobs SET status='succeeded'`)
	_, err = e.s.CancelInstallFor(e.uid, res.JobID)
	wantInstallError(t, err, "cannot-cancel", "")
	if _, err = e.s.CancelInstallFor(e.uid, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	other, _ := e.s.CreateUser("other@install.invalid", "Other", "member", "")
	if _, err = e.s.CancelInstallFor(other.ID, res.JobID); !errors.Is(err, ErrNotFound) {
		t.Fatal("another account cancelled it", err)
	}
}

func TestCancelAnAwaitingApprovalJob(t *testing.T) {
	e := installFixture(t)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	j, err := e.s.ClaimSyncJob(e.uid, e.agent)
	if err != nil || j == nil {
		t.Fatal(j, err)
	}
	if err = e.s.FinishSyncJob(e.uid, e.agent.ID, j.ID, "awaiting-approval", strings.Repeat("a", 64), "", []byte(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	jobs, _ := e.s.SyncJobsFor(e.uid)
	if jobs[0].Status != "awaiting-approval" {
		t.Fatalf("setup: %+v", jobs)
	}
	reverted, err := e.s.CancelInstallFor(e.uid, res.JobID)
	if err != nil || !reverted {
		t.Fatal(reverted, err)
	}
	jobs, _ = e.s.SyncJobsFor(e.uid)
	if jobs[0].Status != "cancelled" || e.hasTarget() {
		t.Fatalf("%+v", jobs)
	}
}

func TestInstallSurfacesAHostedListError(t *testing.T) {
	e := installFixture(t)
	before := e.dump(t)
	in := e.req("pdf")
	in.Origin = ""
	_, err := e.s.InstallHostedSkillFor(e.uid, in)
	var ie *InstallError
	if err == nil || errors.As(err, &ie) {
		t.Fatalf("want a plain error, got %v", err)
	}
	if e.dump(t) != before {
		t.Fatal("error wrote")
	}
	// An earlier refusal still wins over the hosted-list error.
	in.Tool = "codex"
	_, err = e.s.InstallHostedSkillFor(e.uid, in)
	wantInstallError(t, err, "unsupported-agent", "")
	// A readable list without the slug is still unpublished.
	_, err = e.s.InstallHostedSkillFor(e.uid, e.req("nope"))
	wantInstallError(t, err, "unpublished", "")
}
