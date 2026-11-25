package store

import (
	"strings"
	"testing"
	"time"
)

// The Skills page checks whether a published skill's public page still
// follows it, for the skills of the tab it shows and no others: the check
// reads and hashes the whole head revision, and the page is asked for every
// few seconds.
func TestSkillsPageChecksPublicPagesOfItsOwnTabOnly(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	shown := savedWrittenSkill(t, s, uid, "shown-one", true)
	parked := savedWrittenSkill(t, s, uid, "parked-one", true)
	savedWrittenSkill(t, s, uid, "fine-one", true)
	if err := s.SetSkillStatusFor(uid, []string{parked.id}, SkillArchived, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Two skills whose head the public page cannot serve, one on each tab.
	if _, err := s.db.Exec(`UPDATE library_items SET revision_id='' WHERE user_id=? AND id IN (?,?)`, uid, shown.id, parked.id); err != nil {
		t.Fatal(err)
	}
	counter := countStatements(t, s)
	const check = "SELECT name,bundle_id,revision_id FROM library_items" // the first read of a skill's head, once per skill checked

	for _, tc := range []struct {
		tab, withProblem string
		wantNames        []string
	}{
		{SkillActive, "shown-one", []string{"fine-one", "shown-one"}},
		{SkillArchived, "parked-one", []string{"parked-one"}},
	} {
		counter.reset()
		page, err := s.SkillsPageFor(uid, tc.tab, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, row := range page.Skills {
			names = append(names, row.Name)
			if got := strings.HasPrefix(row.PublicProblem, "The public page still shows the version before"); got != (row.Name == tc.withProblem) || (!got && row.PublicProblem != "") {
				t.Errorf("%s tab: %s says %q", tc.tab, row.Name, row.PublicProblem)
			}
		}
		if strings.Join(names, ",") != strings.Join(tc.wantNames, ",") {
			t.Errorf("%s tab shows %v, want %v", tc.tab, names, tc.wantNames)
		}
		if got := counter.count(check); got != len(tc.wantNames) {
			t.Errorf("%s tab checked %d public pages, want %d (its own)", tc.tab, got, len(tc.wantNames))
		}
	}
}
