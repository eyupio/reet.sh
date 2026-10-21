package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
)

func mustRef(t *testing.T, in string) ref.Ref {
	t.Helper()
	r, err := ref.Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFetchPageDescription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vercel-labs/skills/find-skills" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`<html><head><title>x</title><meta name="description" content="Find skills fast"></head></html>`))
	}))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), PageBase: srv.URL + "/"}
	m := f.Fetch(context.Background(), mustRef(t, "vercel-labs/skills/find-skills"))
	if m.Description != "Find skills fast" {
		t.Fatalf("%+v", m)
	}
}

func TestFetchWellKnownPageUsesSitePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/site/open.feishu.cn/lark-doc" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`<meta name="description" content="Docs">`))
	}))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), PageBase: srv.URL + "/"}
	if m := f.Fetch(context.Background(), mustRef(t, "open.feishu.cn/lark-doc")); m.Description != "Docs" {
		t.Fatalf("%+v", m)
	}
}

func TestFetchAPIPrefersToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills/vercel-labs/skills/find-skills" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		w.Write([]byte(`{"installs":24531,"files":[{"path":"SKILL.md","contents":"---\nname: Next.js Development\ndescription: \"Build apps\"\n---\nbody"}]}`))
	}))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), APIBase: srv.URL, Token: "tok"}
	m := f.Fetch(context.Background(), mustRef(t, "vercel-labs/skills/find-skills"))
	if auth != "Bearer tok" {
		t.Fatalf("auth header %q", auth)
	}
	if m.Name != "Next.js Development" || m.Description != "Build apps" || m.Installs != 24531 {
		t.Fatalf("%+v", m)
	}
}

func TestFetchFailureReturnsZero(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500":     func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) },
		"garbage": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("\x00\x01 not html")) },
		"slow":    func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) },
	}
	for name, h := range cases {
		srv := httptest.NewServer(h)
		f := Fetcher{Client: &http.Client{Timeout: 50 * time.Millisecond}, PageBase: srv.URL + "/", APIBase: srv.URL, Token: "t"}
		if m := f.Fetch(context.Background(), mustRef(t, "a/b/c")); !m.Empty() {
			t.Errorf("%s: got %+v", name, m)
		}
		srv.Close()
	}
}

func TestFetchPageJSONLDAndTrend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head>
			<meta property="og:title" content="pptx — anthropics/skills">
			<meta name="description" content="Fallback description">
			<script type="application/ld+json">{"@context":"https://schema.org","@type":"SoftwareApplication","name":"pptx","description":"Make decks","interactionStatistic":{"@type":"InteractionCounter","interactionType":"https://schema.org/InstallAction","userInteractionCount":239629}}</script>
		</head><body><svg aria-label="Weekly installs: 4,298, 4,371, 4,400, 4,500, 4,600, 4,700, 4,800, 4,900"></svg></body></html>`))
	}))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), PageBase: srv.URL + "/"}
	m := f.Fetch(context.Background(), mustRef(t, "a/b/c"))
	if m.Name != "pptx" || m.Description != "Make decks" || m.Installs != 239629 {
		t.Fatalf("%+v", m)
	}
	if len(m.Trend) != 8 || m.Trend[0] != 4298 || m.Trend[7] != 4900 || m.InstallsWeek != 4900 {
		t.Fatalf("trend: %+v", m)
	}
}

func TestFetchPageOGTitleFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<meta property="og:title" content="pptx — anthropics/skills"><meta name="description" content="Desc">`))
	}))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), PageBase: srv.URL + "/"}
	m := f.Fetch(context.Background(), mustRef(t, "a/b/c"))
	if m.Name != "pptx" || m.Installs != 0 || len(m.Trend) != 0 {
		t.Fatalf("%+v", m)
	}
}

func TestDescriptionDecodedButNeverMarkup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<meta name="description" content="Fish &amp; chips &lt;b&gt;x&lt;/b&gt;">`))
	}))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), PageBase: srv.URL + "/"}
	m := f.Fetch(context.Background(), mustRef(t, "a/b/c"))
	if m.Description != "Fish & chips <b>x</b>" {
		t.Fatalf("%q", m.Description)
	}
}
