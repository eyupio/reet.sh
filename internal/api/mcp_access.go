package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

type mcpAccessKey struct{}
type mcpAccess struct {
	User       store.User
	Connection store.MCPConnection
}

func (a mcpAccess) canWrite() bool { return a.Connection.Scope == "write" && a.User.Role != "viewer" }

// MCP credentials are accepted only on the tool API surface, never on maintenance or admin routes.
func mcpAPIPath(method, path string) bool {
	if method == http.MethodGet {
		switch path {
		case "/api/skills", "/api/tags", "/api/environments", "/api/v2/sync", "/api/v2/lists", "/api/events", "/api/ignored":
			return true
		}
		if id, ok := strings.CutPrefix(path, "/api/v2/lists/"); ok {
			n, err := strconv.ParseInt(id, 10, 64)
			return err == nil && n > 0
		}
	}
	switch method + " " + path {
	case "POST /api/skills", "PATCH /api/skill", "DELETE /api/skill", "POST /api/skill/tags", "POST /api/v2/lists", "DELETE /api/skill/ignore":
		return true
	}
	if method == http.MethodPost {
		if middle, ok := strings.CutPrefix(path, "/api/v2/lists/"); ok {
			id, found := strings.CutSuffix(middle, "/skills")
			n, err := strconv.ParseInt(id, 10, 64)
			return found && err == nil && n > 0
		}
	}
	return false
}

func (s *server) mcpCredential(raw, resource string) (mcpAccess, bool) {
	if s.authMode != AuthAccounts || !strings.HasPrefix(raw, "reet_mcp_") {
		return mcpAccess{}, false
	}
	u, c, ok, err := s.st.MCPUserFor(auth.HashToken(raw), time.Now().UnixMilli(), resource)
	return mcpAccess{u, c}, ok && err == nil
}

// Token headers cannot maintain credentials, even if the account also has a browser session.
func (s *server) mcpManagement(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authMode != AuthAccounts {
			writeErr(w, 404, "MCP access requires accounts mode")
			return
		}
		if requestToken(r) != "" {
			writeErr(w, 403, "manage MCP access from a signed-in browser")
			return
		}
		u, ok := s.sessionUser(r)
		if !ok {
			writeErr(w, 401, "sign in to manage MCP access")
			return
		}
		if !mcpSameOrigin(r) {
			writeErr(w, 403, "cross-site requests are not allowed")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)))
	}
}

func mcpSameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host && u.User == nil
	}
	return true
}

func (s *server) registerMCPRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/mcp", s.mcpManagement(s.mcpStatus))
	mux.HandleFunc("PATCH /api/v2/mcp", s.mcpManagement(s.mcpSettings))
	mux.HandleFunc("POST /api/v2/mcp/connections", s.mcpManagement(s.mcpCreate))
	mux.HandleFunc("PATCH /api/v2/mcp/connections/{id}", s.mcpManagement(s.mcpUpdate))
	mux.HandleFunc("DELETE /api/v2/mcp/connections/{id}", s.mcpManagement(s.mcpRevoke))
	mux.HandleFunc("POST /api/v2/mcp/connections/{id}/rotate", s.mcpManagement(s.mcpRotate))
	mux.HandleFunc("DELETE /api/v2/mcp/connections", s.mcpManagement(s.mcpRevokeAll))
	mux.Handle("/mcp", s.hostedMCP(mux))
	mux.HandleFunc("GET /api/v2/mcp/browser/tools", s.mcpManagement(s.mcpBrowserTools))
	mux.HandleFunc("POST /api/v2/mcp/browser/call", s.mcpManagement(func(w http.ResponseWriter, r *http.Request) { s.mcpBrowserCall(w, r, mux) }))
	s.registerMCPOAuth(mux)
}

func (s *server) mcpStatus(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.st.MCPEnabled(s.uid(r))
	if err != nil {
		writeErr(w, 500, "could not read MCP settings")
		return
	}
	connections, err := s.st.MCPConnections(s.uid(r))
	if err != nil {
		writeErr(w, 500, "could not read MCP connections")
		return
	}
	browser, scope, err := s.st.MCPBrowser(s.uid(r))
	if err != nil {
		writeErr(w, 500, "could not read browser access")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"enabled": enabled, "browserEnabled": browser, "browserScope": scope, "connections": connections, "canWrite": s.role(r) != "viewer", "endpoint": s.oauthBase(r) + "/mcp"})
}

