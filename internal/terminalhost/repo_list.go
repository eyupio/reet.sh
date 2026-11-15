package terminalhost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	// repoListTimeout bounds one RepoList on the machine: the sign-in check
	// and both pages.
	repoListTimeout = 30 * time.Second
	// repoListPage is GitHub's largest page; two pages make
	// terminalproto.MaxRepoListEntries.
	repoListPage = 100
	// ghListOutputLimit bounds what one listing page may print. A page of
	// 100 trimmed entries is about 16 KiB; output this long was cut short.
	ghListOutputLimit = 1 << 20
)

// errGHOutputTooLong means gh printed more than ghListOutputLimit.
var errGHOutputTooLong = errors.New("gh output too long")

// ExecGHList is ExecGH for a listing: the same lookup, environment and
// timeout, but only stdout is kept, up to ghListOutputLimit, so a warning gh
// prints on stderr never mixes into the JSON. Tests fake it.
func ExecGHList(ctx context.Context, args ...string) ([]byte, error) {
	path, err := exec.LookPath("gh")
	if err != nil || !filepath.IsAbs(path) {
		return nil, errGHMissing
	}
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = gitEnvironment(os.Environ())
	out := &truncatingBuffer{limit: ghListOutputLimit}
	cmd.Stdout, cmd.Stderr = out, &truncatingBuffer{limit: gitOutputLimit}
	killGroupOnCancel(cmd)
	cmd.WaitDelay = 2 * time.Second
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if len(out.Bytes()) >= ghListOutputLimit {
		return nil, errGHOutputTooLong
	}
	return out.Bytes(), nil
}

// repoListJQ trims each repository to the three fields the list carries, on
// the machine, before anything is read: a full page of GitHub's answer is
// hundreds of KiB.
const repoListJQ = `[.[] | {name: .full_name, private: .private, pushedAt: .pushed_at}]`

// ListRepos lists the GitHub repositories the machine's gh sign-in can reach:
// its own, its organizations' and those shared with it, most recently pushed
// first, at most terminalproto.MaxRepoListEntries. check runs the sign-in
// check (ExecGH); list runs each page (ExecGHList).
func ListRepos(ctx context.Context, check, list GHRunner) terminalproto.RepoListResult {
	refuse := func(reason terminalproto.RepoListReason) terminalproto.RepoListResult {
		return terminalproto.RepoListResult{Reason: reason}
	}
	if check == nil || list == nil {
		return refuse(terminalproto.RepoListFailed)
	}
	ctx, cancel := context.WithTimeout(ctx, repoListTimeout)
	defer cancel()
	if _, err := check(ctx, "auth", "status", "--hostname", "github.com"); err != nil {
		switch {
		case errors.Is(err, errGHMissing):
			return refuse(terminalproto.RepoListDenied)
		case ctx.Err() != nil:
			return refuse(terminalproto.RepoListTimedOut)
		}
		return refuse(terminalproto.RepoListNotSignedIn)
	}
	var result terminalproto.RepoListResult
	seen := map[string]bool{}
	for page := 1; page*repoListPage <= terminalproto.MaxRepoListEntries; page++ {
		out, err := list(ctx, "api", "--hostname", "github.com", "--method", "GET", "user/repos",
			"-f", "affiliation=owner,collaborator,organization_member", "-f", "sort=pushed", "-f", "direction=desc",
			"-f", "per_page="+strconv.Itoa(repoListPage), "-f", "page="+strconv.Itoa(page), "--jq", repoListJQ)
		if err != nil {
			if ctx.Err() != nil {
				return refuse(terminalproto.RepoListTimedOut)
			}
			return refuse(terminalproto.RepoListFailed)
		}
		var entries []struct {
			Name     string  `json:"name"`
			Private  bool    `json:"private"`
			PushedAt *string `json:"pushedAt"`
		}
		if json.Unmarshal(out, &entries) != nil || len(entries) > repoListPage {
			return refuse(terminalproto.RepoListFailed)
		}
		for _, e := range entries {
			key := strings.ToLower(e.Name)
			// A name GitHub would not give, or one already listed (a page
			// boundary shifts when a repository is pushed to meanwhile), is
			// left out rather than failing the list.
			owner, repo, ok := strings.Cut(e.Name, "/")
			if !ok || !terminalproto.ValidGitHubOwner(owner) || !terminalproto.ValidGitHubRepo(repo) || seen[key] {
				continue
			}
			seen[key] = true
			entry := terminalproto.RepoListEntry{Name: e.Name, Private: e.Private}
			if e.PushedAt != nil {
				if at, err := time.Parse(time.RFC3339, *e.PushedAt); err == nil && at.Unix() > 0 {
					entry.PushedAt = at.Unix()
				}
			}
			result.Repos = append(result.Repos, entry)
		}
		if len(entries) < repoListPage {
			return result
		}
	}
	// The last page was full, so GitHub may have more than were asked for.
	result.More = true
	return result
}
