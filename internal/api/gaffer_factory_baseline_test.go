package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
)

// TestFactoryRefreshBaseline is a measurement, not a test: it asserts nothing
// and runs only with REET_FACTORY_BASELINE=1. It builds the stage 0 synthetic
// Factory (6 outcomes × 5 pipelines × 8 stages, three handed over in each)
// and logs what the Factory page's two 5-second reads cost: bytes, gzip bytes,
// the headers that bear on caching and compression, the handler's time, and
// where the outcome read's bytes are.
//
//	REET_FACTORY_BASELINE=1 go test ./internal/api/ -run TestFactoryRefreshBaseline -v -count=1
func TestFactoryRefreshBaseline(t *testing.T) {
	if os.Getenv("REET_FACTORY_BASELINE") != "1" {
		t.Skip("a measurement: set REET_FACTORY_BASELINE=1 to run it")
	}
	for _, v := range []struct {
		name               string
		criteria, evidence int
	}{{"no criteria", 0, 0}, {"5 criteria per pipeline", 5, 0}, {"5 criteria, one evidence note each", 5, 1}} {
		t.Run(v.name, func(t *testing.T) { factoryBaseline(t, v.criteria, v.evidence) })
	}
}

// The lengths the synthetic Factory uses, in bytes of text.
const (
	baselineObjective = 600
	baselineTitle     = 40
	baselinePrompt    = 1500
	baselineNote      = 100
	baselineTierWhy   = 100
	baselineSummary   = 2000 // a handed-over stage: maxOutcomeSummaryBytes
	baselineCriterion = 120
	baselineEvidence  = 300
)

func factoryBaseline(t *testing.T, criteria, evidence int) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	jar := mustJar(t)
	owner := &http.Client{Jar: jar}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	words := rand.New(rand.NewPCG(1, 2))
	for o := range 6 {
		created, err := st.CreateGafferOutcomeFor(1, baselineOutcome(words, o, criteria, evidence))
		if err != nil {
			t.Fatal(err)
		}
		// Saved as starting; give it the shape of an outcome under way, which also lets the next one be created.
		want := baselineOutcome(words, o, criteria, evidence)
		if _, err := st.UpdateGafferOutcomeFor(1, created.ID, created.Revision, func(have *store.GafferOutcome) bool {
			have.State, have.Note, have.Sessions = "settled", want.Note, want.Sessions
			return true
		}); err != nil {
			t.Fatal(err)
		}
	}
	u, _ := url.Parse(srv.URL)
	for _, path := range []string{"/api/v2/gaffer/outcomes", "/api/v2/gaffer/goals"} {
		// Asked as a browser asks: the transport must not decompress, so Content-Encoding is seen as sent.
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		req.Header.Set("Origin", "http://localhost:8080")
		resp, err := (&http.Client{Jar: jar, Transport: &http.Transport{DisableCompression: true}}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, body)
		}
		var zipped bytes.Buffer
		zw := gzip.NewWriter(&zipped)
		_, _ = zw.Write(body)
		_ = zw.Close()
		// The handler alone, on the same request, 20 times.
		var times []time.Duration
		for range 20 {
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("Origin", "http://localhost:8080")
			for _, c := range jar.Cookies(u) {
				r.AddCookie(c)
			}
			rec := httptest.NewRecorder()
			start := time.Now()
			h.ServeHTTP(rec, r)
			times = append(times, time.Since(start))
			if rec.Code != 200 {
				t.Fatalf("%s handler: %d", path, rec.Code)
			}
		}
		slices.Sort(times)
		t.Logf("%s: %d bytes, %d gzip bytes (%.1f%%); Content-Encoding %q; Cache-Control %q, ETag %q, Last-Modified %q, Vary %q; handler median %v, worst %v of 20",
			path, len(body), zipped.Len(), 100*float64(zipped.Len())/float64(len(body)), resp.Header.Get("Content-Encoding"),
			resp.Header.Get("Cache-Control"), resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), resp.Header.Get("Vary"),
			times[len(times)/2].Round(time.Microsecond), times[len(times)-1].Round(time.Microsecond))
		if path == "/api/v2/gaffer/outcomes" {
			baselineBreakdown(t, body)
		}
	}
}

