package terminalhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// InspectSession never writes, fetches git objects, runs a shell, or accepts a command.
// Workspace roots stay inside the enrolled user's home; file opens stay inside that root.
func InspectSession(ctx context.Context, home string, req terminalproto.Inspect) terminalproto.InspectResult {
	if req.Validate() != nil {
		return terminalproto.InspectResult{Reason: "denied"}
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	dir, err := ResolveDir(home, req.Dir)
	if err != nil {
		return terminalproto.InspectResult{Reason: "unavailable"}
	}
	r := terminalproto.InspectResult{CheckedAt: time.Now().UnixMilli()}
	if req.View == "files" || req.View == "file" {
		return inspectFiles(dir, req, r)
	}
	meta, common, err := inspectGitDirs(home, dir)
	if err != nil {
		r.Reason = "not_repo"
		return r
	}
	// Refuse partial clones even on git versions that ignore GIT_NO_LAZY_FETCH.
	if inspectPromisor(common) {
		r.Reason = "git_failed"
		return r
	}
	gitPath, err := exec.LookPath("git")
	if err != nil || !filepath.IsAbs(gitPath) {
		r.Reason = "git_failed"
		return r
	}

	var filterArgs []string
	git := func(args ...string) ([]byte, error) {
		fixed := []string{"--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "diff.external=", "-c", "core.attributesFile=/dev/null"}
		fixed = append(fixed, filterArgs...)
		cmd := exec.CommandContext(ctx, gitPath, append([]string{"-C", dir}, append(fixed, args...)...)...)
		cmd.Env = append(gitEnvironment(os.Environ()), "GIT_DIR="+meta, "GIT_WORK_TREE="+dir, "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir), "GIT_NO_LAZY_FETCH=1", "GIT_OPTIONAL_LOCKS=0")
		cmd.WaitDelay = 2 * time.Second
		killGroupOnCancel(cmd)
		out := &truncatingBuffer{limit: 128 << 10}
		cmd.Stdout, cmd.Stderr = out, out
		err := cmd.Run()
		return out.Bytes(), err
	}
	if _, err := git("rev-parse", "--show-toplevel"); err != nil {
		r.Reason = "not_repo"
		return r
	}
	r.Repo = true
	// Status and diff may apply repository clean/process filters while comparing the
	// working copy. Disable every configured driver before either operation.
	keys, configErr := git("config", "--name-only", "--get-regexp", `^filter\..*\.(clean|process|required)$`)
	if len(keys) >= 128<<10 {
		r.Reason = "git_failed"
		return r
	}
	if configErr != nil && len(keys) > 0 {
		r.Reason = "git_failed"
		return r
	}
	for _, key := range strings.Fields(string(keys)) {
		if strings.HasPrefix(key, "filter.") {
			value := ""
			if strings.HasSuffix(key, ".required") {
				value = "false"
			}
			filterArgs = append(filterArgs, "-c", key+"="+value)
		}
	}
	branch, _ := git("symbolic-ref", "--quiet", "--short", "HEAD")
	r.Branch = inspectPlain(strings.TrimSpace(string(branch)), 1024)
	if req.View == "github" {
		return inspectGitHub(ctx, dir, git, r)
	}
	// Status and numstat both detect renames, regardless of checkout preferences.
	// Listing individual untracked files keeps new directories selectable in Changes.
	out, err := git("status", "--porcelain=v1", "-z", "--renames", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		r.Reason = "git_failed"
		return r
	}
	renameSources := inspectRenameSources(out)
	if req.View == "diff" {
		args := []string{"diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--ignore-submodules=all", "HEAD", "--", req.Path}
		// Restricting a rename to its destination makes Git present a new file. Include
		// its recorded source too, so the preview has the same baseline as the counts.
		if source := renameSources[req.Path]; source != "" {
			args = append(args, source)
		}
		out, err := git(args...)
		if err != nil {
			r.Reason = "git_failed"
			return r
		}
		r.Text, r.More = inspectText(out, 6500)
		fitInspect(&r)
		return r
	}
	r.More = len(out) >= 128<<10
	r.ChangedMore = r.More
	if r.More {
		if end := strings.LastIndexByte(string(out), 0); end >= 0 {
			out = out[:end+1]
		} else {
			out = nil
		}
	}
	parts := strings.Split(string(out), "\x00")
	for i := 0; i < len(parts); i++ {
		line := parts[i]
		if len(line) < 4 {
			continue
		}
		status, p := line[:2], line[3:]
		if strings.ContainsAny(status, "RC") {
			i++
		} // -z emits the original path separately for renames/copies.
		r.Changed++
		conflict := strings.Contains(status, "U") || status == "AA" || status == "DD"
		if conflict {
			r.Conflicts++
		}
		if !terminalproto.ValidInspectPath(p) {
			r.More = true
			continue
		}
		if len(r.Files) < 60 {
			r.Files = append(r.Files, terminalproto.InspectFile{Path: p, Status: status})
		} else {
			r.More = true
			if conflict {
				for j := len(r.Files) - 1; j >= 0; j-- {
					if !inspectConflict(r.Files[j].Status) {
						r.Files[j] = terminalproto.InspectFile{Path: p, Status: status}
						break
					}
				}
			}
		}
	}
	sort.SliceStable(r.Files, func(i, j int) bool {
		a, b := inspectConflict(r.Files[i].Status), inspectConflict(r.Files[j].Status)
		if a != b {
			return a
		}
		return r.Files[i].Path < r.Files[j].Path
	})
	// Numstat against HEAD includes staged and unstaged changes together. An unborn HEAD
	// still has the status listing; it has no baseline for line counts.
	stats, statsErr := git("diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--numstat", "-z", "--ignore-submodules=all", "HEAD", "--")
	counts := map[string]terminalproto.InspectFile{}
	r.CountsKnown = statsErr == nil && len(stats) < 128<<10
	if r.CountsKnown {
		records := strings.Split(string(stats), "\x00")
		for i := 0; i < len(records); i++ {
			p := strings.SplitN(records[i], "\t", 3)
			if len(p) != 3 {
				continue
			}
			name := p[2]
			// Rename numstat records have an empty path followed by old and new
			// NUL-terminated names. Attribute the edit counts to the destination.
			if name == "" {
				if i+2 >= len(records)-1 {
					r.CountsKnown = false
					break
				}
				name = records[i+2]
				i += 2
			}
			a, _ := strconv.Atoi(p[0])
			d, _ := strconv.Atoi(p[1])
			counts[name] = terminalproto.InspectFile{Added: a, Removed: d, Binary: p[0] == "-"}
		}
	}
	for i := range r.Files {
		if n, ok := counts[r.Files[i].Path]; ok {
			r.Files[i].Added = n.Added
			r.Files[i].Removed = n.Removed
			r.Files[i].Binary = n.Binary
			// A rename edited beyond Git's similarity threshold appears as a deletion
			// and addition in the HEAD diff; retain both sides in its displayed totals.
			if old, exists := counts[renameSources[r.Files[i].Path]]; exists {
				r.Files[i].Added += old.Added
				r.Files[i].Removed += old.Removed
				r.Files[i].Binary = r.Files[i].Binary || old.Binary
			}
		}
	}
	upstream, err := git("rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err == nil {
		fields := strings.Fields(string(upstream))
		if len(fields) == 2 {
			r.Upstream = true
			r.Ahead, _ = strconv.Atoi(fields[0])
			r.Behind, _ = strconv.Atoi(fields[1])
		}
	}
	fitInspect(&r)
	return r
}

func inspectFiles(dir string, req terminalproto.Inspect, r terminalproto.InspectResult) terminalproto.InspectResult {
	if !terminalproto.ValidInspectPath(req.Path) {
		r.Reason = "denied"
		return r
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		r.Reason = "unreadable"
		return r
	}
	defer root.Close()
	p := req.Path
	if p == "" {
		p = "."
	}
	// Do not follow links, including links to another workspace folder or metadata.
	walk := ""
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if part == "." {
			continue
		}
		walk = filepath.Join(walk, part)
		info, err := root.Lstat(walk)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			r.Reason = "unreadable"
			return r
		}
	}
	if req.View == "files" {
		folder, err := root.Open(p)
		if err != nil {
			r.Reason = "unreadable"
			return r
		}
		defer folder.Close()
		entries, err := folder.ReadDir(301)
		if err != nil && err != io.EOF {
			r.Reason = "unreadable"
			return r
		}
		r.More = len(entries) > 300
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return entries[i].Name() < entries[j].Name()
		})
		for _, e := range entries {
			path := filepath.ToSlash(filepath.Join(p, e.Name()))
			if e.Type()&os.ModeSymlink != 0 || !terminalproto.ValidInspectPath(path) {
				continue
			}
			if len(r.Files) == 100 {
				r.More = true
				break
			}
			r.Files = append(r.Files, terminalproto.InspectFile{Path: path, Directory: e.IsDir()})
		}
	} else {
		info, err := root.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			r.Reason = "unreadable"
			return r
		}
		f, err := openNoBlockIn(root, p)
		if err != nil {
			r.Reason = "unreadable"
			return r
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil || !opened.Mode().IsRegular() {
			r.Reason = "unreadable"
			return r
		}
		data, err := io.ReadAll(io.LimitReader(f, 6501))
		if err != nil {
			r.Reason = "unreadable"
			return r
		}
		if len(data) > 6500 {
			n := 6500
			for n > 0 && !utf8.RuneStart(data[n]) {
				n--
			}
			data = data[:n]
			r.More = true
		}
		if strings.ContainsRune(string(data), 0) || !utf8.Valid(data) {
			r.Binary = true
			return r
		}
		r.Text = string(data)
	}
	fitInspect(&r)
	return r
}

