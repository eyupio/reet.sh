package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/filelock"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type agentConfig struct {
	Server          string   `json:"server"`
	Token           string   `json:"token"`
	ID              string   `json:"id"`
	Secret          string   `json:"secret"`
	Host            string   `json:"host"`
	Hostname        string   `json:"hostname"`
	Home            string   `json:"home"`
	Projects        []string `json:"projects"`
	AllowApply      bool     `json:"allowApply"`
	TerminalEnabled bool     `json:"terminalEnabled,omitempty"`
}

func agentConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "reet-agent.json"
	}
	return filepath.Join(dir, "reet", "agent.json")
}
func loadAgentConfig(path string) (agentConfig, error) {
	var c agentConfig
	if err := terminalhost.CheckEnrollmentConfig(path); err != nil {
		return c, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	if c.Hostname != hostname || c.Home != home {
		return c, errors.New("enrollment belongs to another local machine or user; enroll this installation separately")
	}
	return c, nil
}
func writePrivate(path string, data any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".reet-agent-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func agentRequest(c agentConfig, path string, body any, result any, version string) error {
	return agentRequestContext(context.Background(), c, path, body, result, version)
}
func agentRequestContext(ctx context.Context, c agentConfig, path string, body any, result any, version string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.Server+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reet-Token", c.Token)
	req.Header.Set("X-Reet-Agent", c.ID)
	req.Header.Set("X-Reet-Agent-Key", c.Secret)
	req.Header.Set("X-Reet-Version", version)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&e)
		return fmt.Errorf("controller %s: %s", resp.Status, e.Error)
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(result)
}

