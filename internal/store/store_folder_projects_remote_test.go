package store

import "testing"

// The GitHub repository a folder added by path pushes to is the controller's
// note: it moves no version, is written only while the project is still in
// the folder that was asked about, and is forgotten when the project moves.
func TestFolderProjectRemoteIsRecordedAndForgottenOnAMove(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := mustFP(t, s, uid, FolderProject{Host: "laptop", Dir: "/home/a/code/site", IsRepo: true})
	if err := s.SetFolderProjectRemoteFor(uid, p.ID, p.Host, p.Dir, "acme/site", 1234); err != nil {
		t.Fatal(err)
	}
	got, _ := s.FolderProjectFor(uid, p.ID)
	if got.Remote != "acme/site" || got.RemoteCheckedAt != 1234 || got.Version != p.Version {
		t.Fatalf("recorded %+v", got)
	}
	// An answer about a folder the project has left is not written.
	if err := s.SetFolderProjectRemoteFor(uid, p.ID, p.Host, "/home/a/elsewhere", "other/thing", 5678); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.FolderProjectFor(uid, p.ID); again.Remote != "acme/site" {
		t.Fatalf("written for another folder: %+v", again)
	}
	// Nor is one from a machine the project has left, though the folder is named the same.
	if err := s.SetFolderProjectRemoteFor(uid, p.ID, "desk", p.Dir, "other/thing", 5678); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.FolderProjectFor(uid, p.ID); again.Remote != "acme/site" {
		t.Fatalf("written for another machine: %+v", again)
	}
	if err := s.SetFolderProjectRemoteFor(uid, p.ID, p.Host, p.Dir, "not a repo", 1); err == nil {
		t.Fatal("an invalid remote was accepted")
	}
	// A rename keeps it; a move forgets it.
	edit := got
	edit.Name = "Site"
	renamed, err := s.UpdateFolderProjectFor(uid, edit, got.Version)
	if err != nil || renamed.Remote != "acme/site" {
		t.Fatalf("renamed: %v %+v", err, renamed)
	}
	moved := renamed
	moved.Dir = "/home/a/code/site2"
	after, err := s.UpdateFolderProjectFor(uid, moved, renamed.Version)
	if err != nil || after.Remote != "" || after.RemoteCheckedAt != 0 {
		t.Fatalf("moved: %v %+v", err, after)
	}
}

// Only repository projects added by path are asked, never-asked and longest
// ago first.
func TestFolderProjectsToAskRemote(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "a"), projectUser(t, s, "b")
	repo := mustFP(t, s, a, FolderProject{Host: "laptop", Dir: "/home/a/code/site", IsRepo: true})
	mustFP(t, s, a, FolderProject{Host: "laptop", Dir: "/home/a/notes"})
	mustFP(t, s, a, FolderProject{Host: "laptop", Dir: "/home/a/reet/repos/acme/docs", IsRepo: true, Origin: "acme/docs", Cloned: true})
	other := mustFP(t, s, b, FolderProject{Host: "desk", Dir: "/home/b/code/app", IsRepo: true})
	if err := s.SetFolderProjectRemoteFor(b, other.ID, other.Host, other.Dir, "", 500); err != nil {
		t.Fatal(err)
	}
	list, err := s.FolderProjectsToAskRemote(1000, 1000, 10)
	if err != nil || len(list) != 2 || list[0].Project.ID != repo.ID || list[0].UserID != a || list[1].Project.ID != other.ID || list[1].UserID != b {
		t.Fatalf("%v %+v", err, list)
	}
	if list, _ := s.FolderProjectsToAskRemote(400, 1000, 10); len(list) != 1 || list[0].Project.ID != repo.ID {
		t.Fatalf("asked again too soon: %+v", list)
	}
	if list, _ := s.FolderProjectsToAskRemote(1000, 1000, 1); len(list) != 1 {
		t.Fatalf("limit: %+v", list)
	}
}

// A project whose machine could not be asked goes to the back once tried, so
// projects that cannot be asked never hold the whole batch; it is tried again
// once the retry time has passed. A move asks the new folder at once.
func TestFolderRemoteTriesDoNotStarveTheBatch(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	off := mustFP(t, s, uid, FolderProject{Host: "offline", Dir: "/home/a/code/old", IsRepo: true})
	on := mustFP(t, s, uid, FolderProject{Host: "laptop", Dir: "/home/a/code/new", IsRepo: true})
	// These projects can be created in one millisecond; the random ID is then the tie-breaker.
	// Give the fixture an explicit chronological order rather than depending on wall-clock speed.
	if _, err := s.db.Exec(`UPDATE folder_projects SET created_at=CASE WHEN id=? THEN 100 ELSE 200 END WHERE id IN (?,?)`, off.ID, off.ID, on.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.FolderProjectsToAskRemote(1000, 1000, 1); len(list) != 1 || list[0].Project.ID != off.ID {
		t.Fatalf("first: %+v", list)
	}
	if err := s.NoteFolderRemoteTried(uid, off.ID, 900); err != nil {
		t.Fatal(err)
	}
	// Tried at 900: not again before the retry time, and behind the untried one after it.
	if list, _ := s.FolderProjectsToAskRemote(1000, 800, 1); len(list) != 1 || list[0].Project.ID != on.ID {
		t.Fatalf("within the retry time: %+v", list)
	}
	if list, _ := s.FolderProjectsToAskRemote(1000, 1000, 2); len(list) != 2 || list[0].Project.ID != on.ID || list[1].Project.ID != off.ID {
		t.Fatalf("after the retry time: %+v", list)
	}
	got, _ := s.FolderProjectFor(uid, off.ID)
	moved := got
	moved.Host = "laptop"
	if _, err := s.UpdateFolderProjectFor(uid, moved, got.Version); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.FolderProjectsToAskRemote(1000, 800, 2); len(list) != 2 {
		t.Fatalf("a moved project waits for its old try: %+v", list)
	}
}
