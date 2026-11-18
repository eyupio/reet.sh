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

// Fetch returns the files of the skill r names. The whole fetch is bounded by
// fetchTimeout, or by the caller's deadline when that is sooner.
func (f Fetcher) Fetch(ctx context.Context, r ref.Ref) (Result, error) {
	if r.SourceType != "github" && r.SourceType != "well-known" {
		return Result{}, fmt.Errorf("skill source type %q is not one reet can fetch from", r.SourceType)
	}
	if r.Slug == "" {
		return Result{}, errors.New("the skill has no name to look for")
	}
	timeout := f.timeout
	if timeout <= 0 {
		timeout = fetchTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if f.Client == nil {
		// One client for the whole fetch, so its requests share connections.
		if f.newClient != nil {
			f.Client = f.newClient()
		} else {
			f.Client = defaultClient(f.proxy, f.lookup)
		}
		defer f.Client.CloseIdleConnections()
	}
	var files []skillbundle.ContentFile
	var origin string
	var err error
	if r.SourceType == "well-known" {
		files, origin, err = f.wellKnown(ctx, r)
	} else {
		files, origin, err = f.github(ctx, r)
	}
	if err != nil {
		return Result{}, err
	}
	m, selected, err := skillimport.Select(files, nil)
	if err != nil {
		return Result{}, err
	}
	return Result{Files: selected, Manifest: m, Origin: origin}, nil
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
			return errors.New("too many redirects")
		}
		allowed := req.URL.Scheme == "https" && strings.EqualFold(req.URL.Host, via[0].URL.Host)
		if github {
			allowed = req.URL.Scheme == "https" && githubHost(req.URL.Hostname())
		}
		if !allowed {
			return errors.New("redirect to a host this server may not fetch from")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch skill source: %w", err)
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

func (f Fetcher) github(ctx context.Context, r ref.Ref) ([]skillbundle.ContentFile, string, error) {
	owner, repo, ok := strings.Cut(r.Source, "/")
	if !ok || owner == "" || repo == "" || strings.ContainsAny(repo, "/?#") || strings.ContainsAny(owner, "?#") {
		return nil, "", ErrNotFound
	}
	base := strings.TrimSuffix(f.GitHub, "/")
	if base == "" {
		base = "https://codeload.github.com"
	}
	resp, err := f.get(ctx, base+"/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/tar.gz/HEAD", true)
	if err != nil {
		return nil, "", err
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
		return nil, "", fmt.Errorf("skill archive: %w", err)
	}
	tallies := map[string]*tally{}
	all := map[string]held{}
	skillDirs := map[string]bool{}
	var retained, streamed int64
	tr := tar.NewReader(gz)
	for entries := 0; ; entries++ {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("skill archive: %w", err)
		}
		if entries > maxArchiveEntries {
			return nil, "", ErrArchiveTooLarge
		}
		if strings.HasPrefix(h.Name, "/") {
			return nil, "", errors.New("skill archive has an absolute path")
		}
		for _, seg := range strings.Split(h.Name, "/") {
			if seg == ".." {
				return nil, "", errors.New("skill archive has a path outside its folder")
			}
		}
		// Whatever the type, the size is what the reader will skip or read.
		if h.Size < 0 || h.Size > streamCap-streamed {
			return nil, "", ErrArchiveTooLarge
		}
		streamed += h.Size
		if h.Typeflag != tar.TypeReg {
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
			// A SKILL.md folder named for the skill that is over the limits is
			// that skill; stop here instead of reading the rest of the archive.
			if over && d != "" && skillDirs[d] && matchesSlug(d, r.Slug) {
				return nil, "", ErrTooLarge
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
			return nil, "", ErrArchiveTooLarge
		}
		data, err := io.ReadAll(io.LimitReader(tr, h.Size))
		if err != nil {
			return nil, "", fmt.Errorf("skill archive: %w", err)
		}
		all[rel] = held{data, h.Mode&0o111 != 0}
	}

	best, ok := pickFolder(skillDirs, all, r.Slug)
	if !ok {
		return nil, "", ErrNotFound
	}
	if t := tallies[best]; t.files > skillbundle.MaxFiles || t.bytes > skillbundle.MaxBytes {
		return nil, "", ErrTooLarge
	}
	var files []skillbundle.ContentFile
	prefix := best
	if prefix != "" {
		prefix += "/"
	}
	for rel, h := range all {
		if sub, ok := strings.CutPrefix(rel, prefix); ok {
			files = append(files, skillbundle.ContentFile{Path: sub, Data: h.data, Executable: h.exec})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	origin := "https://github.com/" + owner + "/" + repo + "/tree/HEAD"
	if best != "" {
		origin += "/" + best
	}
	return files, origin, nil
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
		if !contains(names, "SKILL.md") {
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

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
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
	if err != nil {
		return nil, fmt.Errorf("fetch skill source: %w", err)
	}
	if int64(len(b)) > limit {
		return nil, ErrTooLarge
	}
	return b, nil
}