// RunAgent enrolls a scanner-only installation and processes typed controller jobs.
func RunAgent(args []string, out io.Writer, version string) error {
	return runAgentInput(args, os.Stdin, out, version)
}
func runAgentInput(args []string, in io.Reader, out io.Writer, version string) error {
	if len(args) == 0 {
		return errors.New("usage: reet agent enroll|start|stop|restart|status|run|restore|terminal|service [--config PATH]; run backgrounds by default; --foreground for debugging")
	}
	action := args[0]
	if action == "terminal" {
		return runAgentTerminalInput(args[1:], in, out, version)
	}
	if action == "service" {
		return runAgentService(args[1:], out)
	}
	fs := flag.NewFlagSet("agent "+action, flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	server := fs.String("server", os.Getenv("REET_URL"), "controller URL")
	token := fs.String("token", "", "personal token for enrollment (env REET_TOKEN)")
	tokenFile := fs.String("env-file", "", "read enrollment REET_TOKEN from an explicit env file")
	host := fs.String("env", os.Getenv("REET_ENV"), "stable machine name")
	backup := fs.String("backup", "", "removed skill entry to restore")
	targetID := fs.String("target", "", "target scope for restore")
	once := fs.Bool("once", false, "poll once, then exit (useful for a scheduler)")
	foreground := fs.Bool("foreground", false, "keep the agent attached to this terminal (debugging / supervisors)")
	supervised := fs.Bool("supervised", false, "run under the installed native service manager")
	servicePrefix := fs.String("service-prefix", "reet", "native service name prefix")
	remoteStatus := fs.Bool("remote", false, "also request controller status (may wait for the network)")
	skipTerminalSetup := fs.Bool("skip-terminal-setup", false, "skip the interactive terminal connection wizard after enrollment")
	allow := fs.Bool("allow-apply", false, "allow this installation to execute plans approved in the web UI")
	var projects multi
	fs.Var(&projects, "project", "absolute project directory to track (repeatable at enrollment)")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	absoluteConfig, err := filepath.Abs(*config)
	if err != nil {
		return err
	}
	*config = absoluteConfig
	switch action {
	case "enroll":
		if _, err := os.Stat(*config); err == nil {
			return errors.New("configuration already exists; keep it or revoke the enrollment before replacing it")
		}
		if *server == "" {
			value, err := clientconfig.ResolveURL("", in, os.Stderr)
			if err != nil {
				return err
			}
			*server = value
		}
		u, err := url.Parse(*server)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
			return errors.New("use the controller origin, e.g. https://reet.sh")
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
			return errors.New("remote enrollment requires HTTPS")
		}
		hostname, err := os.Hostname()
		if err != nil {
			return err
		}
		if *host == "" {
			*host = hostname
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		for i, p := range projects {
			if !filepath.IsAbs(p) {
				return errors.New("project paths must be absolute")
			}
			p, err = filepath.EvalSymlinks(p)
			if err != nil {
				return err
			}
			st, e := os.Stat(p)
			if e != nil || !st.IsDir() {
				return errors.New("project directory unavailable")
			}
			projects[i] = p
		}
		tokenValue, err := clientconfig.ResolveToken(*token, *tokenFile, in, os.Stderr)
		if err != nil {
			return err
		}
		c := agentConfig{Server: strings.TrimRight(*server, "/"), Token: tokenValue, Host: *host, Hostname: hostname, Home: home, Projects: projects, AllowApply: *allow}
		var result struct {
			Agent  store.SyncAgent
			Secret string
		}
		if err = agentRequest(c, "/api/v2/sync/agents", map[string]string{"host": *host, "version": version}, &result, version); err != nil {
			return err
		}
		c.ID = result.Agent.ID
		c.Secret = result.Secret
		c.Host = result.Agent.Host
		if err = writePrivate(*config, c); err != nil {
			return fmt.Errorf("enrolled but could not save configuration; revoke enrollment in Machines before retrying: %w", err)
		}
		fmt.Fprintf(out, "Enrolled %s. Private configuration: %s\nRun reet agent run --config %q to report inventories and process approved jobs. Remote apply enabled: %t\n", c.Host, *config, *config, c.AllowApply)
		if !*skipTerminalSetup {
			OfferTerminalHostSetup(TerminalSetupOptions{Config: *config, Version: version}, in, out)
		}
		return nil
	case "restore":
		c, err := loadAgentConfig(*config)
		if err != nil {
			return err
		}
		targetValue, err := clientconfig.Require(*targetID, "--target (target scope)", false, in, os.Stderr)
		if err != nil {
			return err
		}
		backupValue, err := clientconfig.Require(*backup, "--backup (removed skill entry)", false, in, os.Stderr)
		if err != nil {
			return err
		}
		return restoreSkill(c, targetValue, backupValue, in, out, version)
	case "status":
		c, err := loadAgentConfig(*config)
		if err != nil {
			return err
		}
		if err = printAgentStatus(*config, out); err != nil {
			return err
		}
		if !*remoteStatus {
			return nil
		}
		var a store.SyncAgent
		if err = agentRequest(c, "/api/v2/sync/identity", map[string]string{}, &a, version); err != nil {
			return err
		}
		fmt.Fprintf(out, "Machine: %s\nController: %s\nPaused: %t\nRemote apply enabled: %t\nTerminal access enabled locally: %t\n", a.Host, c.Server, a.Paused, c.AllowApply, c.TerminalEnabled)
		return nil
	case "start", "stop", "restart":
		if _, err := loadAgentConfig(*config); err != nil {
			return err
		}
		return controlAgent(action, *config, *servicePrefix, out)
	case "run":
	default:
		return errors.New("unknown agent command")
	}
	c, err := loadAgentConfig(*config)
	if err != nil {
		return err
	}
	if !*once && !*foreground {
		return controlAgent("start", *config, *servicePrefix, out)
	}
	lock, err := filelock.Acquire(*config + ".lock")
	if err != nil {
		return fmt.Errorf("cannot lock agent enrollment: %w", err)
	}
	defer lock.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancelAgent := context.WithCancel(ctx)
	defer cancelAgent()
	state := &agentRuntime{started: time.Now().UnixMilli(), version: version, supervised: *supervised, servicePrefix: *servicePrefix}
	if !*once {
		closeControl, err := serveAgentControl(*config, state, cancelAgent)
		if err != nil {
			return err
		}
		defer closeControl()
		if err := notifyAgentReady(); err != nil {
			return err
		}
	}
	if !*once {
		fmt.Fprintf(out, "Agent running in the foreground for %s. Press Ctrl+C to stop.\n", c.Host)
		fmt.Fprintf(out, "Inventory scans repeat every minute; monitoring continues between scans.\nFor a single upload and exit: reet agent run --once --config %q\n", *config)
	}
	if !*once {
		instance, err := agenthealth.NewInstanceID()
		if err != nil {
			return err
		}
		monitorDone := make(chan struct{})
		defer func() { cancelAgent(); <-monitorDone }()
		go func() {
			defer close(monitorDone)
			if err := agenthealth.RunAgent(ctx, agenthealth.AgentConfig{
				TerminalSession: terminalControl(*config, c),
				OnConnection:    state.connected.Store,
				ServerURL:       c.Server, Token: c.Token, AgentID: c.ID, Secret: c.Secret,
				Version: version, InstanceID: instance,
				TerminalEnabled: c.TerminalEnabled, TerminalReadiness: terminalReadiness(*config, c.ID, c.TerminalEnabled),
				Consent: func() (bool, terminalproto.Readiness, error) {
					current, err := loadAgentConfig(*config)
					if err != nil {
						return false, terminalproto.Unsupported, err
					}
					if current.ID != c.ID || current.Secret != c.Secret || current.Server != c.Server {
						return false, terminalproto.DisabledLocally, errors.New("local enrollment changed; restart the agent")
					}
					return current.TerminalEnabled, terminalReadiness(*config, current.ID, current.TerminalEnabled), nil
				},
			}); err != nil {
				log.Printf("private monitoring: %v", err)
			}
		}()
	}
	lastScan := time.Time{}
	announcedWaiting := false
	for {
		if err = agentCycle(ctx, c, *config, out, version, &lastScan, !*once); err != nil {
			if *once {
				return err
			}
			fmt.Fprintf(out, "Agent: %v\n", err)
		} else if !*once && !announcedWaiting {
			fmt.Fprintln(out, "Agent cycle complete. Still running: waiting for updates and the next inventory scan.")
			announcedWaiting = true
		}
		if err == nil {
			state.lastCycle.Store(time.Now().UnixMilli())
		}
		if *once {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
}

// permanentAgentError reports a controller refusal that will not change by
// resending the same request.
func permanentAgentError(err error) bool {
	message := err.Error()
	for _, code := range []string{"controller 400", "controller 403", "controller 404", "controller 409"} {
		if strings.HasPrefix(message, code) {
			return true
		}
	}
	return false
}

type agentResult struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Hash    string   `json:"hash"`
	Message string   `json:"message"`
	Plan    SyncPlan `json:"plan"`
}

func agentCycle(ctx context.Context, c agentConfig, config string, out io.Writer, version string, lastScan *time.Time, allowRestart bool) error {
	// A completed result is durable before acknowledgment. Reconnect resends the
	// result, never the filesystem operation.
	if b, err := os.ReadFile(config + ".result"); err == nil {
		var result agentResult
		if err = json.Unmarshal(b, &result); err != nil {
			return err
		}
		if err = agentRequestContext(ctx, c, "/api/v2/sync/result", result, nil, version); err != nil {
			// A controller that has already closed this job will never take the
			// result, so drop it instead of blocking every later cycle on it.
			if permanentAgentError(err) {
				_ = os.Remove(config + ".result")
			}
			return err
		}
		if err = os.Remove(config + ".result"); err != nil {
			return err
		}
	}
	var reply struct {
		Agent store.SyncAgent
		Job   *store.SyncJob
	}
	if err := agentRequestContext(ctx, c, "/api/v2/sync/poll", map[string]string{}, &reply, version); err != nil {
		return err
	}
	c.Host = reply.Agent.Host
	if reply.Agent.Paused {
		return nil
	}
	scanAll := func() error {
		dirs := append([]string{c.Home}, c.Projects...)
		var failures []error
		for _, dir := range dirs {
			if err := runVersionAt(dir, []string{"--server", c.Server, "--token", c.Token, "--env", c.Host, "--prune"}, out, version); err != nil {
				failures = append(failures, err)
			}
		}
		return errors.Join(failures...)
	}
	if reply.Job == nil {
		if time.Since(*lastScan) >= time.Minute {
			*lastScan = time.Now()
			return scanAll()
		}
		return nil
	}
	j := reply.Job
	result := agentResult{ID: j.ID, Status: "succeeded"}
	var operationErr error
	restartPath := ""
	if j.Kind == "scan" {
		operationErr = scanAll()
	} else if j.Kind == "update" {
		if !allowRestart {
			operationErr = errors.New("updating reet requires the long-running background or service agent")
		} else {
			result.Message, restartPath, operationErr = updateAgentBinary(ctx, j.Revision, version)
		}
	} else {
		target, err := fetchTarget(c.Server, c.Token, j.TargetID)
		if err != nil {
			operationErr = err
		} else if !strings.EqualFold(target.Target.Host, c.Host) || target.Revision != j.Revision {
			operationErr = errors.New("target identity or revision changed")
		} else {
			dir := c.Home
			if target.Target.Project != "" {
				dir = target.Target.Project
				allowed := false
				for _, p := range c.Projects {
					if p == dir {
						allowed = true
					}
				}
				if !allowed {
					operationErr = errors.New("project is not enrolled; add it to this installation's project scopes locally")
				}
			}
			if j.Kind == "apply" && !c.AllowApply {
				operationErr = errors.New("remote apply disabled locally; enroll with --allow-apply to opt in")
			}
			if operationErr == nil {
				args := []string{"--server", c.Server, "--token", c.Token, "--env", c.Host, "--target", j.TargetID, "--config", config}
				opts := syncOptions{cwd: dir, verifyEnrollment: true, onBackup: func(path string) { result.Plan.Backups = append(result.Plan.Backups, path) }, onPlan: func(p SyncPlan) { result.Plan = p; result.Hash = p.Hash() }}
				if j.Kind == "apply" {
					args = append(args, "--apply")
					opts.approvedHash = j.Hash
				}
				runCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
				operationErr = runSyncWithOptions(args, strings.NewReader(""), out, func(args []string, cwd string) error {
					cmd := exec.CommandContext(runCtx, "npx", args...)
					cmd.Dir = cwd
					cmd.Stdout = out
					cmd.Stderr = out
					return cmd.Run()
				}, version, opts)
				cancel()
				if operationErr == nil && (j.Kind == "preview" || j.Kind == "auto") {
					result.Status = "awaiting-approval"
				}
			}
		}
	}
	if operationErr != nil {
		result.Status = "failed"
		result.Message = operationErr.Error()
	}
	if restartPath != "" {
		// Leave the successful result durable before replacing the process image.
		// The new process sends it on its first cycle. If exec fails, record and
		// send the failure from this process instead.
		if err := writePrivate(config+".result", result); err != nil {
			return err
		}
		if err := restartAgent(restartPath); err != nil {
			result.Status = "failed"
			result.Message = "Binary updated but the agent restart failed: " + err.Error()
		} else {
			return nil
		}
	}
	if err := writePrivate(config+".result", result); err != nil {
		return err
	}
	if err := agentRequestContext(ctx, c, "/api/v2/sync/result", result, nil, version); err != nil {
		return err
	}
	return os.Remove(config + ".result")
}
