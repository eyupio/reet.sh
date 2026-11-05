package terminalproto

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// InspectFeature provides bounded, read-only session workspace views.
const InspectFeature = "session-inspect-v1"
const MaxInspectBytes = 12 << 10

func SupportsInspect(features string) bool {
	for _, f := range strings.Split(features, ",") {
		if strings.TrimSpace(f) == InspectFeature {
			return true
		}
	}
	return false
}

// Inspect's Dir is supplied from the controller's owned session record, never a browser path.
type Inspect struct {
	Dir  string `json:"dir"`
	View string `json:"view"`
	Path string `json:"path,omitempty"`
}

func (Inspect) kind() string { return "inspect" }
func (r Inspect) Validate() error {
	if !r.valid() {
		return ErrInvalid
	}
	return nil
}
func ValidInspectPath(p string) bool {
	return p == "" || (plainText(p, 1024) && !strings.Contains(p, "\\") && !strings.Contains("/"+p+"/", "/.git/") && !strings.HasPrefix(p, "/") && p != ".." && !strings.HasPrefix(p, "../") && path.Clean(p) == p && p != ".git" && !strings.HasPrefix(p, ".git/"))
}
func (r Inspect) valid() bool {
	validDirName := r.Dir != "" && validDir(r.Dir)
	validPath := ValidInspectPath(r.Path)
	if !validDirName || !validPath {
		return false
	}
	switch r.View {
	case "status", "github":
		return r.Path == ""
	case "files":
		return true
	case "file", "diff":
		return r.Path != "" && r.Path != "."
	}
	return false
}

type InspectFile struct {
	Path      string `json:"path"`
	Status    string `json:"status,omitempty"`
	Added     int    `json:"added"`
	Removed   int    `json:"removed"`
	Binary    bool   `json:"binary,omitempty"`
	Directory bool   `json:"directory,omitempty"`
}
type InspectCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
	URL   string `json:"url,omitempty"`
}
type InspectPull struct {
	Number    int            `json:"number"`
	Title     string         `json:"title"`
	URL       string         `json:"url"`
	State     string         `json:"state"`
	Draft     bool           `json:"draft"`
	Review    string         `json:"review"`
	Mergeable string         `json:"mergeable"`
	Added     int            `json:"added"`
	Removed   int            `json:"removed"`
	Checks    []InspectCheck `json:"checks,omitempty"`
	More      bool           `json:"more,omitempty"`

	// Everything below is additive and optional, so InspectFeature does not
	// change and an older machine's payload still decodes.

	// Base and Head are the branch names the pull request merges into and from.
	Base string `json:"base,omitempty"`
	Head string `json:"head,omitempty"`
	// Times are Unix milliseconds. Zero means absent: not yet merged, not
	// closed, or not read.
	CreatedAt int64 `json:"createdAt,omitempty"`
	MergedAt  int64 `json:"mergedAt,omitempty"`
	ClosedAt  int64 `json:"closedAt,omitempty"`
	// LastPushAt, AutoMerge, Reviews and Threads come from a second read that
	// can fail on its own, so each can be absent (zero or nil) even when
	// DetailKnown is true. Absent means "not read", never "zero".
	LastPushAt int64           `json:"lastPushAt,omitempty"`
	AutoMerge  bool            `json:"autoMerge,omitempty"`
	Reviews    *InspectReviews `json:"reviews,omitempty"`
	Threads    *InspectThreads `json:"threads,omitempty"`
	// DetailKnown means the machine read the extended pull request fields. It
	// is how the page tells a machine that has not updated (false) from a pull
	// request that simply has none of them.
	DetailKnown bool `json:"detailKnown,omitempty"`
}

// InspectReviews counts reviewers by their latest review state.
type InspectReviews struct {
	Approved         int `json:"approved"`
	ChangesRequested int `json:"changesRequested"`
	Commented        int `json:"commented"`
	Requested        int `json:"requested"`
}

// InspectThreads counts review threads. More means the machine stopped
// counting at its page limit.
type InspectThreads struct {
	Open     int  `json:"open"`
	Resolved int  `json:"resolved"`
	More     bool `json:"more,omitempty"`
}

