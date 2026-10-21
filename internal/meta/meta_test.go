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
		if m := f.Fetch(context.Background(), mustRef(t, "a/b/c")); m != (Meta{}) {
			t.Errorf("%s: got %+v", name, m)
		}
		srv.Close()
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