// fitInspect leaves room for the private protocol envelope, including JSON escaping.
func fitInspect(r *terminalproto.InspectResult) {
	for {
		b, _ := json.Marshal(r)
		if len(b) <= terminalproto.MaxInspectBytes {
			return
		}
		r.More = true
		if len(r.Files) > 0 {
			r.Files = r.Files[:len(r.Files)-1]
			continue
		}
		if len(r.Text) > 0 {
			r.Text, _ = inspectText([]byte(r.Text), len(r.Text)/2)
			continue
		}
		if r.Pull != nil && len(r.Pull.Checks) > 0 {
			r.Pull.More = true
			r.Pull.Checks = r.Pull.Checks[:len(r.Pull.Checks)-1]
			continue
		}
		r.Pull = nil
		r.Reason = "unreadable"
		return
	}
}
func inspectText(b []byte, n int) (string, bool) {
	more := len(b) > n
	if more {
		b = b[:n]
		for len(b) > 0 && !utf8.Valid(b) {
			b = b[:len(b)-1]
		}
	}
	return strings.ToValidUTF8(string(b), "�"), more
}
func inspectPlain(s string, n int) string {
	var out strings.Builder
	for _, r := range s {
		if terminalproto.ValidPlainText(string(r), 1) {
			out.WriteRune(r)
		}
		if out.Len() >= n {
			break
		}
	}
	return out.String()
}

