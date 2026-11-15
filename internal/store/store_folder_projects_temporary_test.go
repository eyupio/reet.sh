package store

import "testing"

// A temporary project is a clone reet made for one start. Only a cloned
// project can be one; an edit keeps it; moving it clears it, as it clears
// where it came from.
func TestFolderProjectTemporaryIsSetOnCreateAndKeptByAnEdit(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	made := mustFP(t, s, uid, FolderProject{Host: "laptop", Dir: "/home/a/reet/repos/acme/site", IsRepo: true, Origin: "acme/site", Cloned: true, Temporary: true})
	if !made.Temporary || made.KeptBecause != "" {
		t.Fatalf("create: %+v", made)
	}
	byPath := mustFP(t, s, uid, FolderProject{Host: "laptop", Dir: "/home/a/notes", Temporary: true})
	if byPath.Temporary {
		t.Fatal("a folder reet did not clone was made temporary")
	}
	edit := made
	edit.Name, edit.Temporary, edit.KeptBecause = "Site", false, "typed"
	updated, err := s.UpdateFolderProjectFor(uid, edit, made.Version)
	if err != nil || !updated.Temporary || updated.KeptBecause != "" || updated.Name != "Site" {
		t.Fatalf("an edit changed what only reet sets: %v %+v", err, updated)
	}
	moved := updated
	moved.Dir = "/home/a/code/site"
	if after, err := s.UpdateFolderProjectFor(uid, moved, updated.Version); err != nil || after.Temporary {
		t.Fatalf("moved: %v %+v", err, after)
	}
}

func TestTemporaryFolderProjectsListEveryAccountsOldestFirst(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "a"), projectUser(t, s, "b")
	first := mustFP(t, s, a, FolderProject{Host: "laptop", Dir: "/home/a/reet/repos/acme/site", Origin: "acme/site", Cloned: true, Temporary: true})
	mustFP(t, s, a, FolderProject{Host: "laptop", Dir: "/home/a/reet/repos/acme/kept", Origin: "acme/kept", Cloned: true})
	second := mustFP(t, s, b, FolderProject{Host: "desk", Dir: "/home/b/reet/repos/acme/docs", Origin: "acme/docs", Cloned: true, Temporary: true})
	list, err := s.TemporaryFolderProjects()
	if err != nil || len(list) != 2 {
		t.Fatalf("%v %+v", err, list)
	}
	if list[0].UserID != a || list[0].Project != first || list[1].UserID != b || list[1].Project != second {
		t.Fatalf("listed %+v", list)
	}
}

// Why a temporary project was kept is the controller's note: it moves no
// version, so an edit the person has open still saves. Keeping the project
// makes it ordinary for good, and is an edit.
func TestFolderProjectKeptBecauseAndKeep(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	made := mustFP(t, s, uid, FolderProject{Host: "laptop", Dir: "/home/a/reet/repos/acme/site", Origin: "acme/site", Cloned: true, Temporary: true})
	if err := s.SetFolderProjectKeptFor(uid, made.ID, "it holds work that isn't on GitHub."); err != nil {
		t.Fatal(err)
	}
	read, _ := s.FolderProjectFor(uid, made.ID)
	if read.KeptBecause != "it holds work that isn't on GitHub." || read.Version != made.Version {
		t.Fatalf("kept note: %+v", read)
	}
	kept, err := s.KeepFolderProjectFor(uid, made.ID)
	if err != nil || kept.Temporary || kept.KeptBecause != "" || kept.Version != made.Version+1 {
		t.Fatalf("keep: %v %+v", err, kept)
	}
	// A note for a project that is no longer temporary is not written.
	if err := s.SetFolderProjectKeptFor(uid, made.ID, "late"); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.FolderProjectFor(uid, made.ID); again.KeptBecause != "" {
		t.Fatalf("note written on an ordinary project: %+v", again)
	}
	if list, _ := s.TemporaryFolderProjects(); len(list) != 0 {
		t.Fatalf("still listed: %+v", list)
	}
	if _, err := s.KeepFolderProjectFor(uid, "missing"); err != ErrFolderProjectNotFound {
		t.Fatal(err)
	}
	other := projectUser(t, s, "b")
	if _, err := s.KeepFolderProjectFor(other, made.ID); err != ErrFolderProjectNotFound {
		t.Fatal("another account kept it", err)
	}
}
