// Command reet is a self-hosted like/bookmark/used tracker for skills.sh.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/agentupdate"
	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/cliui"
	"github.com/eyupio/reet.sh/internal/lifecycle"
	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/notify"
	"github.com/eyupio/reet.sh/internal/oidc"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/setup"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
	"github.com/eyupio/reet.sh/web"
)

var (
	version = "dev"
	commit  = "unknown"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	if usesClientDefaults(cmd, args) {
		if err := clientconfig.Apply(); err != nil {
			fmt.Fprintln(os.Stderr, "Client defaults:", err)
			os.Exit(1)
		}
	}
	scan.SetBuildCommit(commit)
	if cmd == "configure" || cmd == "install" || cmd == "init" || cmd == "upgrade" || cmd == "update" || cmd == "help" || cmd == "--help" || cmd == "-h" {
		cliui.Banner(os.Stderr)
	}
	switch cmd {
	case "version", "--version", "-v":
		printVersion(os.Stdout, version, commit)
	case "help", "--help", "-h":
		fmt.Print(`reet.sh — your AI agent toolkit. Get your toolkit reet.

Usage: reet <command> [options]

  serve        Run the web app (also the default when no command is given)
  agent        Enroll a machine, process sync jobs and manage local terminal services
  terminal     Start, stop, restart or inspect controller web terminal access
  scan         Report installed skills to your account
  setup        Discover local AI agent setups and support limitations
  sync         Preview a saved skill set; --apply asks before changing files
  configure    Save your release channel, target server and token
  install      Install this binary or set up a managed server
  upgrade      Update a managed deployment with backup and recovery
  backup       Take, verify and restore sealed backups
  doctor       Check your installation and suggest next steps
  healthcheck  Check your saved server, --url, or --local
  version      Show the binary version and source commit

Connect to an existing site:
  Open Account on your site, then Connect a machine.
  Paste its connection command into your terminal, then run:
    reet scan --env my-laptop --prune

Host your own site:
  reet install --mode native
  Or use reet install --mode compose for Docker Compose.

Run reet <command> --help for options. init/update alias install/upgrade.
`)
	case "terminal":
		if err := runTerminal(args, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "configure":
		if err := runConfigure(args, os.Stdin, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "install", "init", "upgrade", "update", "doctor":
		if err := lifecycle.New(version, commit, os.Stdin, os.Stdout).Run(cmd, args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "serve":
		f := flag.NewFlagSet("reet serve", flag.ExitOnError)
		config := f.String("config", "", "managed deployment.json path")
		envFile := f.String("env-file", "", "configuration file (default REET_ENV_FILE or .env)")
		f.Usage = func() {
			fmt.Fprintln(f.Output(), "Usage: reet serve [--config PATH]\n\nRun the web app. Open the listening address in your browser to create the owner account.\n\nOptions:")
			f.PrintDefaults()
			fmt.Fprint(f.Output(), `
Configuration (environment variables):
  REET_ADDR       Listening HOST:PORT (default :8080, all interfaces)
  REET_DATA_DIR   SQLite data directory (default ./data)
  REET_AUTH       accounts (default), token or none
  REET_TOKEN      Optional owner-setup bootstrap / legacy access token
  REET_SIGNUP     open to allow self-serve accounts (default invite-only)
  REET_CONTACT_EMAIL  Optional admin address shown for account/reset requests
  REET_MAIL_OUTBOX_DIR  Local dev only: write emails here instead of sending
  REET_TOPICS     off to disable topic seeding
  REET_REFRESH    off to disable background metadata refresh
  REET_RELAY_MAP_URL  Optional HTTPS Tailcat relay map for private agent connections
  REET_OIDC_ISSUER    Turn on sign-in through an identity provider (see docs/single-sign-on.md;
                      also REET_OIDC_CLIENT_ID, REET_OIDC_CLIENT_SECRET[_FILE], REET_OIDC_*)
  REET_REMOTE_TERMINALS  Legacy initial default; use reet terminal start|stop|restart|status

Local example:
  REET_ADDR=127.0.0.1:8080 REET_DATA_DIR=./data reet serve
  Open http://127.0.0.1:8080 and create the owner account.

For a managed background server, use reet install --mode native or --mode compose.
Keep the data directory when updating. Stop the server or use a consistent SQLite
backup to preserve accounts, lists, teams and sync references.
`)
		}
		f.Parse(args)
		if f.NArg() != 0 {
			log.Fatal("unexpected serve arguments")
		}
		if *config != "" {
			if err := lifecycle.LoadServerConfig(*config); err != nil {
				log.Fatal(err)
			}
		}
		if err := applyServerEnvFile(*envFile); err != nil {
			log.Fatal(err)
		}
		if err := serve(); err != nil {
			log.Fatal(err)
		}
	case "healthcheck":
		if err := runHealthcheck(args, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "agent":
		if err := scan.RunAgent(args, os.Stdout, version); err != nil {
			log.Fatal(err)
		}
	case "sync":
		if err := scan.RunSyncVersion(args, os.Stdin, os.Stdout, version); err != nil {
			log.Fatal(err)
		}
	case "setup":
		if err := setup.Run(args, os.Stdout, profileRemote{}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "scan":
		if err := scan.RunVersion(args, os.Stdout, version); err != nil {
			log.Fatal(err)
		}
	case "backup":
		if err := runBackup(args, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: reet serve | reet terminal | reet install | reet upgrade | reet doctor | reet healthcheck | reet scan | reet setup | reet sync | reet backup | reet version")
		os.Exit(2)
	}
}

// printVersion answers the agent update protocol probe before the ordinary
// version line. The probe lets a newer agent know whether a target build can
// read the pending-update marker it would otherwise leave behind.
func printVersion(out io.Writer, version, commit string) {
	if os.Getenv("REET_AGENT_UPDATE_PROTOCOL") == "1" {
		fmt.Fprintln(out, scan.AgentUpdateProtocol)
		return
	}
	fmt.Fprintf(out, "reet %s (%s)\n", version, commit)
}

func serve() error {
	terminalControl, err := terminalcontrol.Open(env("REET_DATA_DIR", "./data"), os.Getenv("REET_REMOTE_TERMINALS") == "on")
	if err != nil {
		return err
	}
	defer terminalControl.Close()
	publicURL := os.Getenv("REET_PUBLIC_URL")
	if publicURL == "" {
		publicURL = terminalControl.PublicURL()
	}
	if publicURL != "" {
		u, err := url.Parse(publicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("REET_PUBLIC_URL must be an HTTP(S) origin without credentials, path, query or fragment")
		}
	}
	st, err := store.Open(env("REET_DATA_DIR", "./data"))
	if err != nil {
		return err
	}
	defer st.Close()
	// Saved Projects become folder projects once; a later start finds nothing
	// to do. A failure is logged and never stops the controller: the saved
	// Projects stay readable and the next start tries again.
	if rep, err := st.MigrateProjectsToFolders(); err != nil {
		log.Printf("projects migration: not finished, will try again at the next start: %v", err)
	} else if rep.Created > 0 || rep.Skipped > 0 {
		log.Printf("projects migration: %d accounts, %d folder projects made, %d repositories skipped", rep.Users, rep.Created, rep.Skipped)
	}
	_ = st.PruneEvents(time.Now().AddDate(-1, 0, 0).UnixMilli())
	_ = st.PruneSessions(time.Now().UnixMilli())
	_ = st.PruneInvites(time.Now().UnixMilli())
	_, _ = st.PruneMail()
	mode := api.AuthMode(os.Getenv("REET_AUTH"))
	if terminalControl.AuthMode() != "" {
		mode = api.AuthMode(terminalControl.AuthMode())
	}
	switch mode {
	case "":
		mode = api.AuthAccounts
	case api.AuthAccounts, api.AuthToken, api.AuthNone:
	default:
		return fmt.Errorf("REET_AUTH must be accounts, token or none")
	}
	if mode == api.AuthAccounts && os.Getenv("REET_TOKEN") != "" {
		log.Printf("accounts enabled; REET_TOKEN is the setup bootstrap and scan token")
	}
	// A half-set configuration stops the server here, with what to fix, rather
	// than starting with single sign-on quietly off.
	sso, err := oidc.FromEnv(func(k string) string {
		if k == "REET_PUBLIC_URL" {
			return publicURL
		}
		return os.Getenv(k)
	})
	if err != nil {
		return err
	}
	if sso != nil && mode != api.AuthAccounts {
		return fmt.Errorf("single sign-on needs REET_AUTH=accounts (it is %s)", mode)
	}
	if sso != nil {
		log.Printf("single sign-on: enabled for %s; callback %s", sso.Issuer(), sso.RedirectURL())
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	bcfg, berr := backup.FromEnv(os.Getenv)
	if berr != nil {
		log.Printf("backups: %v", berr)
	}
	backups := backup.New(st, bcfg)
	backups.Start(ctx)
	mailSettings := &mail.Settings{Store: st, Sealer: bcfg.Sealer, Env: mail.Config{
		BaseURL: publicURL, OutboxDir: strings.TrimSpace(os.Getenv(mail.EnvOutboxDir)),
	}}
	queue := &mail.Queue{Store: st, Settings: mailSettings}
	mailer := &mail.Mailer{Store: st, Settings: mailSettings, Queue: queue}
	notifier := &notify.Service{Store: st, Mail: mailer}
	go queue.Loop(ctx, time.Minute)
	go notifier.Loop(ctx, time.Hour)
	mailNow := mailSettings.Current()
	if warning := mail.OutboxWarning(mailNow, mailNow.BaseURL); warning != "" {
		log.Printf("email: %s", warning)
	}
	signup := strings.EqualFold(strings.TrimSpace(os.Getenv("REET_SIGNUP")), "open") ||
		strings.TrimSpace(os.Getenv("REET_SIGNUP")) == "1" ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("REET_SIGNUP")), "true")
	releases := agentupdate.NewChecker()
	go releases.Info(context.Background())
	h := api.New(st, meta.Default(os.Getenv("SKILLS_SH_TOKEN")), os.Getenv("REET_TOKEN"),
		api.WithRemoteTerminals(terminalControl.Enabled()),
		api.WithAuthMode(mode), api.WithVersion(version, commit), api.WithDebug(os.Getenv("REET_LOG") == "debug"),
		api.WithTopics(os.Getenv("REET_TOPICS") != "off"), api.WithPublicURL(publicURL),
		api.WithRefresh(os.Getenv("REET_REFRESH") != "off"), api.WithBackups(backups),
		api.WithSignup(signup), api.WithContactEmail(os.Getenv("REET_CONTACT_EMAIL")), api.WithMail(mailer), api.WithNotify(notifier),
		api.WithPrivateConnection(bcfg.Sealer, os.Getenv("REET_RELAY_MAP_URL")),
		api.WithReleaseChecker(releases), api.WithOIDC(sso))
	defer func() { terminalControl.Close(); h.Close() }()
	if err := terminalControl.Serve(h.SetRemoteTerminals, h.RemoteTerminalsAvailable, terminalcontrol.Options{Configure: h.ConfigureTerminalOrigin, Reason: h.RemoteTerminalsReason, PublicURL: h.RemoteTerminalPublicURL, AuthMode: h.RemoteTerminalAuthMode}); err != nil {
		return err
	}
	// Project runs interrupted by the last stop continue their pending members.
	h.ResumeProjectRuns(ctx)
	mux := http.NewServeMux()
	mux.Handle("/api/", h)
	mux.Handle("/quick", h)
	mux.Handle("/healthz", h)
	mux.Handle("/mcp", h)
	mux.Handle("/oauth/", h)
	mux.Handle("/.well-known/", h)
	mux.Handle("/", api.WithHostedPages(h, web.Handler(publicURL)))
	srv := &http.Server{Addr: env("REET_ADDR", ":8080"), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go reportServer(ctx, st)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("reet %s (%s) listening on %s auth=%s", version, commit, srv.Addr, mode)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Server identity belongs to the configured controller account, or the local
// instance owner. A container reports only its own visible filesystem.
func reportServer(ctx context.Context, st *store.Store) {
	report := func() {
		host := env("REET_ENV", "")
		if host == "" {
			host, _ = os.Hostname()
		}
		if host == "" {
			return
		}
		if controller := os.Getenv("REET_CONTROLLER_URL"); controller != "" {
			if err := scan.RunVersion([]string{"--server", controller, "--token", os.Getenv("REET_CONTROLLER_TOKEN"), "--env", host, "--prune", "--server-host"}, io.Discard, version); err != nil {
				log.Printf("controller scan failed: %v", err)
			}
			return
		}
		owner, ok, err := st.OwnerUser()
		if err != nil || !ok {
			return
		}
		if err := st.RegisterMachineFor(owner.ID, store.MachineInfo{Host: host, OS: runtime.GOOS, Arch: runtime.GOARCH, Version: version, Server: true}); err != nil {
			log.Printf("server registration failed: %v", err)
		}
	}
	report()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report()
		}
	}
}
