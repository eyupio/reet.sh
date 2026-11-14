package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Projects from GitHub (docs/superpowers/specs/2026-11-13-github-repo-projects-design.md):
// the person names a repository and a machine; the machine clones it into
// ~/reet/repos/<owner>/<repo> with its own GitHub sign-in, and the folder is
// saved as an ordinary repository project that records where it came from.
// The controller never sees a GitHub credential. A clone can take minutes,
// so it runs as a job that outlives the request and the browser polls it.

const (
	// maxClonesRunning is how many clones one account may have running.
	maxClonesRunning = 5
	// cloneKept is how long a finished job's outcome can still be read.
	cloneKept = 10 * time.Minute
)

const (
	sentenceInvalidRepo = "Write the repository as owner/name, or paste its github.com link."
	sentenceCloneLimit  = "Five repositories are already being cloned. Wait for one to finish."
	sentenceCloneGone   = "That clone isn't on this account."
)

type cloneState string

const (
	cloneRunning cloneState = "cloning"
	cloneDone    cloneState = "done"
	cloneFailed  cloneState = "failed"
)

// cloneJob is one account's clone of one repository on one machine.
type cloneJob struct {
	uid         int64
	agent       string
	owner, repo string // as the person wrote them, after parsing
	name        string
	state       cloneState
	project     store.FolderProject // when done
	code        string              // when failed
	sentence    string              // when failed
	ended       time.Time
}

// repoClones holds the clone jobs of every account in memory. Each job runs
// in its own goroutine under the server's lifetime context, which Close
// cancels before close waits for the jobs.
type repoClones struct {
	mu     sync.Mutex
	jobs   map[string]*cloneJob
	closed bool
	now    func() time.Time
	active sync.WaitGroup
}

var (
	errCloneLimit   = errors.New("too many clones running")
	errClonesClosed = errors.New("the server is closing")
)

func (c *repoClones) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// setClock replaces the clock the jobs are timed by (tests).
func (c *repoClones) setClock(now func() time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

// forgetEnded drops jobs that ended more than cloneKept ago. Callers hold mu.
func (c *repoClones) forgetEnded() {
	now := c.clock()
	for id, j := range c.jobs {
		if j.state != cloneRunning && now.Sub(j.ended) > cloneKept {
			delete(c.jobs, id)
		}
	}
}

// runningFor counts uid's clones that have not ended. Callers hold mu.
func (c *repoClones) runningFor(uid int64) int {
	n := 0
	for _, j := range c.jobs {
		if j.uid == uid && j.state == cloneRunning {
			n++
		}
	}
	return n
}

// start registers a copy of j and reports its id and folder. A running job
// for the same account, machine and repository (owner and repo compared
// without regard to case) is returned instead, with fresh false. The error is
// errCloneLimit when the account already runs maxClonesRunning clones and
// errClonesClosed once the server is closing. A fresh job must be run, and
// its run must call active.Done.
func (c *repoClones) start(j cloneJob) (id, dir string, fresh bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", "", false, errClonesClosed
	}
	c.forgetEnded()
	for id, other := range c.jobs {
		same := other.uid == j.uid && other.agent == j.agent && strings.EqualFold(other.owner, j.owner) && strings.EqualFold(other.repo, j.repo)
		if same && other.state == cloneRunning {
			return id, cloneDir(other.owner, other.repo), false, nil
		}
	}
	if c.runningFor(j.uid) >= maxClonesRunning {
		return "", "", false, errCloneLimit
	}
	if c.jobs == nil {
		c.jobs = map[string]*cloneJob{}
	}
	id = cloneID()
	j.state = cloneRunning
	c.jobs[id] = &j
	c.active.Add(1)
	return id, cloneDir(j.owner, j.repo), true, nil
}

// finish records how job id ended.
func (c *repoClones) finish(id string, project store.FolderProject, code, sentence string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	j := c.jobs[id]
	if code == "" {
		j.state, j.project = cloneDone, project
	} else {
		j.state, j.code, j.sentence = cloneFailed, code, sentence
	}
	j.ended = c.clock()
}

