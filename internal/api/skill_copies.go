package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillfetch"
	"github.com/eyupio/reet.sh/internal/store"
)

// skillFetchTimeout bounds one public skill's download.
const skillFetchTimeout = 2 * time.Minute

// SkillFetcher downloads one public skill. skillfetch.Fetcher is the
// production one, with its guarded client; tests pass a stub.
type SkillFetcher interface {
	Fetch(ctx context.Context, r ref.Ref) (skillfetch.Result, error)
}

// WithSkillFetcher replaces how public skills are fetched. Only tests need it.
func WithSkillFetcher(f SkillFetcher) Option { return func(s *server) { s.skillFetcher = f } }

// noSkillFetcher is the default: it fetches nothing, so a server built without
// WithSkillFetcher (every test) never reaches the network.
type noSkillFetcher struct{}

var errNoSkillFetcher = errors.New("public skills are not fetched by this server")

func (noSkillFetcher) Fetch(context.Context, ref.Ref) (skillfetch.Result, error) {
	return skillfetch.Result{}, errNoSkillFetcher
}

var errNoOriginal = errors.New("this skill has no original link to update from")

func (s *server) registerSkillCopies(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/skills/retry", s.requireWriter(s.retrySkillCopies))
	mux.HandleFunc("GET /api/v2/skills/{item}/original", s.requireWriter(s.skillOriginalChanges))
	mux.HandleFunc("POST /api/v2/skills/{item}/original", s.requireWriter(s.skillOriginalUpdate))
}

// kickSkillCopies makes sure every skill of the account that has no content in
// reet is being fetched or copied. It runs in the background, one run at a
// time per account: a kick while one is running makes that run go round once
// more. It fetches first, then asks machines for the rest, and never blocks the
// request that triggered it.
func (s *server) kickSkillCopies(uid int64) {
	s.skillCopyMu.Lock()
	if s.ctx.Err() != nil {
		s.skillCopyMu.Unlock()
		return
	}
	if s.skillCopyRunning[uid] {
		s.skillCopyAgain[uid] = true
		s.skillCopyMu.Unlock()
		return
	}
	s.skillCopyRunning[uid] = true
	s.skillCopies.Add(1)
	s.workers.Add(1)
	s.skillCopyMu.Unlock()
	go func() {
		defer s.workers.Done()
		defer s.skillCopies.Done()
		for {
			s.runSkillCopies(uid)
			s.skillCopyMu.Lock()
			again := s.skillCopyAgain[uid] && s.ctx.Err() == nil
			s.skillCopyAgain[uid] = false
			if !again {
				s.skillCopyRunning[uid] = false
			}
			s.skillCopyMu.Unlock()
			if !again {
				return
			}
		}
	}()
}

func (s *server) runSkillCopies(uid int64) {
	if err := s.st.PurgeDeletedSkillsFor(uid, time.Now()); err != nil {
		log.Printf("skill copies: purge: %v", err)
	}
	needs, err := s.st.SkillsNeedingCopyFor(uid)
	if err != nil {
		log.Printf("skill copies: %v", err)
		return
	}
	for _, n := range needs {
		if n.Kind == "fetch" && s.ctx.Err() == nil {
			s.fetchSkillCopy(uid, n)
		}
	}
	for _, n := range needs {
		if n.Kind != "capture" || s.ctx.Err() != nil {
			continue
		}
		err := s.st.RequestSkillCaptureFor(uid, n.Item, n.AgentID, n.Folder+"/"+n.Name)
		// A machine that cannot take the request yet is asked again on a later
		// kick or check-in.
		if err != nil && !errors.Is(err, store.ErrRemoteImport) && !errors.Is(err, store.ErrRemoteImportCapacity) && !errors.Is(err, store.ErrSkillNoCaptureSource) {
			log.Printf("skill copies: capture %s: %v", n.Name, err)
		}
	}
	if _, err = s.st.MarkUnsourcedSkillsFor(uid); err != nil {
		log.Printf("skill copies: %v", err)
	}
}

