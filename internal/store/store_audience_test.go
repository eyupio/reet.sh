package store

import (
	"errors"
	"testing"
)

func TestAudienceAccessMembershipExclusionsAndPersonal(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	owner, _ := s.DefaultOwner()
	creator, _ := s.CreateUser("creator@example.com", "Creator", "member", "hash")
	alice, _ := s.CreateUser("alice@example.com", "Alice", "member", "hash")
	bob, _ := s.CreateUser("bob@example.com", "Bob", "member", "hash")
	carol, _ := s.CreateUser("carol@example.com", "Carol", "member", "hash")
	viewer, _ := s.CreateUser("viewer@example.com", "Read only", "viewer", "hash")
	group, err := s.CreateGroup(creator.ID, "member", "Engineering", "team", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []User{alice, bob, viewer} {
		if err = s.SetGroupMember(creator.ID, "member", group.ID, u.Email, "member", false); err != nil {
			t.Fatal(err)
		}
	}
	audience := Audience{GroupID: group.ID, GroupRole: "editor", People: []AudiencePerson{{carol.Email, "viewer"}}, Excluded: []string{bob.Email}}
	list, err := s.SaveAudienceList(creator.ID, "member", 0, "Team toolkit", "", "team", audience)
	if err != nil {
		t.Fatal(err)
	}
	check := func(userID int64, role string, read, edit bool) {
		t.Helper()
		a, e := s.ListAccessFor(userID, role, list.ID)
		if read {
			if e != nil || a.CanEdit != edit {
				t.Fatalf("user %d access=%+v err=%v", userID, a, e)
			}
		} else if !errors.Is(e, ErrNotFound) {
			t.Fatalf("user %d unexpectedly reads: %+v %v", userID, a, e)
		}
	}
	check(alice.ID, "member", true, true)
	check(bob.ID, "member", false, false)
	check(carol.ID, "member", true, false)
	check(viewer.ID, "viewer", true, false)
	check(owner, "owner", true, true)
	if a, e := s.ListAccessFor(alice.ID, "member", list.ID); e != nil || a.List.MemberCount != 4 {
		t.Fatalf("excluded recipient counted %+v %v", a, e)
	}
	if err = s.SetGroupMember(creator.ID, "member", group.ID, alice.Email, "member", true); err != nil {
		t.Fatal(err)
	}
	check(alice.ID, "member", false, false)
	if err = s.SetGroupMember(creator.ID, "member", group.ID, alice.Email, "member", false); err != nil {
		t.Fatal(err)
	}
	check(alice.ID, "member", true, true)
	// Exclusions also override a direct editor invitation.
	if err = s.AddListMember(list.ID, bob.ID, "editor"); err != nil {
		t.Fatal(err)
	}
	check(bob.ID, "member", false, false)
	lists, e := s.ListListsFor(bob.ID, "member")
	if e != nil || len(lists) != 0 {
		t.Fatal("excluded list discoverable")
	}
	r := mustRef(t, "a/b/alpha")
	s.UpsertFor(bob.ID, r, Patch{})
	s.AddListSkill(list.ID, r.ID, "team note", creator.ID)
	skill, ok, e := s.GetFor(bob.ID, r.ID)
	if e != nil || !ok || len(skill.Lists) != 0 {
		t.Fatalf("private list metadata leaked %+v %v", skill, e)
	}
	// Personal is a distinct audience; latent invitations cannot grant access.
	if _, err = s.SaveAudienceList(creator.ID, "member", list.ID, list.Name, "", "personal", Audience{}); err != nil {
		t.Fatal(err)
	}
	check(carol.ID, "member", false, false)
	check(alice.ID, "member", false, false)
	if err = s.AddListMember(list.ID, alice.ID, "viewer"); err == nil {
		t.Fatal("personal list got collaborators")
	}
}
func TestPublicCommunityIsExplicitAndRedacted(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	owner, _ := s.DefaultOwner()
	r := mustRef(t, "o/r/public-skill")
	secret := "PERSONAL SECRET"
	s.UpsertFor(owner, r, Patch{Notes: &secret})
	private, _ := s.CreateList(owner, "PRIVATE SECRET", "hidden", "private")
	legacy, _ := s.CreateList(owner, "LEGACY SECRET", "hidden", "members")
	link, _ := s.CreateList(owner, "UNLISTED SECRET", "hidden", "link")
	for _, l := range []SharedList{private, legacy, link} {
		s.AddListSkill(l.ID, r.ID, "PRIVATE LIST NOTE", owner)
	}
	lists, skills, err := s.Community()
	if err != nil || len(lists) != 0 || len(skills) != 0 {
		t.Fatal("unpublished content discoverable")
	}
	pub, err := s.SaveAudienceList(owner, "owner", 0, "Public toolkit", "Published deliberately", "public", Audience{})
	if err != nil {
		t.Fatal(err)
	}
	s.AddListSkill(pub.ID, r.ID, "Public list note", owner)
	lists, skills, err = s.Community()
	if err != nil || len(lists) != 1 || len(skills) != 1 {
		t.Fatalf("community %v %v %v", lists, skills, err)
	}
	if lists[0].OwnerID != 0 || lists[0].OwnerName != "" || lists[0].MemberCount != 0 || skills[0].Notes != "" || skills[0].Note != "" || skills[0].AddedBy != 0 || skills[0].AddedByName != "" {
		t.Fatal("community leaked private metadata")
	}
	d, err := s.PublicListFor(pub.Slug)
	if err != nil || d.Skills[0].Note != "Public list note" || len(d.Members) != 0 || d.Audience != nil || d.Skills[0].Notes != "" {
		t.Fatalf("public list projection %+v %v", d, err)
	}
	if _, err = s.PublicListFor(private.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatal("private slug is public")
	}
	if _, err = s.PublicListFor(legacy.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatal("legacy audience was made anonymous")
	}
	if _, err = s.SaveAudienceList(owner, "owner", pub.ID, pub.Name, "", "personal", Audience{}); err != nil {
		t.Fatal(err)
	}
	lists, _, _ = s.Community()
	if len(lists) != 0 {
		t.Fatal("unpublishing did not remove discovery")
	}
	if _, err = s.PublicListFor(pub.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatal("old published link still public")
	}
}
func TestAudienceValidationAtomicityAndGroupBoundaries(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	owner, _ := s.DefaultOwner()
	a, _ := s.CreateUser("a@example.com", "A", "member", "hash")
	b, _ := s.CreateUser("b@example.com", "B", "member", "hash")
	org, err := s.CreateGroup(a.ID, "member", "Acme", "organisation", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateGroup(b.ID, "member", "Guessing team", "team", org.ID); err == nil {
		t.Fatal("outsider created an org team")
	}
	team, err := s.CreateGroup(a.ID, "member", "Tools", "team", org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetGroupMember(b.ID, "member", team.ID, b.Email, "manager", false); err == nil {
		t.Fatal("outsider changed group membership")
	}
	list, err := s.SaveAudienceList(a.ID, "member", 0, "Personal", "", "personal", Audience{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveAudienceList(b.ID, "member", list.ID, "Changed", "", "public", Audience{}); err == nil {
		t.Fatal("outsider published private list")
	}
	if _, err = s.SaveAudienceList(a.ID, "member", list.ID, "Changed", "", "team", Audience{}); err == nil {
		t.Fatal("group required")
	}
	got, _ := s.SharedListByID(list.ID)
	if got.Name != "Personal" || got.Visibility != "personal" {
		t.Fatal("failed audience update partially applied")
	}
	if _, err = s.SaveAudienceList(a.ID, "member", 0, "New invalid list", "", "public", Audience{Excluded: []string{b.Email}}); err == nil {
		t.Fatal("public exclusions accepted")
	}
	if _, err = s.SaveAudienceList(b.ID, "member", 0, "Outsider list", "", "team", Audience{GroupID: team.ID}); err == nil {
		t.Fatal("outsider shared with guessed group")
	}
	if _, err = s.SaveAudienceList(owner, "owner", 0, "Owner excluded", "", "private", Audience{Excluded: []string{"missing@example.com"}}); err == nil {
		t.Fatal("unknown account exclusion accepted")
	}
}

func TestPublicListsRequireLinkedSourcesAndKeepAudienceOnFailure(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	owner, _ := s.DefaultOwner()
	local, err := s.UpsertLocalFor(owner, "my-local-skill", Patch{})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := s.CreateList(owner, "Local draft", "", "private")
	if err = s.AddListSkill(list.ID, local.ID, "sensitive local draft", owner); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveAudienceList(owner, "owner", list.ID, list.Name, "", "public", Audience{}); err == nil {
		t.Fatal("published local-only skill")
	}
	got, _ := s.SharedListByID(list.ID)
	if got.Visibility != "private" {
		t.Fatal("failed publish changed visibility")
	}
	pub, _ := s.SaveAudienceList(owner, "owner", 0, "Public", "", "public", Audience{})
	if err = s.AddListSkill(pub.ID, local.ID, "", owner); err == nil {
		t.Fatal("added local-only skill to public list")
	}
}
func TestDeletingGroupRevokesInheritedAccessButKeepsLists(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	creator, _ := s.CreateUser("group-owner@example.com", "Owner", "member", "hash")
	person, _ := s.CreateUser("group-member@example.com", "Member", "member", "hash")
	org, _ := s.CreateGroup(creator.ID, "member", "Org", "organisation", 0)
	team, _ := s.CreateGroup(creator.ID, "member", "Team", "team", org.ID)
	s.SetGroupMember(creator.ID, "member", team.ID, person.Email, "member", false)
	list, err := s.SaveAudienceList(creator.ID, "member", 0, "Team list", "", "team", Audience{GroupID: team.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateGroup(creator.ID, "member", org.ID, "", true); err == nil {
		t.Fatal("deleted org containing teams")
	}
	if err = s.UpdateGroup(person.ID, "member", team.ID, "", true); err == nil {
		t.Fatal("member deleted team")
	}
	if err = s.UpdateGroup(creator.ID, "member", team.ID, "Renamed", false); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateGroup(creator.ID, "member", team.ID, "", true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ListAccessFor(person.ID, "member", list.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted group still grants access")
	}
	if _, err = s.ListAccessFor(creator.ID, "member", list.ID); err != nil {
		t.Fatal("deleting group deleted list")
	}
	if err = s.UpdateGroup(creator.ID, "member", org.ID, "", true); err != nil {
		t.Fatal(err)
	}
}