// Only an explicit github.com origin is eligible. Remote helpers and URLs carrying
// credentials are never passed to gh. gh uses credentials configured on the machine.
func inspectGitHub(ctx context.Context, dir string, git func(...string) ([]byte, error), r terminalproto.InspectResult) terminalproto.InspectResult {
	remote, err := git("config", "--get", "remote.origin.url")
	if err != nil {
		r.Reason = "github_unavailable"
		return r
	}
	repo := inspectGitHubRepo(strings.TrimSpace(string(remote)))
	if repo == "" || r.Branch == "" || strings.HasPrefix(r.Branch, "-") {
		r.Reason = "github_unavailable"
		return r
	}
	gh, err := exec.LookPath("gh")
	if err != nil || !filepath.IsAbs(gh) {
		r.Reason = "github_unavailable"
		return r
	}
	cmd := exec.CommandContext(ctx, gh, "pr", "view", r.Branch, "--repo", repo, "--json", "number,title,url,state,isDraft,reviewDecision,mergeable,additions,deletions,statusCheckRollup")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_HOST=github.com", "NO_COLOR=1")
	cmd.WaitDelay = 2 * time.Second
	killGroupOnCancel(cmd)
	out := &truncatingBuffer{limit: 128 << 10}
	errOut := &truncatingBuffer{limit: 2048}
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		r.Reason = "github_unavailable"
		if strings.Contains(strings.ToLower(string(errOut.Bytes())), "no pull requests found") {
			r.Reason = "no_pr"
		}
		return r
	}
	var pr struct {
		Number                                       int
		Title, URL, State, ReviewDecision, Mergeable string
		IsDraft                                      bool
		Additions, Deletions                         int
		StatusCheckRollup                            []struct{ Name, Context, Status, Conclusion, State, DetailsURL, TargetURL string }
	}
	if json.Unmarshal(out.Bytes(), &pr) != nil {
		r.Reason = "github_unavailable"
		return r
	}
	r.Pull = &terminalproto.InspectPull{Number: pr.Number, Title: inspectPlain(pr.Title, 300), URL: pr.URL, State: pr.State, Draft: pr.IsDraft, Review: pr.ReviewDecision, Mergeable: pr.Mergeable, Added: pr.Additions, Removed: pr.Deletions}
	for _, c := range pr.StatusCheckRollup {
		state := c.Conclusion
		if state == "" {
			state = c.State
		}
		if state == "" {
			state = c.Status
		}
		name := c.Name
		if name == "" {
			name = c.Context
		}
		link := c.DetailsURL
		if link == "" {
			link = c.TargetURL
		}
		r.Pull.Checks = append(r.Pull.Checks, terminalproto.InspectCheck{Name: inspectPlain(name, 180), State: state, URL: link})
	}
	// Failures first, then active, then passing; truncation never hides a failure behind passes.
	sort.SliceStable(r.Pull.Checks, func(i, j int) bool {
		return inspectCheckRank(r.Pull.Checks[i].State) < inspectCheckRank(r.Pull.Checks[j].State)
	})
	if len(r.Pull.Checks) > 40 {
		r.Pull.More = true
		r.Pull.Checks = r.Pull.Checks[:40]
	}
	fitInspect(&r)
	return r
}
func inspectCheckRank(s string) int {
	switch s {
	case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return 0
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return 2
	}
	return 1
}
func inspectGitHubRepo(remote string) string {
	var p string
	if strings.HasPrefix(remote, "git@github.com:") {
		p = strings.TrimPrefix(remote, "git@github.com:")
	} else {
		u, err := url.Parse(remote)
		if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return ""
		}
		p = strings.TrimPrefix(u.Path, "/")
	}
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(p, "/")
	if len(parts) != 2 {
		return ""
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ""
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
				return ""
			}
		}
	}
	return "github.com/" + p
}

