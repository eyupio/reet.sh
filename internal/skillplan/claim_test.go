package skillplan

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

// claimManifest builds a manifest from explicit contents, for the cases the
// shared manifest helper cannot express (a different SKILL.md, executable bits).
func claimManifest(t *testing.T, files ...skillbundle.ContentFile) skillbundle.Manifest {
	t.Helper()
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func claimPaths(m skillbundle.Manifest) []string {
	paths := []string{}
	for _, f := range m.Files {
		paths = append(paths, f.Path)
	}
	return paths
}

func TestClaimSubsetIdentical(t *testing.T) {
	observed := manifest(t, "script=old", "docs/guide.md=guide")
	revision := manifest(t, "script=old", "docs/guide.md=guide")
	got, counts, err := ClaimSubset(observed, revision)
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != revision.Digest || !reflect.DeepEqual(got, revision) {
		t.Fatalf("identical trees should claim the whole revision: %+v", got)
	}
	if counts != (ClaimCounts{Matched: 3}) {
		t.Fatalf("counts %+v", counts)
	}
}

func TestClaimSubsetOlderRevisionWithLocalExtras(t *testing.T) {
	observed := manifest(t, "script=old", "notes/mine.txt=mine")
	revision := manifest(t, "script=old")
	got, counts, err := ClaimSubset(observed, revision)
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != revision.Digest {
		t.Fatalf("claimed %v, want the revision's files", claimPaths(got))
	}
	for _, f := range got.Files {
		if f.Path == "notes/mine.txt" {
			t.Fatalf("a local file was claimed: %+v", got.Files)
		}
	}
	if counts != (ClaimCounts{Matched: 2, ObservedOnly: 1}) {
		t.Fatalf("counts %+v", counts)
	}
}

func TestClaimSubsetCountsFilesOnlyTheRevisionHas(t *testing.T) {
	observed := manifest(t, "script=old")
	revision := manifest(t, "script=old", "extra=new")
	got, counts, err := ClaimSubset(observed, revision)
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != observed.Digest {
		t.Fatalf("claimed %v, want only what is installed", claimPaths(got))
	}
	if counts != (ClaimCounts{Matched: 2, RevisionOnly: 1}) {
		t.Fatalf("counts %+v", counts)
	}
}

func TestClaimSubsetNeverClaimsDifferingBytesOrMode(t *testing.T) {
	skill := skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("skill")}
	cases := []struct {
		name               string
		observed, revision skillbundle.ContentFile
	}{
		{"different bytes", skillbundle.ContentFile{Path: "script", Data: []byte("local")}, skillbundle.ContentFile{Path: "script", Data: []byte("upstream")}},
		{"executable bit gained", skillbundle.ContentFile{Path: "script", Data: []byte("same"), Executable: true}, skillbundle.ContentFile{Path: "script", Data: []byte("same")}},
		{"executable bit lost", skillbundle.ContentFile{Path: "script", Data: []byte("same")}, skillbundle.ContentFile{Path: "script", Data: []byte("same"), Executable: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			observed := claimManifest(t, skill, c.observed)
			revision := claimManifest(t, skill, c.revision)
			got, counts, err := ClaimSubset(observed, revision)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(claimPaths(got), []string{"SKILL.md"}) {
				t.Fatalf("claimed %v, want only SKILL.md", claimPaths(got))
			}
			if counts != (ClaimCounts{Matched: 1, ObservedOnly: 1, RevisionOnly: 1}) {
				t.Fatalf("counts %+v", counts)
			}
		})
	}
}

func TestClaimSubsetRequiresSkillMd(t *testing.T) {
	script := skillbundle.ContentFile{Path: "script", Data: []byte("same")}
	cases := []struct {
		name               string
		observed, revision skillbundle.Manifest
	}{
		{
			"differing SKILL.md",
			claimManifest(t, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("local")}, script),
			claimManifest(t, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("upstream")}, script),
		},
		{
			"SKILL.md mode differs",
			claimManifest(t, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("skill"), Executable: true}, script),
			claimManifest(t, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("skill")}, script),
		},
		{
			"no overlap",
			claimManifest(t, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("local")}, skillbundle.ContentFile{Path: "mine", Data: []byte("1")}),
			claimManifest(t, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("upstream")}, skillbundle.ContentFile{Path: "theirs", Data: []byte("2")}),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, counts, err := ClaimSubset(c.observed, c.revision)
			if !errors.Is(err, ErrNotClaimable) {
				t.Fatalf("err %v, want ErrNotClaimable", err)
			}
			if !reflect.DeepEqual(got, skillbundle.Manifest{}) || counts != (ClaimCounts{}) {
				t.Fatalf("a refused claim must be empty: %+v %+v", got, counts)
			}
		})
	}
}