// lookup returns a copy of uid's job id; another account's is not found.
func (c *repoClones) lookup(uid int64, id string) (cloneJob, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forgetEnded()
	j := c.jobs[id]
	if j == nil || j.uid != uid {
		return cloneJob{}, false
	}
	return *j, true
}

// running reports how many of uid's clones are running (tests).
func (c *repoClones) running(uid int64) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runningFor(uid)
}

// close refuses new jobs and waits for the running ones, whose context the
// server has already cancelled.
func (c *repoClones) close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.active.Wait()
}

func cloneID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}

// cloneDir is where the clone lands, as the person reads it.
func cloneDir(owner, repo string) string { return "~/reet/repos/" + owner + "/" + repo }

// folderProjectFromGitHub starts a clone of a GitHub repository on one of the
// account's machines, to be saved as a project when it is done. Everything a
// create checks without the folder is checked first and refused at once; the
// clone then runs as a job (202 with its id), read with folderProjectClone.
func (s *server) folderProjectFromGitHub(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Host string `json:"host"`
		Repo string `json:"repo"`
		Name string `json:"name"`
	}
	if !folderProjectBody(w, r, &in) {
		return
	}
	owner, repo, ok := terminalproto.ParseGitHubRepo(in.Repo)
	if !ok {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_repo", "repo", sentenceInvalidRepo, "")
		return
	}
	uid := s.uid(r)
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	if _, enrolled := names[in.Host]; !enrolled {
		writeErrCode(w, http.StatusUnprocessableEntity, "unknown_machine", "host", sentenceFolderMachine, "")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name != "" && !folderNameShape(name) {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_name", "name", sentenceFolderName, "")
		return
	}
	machine := machineLabel(names, in.Host)
	if err := s.st.FolderProjectRoomFor(uid, name); err != nil {
		folderRefusal(err, machine).write(w)
		return
	}
	if !s.terminals.Ready(uid, in.Host) {
		writeErrCode(w, http.StatusConflict, "machine_offline", "", machine+" is offline. Try again when it's back.", "")
		return
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, in.Host)
	if err != nil || !caps.Capabilities.RepoClone {
		writeErrCode(w, http.StatusConflict, "repo_clone_unsupported", "", sentenceRepoCloneOld(machine), "")
		return
	}
	job := cloneJob{uid: uid, agent: in.Host, owner: owner, repo: repo, name: name}
	id, dir, fresh, err := s.clones.start(job)
	if errors.Is(err, errCloneLimit) {
		writeErrCode(w, http.StatusTooManyRequests, "clone_limit", "", sentenceCloneLimit, "")
		return
	}
	if err != nil {
		writeErrCode(w, http.StatusServiceUnavailable, "server_closing", "", "reet couldn't do that just now. Try again.", "")
		return
	}
	if fresh {
		// The job holds no part of the request: it reads only its own copy of
		// what was asked, and its outcome is read with folderProjectClone.
		go s.runClone(id, job)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"cloneId": id, "dir": dir})
}

func sentenceRepoCloneOld(machine string) string {
	return "Update reet on " + machine + " to add projects from GitHub. It also needs git and the GitHub CLI."
}

// runClone asks the machine for the clone and saves the project through the
// same checks and store path as a create. It runs under the server's
// lifetime, bounded by the clone's own deadline.
func (s *server) runClone(id string, job cloneJob) {
	defer s.clones.active.Done()
	ctx, cancel := context.WithTimeout(s.baseContext(), terminalbroker.RepoCloneCallTimeout)
	defer cancel()
	result, err := s.terminals.CloneRepo(ctx, job.uid, job.agent, terminalproto.RepoClone{Owner: job.owner, Repo: job.repo})
	machine := s.machineHost(job.uid, job.agent)
	if machine == "" {
		machine = "that machine"
	}
	if err != nil || result.Reason != "" {
		code, sentence := cloneRefusal(err, result.Reason, machine, job.owner+"/"+job.repo)
		s.clones.finish(id, store.FolderProject{}, code, sentence)
		return
	}
	saved, refusal := s.saveCloned(job, result)
	if refusal != nil {
		s.clones.finish(id, store.FolderProject{}, refusal.code, refusal.sentence)
		return
	}
	s.clones.finish(id, saved, "", "")
}