// Resolve ordinary checkouts and linked worktrees, keeping metadata within home.
func inspectGitDirs(home, dir string) (string, string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", "", err
	}
	defer root.Close()
	info, err := root.Lstat(".git")
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", "", errors.New("no git metadata")
	}
	meta := filepath.Join(dir, ".git")
	if info.Mode().IsRegular() {
		data, err := readRegularIn(root, ".git")
		if err != nil {
			return "", "", err
		}
		value, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
		if !ok {
			return "", "", errors.New("invalid git file")
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(dir, value)
		}
		meta, err = ResolveDir(home, value)
		if err != nil {
			return "", "", err
		}
	} else if !info.IsDir() {
		return "", "", errors.New("invalid git metadata")
	}
	common := meta
	if data, err := readRegularPath(filepath.Join(meta, "commondir")); err == nil {
		value := strings.TrimSpace(string(data))
		if !filepath.IsAbs(value) {
			value = filepath.Join(meta, value)
		}
		common, err = ResolveDir(home, value)
		if err != nil {
			return "", "", err
		}
	} else if !os.IsNotExist(err) {
		return "", "", err
	}
	return meta, common, nil
}
func inspectPromisor(meta string) bool {
	root, err := os.OpenRoot(meta)
	if err != nil {
		return true
	}
	defer root.Close()
	if promisorConfig(func() ([]byte, error) { return readRegularIn(root, "config") }, true) || promisorPacks(root) {
		return true
	}
	// Include the user's and system config, matching the existing branch reader.
	for _, p := range []string{filepath.Join(os.Getenv("HOME"), ".gitconfig"), filepath.Join(os.Getenv("HOME"), ".config/git/config"), "/etc/gitconfig"} {
		if promisorConfig(func() ([]byte, error) { return readRegularPath(p) }, true) {
			return true
		}
	}
	return false
}

func inspectConflict(s string) bool { return strings.Contains(s, "U") || s == "AA" || s == "DD" }

// Porcelain v1 -z emits a rename's destination followed by its original path.
func inspectRenameSources(data []byte) map[string]string {
	sources := make(map[string]string)
	records := strings.Split(string(data), "\x00")
	for i := 0; i < len(records)-1; i++ {
		line := records[i]
		if len(line) < 4 || !strings.ContainsAny(line[:2], "RC") {
			continue
		}
		if i+1 < len(records)-1 && terminalproto.ValidInspectPath(line[3:]) && terminalproto.ValidInspectPath(records[i+1]) {
			sources[line[3:]] = records[i+1]
		}
		i++
	}
	return sources
}
