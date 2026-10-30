package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

// Diagnostic is one doctor check. Status is pass, warn or fail.
type Diagnostic struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Advice  string `json:"advice,omitempty"`
}

// Report is what reet doctor found. It is shared by the text output and the
// terminal UI so both always describe the same checks.
type Report struct {
	Version string       `json:"version"`
	Commit  string       `json:"commit"`
	Checks  []Diagnostic `json:"checks"`
}

// Keep the original unexported names for tests and for the package's own use.
type diagnostic = Diagnostic
type report = Report

// DoctorOptions selects what doctor inspects. Interactive allows the CLI to
// prompt for a missing controller URL or token; the terminal UI never prompts.
type DoctorOptions struct {
	StateDir    string
	Server      string
	TokenFile   string
	Interactive bool
	JSON        bool
}

func (a *App) doctor(args []string) error {
	f := flags("doctor", a.Out)
	stateArg := f.String("state-dir", stateDefault(), "managed deployment configuration directory")
	asJSON := f.Bool("json", false, "machine-readable results (no credentials)")
	tokenFile := f.String("env-file", "", "read REET_TOKEN from an explicit env file")
	server := f.String("server", "", "controller URL (defaults to REET_URL or REET_CONTROLLER_URL)")
	ok, err := parse(f, args)
	if !ok {
		return err
	}
	r, err := a.DoctorReport(DoctorOptions{
		StateDir:    *stateArg,
		Server:      *server,
		TokenFile:   *tokenFile,
		Interactive: !*asJSON,
		JSON:        *asJSON,
	})
	if err != nil {
		return err
	}
	if *asJSON {
		if err := json.NewEncoder(a.Out).Encode(r); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(a.Out, "reet doctor — %s (%s)\n", r.Version, r.Commit)
		for _, c := range r.Checks {
			fmt.Fprintf(a.Out, "[%s] %s: %s\n", c.Status, c.Name, c.Message)
			if c.Advice != "" {
				fmt.Fprintln(a.Out, "  "+c.Advice)
			}
		}
		fmt.Fprintf(a.Out, "%d warning(s). Doctor made no configuration changes.\n", r.Warnings())
	}
	if r.HasFailures() {
		return fmt.Errorf("doctor found problems requiring attention")
	}
	return nil
}

// HasFailures reports whether any check needs attention.
func (r Report) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == "fail" {
			return true
		}
	}
	return false
}

// Warnings counts the checks that are worth mentioning but did not fail.
func (r Report) Warnings() int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == "warn" {
			n++
		}
	}
	return n
}