// maxInspectCount bounds every review and thread count; the machine reads one
// bounded page, so anything larger is a corrupt frame.
const maxInspectCount = 10000

var inspectRemote = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func validInspectCount(n int) bool { return n >= 0 && n <= maxInspectCount }

// validRef bounds a branch name by bytes as well as runes: the frame limit is
// in bytes.
func validRef(s string) bool { return len(s) <= 255 && plainText(s, 255) }

func (p *InspectPull) validDetail() bool {
	times := validAttentionTime(p.CreatedAt) && validAttentionTime(p.MergedAt) &&
		validAttentionTime(p.ClosedAt) && validAttentionTime(p.LastPushAt)
	if !times || !validRef(p.Base) || !validRef(p.Head) {
		return false
	}
	if rv := p.Reviews; rv != nil {
		counts := validInspectCount(rv.Approved) && validInspectCount(rv.ChangesRequested) &&
			validInspectCount(rv.Commented) && validInspectCount(rv.Requested)
		if !counts {
			return false
		}
	}
	if th := p.Threads; th != nil {
		return validInspectCount(th.Open) && validInspectCount(th.Resolved)
	}
	return true
}

type InspectResult struct {
	Reason      string        `json:"reason,omitempty"`
	CheckedAt   int64         `json:"checkedAt,omitempty"`
	Branch      string        `json:"branch,omitempty"`
	Repo        bool          `json:"repo,omitempty"`
	Ahead       int           `json:"ahead"`
	Behind      int           `json:"behind"`
	Upstream    bool          `json:"upstream,omitempty"`
	CountsKnown bool          `json:"countsKnown"`
	ChangedMore bool          `json:"changedMore,omitempty"`
	Changed     int           `json:"changed"`
	Conflicts   int           `json:"conflicts"`
	Files       []InspectFile `json:"files,omitempty"`
	More        bool          `json:"more,omitempty"`
	Text        string        `json:"text,omitempty"`
	Binary      bool          `json:"binary,omitempty"`
	Pull        *InspectPull  `json:"pull,omitempty"`
	// Remote is the repository as "owner/name", for the page's link; absent
	// when the machine could not tell.
	Remote string `json:"remote,omitempty"`
}

func (InspectResult) kind() string { return "inspect_result" }
func (r InspectResult) valid() bool {
	switch r.Reason {
	case "", "denied", "unavailable", "busy", "timed_out", "not_repo", "git_failed", "no_pr", "github_unavailable", "unreadable", "binary":
	default:
		return false
	}
	validTime := validAttentionTime(r.CheckedAt) && utf8.ValidString(r.Text)
	validCounts := len(r.Files) <= 100 && r.Changed >= 0 && r.Conflicts >= 0 && r.Ahead >= 0 && r.Behind >= 0
	validBranch := plainText(r.Branch, 1024)
	validRemote := r.Remote == "" || (len(r.Remote) <= 140 && inspectRemote.MatchString(r.Remote))
	valid := validTime && validCounts && validBranch && validRemote
	if !valid {
		return false
	}
	for _, f := range r.Files {
		validPath := ValidInspectPath(f.Path) && f.Path != ""
		validText := plainText(f.Status, 2)
		validCounts := f.Added >= 0 && f.Removed >= 0
		valid := validPath && validText && validCounts
		if !valid {
			return false
		}
	}
	if p := r.Pull; p != nil {
		validNumbers := p.Number > 0 && p.Added >= 0 && p.Removed >= 0
		validText := plainText(p.Title, 400) && plainText(p.URL, 2048) && plainText(p.State, 40) &&
			plainText(p.Review, 80) && plainText(p.Mergeable, 40)
		valid := validNumbers && validText && len(p.Checks) <= 40 && p.validDetail()
		if !valid {
			return false
		}
		for _, c := range p.Checks {
			validText := plainText(c.Name, 256) && plainText(c.State, 80) && plainText(c.URL, 2048)
			if !validText {
				return false
			}
		}
	}
	b, err := json.Marshal(r)
	return err == nil && len(b) <= MaxInspectBytes
}
