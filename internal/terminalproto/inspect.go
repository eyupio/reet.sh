package terminalproto

import (
	"encoding/json"
	"path"
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
	valid := validTime && validCounts && validBranch
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
		valid := validNumbers && validText && len(p.Checks) <= 40
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
