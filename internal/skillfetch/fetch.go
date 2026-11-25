// Package skillfetch downloads one public skill's folder, from a GitHub
// repository or a well-known agent-skills index, and returns checked files.
// The links come from users, so every request is guarded: addresses on this
// machine or private networks are refused, redirects stay on known hosts, no
// credentials are sent, and every body is read under a cap.
package skillfetch

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

var (
	ErrNotFound = errors.New("skill not found in its source")
	ErrTooLarge = errors.New("skill is over 8 MiB or 256 files")
	// ErrArchiveTooLarge is the repository, not the skill, being too big to read.
	ErrArchiveTooLarge = errors.New("repository archive is too large to read")
	// ErrUnreachable is any failure to connect. Its text is all a writer sees: the
	// transport's own names the addresses it tried.
	ErrUnreachable = errors.New("could not reach the source")
)

var (
	errTooManyRedirects = errors.New("too many redirects")
	errRedirectHost     = errors.New("redirect to a host this server may not fetch from")
)

const (
	maxIndexBytes = 1 << 20
	// A repository holds many skills, so the archive as a whole may be larger
	// than one skill. These bound what is read before the skill is chosen.
	maxArchiveBytes   = 128 << 20 // compressed
	maxStreamBytes    = 256 << 20 // uncompressed, entry sizes added up
	maxRetainedBytes  = 64 << 20
	maxArchiveEntries = 20000
	maxRedirects      = 5
)

// Result is a skill's files, checked by skillimport.Select.
type Result struct {
	Files    []skillbundle.ContentFile
	Manifest skillbundle.Manifest
	Origin   string
}

// fetchTimeout bounds one whole Fetch, however many requests it makes.
const fetchTimeout = 2 * time.Minute

// Fetcher downloads skills. A nil Client is a guarded default; GitHub defaults
// to https://codeload.github.com.
type Fetcher struct {
	Client *http.Client
	GitHub string

	// Set by tests to avoid the environment, DNS and the production caps; the
	// first four are used with a nil Client.
	proxy      func(*http.Request) (*url.URL, error)
	lookup     func(context.Context, string) ([]net.IP, error)
	newClient  func() *http.Client
	timeout    time.Duration
	archiveCap int64 // compressed bytes read; maxArchiveBytes when zero
	streamCap  int64 // uncompressed bytes; maxStreamBytes when zero
}

// Outcome is one skill of a FetchMany: its Result, or why it has none.
type Outcome struct {
	Result
	Err error
}

func fetchable(r ref.Ref) error {
	if r.SourceType != "github" && r.SourceType != "well-known" {
		return fmt.Errorf("skill source type %q is not one reet can fetch from", r.SourceType)
	}
	if r.Slug == "" {
		return errors.New("the skill has no name to look for")
	}
	return nil
}