func TestClaimSubsetRejectsInvalidManifests(t *testing.T) {
	good := manifest(t, "script=old")
	incomplete := manifest(t, "script=old")
	incomplete.Complete = false
	withIssue := manifest(t, "script=old")
	withIssue.Issues = []skillbundle.Issue{{Path: "script", Code: "unreadable"}}
	forgedManifestDigest := manifest(t, "script=old")
	forgedManifestDigest.Digest = strings.Repeat("0", len(good.Digest))
	forgedFileDigest := manifest(t, "script=old")
	forgedFileDigest.Files = append([]skillbundle.File{}, forgedFileDigest.Files...)
	forgedFileDigest.Files[1].Digest = manifest(t, "script=other").Files[1].Digest
	forgedTotal := manifest(t, "script=old")
	forgedTotal.TotalBytes++
	unsupported := manifest(t, "script=old")
	unsupported.Algorithm = "md5"
	cases := []struct {
		name string
		m    skillbundle.Manifest
	}{
		{"incomplete", incomplete},
		{"observation issues", withIssue},
		{"forged manifest digest", forgedManifestDigest},
		{"forged file digest", forgedFileDigest},
		{"forged total bytes", forgedTotal},
		{"unsupported algorithm", unsupported},
		{"zero value", skillbundle.Manifest{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for side, args := range map[string][2]skillbundle.Manifest{"observed": {c.m, good}, "revision": {good, c.m}} {
				got, counts, err := ClaimSubset(args[0], args[1])
				if err == nil || errors.Is(err, ErrNotClaimable) {
					t.Fatalf("invalid %s manifest: err %v, want a validation error that is not ErrNotClaimable", side, err)
				}
				if !reflect.DeepEqual(got, skillbundle.Manifest{}) || counts != (ClaimCounts{}) {
					t.Fatalf("invalid %s manifest produced a claim: %+v %+v", side, got, counts)
				}
			}
		})
	}
}

func TestClaimSubsetIsOrderIndependent(t *testing.T) {
	observed := manifest(t, "a=1", "b/c=2", "notes/mine.txt=mine", "z=3")
	revision := manifest(t, "a=1", "b/c=2", "z=other")
	want, wantCounts, err := ClaimSubset(observed, revision)
	if err != nil {
		t.Fatal(err)
	}
	reverse := func(m skillbundle.Manifest) skillbundle.Manifest {
		files := make([]skillbundle.File, len(m.Files))
		for i, f := range m.Files {
			files[len(files)-1-i] = f
		}
		m.Files = files
		return m
	}
	for name, args := range map[string][2]skillbundle.Manifest{
		"observed reversed": {reverse(observed), revision},
		"revision reversed": {observed, reverse(revision)},
		"both reversed":     {reverse(observed), reverse(revision)},
	} {
		got, counts, err := ClaimSubset(args[0], args[1])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(got, want) || counts != wantCounts {
			t.Fatalf("%s: got %+v %+v, want %+v %+v", name, got, counts, want, wantCounts)
		}
	}
	if !reflect.DeepEqual(claimPaths(want), []string{"SKILL.md", "a", "b/c"}) {
		t.Fatalf("claimed %v", claimPaths(want))
	}
	if wantCounts != (ClaimCounts{Matched: 3, ObservedOnly: 2, RevisionOnly: 1}) {
		t.Fatalf("counts %+v", wantCounts)
	}
}

func TestIsSubset(t *testing.T) {
	skill := skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("skill")}
	revision := claimManifest(t, skill, skillbundle.ContentFile{Path: "script", Data: []byte("old"), Executable: true}, skillbundle.ContentFile{Path: "docs/guide.md", Data: []byte("guide")})
	cases := []struct {
		name    string
		managed skillbundle.Manifest
		want    bool
	}{
		{"the whole revision", revision, true},
		{"an exact subset", claimManifest(t, skill, skillbundle.ContentFile{Path: "script", Data: []byte("old"), Executable: true}), true},
		{"SKILL.md alone", claimManifest(t, skill), true},
		{"an extra path", claimManifest(t, skill, skillbundle.ContentFile{Path: "notes/mine.txt", Data: []byte("mine")}), false},
		{"a different digest", claimManifest(t, skill, skillbundle.ContentFile{Path: "script", Data: []byte("edited"), Executable: true}), false},
		{"a different mode", claimManifest(t, skill, skillbundle.ContentFile{Path: "script", Data: []byte("old")}), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSubset(c.managed, revision); got != c.want {
				t.Fatalf("IsSubset = %v, want %v", got, c.want)
			}
		})
	}
}
