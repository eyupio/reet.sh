// Command reet is a self-hosted like/bookmark/used tracker for skills.sh.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
		fmt.Println("usage: reet serve | reet healthcheck | reet scan [options] | reet version")
		fmt.Println("Run reet scan --help for scanner options.")
	case "serve":
		if err := serve(); err != nil {
			log.Fatal(err)
		}
	case "healthcheck":
		if err := healthcheck(healthURL(env("REET_ADDR", ":8080"))); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "scan":
		if err := scan.Run(os.Args[2:], os.Stdout); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: reet serve | reet healthcheck | reet scan [options] | reet version")
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
