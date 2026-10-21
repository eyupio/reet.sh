// A disposable real reet API for protocol tests and reproducible evaluations.
// All writes go to a temporary SQLite directory, never the user's shelf.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	dir, err := os.MkdirTemp("", "reet-mcp-fixture-")
	check(err)
	defer os.RemoveAll(dir)
	st, err := store.Open(dir)
	check(err)
	defer st.Close()
	uid, err := st.DefaultOwner()
	check(err)
	type entry struct {
		slug, name, notes  string
		likes, saved, used bool
		installs           int64
		tags               []string
	}
	entries := []entry{
		{"audit", "Audit Trail", "Check permissions before release", true, false, true, 300, []string{"Security", "Backend"}},
		{"layout", "Layout Lab", "Keyboard and contrast review", true, true, false, 900, []string{"Frontend"}},
		{"tests", "Test Pilot", "Regression suite for checkout", false, false, true, 700, []string{"Testing", "Backend"}},
		{"deploy", "Deploy Guide", "Roll back on failed readiness", false, true, false, 500, []string{"Backend"}},
		{"copy", "Copy Desk", "Tone guidance for forms", false, false, false, 200, []string{"Frontend"}},
		{"metrics", "Metric Watch", "Latency and saturation", true, false, true, 600, []string{"Backend"}},
	}
	for _, e := range entries {
		rf, err := ref.Parse("demo/skills/" + e.slug)
		check(err)
		_, err = st.UpsertFor(uid, rf, store.Patch{Liked: &e.likes, Bookmarked: &e.saved, Used: &e.used, Notes: &e.notes})
		check(err)
		check(st.SetMeta(rf.ID, e.name, e.notes, e.installs, e.installs/10, []int64{1, 2, 3}))
		check(st.AddSkillTagsFor(uid, rf.ID, e.tags))
	}
	for _, env := range []struct {
		host, agent string
		skills      []string
		complete    bool
	}{
		{"laptop", "codex", []string{"audit", "tests", "metrics"}, true},
		{"desktop", "claude", []string{"tests", "layout"}, true},
		{"travel", "cursor", []string{"copy"}, false},
	} {
		for _, slug := range env.skills {
			check(st.RecordEnvironmentFor(uid, "demo/skills/"+slug, env.host, env.agent, "", "/fixture/skills", ""))
		}
		check(st.RecordInventoryFor(uid, store.EnvironmentIDFor(uid, env.host, env.agent, ""), env.complete, store.MachineInfo{Host: env.host, OS: "linux", Arch: "amd64", Version: "fixture"}))
	}
	check(st.SaveSyncTargetFor(uid, store.EnvironmentIDFor(uid, "laptop", "codex", ""), store.EnvironmentIDFor(uid, "desktop", "claude", "")))
	for _, list := range []struct {
		name   string
		skills []string
	}{
		{"Release kit", []string{"audit", "tests", "deploy"}},
		{"Interface kit", []string{"layout", "copy", "tests"}},
	} {
		l, err := st.SaveAudienceList(uid, "owner", 0, list.name, "Frozen MCP evaluation fixture", "personal", store.Audience{})
		check(err)
		for _, slug := range list.skills {
			check(st.AddListSkill(l.ID, "demo/skills/"+slug, "Review before adoption", uid))
		}
	}
	check(st.IgnoreFor(uid, "demo/skills/retired", "Retired Helper", "Replaced in the fixture"))
	mode := api.AuthToken
	if len(os.Args) > 1 && os.Args[1] == "accounts" {
		mode = api.AuthAccounts
		check(st.CreateSession(auth.HashToken("fixture-session"), uid, time.Now().Add(time.Hour).UnixMilli(), "MCP test browser"))
		_, err := st.CreateAPIToken(uid, "fixture owner", auth.HashToken("fixture-token"))
		check(err)
		viewer, err := st.CreateUser("viewer@reet.invalid", "Fixture viewer", "viewer", "")
		check(err)
		_, err = st.CreateAPIToken(viewer.ID, "fixture viewer", auth.HashToken("viewer-token"))
		check(err)
		rf, err := ref.Parse("demo/private/viewer-only")
		check(err)
		notes := "Private viewer notes"
		_, err = st.UpsertFor(viewer.ID, rf, store.Patch{Notes: &notes})
		check(err)
		for _, credential := range []struct {
			userID        int64
			scope, secret string
		}{
			{uid, "read", "reet_mcp_fixture_read"}, {uid, "write", "reet_mcp_fixture_write"}, {viewer.ID, "read", "reet_mcp_fixture_viewer"},
		} {
			check(st.SetMCPEnabled(credential.userID, true))
			_, err = st.CreateMCPConnection(credential.userID, "Fixture MCP", credential.scope, auth.HashToken(credential.secret), time.Now().Add(time.Hour).UnixMilli())
			check(err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	fmt.Printf("http://%s\n", listener.Addr())
	// Empty fetcher never calls external services. Jobs are off by default.
	srv := &http.Server{Handler: api.New(st, meta.Fetcher{}, "fixture-token", api.WithAuthMode(mode))}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); _ = srv.Close() }()
	if err := srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		check(err)
	}
}
