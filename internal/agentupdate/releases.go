// Package agentupdate resolves the published reet channels so the controller
// can show enrolled agents whether their binary is current and queue updates.
package agentupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultReleasesAPI is the GitHub releases endpoint used for stable tags.
	DefaultReleasesAPI = "https://api.github.com/repos/eyupio/reet.sh/releases"
	// DefaultDownloads is the public asset root used for the dev channel.
	DefaultDownloads = "https://github.com/eyupio/reet.sh/releases/download"
	defaultTTL       = 10 * time.Minute
)

var (
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	tagPattern    = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

// Channel is the newest build published for one release channel.
type Channel struct {
	Version   string `json:"version,omitempty"`
	Commit    string `json:"commit,omitempty"`
	CheckedAt int64  `json:"checkedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Info reports the latest stable release and the latest main build.
type Info struct {
	Dev    Channel `json:"dev"`
	Latest Channel `json:"latest"`
}

// Checker caches published channel metadata for the whole controller process.
// Concurrent callers share one refresh, and a stale cache is refreshed in the
// background so a slow GitHub request never blocks an agent poll.
type Checker struct {
	Client        *http.Client
	ReleasesAPI   string
	DownloadsRoot string
	TTL           time.Duration

	mu         sync.Mutex
	info       Info
	checkedAt  time.Time
	refreshing bool
	now        func() time.Time
}

// NewChecker returns a checker pointed at the public reet release channels.
func NewChecker() *Checker {
	return &Checker{
		Client:        &http.Client{Timeout: 4 * time.Second},
		ReleasesAPI:   DefaultReleasesAPI,
		DownloadsRoot: DefaultDownloads,
		TTL:           defaultTTL,
		now:           time.Now,
	}
}

// Info returns the cached channel metadata. The first call is synchronous; later
// stale calls return the cache immediately while refreshing in the background.
func (c *Checker) Info(ctx context.Context) Info {
	c.mu.Lock()
	cached := c.info
	fresh := !c.checkedAt.IsZero() && c.clock().Sub(c.checkedAt) < c.ttl()
	if fresh || c.refreshing {
		c.mu.Unlock()
		return cached
	}
	first := c.checkedAt.IsZero()
	c.refreshing = true
	c.mu.Unlock()
	if first {
		return c.load(ctx)
	}
	go c.load(context.Background())
	return cached
}

func (c *Checker) load(ctx context.Context) Info {
	defer func() {
		c.mu.Lock()
		c.refreshing = false
		c.mu.Unlock()
	}()
	checkedAt := c.clock().UnixMilli()
	var info Info
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); info.Dev = c.fetchDev(ctx) }()
	go func() { defer wg.Done(); info.Latest = c.fetchLatest(ctx) }()
	wg.Wait()
	info.Dev.CheckedAt = checkedAt
	info.Latest.CheckedAt = checkedAt
	c.mu.Lock()
	c.info = info
	c.checkedAt = c.clock()
	c.mu.Unlock()
	return info
}

func (c *Checker) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Checker) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return defaultTTL
}

func (c *Checker) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: 8 * time.Second}
}

func (c *Checker) root() string {
	if c.DownloadsRoot != "" {
		return strings.TrimRight(c.DownloadsRoot, "/")
	}
	return DefaultDownloads
}

func (c *Checker) api() string {
	if c.ReleasesAPI != "" {
		return strings.TrimRight(c.ReleasesAPI, "/")
	}
	return DefaultReleasesAPI
}

func (c *Checker) fetchDev(ctx context.Context) Channel {
	b, err := get(ctx, c.client(), c.root()+"/dev/dev-build.txt", 128)
	if err != nil {
		return Channel{Version: "dev", Error: err.Error()}
	}
	commit := strings.ToLower(strings.TrimSpace(string(b)))
	if !commitPattern.MatchString(commit) {
		return Channel{Version: "dev", Error: "published dev build pointer is invalid"}
	}
	return Channel{Version: "dev", Commit: commit}
}

func (c *Checker) fetchLatest(ctx context.Context) Channel {
	b, err := get(ctx, c.client(), c.api()+"/latest", 1<<20)
	if err != nil {
		return Channel{Error: err.Error()}
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if json.Unmarshal(b, &release) != nil {
		return Channel{Error: "invalid release response"}
	}
	tag := strings.TrimSpace(release.TagName)
	if !tagPattern.MatchString(tag) {
		return Channel{Error: "invalid published release tag"}
	}
	return Channel{Version: tag}
}

func get(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("release check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release check returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("release response exceeds size limit")
	}
	return b, nil
}

// UpdateTarget reports which channel an agent should move to. The bool is false
// when the agent is current, on an unknown channel, or the channel is unknown.
func UpdateTarget(version, commit string, info Info) (string, bool) {
	version = strings.TrimSpace(version)
	if strings.EqualFold(version, "dev") {
		if info.Dev.Commit == "" {
			return "", false
		}
		if strings.TrimSpace(commit) == "" || !strings.EqualFold(strings.TrimSpace(commit), info.Dev.Commit) {
			return "dev", true
		}
		return "", false
	}
	if version == "" || info.Latest.Version == "" {
		return "", false
	}
	if VersionLess(version, info.Latest.Version) {
		return "latest", true
	}
	return "", false
}

// VersionLess reports whether semantic version a is older than b. Unparseable
// versions are never treated as older, so a local build cannot be downgraded.
func VersionLess(a, b string) bool {
	x, ok := parseVersion(a)
	if !ok {
		return false
	}
	y, ok := parseVersion(b)
	if !ok {
		return false
	}
	if x.major != y.major {
		return x.major < y.major
	}
	if x.minor != y.minor {
		return x.minor < y.minor
	}
	if x.patch != y.patch {
		return x.patch < y.patch
	}
	return comparePrerelease(x.pre, y.pre) < 0
}

type version struct {
	major, minor, patch int
	pre                 string
}

func parseVersion(raw string) (version, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	raw = strings.TrimPrefix(raw, "V")
	if i := strings.IndexByte(raw, '+'); i >= 0 {
		raw = raw[:i]
	}
	pre := ""
	if i := strings.IndexByte(raw, '-'); i >= 0 {
		pre = raw[i+1:]
		raw = raw[:i]
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return version{}, false
	}
	values := [3]int{}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version{}, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version{}, false
		}
		values[i] = n
	}
	return version{major: values[0], minor: values[1], patch: values[2], pre: pre}, true
}

func comparePrerelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, xerr := strconv.Atoi(as[i])
		y, yerr := strconv.Atoi(bs[i])
		switch {
		case xerr == nil && yerr == nil:
			if x != y {
				if x < y {
					return -1
				}
				return 1
			}
		case xerr == nil:
			return -1
		case yerr == nil:
			return 1
		default:
			if as[i] != bs[i] {
				if as[i] < bs[i] {
					return -1
				}
				return 1
			}
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}
