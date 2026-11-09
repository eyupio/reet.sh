package store

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
)

// TestListForWholeLibraryMatchesPagedProjection checks that the whole-library
// listing (which reads the user's tags, canonical library items and
// environments by range) attaches exactly what a page or a single read (which
// probe an IN (...) list of ids) attaches, without picking up another user's
// rows or rows for hidden skills.
func TestListForWholeLibraryMatchesPagedProjection(t *testing.T) {
	s := newStore(t)
	owner, err := s.CreateUser("projection@example.com", "projection", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateUser("projection-other@example.com", "projection-other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	var ids []string
	for i := range 5 {
		r := ref.Ref{
			ID:         fmt.Sprintf("projection/repo/skill-%d", i),
			Source:     "projection/repo",
			Slug:       fmt.Sprintf("skill-%d", i),
			SourceType: "github",
			URL:        fmt.Sprintf("https://example.com/projection-%d", i),
		}
		ids = append(ids, r.ID)
		patch := Patch{}
		if i == 1 {
			patch.Used = &yes
		}
		if i == 2 {
			patch.Bookmarked = &yes
		}
		for _, uid := range []int64{owner.ID, other.ID} {
			if _, err := s.UpsertFor(uid, r, patch); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.TagManyFor(owner.ID, []string{r.ID}, []string{fmt.Sprintf("tag-%d", i%2), "shared"}, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := s.TagManyFor(other.ID, []string{r.ID}, []string{"other-user-tag"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range ids[:2] {
		if err := s.RecordEnvironmentFor(owner.ID, id, "owner-host", "claude", "", "/owner", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordEnvironmentFor(other.ID, ids[0], "other-host", "claude", "", "/other", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(other.ID, ids[3], "other-host", "claude", "", "/other", ""); err != nil {
		t.Fatal(err)
	}
	disabled, _, err := s.GetFor(owner.ID, ids[3])
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.LibraryItemFor(owner.ID, disabled.LibraryItemID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetLibraryStatusFor(owner.ID, item.ID, LibraryDisabled, item.Version); err != nil {
		t.Fatal(err)
	}
	// A hidden skill stays on the shelf but out of listings; its tags, item and
	// environment rows must not reach any listed skill.
	if err := s.SetHostedSkillVisible(ids[4], false); err != nil {
		t.Fatal(err)
	}

	type projection struct {
		Tags             []Tag
		LibraryItemID    string
		Status           string
		Environments     []string
		EnvironmentCount int
	}
	project := func(k Skill) projection {
		p := projection{Tags: k.Tags, LibraryItemID: k.LibraryItemID, Status: k.Status, EnvironmentCount: k.EnvironmentCount}
		for _, env := range k.Environments {
			p.Environments = append(p.Environments, env.ID)
		}
		return p
	}
	for _, compact := range []bool{false, true} {
		whole, counts, err := s.ListFor(owner.ID, Filter{Compact: compact})
		if err != nil {
			t.Fatal(err)
		}
		if len(whole) != 4 || counts.All != 4 {
			t.Fatalf("compact=%v: whole listing = %d skills, counts %+v", compact, len(whole), counts)
		}
		statuses := map[string]string{}
		for i, k := range whole {
			statuses[k.ID] = k.Status
			page, _, err := s.ListFor(owner.ID, Filter{Compact: compact, Limit: 1, Offset: i})
			if err != nil || len(page) != 1 || page[0].ID != k.ID {
				t.Fatalf("compact=%v: page %d = %+v, %v", compact, i, page, err)
			}
			if got, want := project(k), project(page[0]); !reflect.DeepEqual(got, want) {
				t.Fatalf("compact=%v %s: whole listing %+v, page %+v", compact, k.ID, got, want)
			}
			if k.LibraryItemID == "" {
				t.Fatalf("compact=%v %s: no canonical library item", compact, k.ID)
			}
			for _, tag := range k.Tags {
				if tag.Name == "other-user-tag" {
					t.Fatalf("compact=%v %s: another user's tag leaked: %+v", compact, k.ID, k.Tags)
				}
			}
			wantEnvs := 0
			if k.ID == ids[0] || k.ID == ids[1] {
				wantEnvs = 1
			}
			if k.EnvironmentCount != wantEnvs {
				t.Fatalf("compact=%v %s: environment count %d, want %d", compact, k.ID, k.EnvironmentCount, wantEnvs)
			}
			if compact {
				continue
			}
			one, ok, err := s.GetFor(owner.ID, k.ID)
			if err != nil || !ok {
				t.Fatalf("GetFor %s: %v %v", k.ID, ok, err)
			}
			if got, want := project(k), project(one); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: whole listing %+v, GetFor %+v", k.ID, got, want)
			}
		}
		want := map[string]string{ids[0]: LibraryOnShelf, ids[1]: LibraryInUse, ids[2]: LibraryToTest, ids[3]: LibraryDisabled}
		if !reflect.DeepEqual(statuses, want) {
			t.Fatalf("compact=%v: statuses %v, want %v", compact, statuses, want)
		}
	}
}
