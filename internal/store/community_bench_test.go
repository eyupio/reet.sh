package store

import (
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
)

func BenchmarkCommunity(b *testing.B) {
	s, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	u, err := s.CreateUser("community-bench@example.com", "community-bench", "owner", "hash")
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
		if _, err := s.UpsertFor(u.ID, r, Patch{}); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 20 {
		list, err := s.CreateList(u.ID, fmt.Sprintf("Public %02d", i), "", "public")
		if err != nil {
			b.Fatal(err)
		}
		for j := range 10 {
			if err := s.AddListSkill(list.ID, fmt.Sprintf("bench/repo/skill-%03d", (i*10+j)%200), "", u.ID); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ResetTimer()
	for range b.N {
		if _, _, err := s.Community(); err != nil {
			b.Fatal(err)
		}
	}
}
