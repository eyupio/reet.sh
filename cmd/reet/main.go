// Command reet is a self-hosted like/bookmark/used tracker for skills.sh.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/store"
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
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("reet %s (%s)\n", version, commit)
	case "help", "--help", "-h":
		fmt.Println("usage: reet serve | reet healthcheck | reet scan [options] | reet sync [options] | reet version")
		fmt.Println("Run reet scan --help or reet sync --help for options.")
	case "serve":
		if err := serve(); err != nil {
			log.Fatal(err)
		}
	case "healthcheck":
		if err := healthcheck(healthURL(env("REET_ADDR", ":8080"))); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "sync":
		if err := scan.RunSyncVersion(os.Args[2:], os.Stdin, os.Stdout, version); err != nil {
			log.Fatal(err)
		}
	case "scan":
		if err := scan.RunVersion(os.Args[2:], os.Stdout, version); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: reet serve | reet healthcheck | reet scan [options] | reet sync [options] | reet version")
		os.Exit(2)
	}
}

func serve() error {
	st, err := store.Open(env("REET_DATA_DIR", "./data"))
	if err != nil {
		return err
	}
	defer st.Close()
	_ = st.PruneEvents(time.Now().AddDate(-1, 0, 0).UnixMilli())
	_ = st.PruneSessions(time.Now().UnixMilli())
	_ = st.PruneInvites(time.Now().UnixMilli())
	mode := api.AuthMode(os.Getenv("REET_AUTH"))
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
	h := api.New(st, meta.Default(os.Getenv("SKILLS_SH_TOKEN")), os.Getenv("REET_TOKEN"),
		api.WithAuthMode(mode), api.WithVersion(version, commit), api.WithDebug(os.Getenv("REET_LOG") == "debug"),
		api.WithTopics(os.Getenv("REET_TOPICS") != "off"),
		api.WithRefresh(os.Getenv("REET_REFRESH") != "off"))
	mux := http.NewServeMux()
	mux.Handle("/api/", h)
	mux.Handle("/quick", h)
	mux.Handle("/healthz", h)
	mux.Handle("/", web.Handler())
	srv := &http.Server{Addr: env("REET_ADDR", ":8080"), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
