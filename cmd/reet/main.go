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
		fmt.Fprintf(os.Stderr, "usage: reet serve | reet healthcheck | reet scan [--server URL] [--token T] [--dir PATH] [--dry-run]\n")
		os.Exit(2)
	}
}

func serve() error {
	st, err := store.Open(env("REET_DATA_DIR", "./data"))
	if err != nil {
		return err
	}
	defer st.Close()
	h := api.New(st, meta.Default(os.Getenv("SKILLS_SH_TOKEN")), os.Getenv("REET_TOKEN"))
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
	log.Printf("reet listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
