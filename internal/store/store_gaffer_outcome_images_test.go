package store

import (
	"strings"
	"testing"
)

// An outcome saved before images were stored is still read: the field is
// absent from its JSON, so it comes back empty rather than failing the read.
func TestGafferOutcomeWithoutStoredImagesStillReads(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u, err := s.CreateUser("old-outcome@example.test", "Owner", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	old := `{"id":"` + id + `","revision":1,"objective":"Old outcome","state":"settled","sessions":[],"createdAt":1,"updatedAt":1}`
	if _, err = s.db.Exec(`INSERT INTO gaffer_outcomes(id,user_id,created_at,data) VALUES(?,?,?,?)`, id, u.ID, 1, old); err != nil {
		t.Fatal(err)
	}
	o, err := s.GafferOutcomeFor(u.ID, id)
	if err != nil || o.Objective != "Old outcome" || len(o.Images) != 0 {
		t.Fatalf("old outcome: %+v %v", o, err)
	}
}
