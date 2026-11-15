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
	// folderRemoteRetry is how soon a project whose machine could not be
	// asked is tried again.
	folderRemoteRetry = 10 * time.Minute
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
	due, err := s.st.FolderProjectsToAskRemote(now.Add(-folderRemoteEvery).UnixMilli(), now.Add(-folderRemoteRetry).UnixMilli(), folderRemoteBatch)
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
// records the answer. When the machine cannot be asked or does not answer,
// only the try is noted, so it is asked again on a later pass, behind the
// projects not tried since.
func (s *server) learnFolderRemote(uid int64, p store.FolderProject, now time.Time) {
	if remote, ok := s.askFolderRemote(uid, p); ok {
		_ = s.st.SetFolderProjectRemoteFor(uid, p.ID, p.Host, p.Dir, remote, now.UnixMilli())
		return
	}
	_ = s.st.NoteFolderRemoteTried(uid, p.ID, now.UnixMilli())
}

// askFolderRemote is the folder's GitHub origin as its machine reports it
// ("" for none), and whether the machine answered.
func (s *server) askFolderRemote(uid int64, p store.FolderProject) (string, bool) {
	if !s.terminals.Ready(uid, p.Host) {
		return "", false
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, p.Host)
	if err != nil || !caps.Capabilities.Inspect {
		return "", false
	}
	ctx, cancel := context.WithTimeout(s.baseContext(), 20*time.Second)
	defer cancel()
	result, err := s.terminals.InspectFolder(ctx, uid, p.Host, p.Dir)
	if err != nil {
		return "", false
	}
	switch result.Reason {
	case "", "not_repo":
		// A folder that is no longer a repository has no origin either.
		return result.Remote, true
	default:
		// busy, denied, timed out, git failed: ask again later.
		return "", false
	}
}