// start bounds a fetch by fetchTimeout, or by the caller's deadline when that
// is sooner, and gives it a client. done ends it.
func (f *Fetcher) start(ctx context.Context) (_ context.Context, done func()) {
	timeout := f.timeout
	if timeout <= 0 {
		timeout = fetchTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	if f.Client != nil {
		return ctx, cancel
	}
	// One client for the whole fetch, so its requests share connections.
	if f.newClient != nil {
		f.Client = f.newClient()
	} else {
		f.Client = defaultClient(f.proxy, f.lookup)
	}
	client := f.Client
	return ctx, func() { client.CloseIdleConnections(); cancel() }
}

// checked turns the files found for one skill into its outcome.
func checked(ctx context.Context, files []skillbundle.ContentFile, origin string, err error) Outcome {
	if err != nil {
		// The deadline is the truth when it has passed: a read it cut short can
		// surface as an EOF or a damaged archive.
		if cerr := ctx.Err(); cerr != nil && !errors.Is(err, cerr) {
			err = fmt.Errorf("fetch skill source: %w", cerr)
		}
		return Outcome{Err: err}
	}
	m, selected, err := skillimport.Select(files, nil)
	if err != nil {
		return Outcome{Err: err}
	}
	return Outcome{Result: Result{Files: selected, Manifest: m, Origin: origin}}
}

// Fetch returns the files of the skill r names. The whole fetch is bounded by
// fetchTimeout, or by the caller's deadline when that is sooner.
func (f Fetcher) Fetch(ctx context.Context, r ref.Ref) (Result, error) {
	if err := fetchable(r); err != nil {
		return Result{}, err
	}
	ctx, done := f.start(ctx)
	defer done()
	var out Outcome
	if r.SourceType == "well-known" {
		files, origin, err := f.wellKnown(ctx, r)
		out = checked(ctx, files, origin, err)
	} else {
		out = f.githubSkills(ctx, r.Source, []string{r.Slug})[0]
	}
	return out.Result, out.Err
}

// FetchMany is Fetch for several skills, with one outcome per ref in the same
// order. Skills of one GitHub repository share one download of it, bounded as
// one Fetch is.
func (f Fetcher) FetchMany(ctx context.Context, refs []ref.Ref) []Outcome {
	out := make([]Outcome, len(refs))
	repos := map[string][]int{}
	var order []string
	for i, r := range refs {
		if r.SourceType != "github" || fetchable(r) != nil {
			out[i].Result, out[i].Err = f.Fetch(ctx, r)
			continue
		}
		if repos[r.Source] == nil {
			order = append(order, r.Source)
		}
		repos[r.Source] = append(repos[r.Source], i)
	}
	for _, source := range order {
		slugs := make([]string, len(repos[source]))
		for n, i := range repos[source] {
			slugs[n] = refs[i].Slug
		}
		got := func() []Outcome {
			f := f
			ctx, done := f.start(ctx)
			defer done()
			return f.githubSkills(ctx, source, slugs)
		}()
		for n, i := range repos[source] {
			out[i] = got[n]
		}
	}
	return out
}

// githubSkills downloads the repository source once and returns the skill of
// each slug in it.
func (f Fetcher) githubSkills(ctx context.Context, source string, slugs []string) []Outcome {
	out := make([]Outcome, len(slugs))
	found, origins, errs, err := f.github(ctx, source, slugs)
	for i := range slugs {
		if err != nil {
			out[i] = checked(ctx, nil, "", err)
			continue
		}
		out[i] = checked(ctx, found[i], origins[i], errs[i])
	}
	return out
}

func githubHost(h string) bool {
	h = strings.ToLower(h)
	return h == "github.com" || h == "codeload.github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

// get requests rawURL with no credentials or cookies. Redirects are allowed
// only to GitHub's hosts (github) or to the same host (well-known), over https.
func (f Fetcher) get(ctx context.Context, rawURL string, github bool) (*http.Response, error) {
	c := *f.Client
	c.Jar = nil
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errTooManyRedirects
		}
		allowed := req.URL.Scheme == "https" && strings.EqualFold(req.URL.Host, via[0].URL.Host)
		if github {
			allowed = req.URL.Scheme == "https" && githubHost(req.URL.Hostname())
		}
		if !allowed {
			return errRedirectHost
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		// The transport's text names the addresses it dialled, resolved or proxied
		// through. What this server refused, a deadline and a cancelled request are said as such.
		for _, known := range []error{errAddress, errRedirectHost, errTooManyRedirects, context.DeadlineExceeded, context.Canceled} {
			if errors.Is(err, known) {
				return nil, known
			}
		}
		return nil, ErrUnreachable
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		resp.Body.Close()
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		resp.Body.Close()
		return nil, fmt.Errorf("skill source answered %d", resp.StatusCode)
	}
	return resp, nil
}

type tally struct {
	files int
	bytes int64
}

// capReader reads at most left bytes of r. If r has more, the read fails with
// ErrArchiveTooLarge, so a cut-off archive is reported as too large and not as a
// damaged gzip stream.
type capReader struct {
	r    io.Reader
	left int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		var b [1]byte
		if n, err := c.r.Read(b[:]); n > 0 {
			return 0, ErrArchiveTooLarge
		} else if err != nil {
			return 0, err
		}
		return 0, nil
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}

// github reads the archive of the repository source once and picks the folder
// of each slug: its files and origin, or its own error. The last error is the
// archive's, and is every slug's.
func (f Fetcher) github(ctx context.Context, source string, slugs []string) ([][]skillbundle.ContentFile, []string, []error, error) {
	fail := func(err error) ([][]skillbundle.ContentFile, []string, []error, error) { return nil, nil, nil, err }
	owner, repo, ok := strings.Cut(source, "/")
	if !ok || owner == "" || repo == "" || strings.ContainsAny(repo, "/?#") || strings.ContainsAny(owner, "?#") {
		return fail(ErrNotFound)
	}
	base := strings.TrimSuffix(f.GitHub, "/")
	if base == "" {
		base = "https://codeload.github.com"
	}
	resp, err := f.get(ctx, base+"/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/tar.gz/HEAD", true)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	archiveCap, streamCap := f.archiveCap, f.streamCap
	if archiveCap <= 0 {
		archiveCap = maxArchiveBytes
	}
	if streamCap <= 0 {
		streamCap = maxStreamBytes
	}
	gz, err := gzip.NewReader(&capReader{r: resp.Body, left: archiveCap})
	if err != nil {
		return fail(fmt.Errorf("skill archive: %w", err))
	}
	// first holds the folder each slug prefers above all others.
	first, tooLarge := map[string]bool{}, map[string]bool{}
	for _, slug := range slugs {
		first["skills/"+slug] = true
	}
	tallies := map[string]*tally{}
	all := map[string]held{}
	skillDirs := map[string]bool{}
	var links []string
	var retained, streamed int64
	tr := tar.NewReader(gz)
	for entries := 0; ; entries++ {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("skill archive: %w", err))
		}
		if entries > maxArchiveEntries {
			return fail(ErrArchiveTooLarge)
		}
		if strings.HasPrefix(h.Name, "/") {
			return fail(errors.New("skill archive has an absolute path"))
		}
		for _, seg := range strings.Split(h.Name, "/") {
			if seg == ".." {
				return fail(errors.New("skill archive has a path outside its folder"))
			}
		}
		// Whatever the type, the size is what the reader will skip or read.
		if h.Size < 0 || h.Size > streamCap-streamed {
			return fail(ErrArchiveTooLarge)
		}
		streamed += h.Size
		if h.Typeflag != tar.TypeReg {
			// A link is never followed, but a skill saved without it would look
			// complete, so where it was is remembered.
			if _, rel, ok := strings.Cut(path.Clean(h.Name), "/"); ok && (h.Typeflag == tar.TypeSymlink || h.Typeflag == tar.TypeLink) {
				links = append(links, rel)
			}
			continue // directories, links, pax headers
		}
		_, rel, ok := strings.Cut(path.Clean(h.Name), "/")
		if !ok || rel == "" || rel == "." {
			continue
		}
		dir := path.Dir(rel)
		if dir == "." {
			dir = ""
		}
		if path.Base(rel) == "SKILL.md" {
			skillDirs[dir] = true
		}
		for d := dir; ; d = parent(d) {
			t := tallies[d]
			if t == nil {
				t = &tally{}
				tallies[d] = t
			}
			t.files++
			t.bytes += h.Size
			over := t.files > skillbundle.MaxFiles || t.bytes > skillbundle.MaxBytes
			// skills/<slug> with a SKILL.md is the skill whatever else the
			// archive holds. When every skill asked for is such a folder over
			// the limits, stop here instead of reading the rest of the archive.
			if over && skillDirs[d] && first[d] {
				tooLarge[d] = true
				if len(tooLarge) == len(first) {
					return fail(ErrTooLarge)
				}
			}
			if d == "" {
				break
			}
		}
		if h.Size > skillbundle.MaxBytes {
			continue
		}
		retained += h.Size
		if retained > maxRetainedBytes {
			return fail(ErrArchiveTooLarge)
		}
		data, err := io.ReadAll(io.LimitReader(tr, h.Size))
		if err != nil {
			return fail(fmt.Errorf("skill archive: %w", err))
		}
		all[rel] = held{data, h.Mode&0o111 != 0}
	}

	if err := ctx.Err(); err != nil {
		return fail(fmt.Errorf("fetch skill source: %w", err))
	}
	found, origins, errs := make([][]skillbundle.ContentFile, len(slugs)), make([]string, len(slugs)), make([]error, len(slugs))
	for i, slug := range slugs {
		best, ok := pickFolder(skillDirs, all, slug)
		if !ok {
			errs[i] = ErrNotFound
			continue
		}
		if t := tallies[best]; t.files > skillbundle.MaxFiles || t.bytes > skillbundle.MaxBytes {
			errs[i] = ErrTooLarge
			continue
		}
		prefix := best
		if prefix != "" {
			prefix += "/"
		}
		for _, l := range links {
			if sub, ok := strings.CutPrefix(l, prefix); ok {
				errs[i] = fmt.Errorf("the skill has a link (%s) that reet does not copy", sub)
				break
			}
		}
		if errs[i] != nil {
			continue
		}
		var files []skillbundle.ContentFile
		for rel, h := range all {
			if sub, ok := strings.CutPrefix(rel, prefix); ok {
				files = append(files, skillbundle.ContentFile{Path: sub, Data: h.data, Executable: h.exec})
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		found[i] = files
		origins[i] = "https://github.com/" + owner + "/" + repo + "/tree/HEAD"
		if best != "" {
			origins[i] += "/" + best
		}
	}
	return found, origins, errs, nil
}

func (f Fetcher) wellKnown(ctx context.Context, r ref.Ref) ([]skillbundle.ContentFile, string, error) {
	if r.Source == "" || strings.ContainsAny(r.Source, "/\\?#@ ") {
		return nil, "", ErrNotFound
	}
	root := "https://" + r.Source + "/.well-known/agent-skills/"
	body, err := f.readAll(ctx, root+"index.json", maxIndexBytes)
	if err != nil {
		return nil, "", err
	}
	var index struct {
		Skills []struct {
			Name  string   `json:"name"`
			Files []string `json:"files"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(body, &index); err != nil {
		return nil, "", errors.New("skill index is not valid JSON")
	}
	slug := normalize(r.Slug)
	for _, s := range index.Skills {
		if normalize(s.Name) != slug {
			continue
		}
		if !safeSegment(s.Name) {
			return nil, "", errors.New("skill index lists an unsafe skill name")
		}
		names := s.Files
		if !slices.Contains(names, "SKILL.md") {
			names = append([]string{"SKILL.md"}, names...)
		}
		if len(names) > skillbundle.MaxFiles {
			return nil, "", ErrTooLarge
		}
		var files []skillbundle.ContentFile
		var total int64
		seen := map[string]bool{}
		for _, name := range names {
			if !safeRel(name) || seen[name] {
				return nil, "", errors.New("skill index lists an unsafe file path")
			}
			seen[name] = true
			segs := strings.Split(name, "/")
			for i := range segs {
				segs[i] = url.PathEscape(segs[i])
			}
			data, err := f.readAll(ctx, root+url.PathEscape(s.Name)+"/"+strings.Join(segs, "/"), skillbundle.MaxBytes-total)
			if err != nil {
				if errors.Is(err, ErrNotFound) && name != "SKILL.md" {
					return nil, "", fmt.Errorf("skill file %s listed but missing: %w", name, err)
				}
				return nil, "", err
			}
			total += int64(len(data))
			files = append(files, skillbundle.ContentFile{Path: name, Data: data})
		}
		return files, r.URL, nil
	}
	return nil, "", ErrNotFound
}

// safeSegment reports whether name can be one path segment of a request.
func safeSegment(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\")
}

func safeRel(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || path.Clean(p) != p {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." {
			return false
		}
	}
	return true
}

// readAll reads a well-known body of at most limit bytes; more is ErrTooLarge
// for files and an error for the index (limit there is maxIndexBytes).
func (f Fetcher) readAll(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	resp, err := f.get(ctx, rawURL, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	// A body cut short by the deadline or a cancel can look like a clean end;
	// it must never be taken as the whole file.
	if cerr := ctx.Err(); cerr != nil {
		return nil, fmt.Errorf("fetch skill source: %w", cerr)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch skill source: %w", err)
	}
	if int64(len(b)) > limit {
		return nil, ErrTooLarge
	}
	return b, nil
}