func (s *server) fetchSkillCopy(uid int64, n store.SkillCopyNeed) {
	r, ok, err := s.st.SkillSourceFor(uid, n.Item)
	if err != nil || !ok {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, skillFetchTimeout)
	defer cancel()
	res, err := s.skillFetcher.Fetch(ctx, r)
	if err == nil {
		_, err = s.st.SetSkillContentFor(uid, n.Item, res.Files, store.PrivateSkillOrigin{Kind: "original", Reference: res.Origin})
	}
	if err == nil || s.ctx.Err() != nil || errors.Is(err, errNoSkillFetcher) {
		return
	}
	if errors.Is(err, store.ErrPrivateSkillConflict) || errors.Is(err, store.ErrPrivateSkillMissing) {
		return // changed under us; the next run looks again
	}
	if perr := s.st.SetSkillCopyProblemFor(uid, n.Item, skillCopyProblem(err)); perr != nil && !errors.Is(perr, store.ErrPrivateSkillMissing) {
		log.Printf("skill copies: %v", perr)
	}
}

// skillCopyProblem words why a skill's files could not be fetched or saved.
func skillCopyProblem(err error) string {
	switch {
	case errors.Is(err, skillfetch.ErrNotFound):
		return "The skill is no longer at its original link"
	case errors.Is(err, skillfetch.ErrTooLarge):
		return "The skill is over 8 MiB or 256 files"
	case errors.Is(err, store.ErrPrivateSkillInvalid):
		return "The files at the original link did not pass reet's checks"
	case errors.Is(err, store.ErrPrivateSkillCapacity):
		return "reet's storage for skills is full"
	case errors.Is(err, context.DeadlineExceeded):
		return "Getting the files took too long"
	}
	msg := strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0])
	if len(msg) > 200 {
		msg = strings.ToValidUTF8(msg[:200], "")
	}
	return "Couldn't get the files: " + msg
}

// retrySkillCopies is Retry on a skill that could not be fetched or copied.
// The body is {"items":["<library item id>", ...]}. It clears each skill's
// problem and runs the copy again.
func (s *server) retrySkillCopies(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items []string `json:"items"`
	}
	if !decode(w, r, &in) {
		return
	}
	items := uniqueStrings(in.Items)
	if len(items) == 0 || len(items) > 500 {
		writeErrCode(w, 400, errInvalidArgument, "items", "supply between 1 and 500 skill ids", "")
		return
	}
	n, err := s.st.RetrySkillCopiesFor(s.uid(r), items)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.kickSkillCopies(s.uid(r))
	writeJSON(w, 200, map[string]int{"retried": n})
}

// fetchOriginal downloads the original of an imported skill and the manifest
// reet holds now (nil when it has no content), and the id of the revision it
// read that manifest from, taken before the download starts.
func (s *server) fetchOriginal(ctx context.Context, uid int64, item string) (skillfetch.Result, *skillbundle.Manifest, string, error) {
	var none skillfetch.Result
	li, err := s.st.LibraryItemFor(uid, item)
	if err != nil {
		return none, nil, "", err
	}
	r, ok, err := s.st.SkillSourceFor(uid, item)
	if err != nil {
		return none, nil, "", err
	}
	if !ok || li.ItemType != store.LibrarySkill {
		return none, nil, "", errNoOriginal
	}
	var current *skillbundle.Manifest
	if li.BundleID != "" && li.RevisionID != "" {
		rev, e := s.st.PrivateSkillRevisionFor(uid, li.BundleID, li.RevisionID)
		if e != nil {
			return none, nil, "", e
		}
		current = rev.Manifest
	}
	ctx, cancel := context.WithTimeout(ctx, skillFetchTimeout)
	defer cancel()
	res, err := s.skillFetcher.Fetch(ctx, r)
	return res, current, li.RevisionID, err
}

