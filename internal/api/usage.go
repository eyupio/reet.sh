package api

import (
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The Usage page's read: what each agent reported using in a range the
// browser chooses (it knows the person's time zone, so it says where a day
// starts), the sessions behind those figures, and Gaffer's own replies. It
// reads only the controller's own record and asks no machine, so it answers
// while remote terminals are off, as the session list does. Like the session
// list it is for an account browser session that is not a viewer.

const (
	// usageMaxRange is the longest range a read may ask for: 30 days with
	// room for a time-zone change, and the 90 days events are kept.
	usageMaxRange = 92 * 24 * 60 * 60 * 1000
	// usageSessionsMax bounds the session list; the answer says when it was cut.
	usageSessionsMax = 200
)

func (s *server) registerUsageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/usage", s.terminalAccess(s.sameOriginRead(s.usage), true))
}

type usageAgent struct {
	Tool string `json:"tool"`
	Name string `json:"name"`
	// Reports is what reet can count for the agent: "chat" (it reports tokens
	// in a chat) or "terminal_only" (it has no chat here, and a terminal
	// reports nothing). An agent reet no longer starts that still has
	// figures in the range reports "retired".
	Reports       string `json:"reports"`
	Input         int64  `json:"input"`
	Output        int64  `json:"output"`
	Cached        int64  `json:"cached"`
	Sessions      int    `json:"sessions"`
	FactoryStages int    `json:"factoryStages"`
	FirstAt       int64  `json:"firstAt,omitempty"`
}

type usageGaffer struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Input    int64  `json:"input"`
	Output   int64  `json:"output"`
	Replies  int    `json:"replies"`
}

type usageSession struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Tool        string  `json:"tool"`
	Model       string  `json:"model"`
	AgentID     string  `json:"agentId"`
	Host        string  `json:"host"`
	ProjectID   *string `json:"projectId"`
	Structured  bool    `json:"structured"`
	State       string  `json:"state"`
	Archived    bool    `json:"archived"`
	Factory     bool    `json:"factory"`
	Input       int64   `json:"input"`
	Output      int64   `json:"output"`
	Cached      int64   `json:"cached"`
	LastAt      int64   `json:"lastAt"`
	ContextUsed int64   `json:"contextUsed"`
	ContextSize int64   `json:"contextSize"`
}

func usageReports(tool string) string {
	switch {
	case terminalproto.ReviewedChatTool(tool):
		return "chat"
	case terminalproto.ValidToolV2(tool):
		return "terminal_only"
	}
	return "retired"
}

func (s *server) usage(w http.ResponseWriter, r *http.Request) {
	q, ok := machineQuery(r, "from", "to")
	if !ok || !q.Has("from") || !q.Has("to") {
		writeErr(w, http.StatusBadRequest, "usage needs a from and a to, in milliseconds")
		return
	}
	from, errFrom := strconv.ParseInt(q.Get("from"), 10, 64)
	to, errTo := strconv.ParseInt(q.Get("to"), 10, 64)
	if errFrom != nil || errTo != nil || from < 0 || to <= from || to-from > usageMaxRange {
		writeErr(w, http.StatusBadRequest, "usage covers a range of up to 92 days that ends after it starts")
		return
	}
	report, err := s.st.UsageFor(s.uid(r), from, to, usageSessionsMax)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "usage could not be read")
		return
	}
	totals := map[string]usageAgent{}
	for _, t := range report.Tools {
		totals[t.Tool] = usageAgent{Tool: t.Tool, Input: t.Input, Output: t.Output, Cached: t.Cached, Sessions: t.Sessions, FactoryStages: t.FactoryStages, FirstAt: t.FirstAt}
	}
	agents := []usageAgent{}
	add := func(tool string) {
		a := totals[tool]
		a.Tool, a.Name, a.Reports = tool, terminalproto.ToolName(tool), usageReports(tool)
		agents = append(agents, a)
		delete(totals, tool)
	}
	for _, tool := range terminalproto.LaunchTools() {
		add(tool)
	}
	// Figures from an agent reet no longer starts still count, after the rest.
	for _, t := range report.Tools {
		if _, left := totals[t.Tool]; left {
			add(t.Tool)
		}
	}
	gaffer := make([]usageGaffer, 0, len(report.Gaffer))
	for _, g := range report.Gaffer {
		gaffer = append(gaffer, usageGaffer(g))
	}
	sessions := make([]usageSession, 0, len(report.Sessions))
	for _, x := range report.Sessions {
		sessions = append(sessions, usageSession(x))
	}
	// sessionCount is each session that reported in the range once; the
	// agents' own counts add a session whose agent changed under each.
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "since": report.Since, "sessionCount": report.SessionCount, "agents": agents, "gaffer": gaffer, "sessions": sessions, "more": report.More})
}
