package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) event(userID int64, source, action, skillID string, detail any) {
	if err := s.st.AddEventFor(userID, source, action, skillID, detail); err != nil && s.debug {
		log.Printf("event store action=%s error=%v", action, err)
	}
}

// eventRequest attributes a change to the MCP client that made it while keeping
// the existing UI source for browser-originated calls.
func (s *server) eventRequest(r *http.Request, action, skillID string, detail any) {
	source := "ui"
	if strings.EqualFold(r.Header.Get("X-Reet-Source"), "mcp") {
		source = "mcp"
	}
	if client := strings.TrimSpace(r.Header.Get("X-Reet-Client")); client != "" {
		switch d := detail.(type) {
		case nil:
			detail = map[string]any{"client": client}
		case map[string]any:
			d["client"] = client
			detail = d
		case map[string]string:
			merged := make(map[string]any, len(d)+1)
			for k, v := range d {
				merged[k] = v
			}
			merged["client"] = client
			detail = merged
		default:
			detail = map[string]any{"detail": detail, "client": client}
		}
	}
	s.event(s.uid(r), source, action, skillID, detail)
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 20
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	after, _ := strconv.ParseInt(q.Get("after"), 10, 64)
	// exclude=scan,machine leaves those sources out; at most eight are read.
	var exclude []string
	for _, source := range strings.Split(q.Get("exclude"), ",") {
		if source = strings.TrimSpace(source); source != "" && len(source) <= 32 && len(exclude) < 8 {
			exclude = append(exclude, source)
		}
	}
	events, hasMore, err := s.st.ListEventsPageFor(s.uid(r), store.EventQuery{
		Limit: limit, SkillID: q.Get("skill"), Action: q.Get("action"), Source: q.Get("source"), Exclude: exclude,
		Before: before, After: after,
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := map[string]any{"events": events, "has_more": hasMore, "next_before": nil, "next_after": nil}
	if len(events) > 0 {
		out["next_before"] = events[0].ID
		out["next_after"] = events[0].ID
		oldest, newest := events[0].ID, events[0].ID
		for _, ev := range events {
			if ev.ID < oldest {
				oldest = ev.ID
			}
			if ev.ID > newest {
				newest = ev.ID
			}
		}
		out["next_before"] = oldest
		out["next_after"] = newest
	}
	writeJSON(w, 200, out)
}
