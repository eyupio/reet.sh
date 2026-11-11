package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
)

// Gaffer's use of reet's own tools. The tools are the MCP tool definitions,
// run in-process as the account that owns the chat, under the same checks an
// MCP client with management scope gets. Which tools Gaffer may use, and
// whether each asks first, is the person's choice in Gaffer settings; every
// tool starts off. The tools that read or message a working session through
// a per-connection grant are not offered: Gaffer has its own, per-session
// grants for those.

// gafferToolGroup names the settings group a tool is listed under.
func gafferToolGroup(name string, readOnly bool) string {
	switch {
	case mcpFactoryReads[name]:
		return "Factory: read"
	case name == "reet_approve_pipeline_merge":
		return "Factory: merge"
	case name == "reet_start_outcome" || name == "reet_add_pipelines":
		return "Factory: start agents"
	case mcpFactoryWrites[name]:
		return "Factory: control"
	case strings.Contains(name, "machine") || strings.Contains(name, "environment") || strings.Contains(name, "sync"):
		if readOnly {
			return "Machines: read"
		}
		return "Machines: change"
	case readOnly:
		return "Shelf and activity: read"
	}
	return "Shelf: change"
}

func gafferToolOffered(name string) bool { return !mcpWorkerTool(name) && !mcpContextTool(name) }

