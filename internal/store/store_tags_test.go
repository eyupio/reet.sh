package store

import (
	"github.com/eyupio/reet.sh/internal/ref"
	"testing"
)

func TestRemovedTagsStayRemovedAcrossTopicSync(t *testing.T) {
	for _, account := range []bool{false, true} {
		for _, source := range []string{"user", "topic"} {
			for _, bulk := range []bool{false, true} {
				name := source
				if account {
					name += "/account"
				} else {
					name += "/legacy"
				}
				if bulk {
					name += "/bulk"
				} else {
					name += "/single"
				}
				t.Run(name, func(t *testing.T) {
					s := newStore(t)
					var userID int64
					if account {
						var err error
						userID, err = s.DefaultOwner()
						if err != nil {
							t.Fatal(err)
						}
					}
					upsert := func(r ref.Ref) error {
						if account {
							_, err := s.UpsertFor(userID, r, Patch{})
							return err
						}
						_, err := s.Upsert(r, Patch{})
						return err
					}
					get := func() (Skill, bool, error) {
						if account {
							return s.GetFor(userID, "a/b/alpha")
						}
						return s.Get("a/b/alpha")
					}
					r := mustRef(t, "a/b/alpha")
					other := mustRef(t, "a/b/beta")
					if err := upsert(r); err != nil {
						t.Fatal(err)
					}
					sync := func(ids []string) {
						t.Helper()
						topics := []Topic{{Name: "Design", Skills: ids}}
						var err error
						if account {
							err = s.ReplaceTopicsForAll(topics)
						} else {
							err = s.ReplaceTopics(topics)
						}
						if err != nil {
							t.Fatal(err)
						}
					}
					if source == "topic" {
						sync([]string{r.ID})
					} else if err := s.AddSkillTagsFor(userID, r.ID, []string{"Design"}); err != nil {
						t.Fatal(err)
					}
					sk, _, err := get()
					if err != nil || len(sk.Tags) != 1 {
						t.Fatalf("initial tags: %+v, %v", sk.Tags, err)
					}
					tagID := sk.Tags[0].ID
					if bulk {
						if account {
							_, err = s.TagManyFor(userID, []string{r.ID}, nil, []int64{tagID})
						} else {
							_, err = s.TagMany([]string{r.ID}, nil, []int64{tagID})
						}
					} else if account {
						err = s.RemoveSkillTagFor(userID, r.ID, tagID)
					} else {
						err = s.RemoveSkillTag(r.ID, tagID)
					}
					if err != nil {
						t.Fatal(err)
					}
					for _, ids := range [][]string{{r.ID}, {}, {r.ID}, {other.ID}, {r.ID}} {
						sync(ids)
						if err := upsert(r); err != nil {
							t.Fatal(err)
						}
						sk, _, err = get()
						if err != nil || len(sk.Tags) != 0 {
							t.Fatalf("removed tag reappeared after sync: %+v, %v", sk.Tags, err)
						}
						var matches []Skill
						if account {
							matches, _, err = s.ListFor(userID, Filter{Tags: []int64{tagID}})
						} else {
							matches, _, err = s.List(Filter{Tags: []int64{tagID}})
						}
						if err != nil || len(matches) != 0 {
							t.Fatalf("removed tag still matches filter: %+v, %v", matches, err)
						}
					}
					if err := s.AddSkillTagsFor(userID, r.ID, []string{"Design"}); err != nil {
						t.Fatal(err)
					}
					sk, _, err = get()
					if err != nil || len(sk.Tags) != 1 {
						t.Fatalf("explicit re-add failed: %+v, %v", sk.Tags, err)
					}
				})
			}
		}
	}
}

func TestLegacyBulkTagRemovalDoesNotChangeAnotherUser(t *testing.T) {
	s := newStore(t)
	user, err := s.CreateUser("member@example.com", "Member", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	r := mustRef(t, "a/b/alpha")
	for _, id := range []int64{0, user.ID} {
		if id == 0 {
			_, err = s.Upsert(r, Patch{})
		} else {
			_, err = s.UpsertFor(id, r, Patch{})
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := s.AddSkillTagsFor(id, r.ID, []string{"Shared name"}); err != nil {
			t.Fatal(err)
		}
	}
	tags, err := s.ListTagsFor(0)
	if err != nil || len(tags) != 1 {
		t.Fatalf("initial tags: %+v, %v", tags, err)
	}
	if _, err := s.TagMany([]string{r.ID}, nil, []int64{tags[0].ID}); err != nil {
		t.Fatal(err)
	}
	sk, _, err := s.GetFor(user.ID, r.ID)
	if err != nil || len(sk.Tags) != 1 {
		t.Fatalf("another user's tag changed: %+v, %v", sk.Tags, err)
	}
}
