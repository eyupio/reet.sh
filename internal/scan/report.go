package scan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

// ReportOptions is one inventory scan. Interactive allows the CLI to prompt for
// a missing controller URL or token; the terminal interface always supplies them
// and never prompts.
type ReportOptions struct {
	Server      string
	Token       string
	TokenFile   string
	Host        string
	Dir         string
	Dirs        []string
	Prune       bool
	Reconcile   bool
	ServerHost  bool
	DryRun      bool
	Version     string
	Interactive bool
	In          io.Reader
	Out         io.Writer

	// OnScan runs after collection and before any network request.
	OnScan func(Scan)
	// OnReconciled runs after a successful hostname merge, before the inventory
	// is sent.
	OnReconciled func(from, to string)
}

// Report is what a scan found and what the controller did with it. The text
// command and the terminal interface render the same value.
type Report struct {
	Host           string
	Found          []Found
	Envs           []ScannedEnv
	DryRun         bool
	Sent           bool
	ReconciledFrom string
	Imported       int
	Created        int
	Ignored        int
	Pruned         int
	User           string
	Errors         []string
}

// RunReport collects installed skills and, unless this is a dry run, reports
// them to the controller. It returns the report even when the controller replies
// with a per-skill error, so callers can show what was accepted.
func RunReport(opts ReportOptions) (Report, error) {
	host := strings.TrimSpace(opts.Host)
	if host == "" {
		host, _ = os.Hostname()
	}
	if host == "" {
		host = "unknown"
	}
	r := Report{Host: host, DryRun: opts.DryRun}

	input := opts.In
	if input == nil {
		input = os.Stdin
	}
	if !opts.Interactive {
		input = strings.NewReader("")
	}
	output := opts.Out
	if output == nil {
		output = os.Stderr
	}
	server := opts.Server
	if !opts.DryRun && server == "" {
		if !opts.Interactive {
			return r, errors.New("server URL required; set REET_URL or pass --server")
		}
		value, err := clientconfig.ResolveURL("", input, output)
		if err != nil {
			return r, err
		}
		server = value
	}
	token := opts.Token
	if !opts.DryRun {
		tokenInput := input
		if opts.ServerHost {
			tokenInput = strings.NewReader("")
		}
		value, err := clientconfig.ResolveToken(token, opts.TokenFile, tokenInput, output)
		if err != nil {
			return r, err
		}
		token = value
	}
	home, _ := os.UserHomeDir()
	cwd := opts.Dir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	found := CollectScan(home, cwd, opts.Dirs, host)
	r.Found, r.Envs = found.Found, found.Envs
	if opts.OnScan != nil {
		opts.OnScan(found)
	}
	if opts.DryRun {
		return r, found.Err()
	}
	if opts.Reconcile {
		hostname, err := os.Hostname()
		if err != nil || hostname == "" {
			return r, errors.New("cannot reconcile this machine without a hostname")
		}
		if strings.TrimSpace(opts.Host) == "" {
			return r, errors.New("--reconcile requires --env NAME (or REET_ENV) so the preferred machine name is explicit")
		}
		body, _ := json.Marshal(map[string]string{"from": hostname, "to": host})
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(server, "/")+"/api/v2/machines/merge", bytes.NewReader(body))
		if err != nil {
			return r, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Reet-Token", token)
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return r, err
		}
		if resp.StatusCode != http.StatusOK {
			var result struct {
				Error string `json:"error"`
			}
			_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
			resp.Body.Close()
			return r, fmt.Errorf("machine reconciliation failed (%s): %s", resp.Status, result.Error)
		}
		resp.Body.Close()
		r.ReconciledFrom = hostname
		if opts.OnReconciled != nil {
			opts.OnReconciled(hostname, host)
		}
	}

	type item struct {
		Name    string `json:"name"`
		Source  string `json:"source,omitempty"`
		Host    string `json:"host,omitempty"`
		Agent   string `json:"agent,omitempty"`
		Project string `json:"project,omitempty"`
		Path    string `json:"path,omitempty"`
	}
	items := make([]item, len(found.Found))
	for i, f := range found.Found {
		items[i] = item{f.Name, f.Source, f.Host, f.Agent, f.Project, f.Path}
	}
	body, _ := json.Marshal(map[string]any{"items": items, "environments": found.Envs, "prune": opts.Prune, "machine": map[string]any{"host": host, "os": runtime.GOOS, "arch": runtime.GOARCH, "version": opts.Version, "server": opts.ServerHost}})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(server, "/")+"/api/import", bytes.NewReader(body))
	if err != nil {
		return r, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reet-Source", "scan")
	if token != "" {
		req.Header.Set("X-Reet-Token", token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return r, errors.New("server replied 401 Unauthorized; use a personal API token from Account on the target server via REET_TOKEN, --env-file .env, or the interactive token prompt; delete a stale saved token with reet configure --forget-token")
		}
		return r, fmt.Errorf("server replied %s", resp.Status)
	}
	var res struct {
		Imported int      `json:"imported"`
		Created  int      `json:"created"`
		Ignored  int      `json:"ignored"`
		Pruned   int      `json:"pruned"`
		Errors   []string `json:"errors"`
		User     string   `json:"user"`
	}
	if json.NewDecoder(resp.Body).Decode(&res) != nil {
		return r, errors.New("unreadable reply from server")
	}
	r.Sent = true
	r.Imported, r.Created, r.Ignored, r.Pruned, r.User, r.Errors = res.Imported, res.Created, res.Ignored, res.Pruned, res.User, res.Errors
	if len(res.Errors) > 0 {
		return r, fmt.Errorf("scan reported %d errors; inventory is incomplete", len(res.Errors))
	}
	return r, nil
}

// RenderFindings prints one line per installed skill, as reet scan does.
func RenderFindings(out io.Writer, found []Found) {
	for _, f := range found {
		src := f.Source
		if src == "" {
			src = "(unknown source)"
		}
		place := f.Agent
		if f.Project != "" {
			place += " · " + f.Project
		}
		if place == "" {
			place = "(lock file)"
		}
		fmt.Fprintf(out, "%-40s %-26s %s\n", f.Name, src, place)
	}
}

// RenderResult prints the outcome of a completed or dry-run scan.
func RenderResult(out io.Writer, r Report) {
	if r.DryRun {
		fmt.Fprintf(out, "%d skills found in %d environments (dry run, nothing sent)\n", len(r.Found), len(r.Envs))
		return
	}
	if !r.Sent {
		return
	}
	fmt.Fprintf(out, "%d new, %d ignored, %d pruned on %s\n", r.Created, r.Ignored, r.Pruned, r.Host)
	if r.User != "" {
		fmt.Fprintf(out, "reported as %s\n", r.User)
	}
	fmt.Fprintf(out, "reported %d skills; existing shelf choices preserved\n", r.Imported)
	for _, e := range r.Errors {
		fmt.Fprintf(out, "  skipped: %s\n", e)
	}
}
