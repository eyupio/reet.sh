package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

func benchmarkSkillsServer(b *testing.B) *httptest.Server {
	b.Helper()
	st, err := storetest.Open(b)
	if err != nil {
		b.Fatal(err)
	}
	u, err := st.CreateUser("api-bench@example.com", "api-bench", "owner", "hash")
	if err != nil {
		b.Fatal(err)
	}
	for i := range 200 {
		r := ref.Ref{
			ID:         fmt.Sprintf("bench/repo/skill-%03d", i),
			Source:     "bench/repo",
			Slug:       fmt.Sprintf("skill-%03d", i),
			SourceType: "github",
			URL:        fmt.Sprintf("https://example.com/skill-%03d", i),
		}
		if _, err := st.UpsertFor(u.ID, r, store.Patch{}); err != nil {
			b.Fatal(err)
		}
		if _, err := st.TagManyFor(u.ID, []string{r.ID}, []string{fmt.Sprintf("tag-%d", i%10)}, nil); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 20 {
		list, err := st.CreateList(u.ID, fmt.Sprintf("List %02d", i), "", "members")
		if err != nil {
			b.Fatal(err)
		}
		for j := range 10 {
			if err := st.AddListSkill(list.ID, fmt.Sprintf("bench/repo/skill-%03d", (i*10+j)%200), "", u.ID); err != nil {
				b.Fatal(err)
			}
		}
	}
	srv := httptest.NewServer(New(st, meta.Fetcher{}, ""))
	b.Cleanup(func() { srv.Close(); _ = st.Close() })
	return srv
}

func benchmarkSkillsRequest(b *testing.B, srv *httptest.Server, path string) {
	b.Helper()
	client := srv.Client()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		resp, err := client.Get(srv.URL + path)
		if err != nil {
			b.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			b.Fatalf("%s: %s", path, resp.Status)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			resp.Body.Close()
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}

func BenchmarkListSkillsAPI(b *testing.B) {
	srv := benchmarkSkillsServer(b)
	for _, tc := range []struct {
		name string
		path string
	}{
		{"full", "/api/skills"},
		{"compact", "/api/skills?compact=1"},
		{"full_page", "/api/skills?limit=50"},
		{"compact_page", "/api/skills?compact=1&limit=50"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkSkillsRequest(b, srv, tc.path)
		})
	}
}
