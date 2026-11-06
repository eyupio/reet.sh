package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
)

func mcpContextTool(name string) bool {
	return name == "reet_list_context_access" || name == "reet_get_context"
}

func (s *server) registerContextRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{accounts: "Context sharing requires a browser account.", signedOut: "Sign in to manage context sharing.", readOnly: "A read-only account cannot grant session access."}, h)
	}
	for _, path := range []string{"/api/v2/gaffer/conversations/{id}/context", "/api/v2/mcp/connections/{id}/context", "/api/v2/mcp/browser/context"} {
		mux.HandleFunc("GET "+path, access(s.contextSettings))
		mux.HandleFunc("PUT "+path, access(s.contextSave))
		mux.HandleFunc("POST "+path+"/preview", access(s.contextPreview))
	}
}

func (s *server) contextTarget(r *http.Request) (string, string, string, error) {
	if strings.HasPrefix(r.URL.Path, "/api/v2/gaffer/") {
		c, err := s.st.GafferConversationFor(s.uid(r), r.PathValue("id"))
		if err != nil {
			return "", "", "", err
		}
		if r.URL.Query().Has("sessionId") && r.URL.Query().Get("sessionId") != c.SessionID {
			return "", "", "", store.ErrNotFound
		}
		return "gaffer", c.ID, c.SessionID, nil
	}
	if strings.HasPrefix(r.URL.Path, "/api/v2/mcp/browser/") {
		return "browser", "browser", r.URL.Query().Get("sessionId"), nil
	}
	return "mcp", r.PathValue("id"), r.URL.Query().Get("sessionId"), nil
}

func (s *server) contextSettings(w http.ResponseWriter, r *http.Request) {
	kind, subject, session, err := s.contextTarget(r)
	if err != nil {
		gafferError(w, err)
		return
	}
	v, err := s.gafferProviders.ContextSettings(s.uid(r), kind, subject, session)
	if err != nil {
		gafferError(w, err)
		return
	}
	out := map[string]any{"context": v}
	if kind != "gaffer" {
		grants, err := s.st.ContextGrantsFor(s.uid(r), kind, subject)
		if err != nil {
			gafferError(w, err)
			return
		}
		choices, err := s.st.ContextSessionChoicesFor(s.uid(r))
		if err != nil {
			gafferError(w, err)
			return
		}
		out["scopes"] = grants
		out["sessions"] = choices
	}
	writeJSON(w, 200, out)
}

func (s *server) contextSave(w http.ResponseWriter, r *http.Request) {
	kind, subject, session, err := s.contextTarget(r)
	if err != nil {
		gafferError(w, err)
		return
	}
	var in gaffer.ContextInput
	if !decodeStrictJSON(w, r, 2048, &in, "Reload the context permissions before saving.") {
		return
	}
	v, err := s.gafferProviders.ConfigureContext(s.uid(r), kind, subject, session, in)
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "context-sharing-changed", "", map[string]any{"kind": kind, "subject": subject, "session": session, "status": v.Grant.Status, "history": v.Grant.History, "text": v.Grant.Text, "revision": v.Grant.Revision})
	writeJSON(w, 200, map[string]any{"context": v})
}

func (s *server) contextPreview(w http.ResponseWriter, r *http.Request) {
	kind, subject, session, err := s.contextTarget(r)
	if err != nil {
		gafferError(w, err)
		return
	}
	var flags store.ContextFlags
	if !decodeStrictJSON(w, r, 1024, &flags, "Choose the context to preview.") {
		return
	}
	v, err := s.gafferProviders.PreviewContext(s.uid(r), kind, subject, session, flags)
	if err != nil {
		gafferError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"context": v})
}

func (s *server) executeMCPContext(access mcpAccess, name string, args map[string]any) (map[string]any, error) {
	kind, subject := "", ""
	if access.Connection.ID > 0 {
		kind = "mcp"
		subject = strconv.FormatInt(access.Connection.ID, 10)
	} else if access.Connection.Kind == "browser" {
		kind = "browser"
		subject = "browser"
	} else {
		return nil, gaffer.ErrContextForbidden
	}
	if name == "reet_list_context_access" {
		grants, err := s.gafferProviders.MCPContextAccess(access.User.ID, kind, subject)
		return map[string]any{"scopes": grants}, err
	}
	session, _ := args["session_id"].(string)
	v, err := s.gafferProviders.ReadMCPContext(access.User.ID, kind, subject, session)
	if err != nil {
		return nil, err
	}
	// Convert through JSON so native and stdio structured results have identical
	// shapes, including optional fields and integer encoding.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(raw, &out)
	return map[string]any{"context": out}, err
}
