package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Removing a project reet cloned can also delete the clone
// (docs/superpowers/specs/2026-11-13-github-repo-projects-design.md,
// "Removing"). The machine decides: it reports what the clone holds, and
// deletes it only when it is still what the person was shown and nothing in
// it would be lost. The controller words the machine's answer, never names a
// folder a browser gave it, and removes the project only once the machine
// has confirmed the folder is gone.

const (
	sentenceNotCloned        = "reet didn't clone that project's folder, so it won't delete it."
	sentenceCloneChanged     = "The clone changed just now, so nothing was removed. Check again."
	sentenceCloneFingerprint = "Check the clone again before deleting it."
)

// cloneShown is where the project's clone is, as the person reads it.
func cloneShown(p store.FolderProject) string { return "~/reet/repos/" + p.Origin }

// cloneStays says the clone stays, and why.
func cloneStays(machine, why string) string {
	return "The clone on " + machine + " is kept: " + why
}

func cloneCantAsk(machine string) string {
	return cloneStays(machine, machine+" can't be asked right now.")
}

func cloneNotFound(machine string, p store.FolderProject) string {
	return cloneStays(machine, "reet can't find it at "+cloneShown(p)+".")
}

func cloneUnconfirmed(machine string) string {
	return "reet couldn't confirm whether the clone on " + machine + " was removed. Check again."
}

// cloneFolder is the project's folder and origin for a machine, and whether
// they may be sent at all: the project must be one reet cloned, and its
// stored folder exactly <home>/reet/repos/<owner>/<repo> for its stored
// origin, home being any clean absolute path. The machine checks the folder
// against its real home again; nothing else about the stored folder is
// trusted, and no request names one.
func cloneFolder(p store.FolderProject) (dir, origin string, ok bool) {
	owner, repo, cut := strings.Cut(p.Origin, "/")
	if !p.Cloned || !cut || !terminalproto.ValidGitHubOwner(owner) || !terminalproto.ValidGitHubRepo(repo) {
		return "", "", false
	}
	home, found := strings.CutSuffix(p.Dir, "/reet/repos/"+p.Origin)
	if !found || !path.IsAbs(home) || path.Clean(p.Dir) != p.Dir {
		return "", "", false
	}
	return p.Dir, p.Origin, true
}

// cloneFingerprintShape reports whether s is a SHA-256 in lower-case hex,
// the only fingerprint a machine gives (terminalproto.RepoCloneState).
func cloneFingerprintShape(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := range len(s) {
		if (s[i] < '0' || s[i] > '9') && (s[i] < 'a' || s[i] > 'f') {
			return false
		}
	}
	return true
}

// terminalAccessOff reports whether the machine last said its terminal
// access is off.
func (s *server) terminalAccessOff(uid int64, agent string) bool {
	caps, err := s.st.RemoteCapabilitiesFor(uid, agent)
	return err == nil && !caps.Capabilities.TerminalEnabled
}

// cloneReasonSentence words a machine's refusal to report on, or delete, a
// clone. It covers the reasons a RepoCloneState carries.
func (s *server) cloneReasonSentence(uid int64, p store.FolderProject, machine string, reason terminalproto.RepoCloneReason) string {
	switch reason {
	case terminalproto.RepoCloneDenied:
		// The machine answers denied when its terminal access is off, when
		// the folder is not where a clone goes, and when nothing is there.
		// Only the first is something the controller also hears about.
		if s.terminalAccessOff(uid, p.Host) {
			return cloneStays(machine, "terminal access is off on "+machine+".")
		}
		return cloneNotFound(machine, p)
	case terminalproto.RepoCloneExists:
		return cloneStays(machine, "the folder there is no longer a plain clone of "+p.Origin+".")
	case terminalproto.RepoCloneNotRemovable:
		// What the counts cannot show: a repository inside the clone, files
		// git was told not to look at, a rebase or merge under way, git
		// writing. reet cannot say work is there, only that it may be.
		return cloneStays(machine, "it may hold work that isn't on GitHub.")
	}
	// busy, timed_out, failed, and anything a newer machine might add.
	return cloneCantAsk(machine)
}

// cloneStateSentence is why a clone that is not removable stays, first match.
func (s *server) cloneStateSentence(uid int64, p store.FolderProject, machine string, state terminalproto.RepoCloneState) string {
	switch {
	case state.Reason != "":
		return s.cloneReasonSentence(uid, p, machine, state.Reason)
	case !state.Marked:
		return cloneStays(machine, "reet didn't make that folder.")
	case state.Worktrees == 1:
		return cloneStays(machine, "1 session still has a worktree from it. Remove that session first.")
	case state.Worktrees > 1:
		return cloneStays(machine, fmt.Sprintf("%d sessions still have worktrees from it. Remove those sessions first.", state.Worktrees))
	}
	return cloneStays(machine, "it holds work that isn't on GitHub.")
}