// DoctorReport runs the checks without writing anything to a.Out. The terminal
// UI renders the returned Report; the text command renders the same value.
func (a *App) DoctorReport(opts DoctorOptions) (Report, error) {
	r := Report{Version: a.Version, Commit: a.Commit, Checks: []Diagnostic{}}
	add := func(name, status, message, advice string) {
		r.Checks = append(r.Checks, Diagnostic{name, status, message, advice})
	}
	state, err := absolute(opts.StateDir)
	if err != nil {
		return r, err
	}
	path, err := a.executable()
	if err != nil {
		add("binary", "fail", "Cannot find the running binary.", "Reinstall reet using the release installer.")
	} else {
		add("binary", "pass", "Running "+path, "")
	}
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "reet")
		if abs, e := filepath.Abs(p); e == nil {
			p = abs
		}
		if p, e := filepath.EvalSymlinks(p); e == nil {
			if s, e := os.Stat(p); e == nil && s.Mode().IsRegular() && s.Mode()&0111 != 0 {
				seen[p] = true
			}
		}
	}
	if len(seen) > 1 {
		add("path", "warn", "Multiple reet binaries are on PATH.", "Use command -v reet and remove obsolete installations after checking their versions.")
	} else {
		add("path", "pass", "No competing reet binaries found on PATH.", "")
	}
	if _, err = os.Stat(filepath.Join(state, "lifecycle.lock")); err == nil {
		add("lock", "warn", "A lifecycle lock exists.", "Wait for install/upgrade to finish. Remove lifecycle.lock only after confirming its process is no longer running.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	d, err := load(state)
	if os.IsNotExist(err) {
		add("deployment", "pass", "Standalone binary; no managed deployment.", "Use reet install --mode native or --mode compose to manage a server.")
	} else if err != nil {
		add("deployment", "fail", "Deployment configuration is unreadable or invalid.", "Inspect deployment.json locally; do not share its credentials.")
	} else {
		add("deployment", "pass", "Managed "+d.Mode+" deployment.", "")
		if info, e := os.Stat(filepath.Join(state, "deployment.json")); e == nil && info.Mode().Perm()&0077 != 0 {
			add("permissions", "fail", "Deployment settings are accessible by other users.", "chmod 600 deployment.json; the file may contain tokens.")
		}
		resolved, _ := filepath.EvalSymlinks(path)
		if resolved != d.Binary {
			add("managed-binary", "warn", "This is not the binary recorded in the deployment.", "Run the installed binary's version command and check PATH before upgrading.")
		}
		if d.Mode != "binary" {
			dbPath := filepath.Join(d.Data, "reet.db")
			if _, e := os.Stat(dbPath); os.IsNotExist(e) {
				add("database", "warn", "The server database does not exist yet.", "Start the server and create the owner account.")
			} else if e != nil {
				add("database", "fail", "Cannot access the database.", "Check ownership of the configured data directory.")
			} else if e = checkDB(ctx, dbPath); e != nil {
				add("database", "fail", "SQLite quick_check could not complete successfully.", "Check server logs, permissions and free disk space; retain backups before attempting recovery.")
			} else {
				add("database", "pass", "SQLite quick_check: ok.", "")
			}
			active, e := a.active(ctx, d, state)
			if e != nil {
				add("service", "fail", "Cannot inspect the managed server.", "Check your user service manager or docker compose availability and permissions.")
			} else if !active {
				add("service", "warn", "The managed server is stopped.", "Start the configured user service or Compose deployment when ready.")
			} else {
				add("service", "pass", "Managed server is running.", "")
				if _, e = get(ctx, a.client, localHealth(d.Addr), 1<<20); e != nil {
					add("health", "fail", "Server health endpoint did not return success.", "Inspect service logs and verify the configured address and port.")
				} else {
					add("health", "pass", "Server health endpoint is reachable.", "")
				}
			}
		}
	}
	for _, tool := range []string{"node", "npx"} {
		if _, e := exec.LookPath(tool); e != nil {
			add(tool, "warn", tool+" is unavailable.", "Install Node.js/npm to apply skill sync changes; scanning and the server do not need them.")
		} else {
			add(tool, "pass", tool+" is available for skill sync.", "")
		}
	}
	controller := opts.Server
	if controller == "" {
		controller = os.Getenv("REET_URL")
	}
	if controller == "" {
		controller = os.Getenv("REET_CONTROLLER_URL")
	}
	token := os.Getenv("REET_TOKEN")
	if os.Getenv("REET_URL") == "" && opts.Server == "" && os.Getenv("REET_CONTROLLER_URL") != "" {
		token = os.Getenv("REET_CONTROLLER_TOKEN")
	}
	input := a.In
	if !opts.Interactive {
		input = strings.NewReader("")
	}
	if controller == "" && !opts.JSON && opts.Interactive && clientconfig.Interactive(input) {
		value, err := clientconfig.ResolveURL("", input, os.Stderr)
		if err != nil {
			return r, err
		}
		controller = value
	}
	if controller == "" {
		add("controller", "warn", "No controller URL configured.", "Set REET_URL to the Account page site URL and REET_TOKEN to a personal API token, then run reet scan --prune.")
	} else {
		u, e := url.Parse(controller)
		if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			add("controller", "fail", "Invalid controller URL.", "Use an http(s) site URL; keep tokens in REET_TOKEN, not the URL.")
		} else {
			if !opts.JSON || opts.TokenFile != "" {
				value, err := clientconfig.ResolveToken(token, opts.TokenFile, input, os.Stderr)
				if err != nil {
					return r, err
				}
				token = value
			}
			req, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(controller, "/")+"/api/v2/auth/me", nil)
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			client := *a.client
			client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
			resp, e := client.Do(req)
			if e != nil {
				add("controller", "fail", "Cannot reach the controller.", "Check REET_URL, DNS, TLS and network access.")
			} else {
				resp.Body.Close()
				switch resp.StatusCode {
				case 200:
					add("controller", "pass", "Controller accepted the account credentials.", "")
				case 401, 403:
					add("controller", "fail", "Controller rejected the account credentials.", "Create a personal token on Account and export REET_TOKEN; check it has the needed role.")
				default:
					add("controller", "fail", fmt.Sprintf("Controller returned HTTP %d.", resp.StatusCode), "Check the site URL and reverse proxy configuration.")
				}
			}
		}
	}
	return r, nil
}
