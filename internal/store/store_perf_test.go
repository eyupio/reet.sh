package store

import (
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
)

// benchStore opens a fresh store outside the testing.T-only template helper so
// it can be used by benchmarks as well.
func benchStore(b *testing.B) *Store {
	b.Helper()
	s, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	return s
}

func BenchmarkListFor(b *testing.B) {
	s := benchStore(b)
	u, err := s.CreateUser("bench@example.com", "bench", "owner", "hash")
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
		if _, err := s.TagManyFor(u.ID, []string{r.ID}, []string{fmt.Sprintf("tag-%d", i%10)}, nil); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 20 {
		list, err := s.CreateList(u.ID, fmt.Sprintf("List %02d", i), "", "members")
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
		if _, _, err := s.ListFor(u.ID, Filter{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAppendTerminalChatEventFor(b *testing.B) {
	s := benchStore(b)
	u, err := s.CreateUser("bench@example.com", "bench", "owner", "hash")
	if err != nil {
		b.Fatal(err)
	}
	event := []byte(`{"kind":"agent","text":"hello"}`)
	b.ResetTimer()
	for i := range b.N {
		if err := s.AppendTerminalChatEventFor(u.ID, "bench-session-0001", int64(i+1), int64(i+1), event); err != nil {
			b.Fatal(err)
		}
	}
}

func TestTrimTerminalChatEventsFor(t *testing.T) {
	s := newStore(t)
	uid, err := s.CreateUser("trim@example.com", "trim", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	session := "trim-session-0001"
	for seq := int64(1); seq <= 5; seq++ {
		if err := s.AppendTerminalChatEventFor(uid.ID, session, seq, seq, []byte(`{"kind":"agent"}`)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.TrimTerminalChatEventsFor(uid.ID, session, 3); err != nil {
		t.Fatal(err)
	}
	events, err := s.TerminalChatEventsFor(uid.ID, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Seq != 3 || events[2].Seq != 5 {
		t.Fatalf("trimmed events = %+v", events)
	}
}

func TestListForLimitAndOffset(t *testing.T) {
	s := newStore(t)
	u, err := s.CreateUser("page@example.com", "page", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		r := ref.Ref{
			ID:         fmt.Sprintf("page/repo/skill-%d", i),
			Source:     "page/repo",
			Slug:       fmt.Sprintf("skill-%d", i),
			SourceType: "github",
			URL:        fmt.Sprintf("https://example.com/skill-%d", i),
		}
		if _, err := s.UpsertFor(u.ID, r, Patch{}); err != nil {
			t.Fatal(err)
		}
	}
	all, counts, err := s.ListFor(u.ID, Filter{})
	if err != nil || len(all) != 3 || counts.All != 3 {
		t.Fatalf("all: %d %+v %v", len(all), counts, err)
	}
	page, counts, err := s.ListFor(u.ID, Filter{Limit: 1, Offset: 1})
	if err != nil || len(page) != 1 || counts.All != 3 {
		t.Fatalf("page: %d %+v %v", len(page), counts, err)
	}
}

func BenchmarkListForCompact(b *testing.B) {
	s := benchStore(b)
	u, err := s.CreateUser("bench-compact@example.com", "bench", "owner", "hash")
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
		if _, err := s.TagManyFor(u.ID, []string{r.ID}, []string{fmt.Sprintf("tag-%d", i%10)}, nil); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 20 {
		list, err := s.CreateList(u.ID, fmt.Sprintf("List %02d", i), "", "members")
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
		if _, _, err := s.ListFor(u.ID, Filter{Compact: true}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestAppendTerminalChatEventsForBatch(t *testing.T) {
	s := newStore(t)
	u, err := s.CreateUser("batch@example.com", "batch", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	session := "batch-session-0001"
	events := []StoredChatEvent{
		{Seq: 1, At: 1, Event: []byte(`{"kind":"agent"}`)},
		{Seq: 2, At: 2, Event: []byte(`{"kind":"agent"}`)},
		{Seq: 3, At: 3, Event: []byte(`{"kind":"agent"}`)},
	}
	if err := s.AppendTerminalChatEventsFor(u.ID, session, events); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendTerminalChatEventsFor(u.ID, session, events); err != nil {
		t.Fatal(err)
	}
	stored, err := s.TerminalChatEventsFor(u.ID, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 3 {
		t.Fatalf("stored %d events, want 3", len(stored))
	}
}

func BenchmarkAppendTerminalChatEventsFor32(b *testing.B) {
	s := benchStore(b)
	u, err := s.CreateUser("bench-batch@example.com", "bench", "owner", "hash")
	if err != nil {
		b.Fatal(err)
	}
	event := []byte(`{"kind":"agent","text":"hello"}`)
	batch := make([]StoredChatEvent, 32)
	for i := range batch {
		batch[i] = StoredChatEvent{At: int64(i + 1), Event: event}
	}
	base := int64(0)
	b.ResetTimer()
	for range b.N {
		for i := range batch {
			base++
			batch[i].Seq = base
		}
		if err := s.AppendTerminalChatEventsFor(u.ID, "bench-batch-0001", batch); err != nil {
			b.Fatal(err)
		}
	}
}

// List rows must retain independent metadata when the SQL scan buffer is reused.
func TestListForIndependentMetadata(t *testing.T) {
	s := newStore(t)
	u, err := s.CreateUser("independent@example.com", "independent", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		r := mustRef(t, fmt.Sprintf("rows/repo/skill-%d", i))
		if _, err := s.UpsertFor(u.ID, r, Patch{Liked: bp(i == 0), Bookmarked: bp(i == 1), Used: bp(i == 2)}); err != nil {
			t.Fatal(err)
		}
		var trend []int64
		if i != 1 {
			trend = []int64{int64(i), 10}
		}
		if err := s.SetMeta(r.ID, fmt.Sprintf("Row %d", i), "", int64(i), int64(i), trend); err != nil {
			t.Fatal(err)
		}
		if _, err := s.TagManyFor(u.ID, []string{r.ID}, []string{fmt.Sprintf("tag-%d", i)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []Filter{{Sort: "name"}, {Sort: "name", Limit: 3}, {Sort: "name", Compact: true}} {
		rows, _, err := s.ListFor(u.ID, f)
		if err != nil || len(rows) != 3 {
			t.Fatalf("rows: %v %v", rows, err)
		}
		ids := map[string]bool{}
		for i, row := range rows {
			if row.ID != fmt.Sprintf("rows/repo/skill-%d", i) || row.Name != fmt.Sprintf("Row %d", i) || row.Liked != (i == 0) || row.Bookmarked != (i == 1) || row.Used != (i == 2) {
				t.Fatalf("row %d: %+v", i, row)
			}
			if row.LibraryItemID == "" || ids[row.LibraryItemID] {
				t.Fatalf("non-independent canonical identity: %+v", rows)
			}
			ids[row.LibraryItemID] = true
			if len(row.Tags) != 1 || row.Tags[0].Name != fmt.Sprintf("tag-%d", i) {
				t.Fatalf("tags %d: %+v", i, row.Tags)
			}
			if i == 1 {
				if len(row.Trend) != 0 {
					t.Fatalf("previous trend leaked: %+v", row)
				}
			} else if len(row.Trend) != 2 || row.Trend[0] != int64(i) {
				t.Fatalf("trend %d: %+v", i, row.Trend)
			}
		}
		rows[0].Trend[0] = 99
		if rows[2].Trend[0] != 2 {
			t.Fatal("trend slices alias")
		}
	}
}