// saveCloned saves the clone as a repository project, as a create would, and
// audits it as one with where it came from.
func (s *server) saveCloned(job cloneJob, result terminalproto.RepoCloneResult) (store.FolderProject, *projectRefusal) {
	names, err := s.machineNames(job.uid)
	if err != nil {
		return store.FolderProject{}, folderRefusal(err, "")
	}
	p := store.FolderProject{Host: job.agent, Dir: result.Dir, Name: job.name, IsRepo: true, BaseBranch: result.DefaultBranch,
		Origin: job.owner + "/" + job.repo, Cloned: result.Marked}
	if refusal := s.checkFolderProject(names, &p, nil); refusal != nil {
		return store.FolderProject{}, refusal
	}
	saved, err := s.st.CreateFolderProjectFor(job.uid, p)
	if err != nil {
		return store.FolderProject{}, folderRefusal(err, machineLabel(names, p.Host))
	}
	detail := folderAuditDetail(saved, names)
	detail["origin"], detail["cloned"] = saved.Origin, saved.Cloned
	s.event(job.uid, "ui", "project-add", "", detail)
	return saved, nil
}

// cloneRefusal words a clone that left no project: the machine's reason, or
// the broker's error. machine is the machine's current name; repo is
// owner/name as the person wrote it.
func cloneRefusal(err error, reason terminalproto.RepoCloneReason, machine, repo string) (code, sentence string) {
	switch {
	case errors.Is(err, terminalbroker.ErrRepoCloneUnsupported):
		return "repo_clone_unsupported", sentenceRepoCloneOld(machine)
	case errors.Is(err, terminalbroker.ErrOffline):
		return "machine_offline", machine + " is offline. Try again when it's back."
	case err != nil:
		// No answer in time, an answer reet didn't expect, or the server
		// shutting down: whether anything was kept there is unknown.
		return string(terminalproto.RepoCloneFailed), "Couldn't clone " + repo + " on " + machine + "."
	}
	switch reason {
	case terminalproto.RepoCloneNotSignedIn:
		return string(reason), machine + " isn't signed in to GitHub. Run gh auth login there, then try again."
	case terminalproto.RepoCloneNoAccess:
		return string(reason), "GitHub has no " + repo + " that " + machine + "'s sign-in can read."
	case terminalproto.RepoCloneExists:
		return string(reason), "~/reet/repos/" + repo + " on " + machine + " already holds something else. Move it, or add that folder as a project."
	case terminalproto.RepoCloneTimedOut:
		return string(reason), "Cloning " + repo + " took longer than 10 minutes and was stopped. Nothing was kept."
	case terminalproto.RepoCloneBusy:
		return string(reason), machine + " is already cloning a repository. Try again when it finishes."
	case terminalproto.RepoCloneDenied:
		// The machine answers denied for each of these and the controller is
		// not told which: a missing gh is not advertised again.
		return string(reason), machine + " refused to clone: terminal access is off there, or git or the GitHub CLI is missing."
	case terminalproto.RepoCloneSSHRewrite:
		return string(reason), machine + "'s git settings send github.com over ssh, which reet doesn't use for cloning. Clone " + repo + " there yourself and add the folder as a project."
	}
	return string(terminalproto.RepoCloneFailed), "Couldn't clone " + repo + " on " + machine + "."
}

// folderProjectClone answers one of the account's clone jobs: cloning, done
// with the saved project, or failed with a code and one sentence.
func (s *server) folderProjectClone(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	job, ok := s.clones.lookup(uid, r.PathValue("id"))
	if !ok {
		writeErrCode(w, http.StatusNotFound, "clone_not_found", "", sentenceCloneGone, "")
		return
	}
	switch job.state {
	case cloneDone:
		names, err := s.machineNames(uid)
		if err != nil {
			folderRefusal(err, "").write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"state": cloneDone, "project": viewFolderProject(job.project, names)})
	case cloneFailed:
		writeJSON(w, http.StatusOK, map[string]any{"state": cloneFailed, "code": job.code, "sentence": job.sentence})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"state": cloneRunning})
	}
}
