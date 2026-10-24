package skillplan

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

const clock int64 = 1000000

func manifest(t *testing.T, entries ...string) skillbundle.Manifest {
	t.Helper()
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("skill")}}
	for _, e := range entries {
		p, v, _ := strings.Cut(e, "=")
		files = append(files, skillbundle.ContentFile{Path: p, Data: []byte(v)})
	}
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func fixture(t *testing.T) Snapshot {
	m := manifest(t, "script=old")
	target := Target{OwnerID: 1, AgentID: "agent-1", EnvironmentID: "1/host/agents", Generation: "enrollment-1", RootID: "physical-root-1", SkillName: "my-skill", Adapter: "posix-executable-v1", Schema: 1}
	return Snapshot{InventoryRevision: 123, Target: target, Desired: Revision{OwnerID: 1, BundleID: strings.Repeat("a", 32), RevisionID: strings.Repeat("b", 32), Name: "my-skill", Manifest: m}, Observation: Observation{InventoryRevision: 123, Generation: target.Generation, Presence: "present", Complete: true, ReceivedAt: clock, Manifest: &m}, Baseline: &Baseline{Target: target, BundleID: strings.Repeat("a", 32), RevisionID: strings.Repeat("c", 32), Manifest: m}, RootOwnership: "exclusive", Mode: "managed"}
}
func build(t *testing.T, s Snapshot) Plan {
	t.Helper()
	p, err := Build(s, clock)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func conflict(t *testing.T, p Plan, code string) {
	t.Helper()
	if p.Ready || len(p.Actions) != 0 || p.ExpectedInstalled != nil || p.ExpectedManaged != nil {
		t.Fatalf("blocked plan exposes changes: %+v", p)
	}
	for _, c := range p.Conflicts {
		if c.Code == code {
			return
		}
	}
	t.Fatalf("missing conflict %q: %+v", code, p.Conflicts)
}
func TestCreateFromExplicitAbsence(t *testing.T) {
	s := fixture(t)
	s.Baseline = nil
	s.Observation.Presence = "absent"
	s.Observation.Manifest = nil
	p := build(t, s)
	if !p.Ready || len(p.Actions) != 2 || p.ExpectedInstalled.Digest != s.Desired.Manifest.Digest || p.ExpectedManaged.Digest != s.Desired.Manifest.Digest {
		t.Fatalf("bad create plan %+v", p)
	}
	for _, a := range p.Actions {
		if a.Kind != "create" || a.Before != nil || a.After == nil {
			t.Fatal(a)
		}
	}
}
func TestUpdatesAndModes(t *testing.T) {
	s := fixture(t)
	s.Desired.Manifest = manifest(t, "script=new")
	p := build(t, s)
	if !p.Ready || len(p.Actions) != 1 || p.Actions[0].Kind != "update" || p.Actions[0].Before.Digest == p.Actions[0].After.Digest {
		t.Fatalf("bad update %+v", p)
	}
	files := append([]skillbundle.File{}, s.Baseline.Manifest.Files...)
	files[1].Executable = true
	m, err := skillbundle.FromMetadata(files)
	if err != nil {
		t.Fatal(err)
	}
	s.Desired.Manifest = m
	p = build(t, s)
	if !p.Ready || len(p.Actions) != 1 || !p.Actions[0].After.Executable {
		t.Fatalf("mode change lost %+v", p)
	}
}
func TestRemoveOnlyUnchangedManagedFiles(t *testing.T) {
	for _, mode := range []string{"managed", "additive"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t)
			s.Mode = mode
			s.Desired.Manifest = manifest(t)
			observed := manifest(t, "script=old", "notes=keep me")
			s.Observation.Manifest = &observed
			p := build(t, s)
			if !p.Ready {
				t.Fatal(p)
			}
			if _, ok := fileMap(*p.ExpectedInstalled)["notes"]; !ok {
				t.Fatal("local-only file removed")
			}
			if _, ok := fileMap(*p.ExpectedManaged)["notes"]; ok {
				t.Fatal("local-only file adopted")
			}
			if mode == "managed" {
				if len(p.Actions) != 1 || p.Actions[0].Kind != "remove" || p.Actions[0].Path != "script" {
					t.Fatal(p.Actions)
				}
			} else {
				if len(p.Actions) != 0 || len(p.Preserved) != 2 {
					t.Fatal(p)
				}
				if _, ok := fileMap(*p.ExpectedManaged)["script"]; !ok {
					t.Fatal("additive ownership lost")
				}
			}
			if p.ExpectedInstalled.Digest == s.Desired.Manifest.Digest {
				t.Fatal("extras incorrectly reported as desired tree")
			}
		})
	}
}
func TestLocalConflictMatrix(t *testing.T) {
	cases := []struct{ name, desired, observed, code string }{
		{"local edit", "old", "local", "local_modified"},
		{"both edit", "remote", "local", "both_modified"},
		{"local deletion", "remote", "", "local_deleted"},
		{"remove edited", "", "local", "local_modified"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fixture(t)
			s.Desired.Manifest = manifest(t)
			observed := manifest(t)
			if tc.desired != "" {
				s.Desired.Manifest = manifest(t, "script="+tc.desired)
			}
			if tc.observed != "" {
				observed = manifest(t, "script="+tc.observed)
			}
			s.Observation.Manifest = &observed
			conflict(t, build(t, s), tc.code)
		})
	}
	t.Run("same independent edit on owned file", func(t *testing.T) {
		s := fixture(t)
		m := manifest(t, "script=new")
		s.Desired.Manifest = m
		s.Observation.Manifest = &m
		p := build(t, s)
		if !p.Ready || len(p.Actions) != 0 {
			t.Fatal(p)
		}
	})
	t.Run("both deleted", func(t *testing.T) {
		s := fixture(t)
		m := manifest(t)
		s.Desired.Manifest = m
		s.Observation.Manifest = &m
		p := build(t, s)
		if !p.Ready || len(p.Actions) != 0 || len(p.ExpectedManaged.Files) != 1 {
			t.Fatal(p)
		}
	})
	t.Run("mixed safe and conflicting", func(t *testing.T) {
		s := fixture(t)
		s.Desired.Manifest = manifest(t, "script=remote", "new=new")
		m := manifest(t, "script=local")
		s.Observation.Manifest = &m
		conflict(t, build(t, s), "both_modified")
	})
	t.Run("additive does not accept edits", func(t *testing.T) {
		s := fixture(t)
		s.Mode = "additive"
		s.Desired.Manifest = manifest(t)
		m := manifest(t, "script=local")
		s.Observation.Manifest = &m
		conflict(t, build(t, s), "local_modified")
	})
}
func TestNoImplicitAdoption(t *testing.T) {
	s := fixture(t)
	s.Baseline = nil
	conflict(t, build(t, s), "adoption_required")
	s = fixture(t)
	s.Baseline.Manifest = manifest(t)
	conflict(t, build(t, s), "unmanaged_path")
}
func TestFailClosedContext(t *testing.T) {
	cases := []struct {
		name, code string
		change     func(*Snapshot)
	}{
		{"shared", "root_shared", func(s *Snapshot) { s.RootOwnership = "shared" }},
		{"unknown root", "root_unknown", func(s *Snapshot) { s.RootOwnership = "unknown" }},
		{"generation", "generation_changed", func(s *Snapshot) { s.Observation.Generation = "old" }},
		{"changed inventory", "inventory_changed", func(s *Snapshot) { s.Observation.InventoryRevision-- }},
		{"incomplete", "observation_unknown", func(s *Snapshot) { s.Observation.Complete = false }},
		{"unknown", "observation_unknown", func(s *Snapshot) { s.Observation.Presence = "unknown"; s.Observation.Manifest = nil }},
		{"stale", "observation_stale", func(s *Snapshot) { s.Observation.ReceivedAt = clock - MaxAgeMillis - 1 }},
		{"future receipt", "observation_stale", func(s *Snapshot) { s.Observation.ReceivedAt = clock + 1 }},
		{"missing receipt", "observation_stale", func(s *Snapshot) { s.Observation.ReceivedAt = 0 }},
		{"missing managed root", "managed_target_missing", func(s *Snapshot) { s.Observation.Presence = "absent"; s.Observation.Manifest = nil }},
		{"legacy", "observation_unknown", func(s *Snapshot) { s.Observation.Manifest = nil }},
		{"future adapter", "adapter_unsupported", func(s *Snapshot) { s.Target.Schema++; s.Baseline.Target = s.Target }},
		{"unsupported adapter", "adapter_unsupported", func(s *Snapshot) { s.Target.Adapter = "windows"; s.Baseline.Target = s.Target }},
		{"corrupt observed hash", "observation_invalid", func(s *Snapshot) { s.Observation.Manifest.Digest = strings.Repeat("0", 64) }},
		{"future manifest", "observation_invalid", func(s *Snapshot) { s.Observation.Manifest.SchemaVersion++ }},
		{"baseline root moved", "baseline_target_mismatch", func(s *Snapshot) { s.Baseline.Target.RootID = "elsewhere" }},
		{"baseline generation", "baseline_target_mismatch", func(s *Snapshot) { s.Baseline.Target.Generation = "old" }},
		{"baseline other connector", "baseline_target_mismatch", func(s *Snapshot) { s.Baseline.Target.AgentID = "agent-2" }},
		{"baseline other bundle", "baseline_bundle_mismatch", func(s *Snapshot) { s.Baseline.BundleID = strings.Repeat("d", 32) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { s := fixture(t); tc.change(&s); conflict(t, build(t, s), tc.code) })
	}
}

// A baseline that does not fit the target or bundle is a diagnostic plan, not an
// error. It is only reached once the earlier context fences have all passed, and
// the target is reported before the bundle.
func TestBaselineMismatchPrecedence(t *testing.T) {
	only := func(t *testing.T, p Plan, code string) {
		t.Helper()
		conflict(t, p, code)
		if len(p.Conflicts) != 1 || p.Conflicts[0] != (Conflict{Path: ".", Code: code}) {
			t.Fatalf("want only %q at the root, got %+v", code, p.Conflicts)
		}
	}
	t.Run("shared root wins", func(t *testing.T) {
		s := fixture(t)
		s.RootOwnership = "shared"
		s.Baseline.Target.RootID = "elsewhere"
		only(t, build(t, s), "root_shared")
	})
	t.Run("stale observation wins", func(t *testing.T) {
		s := fixture(t)
		s.Observation.ReceivedAt = clock - MaxAgeMillis - 1
		s.Baseline.Target.RootID = "elsewhere"
		s.Baseline.BundleID = strings.Repeat("d", 32)
		only(t, build(t, s), "observation_stale")
	})
	t.Run("target before bundle", func(t *testing.T) {
		s := fixture(t)
		s.Baseline.Target.RootID = "elsewhere"
		s.Baseline.BundleID = strings.Repeat("d", 32)
		only(t, build(t, s), "baseline_target_mismatch")
	})
}

func TestBaselineMismatchPlanIsDiagnosticAndDeterministic(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		change func(*Snapshot)
	}{
		{"target", "baseline_target_mismatch", func(s *Snapshot) { s.Baseline.Target.RootID = "elsewhere" }},
		{"bundle", "baseline_bundle_mismatch", func(s *Snapshot) { s.Baseline.BundleID = strings.Repeat("d", 32) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fixture(t)
			tc.change(&s)
			first := build(t, s)
			conflict(t, first, tc.code)
			if first.Preconditions.Baseline == nil {
				t.Fatal("diagnostic plan dropped the baseline it was built from")
			}
			if first.Version != 1 {
				t.Fatalf("planner version changed: %d", first.Version)
			}

			// Reorder only the baseline files, on a private copy so the desired and
			// observed manifests that share the fixture's backing array stay put.
			r := fixture(t)
			tc.change(&r)
			files := append([]skillbundle.File(nil), r.Baseline.Manifest.Files...)
			for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
				files[i], files[j] = files[j], files[i]
			}
			if reflect.DeepEqual(files, r.Baseline.Manifest.Files) {
				t.Fatal("fixture reorder is a no-op")
			}
			r.Baseline.Manifest.Files = files
			second := build(t, r)
			conflict(t, second, tc.code)
			if first.Hash == "" || first.Hash != second.Hash {
				t.Fatalf("baseline file order changed the diagnostic hash: %q vs %q", first.Hash, second.Hash)
			}
			if second.Preconditions.Baseline == nil || len(second.Actions) != 0 {
				t.Fatalf("reordered diagnostic plan is not diagnostic only: %+v", second)
			}
		})
	}
}
func TestRejectInvalidInputs(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Snapshot)
	}{
		{"missing inventory identity", func(s *Snapshot) { s.InventoryRevision = 0 }},
		{"owner", func(s *Snapshot) { s.Desired.OwnerID++ }},
		{"desired revision", func(s *Snapshot) { s.Desired.RevisionID = "latest" }},
		{"baseline owner", func(s *Snapshot) { s.Baseline.Target.OwnerID++ }},
		{"baseline revision", func(s *Snapshot) { s.Baseline.RevisionID = "" }},
		{"mode", func(s *Snapshot) { s.Mode = "mirror" }},
		{"name", func(s *Snapshot) { s.Desired.Name = "other-skill" }},
		{"invalid slot", func(s *Snapshot) { s.Target.SkillName = "../skill" }},
		{"control in identity", func(s *Snapshot) { s.Target.AgentID = "agent\n" }},
		{"desired digest", func(s *Snapshot) { s.Desired.Manifest.Digest = "" }},
		{"baseline metadata", func(s *Snapshot) { s.Baseline.Manifest.TotalBytes++ }},
		{"root enum", func(s *Snapshot) { s.RootOwnership = "yes" }},
		{"absent with files", func(s *Snapshot) { s.Observation.Presence = "absent" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fixture(t)
			tc.change(&s)
			if _, err := Build(s, clock); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	if _, err := Build(fixture(t), 0); err == nil {
		t.Fatal("missing authoritative clock")
	}
}
func TestResultCollisionAndBounds(t *testing.T) {
	t.Run("file directory", func(t *testing.T) {
		s := fixture(t)
		s.Baseline.Manifest = manifest(t)
		m := manifest(t, "local/notes=keep")
		s.Observation.Manifest = &m
		s.Desired.Manifest = manifest(t, "local=replace")
		conflict(t, build(t, s), "result_invalid")
	})
	t.Run("case", func(t *testing.T) {
		s := fixture(t)
		s.Baseline.Manifest = manifest(t)
		m := manifest(t, "Local=keep")
		s.Observation.Manifest = &m
		s.Desired.Manifest = manifest(t, "local=replace")
		conflict(t, build(t, s), "result_invalid")
	})
	t.Run("combined file count", func(t *testing.T) {
		s := fixture(t)
		s.Baseline.Manifest = manifest(t)
		a, b := []string{}, []string{}
		for i := 0; i < 130; i++ {
			a = append(a, fmt.Sprintf("a%d=x", i))
			b = append(b, fmt.Sprintf("b%d=x", i))
		}
		m := manifest(t, a...)
		s.Observation.Manifest = &m
		s.Desired.Manifest = manifest(t, b...)
		conflict(t, build(t, s), "result_invalid")
	})
	t.Run("combined bytes", func(t *testing.T) {
		s := fixture(t)
		s.Baseline.Manifest = manifest(t)
		m := manifest(t, "local=x")
		m.Files[1].Bytes = 5 << 20
		m, _ = skillbundle.FromMetadata(m.Files)
		s.Observation.Manifest = &m
		d := manifest(t, "new=x")
		d.Files[1].Bytes = 5 << 20
		d, _ = skillbundle.FromMetadata(d.Files)
		s.Desired.Manifest = d
		conflict(t, build(t, s), "result_invalid")
	})
}
func TestDeterminismIsolationAndBinding(t *testing.T) {
	s := fixture(t)
	s.Desired.Manifest = manifest(t, "script=new", "new=y")
	before, _ := json.Marshal(s)
	p := build(t, s)
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("input mutated")
	}
	p2, err := Build(s, clock+10)
	if err != nil || !reflect.DeepEqual(p, p2) {
		t.Fatal("fresh repeat changed")
	}
	reverse := func(m *skillbundle.Manifest) {
		for i, j := 0, len(m.Files)-1; i < j; i, j = i+1, j-1 {
			m.Files[i], m.Files[j] = m.Files[j], m.Files[i]
		}
	}
	reverse(&s.Desired.Manifest)
	reverse(s.Observation.Manifest)
	reverse(&s.Baseline.Manifest)
	if q := build(t, s); q.Hash != p.Hash {
		t.Fatal("file order changed hash")
	}
	p.Preconditions.Desired.Manifest.Files[0].Digest = "changed"
	p.Actions[0].After.Digest = "changed"
	if q := build(t, s); q.Hash != p2.Hash {
		t.Fatal("plan aliases input")
	}
	changes := []func(*Snapshot){
		func(s *Snapshot) { s.InventoryRevision++; s.Observation.InventoryRevision++ },
		func(s *Snapshot) { s.Desired.RevisionID = strings.Repeat("d", 32) },
		func(s *Snapshot) { s.Baseline.RevisionID = strings.Repeat("e", 32) },
		func(s *Snapshot) { s.Target.OwnerID = 2; s.Desired.OwnerID = 2; s.Baseline.Target = s.Target },
		func(s *Snapshot) { s.Target.AgentID = "agent-2"; s.Baseline.Target = s.Target },
		func(s *Snapshot) { s.Target.EnvironmentID = "2/other/agents"; s.Baseline.Target = s.Target },
		func(s *Snapshot) {
			s.Target.Generation = "enrollment-2"
			s.Observation.Generation = s.Target.Generation
			s.Baseline.Target = s.Target
		},
		func(s *Snapshot) { s.Target.RootID = "root-2"; s.Baseline.Target = s.Target },
		func(s *Snapshot) { s.Observation.ReceivedAt-- },
		func(s *Snapshot) { s.Mode = "additive" },
	}
	for i, change := range changes {
		raw, _ := json.Marshal(s)
		var c Snapshot
		_ = json.Unmarshal(raw, &c)
		change(&c)
		q := build(t, c)
		if q.Hash == p2.Hash {
			t.Fatalf("binding %d omitted", i)
		}
	}
}

func TestBoundsBeforeCloning(t *testing.T) {
	s := fixture(t)
	s.Desired.Manifest.Files = make([]skillbundle.File, skillbundle.MaxFiles+1)
	if _, err := Build(s, clock); err == nil {
		t.Fatal("oversized metadata accepted")
	}
	s = fixture(t)
	s.Observation.Manifest.Files[0].Path = strings.Repeat("x", skillbundle.MaxPathBytes+1)
	if _, err := Build(s, clock); err == nil {
		t.Fatal("oversized path accepted")
	}
	s = fixture(t)
	s.Target.AgentID = strings.Repeat("x", 1025)
	if _, err := Build(s, clock); err == nil {
		t.Fatal("oversized identity accepted")
	}
}
