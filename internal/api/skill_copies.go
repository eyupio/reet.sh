package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillfetch"
	"github.com/eyupio/reet.sh/internal/store"
	"gopkg.in/yaml.v3"
)

// skillFetchTimeout bounds one public skill's download.
const skillFetchTimeout = 2 * time.Minute

// skillCopyBodyLimit bounds the small JSON bodies of Retry and Update from
// original: at most 500 ids, or a digest and a revision.
const skillCopyBodyLimit = 64 << 10

// skillOriginalTTL is how long a fetched original is remembered for GET, so a
// page that draws again does not download it again. A POST always fetches.
const skillOriginalTTL = 60 * time.Second

// skillOriginalPerAccount is how many originals one account downloads at a
// time; a request beyond it is told to wait (429).
const skillOriginalPerAccount = 2

// skillOriginalCacheMax bounds the remembered originals; only manifests are
// kept, which are small.
const skillOriginalCacheMax = 128

type originalKey struct {
	uid  int64
	item string
}

type originalEntry struct {
	manifest skillbundle.Manifest
	at       time.Time
}

// originalCache remembers the manifest of the last fetch of an original, per
// account and skill.
type originalCache struct {
	mu      sync.Mutex
	now     func() time.Time // tests only
	entries map[originalKey]originalEntry
}

func (c *originalCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *originalCache) get(k originalKey, ttl time.Duration) (skillbundle.Manifest, bool) {
	if ttl <= 0 {
		return skillbundle.Manifest{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok || c.clock().Sub(e.at) >= ttl {
		return skillbundle.Manifest{}, false
	}
	return e.manifest, true
}

func (c *originalCache) put(k originalKey, m skillbundle.Manifest, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	if c.entries == nil {
		c.entries = map[originalKey]originalEntry{}
	}
	if _, ok := c.entries[k]; !ok && len(c.entries) >= skillOriginalCacheMax {
		oldest, found := originalKey{}, false
		for key, e := range c.entries {
			if now.Sub(e.at) >= ttl {
				delete(c.entries, key)
			} else if !found || e.at.Before(c.entries[oldest].at) {
				oldest, found = key, true
			}
		}
		if len(c.entries) >= skillOriginalCacheMax && found {
			delete(c.entries, oldest)
		}
	}
	c.entries[k] = originalEntry{m, now}
}

// fetchError marks an error from downloading the original, as opposed to one
// from reet's own storage.
type fetchError struct{ err error }

func (e fetchError) Error() string { return e.err.Error() }
func (e fetchError) Unwrap() error { return e.err }

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

var errOriginalBusy = errors.New("this account is already downloading originals")

// originalSlot takes one of the account's download slots, or reports there is
// none free; release gives it back.
func (s *server) originalSlot(uid int64) (release func(), ok bool) {
	s.originalMu.Lock()
	defer s.originalMu.Unlock()
	if s.originalRunning[uid] >= skillOriginalPerAccount {
		return nil, false
	}
	if s.originalRunning == nil {
		s.originalRunning = map[int64]int{}
	}
	s.originalRunning[uid]++
	return func() {
		s.originalMu.Lock()
		defer s.originalMu.Unlock()
		if s.originalRunning[uid]--; s.originalRunning[uid] <= 0 {
			delete(s.originalRunning, uid)
		}
	}, true
}

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
	if err != nil {
		log.Printf("skill copies: source of %s: %v", n.Name, err)
		return
	}
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, skillFetchTimeout)
	defer cancel()
	res, err := s.skillFetcher.Fetch(ctx, r)
	problem := ""
	if err == nil {
		// The fetch picked the folder by a loose match of the name, so the
		// SKILL.md is compared the same way.
		if got := skillMDName(res.Files); got != "" && !skillfetch.SameName(got, n.Name) {
			problem = fmt.Sprintf("The original's SKILL.md is named %q, but this skill is named %q", truncate(got, 64), truncate(n.Name, 64))
		} else {
			_, err = s.st.SetSkillContentFor(uid, n.Item, res.Files, store.PrivateSkillOrigin{Kind: "original", Reference: res.Origin})
		}
	}
	if (err == nil && problem == "") || s.ctx.Err() != nil || errors.Is(err, errNoSkillFetcher) {
		return
	}
	if errors.Is(err, store.ErrPrivateSkillConflict) || errors.Is(err, store.ErrPrivateSkillMissing) {
		return // changed under us; the next run looks again
	}
	if problem == "" {
		problem = skillCopyProblem(err)
	}
	if perr := s.st.SetSkillCopyProblemFor(uid, n.Item, problem); perr != nil && !errors.Is(perr, store.ErrPrivateSkillMissing) {
		log.Printf("skill copies: %v", perr)
	}
}