func (s *server) mcpSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled        *bool   `json:"enabled"`
		BrowserEnabled *bool   `json:"browserEnabled"`
		BrowserScope   *string `json:"browserScope"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled == nil && in.BrowserEnabled == nil && in.BrowserScope == nil {
		writeErr(w, 400, "supply an MCP access setting")
		return
	}
	browser, scope, err := s.st.MCPBrowser(s.uid(r))
	if err != nil {
		writeErr(w, 500, "could not read browser access")
		return
	}
	if in.BrowserEnabled != nil {
		browser = *in.BrowserEnabled
	}
	if in.BrowserScope != nil {
		scope = *in.BrowserScope
	}
	if s.role(r) == "viewer" && in.BrowserScope == nil {
		scope = "read"
	}
	if (scope != "read" && scope != "write") || (scope == "write" && s.role(r) == "viewer") {
		writeErr(w, 403, "choose a permission available to your account")
		return
	}
	if err = s.st.SetMCPBrowser(s.uid(r), browser, scope); err != nil {
		writeErr(w, 500, "could not save browser access")
		return
	}
	if in.Enabled != nil {
		if err := s.st.SetMCPEnabled(s.uid(r), *in.Enabled); err != nil {
			writeErr(w, 500, "could not save MCP settings")
			return
		}
	}
	s.event(s.uid(r), "ui", "mcp-access", "", map[string]any{"enabled": in.Enabled, "browserEnabled": browser, "browserScope": scope})
	s.mcpStatus(w, r)
}

func mcpExpiry(days int) (int64, error) {
	if days < 1 || days > 365 {
		return 0, errors.New("expiry must be 1–365 days")
	}
	return time.Now().Add(time.Duration(days) * 24 * time.Hour).UnixMilli(), nil
}

func (s *server) mcpCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name          string `json:"name"`
		Scope         string `json:"scope"`
		ExpiresInDays int    `json:"expiresInDays"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Scope == "" {
		in.Scope = "read"
	}
	if in.ExpiresInDays == 0 {
		in.ExpiresInDays = 30
	}
	if s.role(r) == "viewer" && in.Scope != "read" {
		writeErr(w, 403, "viewer connections are read-only")
		return
	}
	expires, err := mcpExpiry(in.ExpiresInDays)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not create MCP credential")
		return
	}
	secret := "reet_mcp_" + raw
	c, err := s.st.CreateMCPConnection(s.uid(r), in.Name, in.Scope, auth.HashToken(secret), expires)
	if err != nil {
		writeErr(w, 400, "use a name of 1–100 characters and read or write permission")
		return
	}
	s.event(s.uid(r), "ui", "mcp-create", "", map[string]any{"name": c.Name, "scope": c.Scope})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"connection": c, "token": secret})
}

func (s *server) mcpFind(w http.ResponseWriter, r *http.Request) (store.MCPConnection, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, 404, "connection not found")
		return store.MCPConnection{}, false
	}
	connections, err := s.st.MCPConnections(s.uid(r))
	if err != nil {
		writeErr(w, 500, "could not read connections")
		return store.MCPConnection{}, false
	}
	for _, c := range connections {
		if c.ID == id {
			return c, true
		}
	}
	writeErr(w, 404, "connection not found")
	return store.MCPConnection{}, false
}

func (s *server) mcpUpdate(w http.ResponseWriter, r *http.Request) {
	c, ok := s.mcpFind(w, r)
	if !ok {
		return
	}
	var in struct {
		Name          *string `json:"name"`
		Scope         *string `json:"scope"`
		Enabled       *bool   `json:"enabled"`
		ExpiresInDays *int    `json:"expiresInDays"`
	}
	if !decode(w, r, &in) {
		return
	}
	oldScope := c.Scope
	if in.Name != nil {
		c.Name = *in.Name
	}
	if in.Scope != nil {
		c.Scope = *in.Scope
	}
	if in.Enabled != nil {
		c.Enabled = *in.Enabled
	}
	if c.Kind == "oauth" && oldScope == "read" && in.Scope != nil && *in.Scope == "write" {
		writeErr(w, 400, "reconnect this client to approve broader OAuth permissions")
		return
	}
	if s.role(r) == "viewer" && in.Scope != nil && c.Scope != "read" {
		writeErr(w, 403, "viewer connections are read-only")
		return
	}
	if in.ExpiresInDays != nil {
		var err error
		expiry, expiryErr := mcpExpiry(*in.ExpiresInDays)
		err = expiryErr
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if c.Kind == "oauth" && expiry > c.ExpiresAt {
			writeErr(w, 400, "reconnect this OAuth client to renew authorization")
			return
		}
		c.ExpiresAt = expiry
	}
	if err := s.st.UpdateMCPConnection(s.uid(r), c.ID, c.Name, c.Scope, c.Enabled, c.ExpiresAt); err != nil {
		writeErr(w, 400, "use a name of 1–100 characters and read or write permission")
		return
	}
	s.event(s.uid(r), "ui", "mcp-update", "", map[string]any{"name": c.Name, "scope": c.Scope, "enabled": c.Enabled})
	s.mcpStatus(w, r)
}

func (s *server) mcpRotate(w http.ResponseWriter, r *http.Request) {
	c, ok := s.mcpFind(w, r)
	if !ok {
		return
	}
	if c.Kind != "bearer" {
		writeErr(w, 400, "reconnect this OAuth client to renew authorization")
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not rotate credential")
		return
	}
	secret := "reet_mcp_" + raw
	if err := s.st.RotateMCPConnection(s.uid(r), c.ID, auth.HashToken(secret)); err != nil {
		writeErr(w, 500, "could not rotate credential")
		return
	}
	s.event(s.uid(r), "ui", "mcp-rotate", "", map[string]any{"name": c.Name})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"connection": c, "token": secret})
}

func (s *server) mcpRevoke(w http.ResponseWriter, r *http.Request) {
	c, ok := s.mcpFind(w, r)
	if !ok {
		return
	}
	if err := s.st.RevokeMCPConnection(s.uid(r), c.ID); err != nil {
		writeErr(w, 500, "could not revoke credential")
		return
	}
	s.event(s.uid(r), "ui", "mcp-revoke", "", map[string]any{"name": c.Name})
	w.WriteHeader(204)
}

func (s *server) mcpRevokeAll(w http.ResponseWriter, r *http.Request) {
	if err := s.st.RevokeMCPConnections(s.uid(r)); err != nil {
		writeErr(w, 500, "could not revoke credentials")
		return
	}
	s.event(s.uid(r), "ui", "mcp-revoke-all", "", nil)
	w.WriteHeader(204)
}
