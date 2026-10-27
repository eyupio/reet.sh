package agentupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckerFetchesPublishedChannels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dev/dev-build.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.4.2"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Checker{Client: srv.Client(), ReleasesAPI: srv.URL + "/releases", DownloadsRoot: srv.URL, TTL: time.Minute}
	info := c.Info(context.Background())
	if info.Dev.Commit != "0123456789abcdef0123456789abcdef01234567" || info.Dev.Version != "dev" {
		t.Fatalf("dev channel: %+v", info.Dev)
	}
	if info.Latest.Version != "v1.4.2" {
		t.Fatalf("stable channel: %+v", info.Latest)
	}
	if info.Dev.CheckedAt == 0 || info.Latest.CheckedAt == 0 {
		t.Fatalf("missing checked time: %+v", info)
	}
}

func TestCheckerReportsChannelErrorsIndependently(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dev/dev-build.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not-a-commit"))
	})
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"tag_name":"v2.0.0"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Checker{Client: srv.Client(), ReleasesAPI: srv.URL + "/releases", DownloadsRoot: srv.URL}
	info := c.Info(context.Background())
	if info.Dev.Error == "" {
		t.Fatalf("invalid dev pointer accepted: %+v", info.Dev)
	}
	if info.Latest.Version != "v2.0.0" || info.Latest.Error != "" {
		t.Fatalf("stable channel should survive a dev failure: %+v", info.Latest)
	}
}

func TestUpdateTargetFollowsTheAgentChannel(t *testing.T) {
	info := Info{
		Dev:    Channel{Version: "dev", Commit: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		Latest: Channel{Version: "v1.4.2"},
	}
	for _, tc := range []struct {
		name    string
		version string
		commit  string
		target  string
		update  bool
	}{
		{"current dev", "dev", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "", false},
		{"old dev", "dev", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "dev", true},
		{"unknown dev commit", "dev", "", "dev", true},
		{"old release", "v1.2.9", "", "latest", true},
		{"current release", "v1.4.2", "", "", false},
		{"newer local release", "v1.5.0", "", "", false},
		{"unknown version", "", "", "", false},
	} {
		target, update := UpdateTarget(tc.version, tc.commit, info)
		if target != tc.target || update != tc.update {
			t.Fatalf("%s: got %q %t, want %q %t", tc.name, target, update, tc.target, tc.update)
		}
	}
}

func TestUpdateTargetDoesNotGuessWithoutChannelData(t *testing.T) {
	for _, info := range []Info{
		{},
		{Dev: Channel{Version: "dev", Error: "offline"}},
		{Latest: Channel{Error: "offline"}},
	} {
		if target, update := UpdateTarget("dev", "", info); update || target != "" {
			t.Fatalf("dev without a published commit: %q %t", target, update)
		}
		if target, update := UpdateTarget("v1.0.0", "", info); update || target != "" {
			t.Fatalf("release without a published tag: %q %t", target, update)
		}
	}
}

func TestVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		less bool
	}{
		{"v1.2.3", "v1.2.4", true},
		{"v1.2.10", "v1.2.9", false},
		{"v1.9.0", "v2.0.0", true},
		{"v2.0.0-rc.1", "v2.0.0", true},
		{"v2.0.0", "v2.0.0", false},
		{"not-a-version", "v1.0.0", false},
	} {
		if got := VersionLess(tc.a, tc.b); got != tc.less {
			t.Fatalf("%s < %s: got %t", tc.a, tc.b, got)
		}
	}
}