// skillMDName is the name in the front matter of the SKILL.md in files, or ""
// when there is none to read.
func skillMDName(files []skillbundle.ContentFile) string {
	for _, f := range files {
		if f.Path != "SKILL.md" {
			continue
		}
		md := strings.ReplaceAll(string(f.Data), "\r\n", "\n")
		if !strings.HasPrefix(md, "---\n") {
			return ""
		}
		end := strings.Index(md[4:], "\n---\n")
		if end < 0 {
			return ""
		}
		var header struct {
			Name string `yaml:"name"`
		}
		if yaml.Unmarshal([]byte(md[4:4+end]), &header) != nil {
			return ""
		}
		return header.Name
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) > n {
		return strings.ToValidUTF8(s[:n], "") + "..."
	}
	return s
}

// skillCopyProblem words why a skill's files could not be fetched or saved.
func skillCopyProblem(err error) string {
	switch {
	case errors.Is(err, skillfetch.ErrNotFound):
		return "The skill is no longer at its original link"
	case errors.Is(err, skillfetch.ErrTooLarge):
		return "The skill is over 8 MiB or 256 files"
	case errors.Is(err, skillfetch.ErrArchiveTooLarge):
		return "The repository is too large to download"
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
	if !decodeStrictJSON(w, r, skillCopyBodyLimit, &in, "Invalid or oversized retry request.") {
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
//
// With cached, a fetch made in the last skillOriginalTTL is reused: the result
// then has its manifest only, no files.
func (s *server) fetchOriginal(ctx context.Context, uid int64, item string, cached bool) (skillfetch.Result, *skillbundle.Manifest, string, error) {
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
	key := originalKey{uid, item}
	if cached {
		if m, ok := s.original.get(key, s.originalTTL); ok {
			return skillfetch.Result{Manifest: m}, current, li.RevisionID, nil
		}
	}
	release, ok := s.originalSlot(uid)
	if !ok {
		return none, nil, "", errOriginalBusy
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, skillFetchTimeout)
	defer cancel()
	res, err := s.skillFetcher.Fetch(ctx, r)
	if err != nil {
		return none, nil, "", fetchError{err}
	}
	s.original.put(key, res.Manifest, s.originalTTL)
	return res, current, li.RevisionID, nil
}

// originalChanges fetches the original of an imported skill and compares it
// with reet's copy. It returns the changes, the digest of what it fetched, the
// revision it compared against and whether the skill was edited since it was imported. It changes nothing.
func (s *server) originalChanges(ctx context.Context, uid int64, item string) (skillbundle.Diff, string, string, bool, error) {
	res, current, revision, err := s.fetchOriginal(ctx, uid, item, true)
	if err != nil {
		return skillbundle.Diff{}, "", "", false, err
	}
	edited, err := s.st.SkillEditedSinceImportFor(uid, item)
	if err != nil {
		return skillbundle.Diff{}, "", "", false, err
	}
	return compareWithCopy(current, &res.Manifest), res.Manifest.Digest, revision, edited, nil
}

// compareWithCopy compares reet's copy of a skill with its original. A skill
// with no copy yet (current is nil) gets every file of the original as new.
func compareWithCopy(current, original *skillbundle.Manifest) skillbundle.Diff {
	if current != nil || original == nil {
		return skillbundle.Compare(current, original)
	}
	diff := skillbundle.Diff{State: "changed", Changes: []skillbundle.Change{}}
	for _, f := range original.Files {
		file := f
		diff.Changes = append(diff.Changes, skillbundle.Change{Path: file.Path, Kind: "added", After: &file})
	}
	return diff
}

func originalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoOriginal), errors.Is(err, store.ErrPrivateSkillMissing):
		writeErr(w, 404, "This skill has no original link to update from.")
	case errors.Is(err, errOriginalBusy):
		writeErr(w, http.StatusTooManyRequests, "reet is already getting originals for you. Try again in a moment.")
	case errors.Is(err, skillfetch.ErrNotFound):
		writeErr(w, 502, "The skill is no longer at its original link.")
	case errors.Is(err, skillfetch.ErrTooLarge):
		writeErr(w, 502, "The original is over 8 MiB or 256 files.")
	case errors.Is(err, skillfetch.ErrArchiveTooLarge):
		writeErr(w, 502, "The original's repository is too large to download.")
	case errors.Is(err, context.DeadlineExceeded):
		writeErr(w, 504, "Getting the original took too long.")
	case errors.Is(err, store.ErrPrivateSkillInvalid), errors.Is(err, store.ErrPrivateSkillConflict), errors.Is(err, store.ErrPrivateSkillCapacity):
		privateSkillError(w, err)
	case errors.As(err, new(fetchError)):
		writeErr(w, 502, "Couldn't get the original.")
	default:
		writeErr(w, 500, "Could not access private skill storage.")
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
		Digest string `json:"digest"`
		// Revision is the revision GET compared against: "" for a skill
		// that had no content, whose update is then its first fetch. It
		// must be sent either way.
		Revision *string `json:"revision"`
	}
	if !decodeStrictJSON(w, r, skillCopyBodyLimit, &in, "Invalid or oversized update request.") {
		return
	}
	if in.Revision == nil {
		writeErr(w, 400, "Say which revision you looked at. Review the changes again.")
		return
	}
	if in.Digest == "" {
		writeErr(w, 400, "Say which original you looked at. Review the changes again.")
		return
	}
	uid, item := s.uid(r), r.PathValue("item")
	res, current, revision, err := s.fetchOriginal(r.Context(), uid, item, false)
	if err != nil {
		originalError(w, err)
		return
	}
	if res.Manifest.Digest != in.Digest {
		writeErr(w, 409, "The original changed since you looked at it. Review the changes again.")
		return
	}
	// A revision of "" is accepted only while the skill still has no content;
	// content that landed meanwhile makes it stale like any other.
	if revision != *in.Revision {
		writeErr(w, 409, "The skill changed since you looked at it. Review the changes again.")
		return
	}
	edited, err := s.st.SkillEditedSinceImportFor(uid, item)
	if err != nil {
		originalError(w, err)
		return
	}
	if compareWithCopy(current, &res.Manifest).State == "equal" {
		writeJSON(w, 200, map[string]any{"updated": false, "digest": res.Manifest.Digest, "edited": edited})
		return
	}
	if _, err = s.st.SetSkillContentFromFor(uid, item, res.Files, store.PrivateSkillOrigin{Kind: "original", Reference: res.Origin}, *in.Revision); err != nil {
		if errors.Is(err, store.ErrPrivateSkillConflict) {
			writeErr(w, 409, "The skill changed since you looked at it. Review the changes again.")
			return
		}
		originalError(w, err)
		return
	}
	s.eventRequest(r, "update-from-original", item, map[string]any{"replacedEdits": edited})
	out := map[string]any{"updated": true, "digest": res.Manifest.Digest, "edited": edited}
	if problem := s.publicProblem(uid, item); problem != "" {
		out["publicProblem"] = problem
	}
	writeJSON(w, 200, out)
}