// baselineBreakdown logs where the outcome read's bytes are: per outcome, per
// pipeline, per stage, and the fields carrying the most.
func baselineBreakdown(t *testing.T, body []byte) {
	var doc struct {
		Outcomes []map[string]json.RawMessage `json:"outcomes"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	var outcomes, pipelines, stageBytes []int
	var pipelineOwn int
	stages := map[string][]int{}
	for _, o := range doc.Outcomes {
		raw, _ := json.Marshal(o)
		outcomes = append(outcomes, len(raw))
		var sessions []map[string]json.RawMessage
		_ = json.Unmarshal(o["sessions"], &sessions)
		for _, p := range sessions {
			raw, _ := json.Marshal(p)
			pipelines = append(pipelines, len(raw))
			pipelineOwn += len(raw) - len(p["stages"])
			var list []map[string]any
			_ = json.Unmarshal(p["stages"], &list)
			for _, s := range list {
				raw, _ := json.Marshal(s)
				stageBytes = append(stageBytes, len(raw))
				state, _ := s["state"].(string)
				stages[state] = append(stages[state], len(raw))
			}
		}
	}
	mean := func(n []int) int {
		sum := 0
		for _, x := range n {
			sum += x
		}
		if len(n) == 0 {
			return 0
		}
		return sum / len(n)
	}
	t.Logf("  per outcome: %d bytes on average (%d outcomes); outcome fields other than sessions: %d bytes on average", mean(outcomes), len(outcomes), mean(outcomes)-5*mean(pipelines))
	t.Logf("  per pipeline: %d bytes on average (%d pipelines); of which its stages %d, the rest %d", mean(pipelines), len(pipelines), mean(pipelines)-pipelineOwn/max(1, len(pipelines)), pipelineOwn/max(1, len(pipelines)))
	for _, state := range []string{"done", "running", "pending"} {
		t.Logf("  per stage %s: %d bytes on average (%d stages)", state, mean(stages[state]), len(stages[state]))
	}
	// Each field's encoded name and value, summed over the document. A field holding others counts them too.
	fields := map[string]int{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, inner := range x {
				raw, _ := json.Marshal(inner)
				fields[k] += len(k) + 3 + len(raw)
				walk(inner)
			}
		case []any:
			for _, inner := range x {
				walk(inner)
			}
		}
	}
	var whole any
	_ = json.Unmarshal(body, &whole)
	walk(whole)
	names := make([]string, 0, len(fields))
	for k := range fields {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return fields[names[i]] > fields[names[j]] })
	var top []string
	for _, k := range names[:min(15, len(names))] {
		top = append(top, fmt.Sprintf("%s %d (%.1f%%)", k, fields[k], 100*float64(fields[k])/float64(len(body))))
	}
	t.Logf("  top fields by encoded bytes (nested fields counted in their parents too): %s", strings.Join(top, ", "))
	// An estimate of a list-only read: the same document without what only the open pipeline's detail, or nothing
	// on the page, reads (handovers, prompts, request ids, work folders, criteria text: the list needs only a count).
	var strip func(any)
	strip = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, k := range []string{"summary", "prompt", "requestId", "workDir", "report", "decisions", "skillGuidance", "brief"} {
				delete(x, k)
			}
			if list, ok := x["criteria"].([]any); ok {
				x["criteria"] = len(list)
			}
			for _, inner := range x {
				strip(inner)
			}
		case []any:
			for _, inner := range x {
				strip(inner)
			}
		}
	}
	strip(whole)
	compact, _ := json.Marshal(whole)
	var zipped bytes.Buffer
	zw := gzip.NewWriter(&zipped)
	_, _ = zw.Write(compact)
	_ = zw.Close()
	t.Logf("  list-only estimate: %d bytes (%.1f%%), %d gzip bytes", len(compact), 100*float64(len(compact))/float64(len(body)), zipped.Len())
}

// baselineText is plain prose of n bytes, drawn from a small vocabulary so it
// compresses about as well as real notes do, not as well as a repeated phrase.
func baselineText(r *rand.Rand, n int) string {
	vocab := strings.Fields("the export reads ledger page each line row totals billing csv endpoint test tests handler store migration review branch pull request commit check passes fails error case empty account owner date range header column format writes returns handles invalid limit retry timeout queue worker stage plan design code merge approve before after with without only every one two three it is was are and or but not so then also file files function change changes keeps adds removes says shows from into for of on in at by to a an")
	var b strings.Builder
	for b.Len() < n {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(vocab[r.IntN(len(vocab))])
	}
	return b.String()[:n]
}

func baselineOutcome(r *rand.Rand, o, criteria, evidence int) store.GafferOutcome {
	now := time.Now().UnixMilli()
	out := store.GafferOutcome{
		ID: fmt.Sprintf("%031x%d", 0xfac70, o), Objective: baselineText(r, baselineObjective), Note: baselineText(r, baselineNote),
		AgentID: "connector-baseline00000001", ProjectID: "project-billing", ProjectName: "billing", Dir: "/home/dev/work/billing",
		Tool: "opencode", Model: "anthropic/claude-sonnet-4-5", ProviderID: strings.Repeat("b", 32), MaxReviews: 12, Hours: 24,
	}
	for p := range 5 {
		piece := store.GafferOutcomeSession{
			Key: string(rune('a' + p)), Title: baselineText(r, baselineTitle), Prompt: baselineText(r, baselinePrompt),
			RequestID: terminalbroker.ID()[:32], State: "following", Note: baselineText(r, baselineNote),
			SessionID: terminalbroker.ID(), ConversationID: terminalbroker.ID()[:32], UpdatedAt: now, Stage: 3,
			WorkDir: "/home/dev/work/billing-" + string(rune('a'+p)),
		}
		for s, kind := range gaffer.StageKinds {
			stage := store.GafferOutcomeStage{Kind: kind, Tool: "opencode", Model: "anthropic/claude-sonnet-4-5", Effort: "high", Tier: "standard",
				TierWhy: baselineText(r, baselineTierWhy), RequestID: terminalbroker.ID()[:32], State: "pending"}
			switch {
			case s < 3:
				stage.State, stage.Summary, stage.ClosedBy = "done", baselineText(r, baselineSummary), "gaffer"
				stage.SessionID, stage.ConversationID, stage.StartedAt, stage.EndedAt = terminalbroker.ID(), terminalbroker.ID()[:32], now-3600000, now-1800000
				stage.Reviews, stage.InputTokens, stage.OutputTokens, stage.UsageReported = 3, 41_000, 2_300, true
				stage.AgentInputTokens, stage.AgentOutputTokens, stage.AgentCachedTokens, stage.AgentUsageReported = 1_200_000, 48_000, 900_000, true
			case s == 3:
				stage.State, stage.SessionID, stage.ConversationID, stage.StartedAt = "running", piece.SessionID, piece.ConversationID, now-600000
				stage.ContextUsed, stage.ContextSize = 84_000, 200_000
			}
			piece.Stages = append(piece.Stages, stage)
		}
		for c := range criteria {
			id := fmt.Sprintf("crit%028x", c)
			piece.Criteria = append(piece.Criteria, store.GafferCriterion{ID: id, Text: baselineText(r, baselineCriterion), Revision: 1})
			for e := range evidence {
				piece.Evidence = append(piece.Evidence, store.GafferEvidence{ID: fmt.Sprintf("evid%024x%04x", c, e), CriterionID: id, Kind: "note", Verdict: "passed",
					Text: baselineText(r, baselineEvidence), CriteriaRevision: 1, By: 1, ByName: "Owner", At: now})
			}
		}
		if criteria > 0 {
			piece.CriteriaRevision, piece.ReviewRevision = 1, 1
		}
		out.Sessions = append(out.Sessions, piece)
	}
	return out
}