// cloneReachable reports whether the project's machine can be asked about
// its clone now: connected, with terminal access on, and a reet that knows
// how. Nothing is sent to a machine that is not.
func (s *server) cloneReachable(uid int64, agent string) bool {
	if !s.terminals.Ready(uid, agent) {
		return false
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, agent)
	return err == nil && caps.Capabilities.RepoCloneRemove
}

// folderProjectCloneState answers GET /api/v2/folder-projects/{id}/clone:
// whether the clone of a project reet cloned can be deleted with it, as the
// machine sees it at this moment. A clone that stays comes with one sentence
// saying why and no fingerprint; a removable one with the fingerprint a
// delete must repeat. A project reet did not clone is 404 not_cloned.
func (s *server) folderProjectCloneState(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("part") != "clone" {
		http.NotFound(w, r)
		return
	}
	uid := s.uid(r)
	p, err := s.st.FolderProjectFor(uid, r.PathValue("id"))
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	if !p.Cloned {
		writeErrCode(w, http.StatusNotFound, "not_cloned", "", sentenceNotCloned, "")
		return
	}
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	machine := machineLabel(names, p.Host)
	kept := func(state terminalproto.RepoCloneState, sentence string) {
		writeJSON(w, http.StatusOK, map[string]any{"removable": false, "worktrees": state.Worktrees, "changed": state.Changed,
			"stashes": state.Stashes, "unpushed": state.Unpushed, "sentence": sentence})
	}
	dir, origin, ok := cloneFolder(p)
	if !ok {
		kept(terminalproto.RepoCloneState{}, cloneNotFound(machine, p))
		return
	}
	if !s.cloneReachable(uid, p.Host) {
		kept(terminalproto.RepoCloneState{}, cloneCantAsk(machine))
		return
	}
	state, err := s.terminals.CloneStatus(r.Context(), uid, p.Host, terminalproto.RepoCloneStatus{Dir: dir, Origin: origin})
	if err != nil {
		kept(terminalproto.RepoCloneState{}, cloneCantAsk(machine))
		return
	}
	if !state.Removable() {
		kept(state, s.cloneStateSentence(uid, p, machine, state))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removable": true, "worktrees": 0, "changed": 0, "stashes": 0, "unpushed": 0,
		"fingerprint": state.Fingerprint, "sentence": ""})
}

// removeProjectClone asks the project's machine to delete the clone the
// person was shown as fingerprint, and returns the folder the machine says
// it deleted. Every other outcome is a refusal and the project must stay:
// 422 not_cloned or invalid_fingerprint before anything is sent, 409
// clone_changed when the clone is no longer what was shown, 409 clone_kept
// with why, and 409 clone_unconfirmed when the answer was lost or the
// machine may have begun deleting, so neither outcome can be claimed.
//
// The request does not end with the browser's: once the machine has been
// asked, its answer decides whether the project goes, whoever is listening.
func (s *server) removeProjectClone(uid int64, p store.FolderProject, machine, fingerprint string) (string, *projectRefusal) {
	if !p.Cloned {
		return "", &projectRefusal{http.StatusUnprocessableEntity, "not_cloned", "", sentenceNotCloned, ""}
	}
	if !cloneFingerprintShape(fingerprint) {
		return "", &projectRefusal{http.StatusUnprocessableEntity, "invalid_fingerprint", "fingerprint", sentenceCloneFingerprint, ""}
	}
	kept := func(sentence string) (string, *projectRefusal) {
		return "", &projectRefusal{http.StatusConflict, "clone_kept", "", sentence, ""}
	}
	dir, origin, ok := cloneFolder(p)
	if !ok {
		return kept(cloneNotFound(machine, p))
	}
	if !s.cloneReachable(uid, p.Host) {
		return kept(cloneCantAsk(machine))
	}
	ctx, cancel := context.WithTimeout(s.baseContext(), terminalbroker.RepoCloneRemoveCallTimeout)
	defer cancel()
	removed, err := s.terminals.RemoveClone(ctx, uid, p.Host, terminalproto.RepoCloneRemove{Dir: dir, Origin: origin, Fingerprint: fingerprint})
	unconfirmed := &projectRefusal{http.StatusConflict, "clone_unconfirmed", "", cloneUnconfirmed(machine), ""}
	switch {
	case errors.Is(err, terminalbroker.ErrRepoCloneRemoveUnsupported):
		// Nothing was sent.
		return kept(cloneCantAsk(machine))
	case err != nil:
		// No answer, or one reet could not read: the machine may have
		// deleted the folder.
		return "", unconfirmed
	}
	switch removed.Reason {
	case "":
		return removed.Dir, nil
	case terminalproto.RepoCloneChanged, terminalproto.RepoCloneNotRemovable:
		// It was removable when shown, so not removable now is a change too.
		return "", &projectRefusal{http.StatusConflict, "clone_changed", "", sentenceCloneChanged, ""}
	case terminalproto.RepoCloneFailed, terminalproto.RepoCloneTimedOut:
		// A delete that failed may have removed part of the folder, and one
		// that outlasted its answer goes on.
		return "", unconfirmed
	}
	return kept(s.cloneReasonSentence(uid, p, machine, removed.Reason))
}
