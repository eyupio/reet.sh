package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTopicsReadsIndexAndPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/topic":
			w.Write([]byte(`<a href="/topic/design">Design</a><a href="/topic/testing">Testing</a>`))
		case "/topic/design":
			w.Write([]byte(`<title>Design &amp; UI skills — skills.sh</title>
				<a href="/anthropics/skills/frontend-design">one</a>
				<a href="/site/open.feishu.cn/lark-doc">two</a>
				<a href="/topic/design">topic</a>`))
		case "/topic/testing":
			w.Write([]byte(`<title>Testing skills — skills.sh</title><a href="/vercel-labs/skills/find-skills">one</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), PageBase: srv.URL + "/"}
	topics, err := f.Topics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 2 || topics[0].Name != "Design & UI" || topics[1].Name != "Testing" {
		t.Fatalf("%+v", topics)
	}
	if len(topics[0].Skills) != 2 || topics[0].Skills[0] != "anthropics/skills/frontend-design" || topics[0].Skills[1] != "open.feishu.cn/lark-doc" {
		t.Fatalf("design: %+v", topics[0].Skills)
	}
}
