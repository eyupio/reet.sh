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
	"sync"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

var (
	ErrNotFound = errors.New("skill not found in its source")
	ErrTooLarge = errors.New("skill is over 8 MiB or 256 files")
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

// Fetcher downloads skills. A nil Client is a guarded default; GitHub defaults
// to https://codeload.github.com.
type Fetcher struct {
	Client *http.Client
	GitHub string

	// Set by tests to avoid the environment and DNS; used with a nil Client.
	proxy  func(*http.Request) (*url.URL, error)
	lookup func(context.Context, string) ([]net.IP, error)
}

// Fetch returns the files of the skill r names.
func (f Fetcher) Fetch(ctx context.Context, r ref.Ref) (Result, error) {
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

var errAddress = errors.New("skill source address is not one this server may connect to")

// publicAddress reports whether ip is one skills may be fetched from: not this
// machine, a private or link-local network, or any other non-unicast address.
// The dial guard and the check made before a request goes to a proxy share it.
func publicAddress(ip net.IP) bool {
	cgnat := net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || cgnat.Contains(ip))
}

// refuseAddress is the dial-time guard. Skills are public, so unlike an
// identity provider nothing internal is a legitimate source.
func refuseAddress(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("skill source address is not an IP address")
	}
	if !publicAddress(ip) {
		return errAddress
	}
	return nil
}

func lookupIPs(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	ips := make([]net.IP, len(addrs))
	for i, a := range addrs {
		ips[i] = a.IP
	}
	return ips, err
}

// defaultClient honours the proxy settings of the environment, as the rest of
// reet does. Without a proxy the dial guard refuses non-public addresses. With
// one, the proxy itself is operator configuration and may be private, but the
// destination is resolved here first and refused unless every address is public;
// that runs for each redirect hop too, since each hop asks for its proxy.
func defaultClient(proxy func(*http.Request) (*url.URL, error), lookup func(context.Context, string) ([]net.IP, error)) *http.Client {
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	if lookup == nil {
		lookup = lookupIPs
	}
	var proxies sync.Map
	guarded := &net.Dialer{Timeout: 5 * time.Second, Control: refuseAddress}
	plain := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				p, err := proxy(req)
				if err != nil || p == nil {
					return p, err
				}
				host := req.URL.Hostname()
				ips := []net.IP{net.ParseIP(host)}
				if ips[0] == nil {
					if ips, err = lookup(req.Context(), host); err != nil {
						return nil, fmt.Errorf("resolve skill source: %w", err)
					}
				}
				for _, ip := range ips {
					if !publicAddress(ip) {
						return nil, errAddress
					}
				}
				proxies.Store(proxyAddress(p), true)
				return p, nil
			},
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if _, ok := proxies.Load(addr); ok {
					return plain.DialContext(ctx, network, addr)
				}
				return guarded.DialContext(ctx, network, addr)
			},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			MaxIdleConns:          4,
			IdleConnTimeout:       30 * time.Second,
		},
	}
}

func proxyAddress(p *url.URL) string {
	port := p.Port()
	if port == "" {
		switch p.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(p.Hostname(), port)
}

func githubHost(h string) bool {
	h = strings.ToLower(h)
	return h == "github.com" || h == "codeload.github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

// get requests rawURL with no credentials or cookies. Redirects are allowed
// only to GitHub's hosts (github) or to the same host (well-known), over https.
func (f Fetcher) get(ctx context.Context, rawURL string, github bool) (*http.Response, error) {
	client := f.Client
	if client == nil {
		client = defaultClient(f.proxy, f.lookup)
	}
	c := *client
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

// normalize compares names loosely: case, spaces and hyphens do not matter.
func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(s)), "-"))
}

func frontMatterName(data []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if v, ok := strings.CutPrefix(l, "name:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

type tally struct {
	files int
	bytes int64
}

type held struct {
	data []byte
	exec bool
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
	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxArchiveBytes))
	if err != nil {
		return nil, "", fmt.Errorf("skill archive: %w", err)
	}
	slug := normalize(r.Slug)
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
			return nil, "", ErrTooLarge
		}
		if strings.HasPrefix(h.Name, "/") {
			return nil, "", errors.New("skill archive has an absolute path")
		}
		for _, seg := range strings.Split(h.Name, "/") {
			if seg == ".." {
				return nil, "", errors.New("skill archive has a path outside its folder")
			}
		}
		if h.Typeflag != tar.TypeReg {
			continue // directories, links, pax headers
		}
		_, rel, ok := strings.Cut(path.Clean(h.Name), "/")
		if !ok || rel == "" || rel == "." {
			continue
		}
		streamed += h.Size
		if streamed > maxStreamBytes || h.Size < 0 {
			return nil, "", ErrTooLarge
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
			if over && d != "" && skillDirs[d] && (d == "skills/"+r.Slug || normalize(path.Base(d)) == slug) {
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
			return nil, "", ErrTooLarge
		}
		data, err := io.ReadAll(io.LimitReader(tr, h.Size))
		if err != nil {
			return nil, "", fmt.Errorf("skill archive: %w", err)
		}
		all[rel] = held{data, h.Mode&0o111 != 0}
	}

	best, bestRank := "", 4
	for dir := range skillDirs {
		rank := 4
		switch {
		case dir == "skills/"+r.Slug:
			rank = 0
		case dir != "" && path.Base(dir) == r.Slug:
			rank = 1
		case normalize(frontMatterName(all[join(dir, "SKILL.md")].data)) == slug:
			rank = 2
		case dir != "" && normalize(path.Base(dir)) == slug:
			rank = 3
		}
		if rank == 4 {
			continue
		}
		shallower := strings.Count(dir, "/") < strings.Count(best, "/")
		if rank < bestRank || (rank == bestRank && (shallower || (strings.Count(dir, "/") == strings.Count(best, "/") && dir < best))) {
			best, bestRank = dir, rank
		}
	}
	if bestRank == 4 {
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

func parent(d string) string {
	if p := path.Dir(d); p != "." {
		return p
	}
	return ""
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
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
