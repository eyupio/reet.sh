package api

import (
	"context"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// A repository added as a folder says which GitHub repository it pushes to,
// as a project added From GitHub does: its machine reports the folder's
// origin (the status view of Inspect, which names only a github.com origin).
// The controller asks for each repository project added by path that was
// never asked or was last asked over a day ago, a few at a time, from the
// same worker as the temporary projects, and at once (nudged) when one is
// added or moved. A machine that cannot be asked is asked again later.

var (
	// folderRemoteEvery is how often a project's origin is asked again.
	folderRemoteEvery = 24 * time.Hour
	// folderRemoteBatch bounds one pass.
	folderRemoteBatch = 20
	// folderRemoteNudge wakes the worker for a project just added or moved.
	folderRemoteNudge = make(chan struct{}, 1)
)

func (s *server) nudgeFolderRemotes() {
	select {
	case folderRemoteNudge <- struct{}{}:
	default:
	}
}

// learnFolderRemotes asks about each repository project that is due.
func (s *server) learnFolderRemotes(now time.Time) {
	if s.st.Fenced() {
		return
	}
	due, err := s.st.FolderProjectsToAskRemote(now.Add(-folderRemoteEvery).UnixMilli(), folderRemoteBatch)
	if err != nil {
		return
	}
	for _, item := range due {
		select {
		case <-s.stop:
			return
		default:
		}
		s.learnFolderRemote(item.UserID, item.Project, now)
	}
}

// learnFolderRemote asks the project's machine for its folder's origin and
// records the answer. Nothing is recorded when the machine cannot be asked
// or does not answer, so it is asked again on a later pass.
func (s *server) learnFolderRemote(uid int64, p store.FolderProject, now time.Time) {
	if !s.terminals.Ready(uid, p.Host) {
		return
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, p.Host)
	if err != nil || !caps.Capabilities.Inspect {
		return
	}
	ctx, cancel := context.WithTimeout(s.baseContext(), 20*time.Second)
	defer cancel()
	result, err := s.terminals.InspectFolder(ctx, uid, p.Host, p.Dir)
	if err != nil {
		return
	}
	switch result.Reason {
	case "", "not_repo":
		// A folder that is no longer a repository has no origin either.
	default:
		// busy, denied, timed out, git failed: ask again later.
		return
	}
	_ = s.st.SetFolderProjectRemoteFor(uid, p.ID, p.Dir, result.Remote, now.UnixMilli())
}