// originalChanges fetches the original of an imported skill and compares it
// with reet's copy. It returns the changes, the digest of what it fetched, the
// revision it compared against and whether the skill was edited since it was imported. It changes nothing.
func (s *server) originalChanges(ctx context.Context, uid int64, item string) (skillbundle.Diff, string, string, bool, error) {
	res, current, revision, err := s.fetchOriginal(ctx, uid, item)
	if err != nil {
		return skillbundle.Diff{}, "", "", false, err
	}
	edited, err := s.st.SkillEditedSinceImportFor(uid, item)
	if err != nil {
		return skillbundle.Diff{}, "", "", false, err
	}
	return skillbundle.Compare(current, &res.Manifest), res.Manifest.Digest, revision, edited, nil
}

func originalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoOriginal), errors.Is(err, store.ErrPrivateSkillMissing):
		writeErr(w, 404, "This skill has no original link to update from.")
	case errors.Is(err, skillfetch.ErrNotFound):
		writeErr(w, 502, "The skill is no longer at its original link.")
	case errors.Is(err, skillfetch.ErrTooLarge):
		writeErr(w, 502, "The original is over 8 MiB or 256 files.")
	case errors.Is(err, context.DeadlineExceeded):
		writeErr(w, 504, "Getting the original took too long.")
	case errors.Is(err, store.ErrPrivateSkillInvalid), errors.Is(err, store.ErrPrivateSkillConflict), errors.Is(err, store.ErrPrivateSkillCapacity):
		privateSkillError(w, err)
	default:
		writeErr(w, 502, "Couldn't get the original.")
	}
}

// skillOriginalChanges is the first half of Update from original: which
// files differ, the digest to confirm, and whether the person edited the skill.
func (s *server) skillOriginalChanges(w http.ResponseWriter, r *http.Request) {
	diff, digest, revision, edited, err := s.originalChanges(r.Context(), s.uid(r), r.PathValue("item"))
	if err != nil {
		originalError(w, err)
		return
	}
	type change struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}
	changes := make([]change, 0, len(diff.Changes))
	for _, c := range diff.Changes {
		changes = append(changes, change{c.Path, c.Kind})
	}
	writeJSON(w, 200, map[string]any{"changes": changes, "state": diff.State, "digest": digest, "revision": revision, "edited": edited})
}

// skillOriginalUpdate is the confirmed half: body {"digest":"<digest from
// GET>","revision":"<revision from GET>"}. It fetches again and saves a new
// revision only when what it fetched is what was shown and the skill is still
// at the revision that was compared (409 otherwise) and differs from reet's copy.
func (s *server) skillOriginalUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Digest   string `json:"digest"`
		Revision string `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision == "" {
		writeErr(w, 400, "Say which revision you looked at. Review the changes again.")
		return
	}
	uid, item := s.uid(r), r.PathValue("item")
	res, current, revision, err := s.fetchOriginal(r.Context(), uid, item)
	if err != nil {
		originalError(w, err)
		return
	}
	if in.Digest == "" || res.Manifest.Digest != in.Digest {
		writeErr(w, 409, "The original changed since you looked at it. Review the changes again.")
		return
	}
	if revision != in.Revision {
		writeErr(w, 409, "The skill changed since you looked at it. Review the changes again.")
		return
	}
	edited, err := s.st.SkillEditedSinceImportFor(uid, item)
	if err != nil {
		originalError(w, err)
		return
	}
	if skillbundle.Compare(current, &res.Manifest).State == "equal" {
		writeJSON(w, 200, map[string]any{"updated": false, "digest": res.Manifest.Digest, "edited": edited})
		return
	}
	if _, err = s.st.SetSkillContentFromFor(uid, item, res.Files, store.PrivateSkillOrigin{Kind: "original", Reference: res.Origin}, in.Revision); err != nil {
		if errors.Is(err, store.ErrPrivateSkillConflict) {
			writeErr(w, 409, "The skill changed since you looked at it. Review the changes again.")
			return
		}
		originalError(w, err)
		return
	}
	s.eventRequest(r, "update-from-original", item, map[string]any{"replacedEdits": edited})
	writeJSON(w, 200, map[string]any{"updated": true, "digest": res.Manifest.Digest, "edited": edited})
}