type gafferToolView struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"readOnly"`
	Destructive bool   `json:"destructive"`
	Group       string `json:"group"`
	Mode        string `json:"mode"`
}

// gafferToolViews lists every tool Gaffer could be given, with the account's choice for each.
func (s *server) gafferToolViews(uid int64) ([]gafferToolView, error) {
	modes, err := s.st.GafferToolModesFor(uid)
	if err != nil {
		return nil, err
	}
	out := []gafferToolView{}
	for _, tool := range mcpToolDefinitions {
		if !gafferToolOffered(tool.Name) {
			continue
		}
		readOnly, destructive := false, false
		if tool.Annotations != nil {
			readOnly = tool.Annotations.ReadOnlyHint
			destructive = tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
		}
		mode := modes[tool.Name]
		if mode != "ask" && mode != "allow" {
			mode = "off"
		}
		out = append(out, gafferToolView{Name: tool.Name, Title: tool.Title, Description: tool.Description, ReadOnly: readOnly, Destructive: destructive, Group: gafferToolGroup(tool.Name, readOnly), Mode: mode})
	}
	return out, nil
}

type gafferToolHost struct{ s *server }

func (h gafferToolHost) access(uid int64) (mcpAccess, bool) {
	user, ok, err := h.s.st.UserByID(uid)
	if err != nil || !ok || user.Role == "viewer" {
		return mcpAccess{}, false
	}
	return mcpAccess{User: user, Connection: store.MCPConnection{Name: "Gaffer", Scope: "write", Kind: "gaffer"}}, true
}

func (h gafferToolHost) GafferTools(uid int64) []gaffer.HostTool {
	if _, ok := h.access(uid); !ok {
		return nil
	}
	views, err := h.s.gafferToolViews(uid)
	if err != nil {
		return nil
	}
	schemas := map[string]json.RawMessage{}
	for _, tool := range mcpToolDefinitions {
		if raw, err := json.Marshal(tool.InputSchema); err == nil {
			schemas[tool.Name] = raw
		}
	}
	out := []gaffer.HostTool{}
	for _, view := range views {
		if view.Mode == "off" || schemas[view.Name] == nil {
			continue
		}
		out = append(out, gaffer.HostTool{Name: view.Name, Title: view.Title, Description: view.Description, Parameters: schemas[view.Name], ReadOnly: view.ReadOnly, Mode: view.Mode})
	}
	return out
}

func (h gafferToolHost) CallGafferTool(ctx context.Context, uid int64, name string, raw json.RawMessage) (string, error) {
	access, ok := h.access(uid)
	if !ok {
		return "", errors.New("This account cannot use reet's tools.")
	}
	modes, err := h.s.st.GafferToolModesFor(uid)
	if err != nil {
		return "", err
	}
	if mode := modes[name]; !gafferToolOffered(name) || mode != "ask" && mode != "allow" {
		return "", errors.New("That tool is switched off for Gaffer.")
	}
	schema, ok := mcpInputSchemas[name]
	api := h.s.mcpAPI
	if !ok || api == nil {
		return "", errors.New("That tool is not available on this controller.")
	}
	args := map[string]any{}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &args); err != nil || args == nil {
			return "", errors.New("The tool's arguments are not a JSON object.")
		}
	}
	if err = schema.ApplyDefaults(&args); err != nil {
		return "", errors.New("The tool's arguments are invalid.")
	}
	if err = schema.Validate(args); err != nil {
		return "", errors.New("The arguments do not match the tool's schema: " + err.Error())
	}
	origin := strings.TrimRight(h.s.publicOrigin(), "/")
	if origin == "" {
		origin = "http://localhost"
	}
	data, err := h.s.executeMCP(ctx, api, access, origin, name, args)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(data)
	return string(out), err
}

func (s *server) registerGafferToolRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{
			accounts: "Gaffer’s tools need a browser account.", signedOut: "Sign in to choose Gaffer’s tools.", readOnly: "This account is read-only, so Gaffer cannot be given tools.",
		}, h)
	}
	mux.HandleFunc("GET /api/v2/gaffer/tools", access(s.gafferToolsGet))
	mux.HandleFunc("PUT /api/v2/gaffer/tools", access(s.gafferToolsPut))
	mux.HandleFunc("POST /api/v2/gaffer/conversations/{id}/turns/{turnId}/tools/{toolId}", access(s.gafferToolAnswer))
}

func (s *server) gafferToolsGet(w http.ResponseWriter, r *http.Request) {
	views, err := s.gafferToolViews(s.uid(r))
	if err != nil {
		writeErr(w, 500, "Gaffer’s tools could not be read.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"tools": views})
}

// gafferToolsPut replaces the account's whole choice: a tool that is left out is off.
func (s *server) gafferToolsPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Modes map[string]string `json:"modes"`
	}
	if !decodeStrictJSON(w, r, 32<<10, &in, "That choice of tools is invalid.") {
		return
	}
	known := map[string]bool{}
	for _, tool := range mcpToolDefinitions {
		known[tool.Name] = gafferToolOffered(tool.Name)
	}
	clean, summary := map[string]string{}, map[string]int{"ask": 0, "allow": 0}
	for name, mode := range in.Modes {
		if mode == "off" || mode == "" {
			continue
		}
		if !known[name] || mode != "ask" && mode != "allow" {
			writeErr(w, 400, "Choose Off, Ask me or Allow for tools Gaffer can be given.")
			return
		}
		clean[name] = mode
		summary[mode]++
	}
	if err := s.st.SetGafferToolModesFor(s.uid(r), clean); err != nil {
		writeErr(w, 500, "Gaffer’s tools could not be saved.")
		return
	}
	allowed := []string{}
	for name, mode := range clean {
		if mode == "allow" {
			allowed = append(allowed, name)
		}
	}
	s.eventRequest(r, "gaffer-tools-changed", "", map[string]any{"ask": summary["ask"], "allow": summary["allow"], "allowed": allowed})
	s.gafferToolsGet(w, r)
}

// gafferToolAnswer is the person approving or declining one call Gaffer asked to make.
func (s *server) gafferToolAnswer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Approve *bool `json:"approve"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That answer is invalid.") {
		return
	}
	if in.Approve == nil {
		writeErr(w, 400, "Say whether Gaffer may run it.")
		return
	}
	turn, err := s.gafferProviders.AnswerTool(r.Context(), s.uid(r), r.PathValue("id"), r.PathValue("turnId"), r.PathValue("toolId"), *in.Approve)
	if err != nil {
		gafferError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, turn)
}
