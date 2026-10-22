package api

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

var pkceChallenge = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
var pkceVerifier = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

func (s *server) oauthBase(r *http.Request) string {
	if s.publicURL != "" {
		return strings.TrimRight(s.publicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func oauthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]string{"error": code, "error_description": message})
}

func (s *server) oauthPublic(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authMode != AuthAccounts {
			oauthError(w, 404, "temporarily_unavailable", "MCP sign-in requires accounts mode")
			return
		}
		origin, _ := url.Parse(s.oauthBase(r))
		if origin == nil || (origin.Scheme != "https" && !(origin.Scheme == "http" && mcpLoopback(origin.Hostname()))) {
			oauthError(w, 503, "temporarily_unavailable", "OAuth requires a reachable HTTPS origin or localhost; configure REET_PUBLIC_URL behind your proxy")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next(w, r)
	}
}

func (s *server) registerMCPOAuth(mux *http.ServeMux) {
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		mux.HandleFunc("GET "+path, s.oauthPublic(s.oauthResource))
	}
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.oauthPublic(s.oauthMetadata))
	mux.HandleFunc("POST /oauth/register", s.oauthPublic(s.oauthRegister))
	mux.HandleFunc("GET /oauth/authorize", s.oauthPublic(s.oauthAuthorize))
	mux.HandleFunc("POST /oauth/token", s.oauthPublic(s.oauthToken))
	mux.HandleFunc("POST /oauth/revoke", s.oauthPublic(s.oauthRevoke))
	mux.HandleFunc("OPTIONS /oauth/{path}", s.oauthPublic(func(http.ResponseWriter, *http.Request) {}))
	mux.HandleFunc("GET /api/v2/mcp/authorization", s.mcpManagement(s.oauthConsentInfo))
	mux.HandleFunc("POST /api/v2/mcp/authorization", s.mcpManagement(s.oauthConsent))
}

func (s *server) oauthResource(w http.ResponseWriter, r *http.Request) {
	b := s.oauthBase(r)
	writeJSON(w, 200, map[string]any{"resource": b + "/mcp", "resource_name": "reet.sh personal shelf", "authorization_servers": []string{b}, "scopes_supported": []string{"mcp:read", "mcp:write"}, "bearer_methods_supported": []string{"header"}})
}

func (s *server) oauthMetadata(w http.ResponseWriter, r *http.Request) {
	b := s.oauthBase(r)
	writeJSON(w, 200, map[string]any{
		"issuer": b, "authorization_endpoint": b + "/oauth/authorize", "token_endpoint": b + "/oauth/token", "registration_endpoint": b + "/oauth/register", "revocation_endpoint": b + "/oauth/revoke",
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{"mcp:read", "mcp:write"}, "authorization_response_iss_parameter_supported": true,
	})
}

func validMCPRedirect(raw string) bool {
	if len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && mcpLoopback(u.Hostname())
}

func mcpLoopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func mcpRedirectMatches(registered, requested string) bool {
	if registered == requested {
		return true
	}
	a, errA := url.Parse(registered)
	b, errB := url.Parse(requested)
	// Native clients may select a new loopback port (RFC 8252 §7.3).
	return errA == nil && errB == nil && validMCPRedirect(requested) && a.Scheme == "http" && b.Scheme == "http" && a.Hostname() == b.Hostname() && mcpLoopback(a.Hostname()) && a.EscapedPath() == b.EscapedPath() && a.RawQuery == b.RawQuery
}

func (s *server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	key := "oauth-register:" + ip
	if !s.oauthLimiter.Allow(key) {
		oauthError(w, 429, "temporarily_unavailable", "try registration again later")
		return
	}
	s.oauthLimiter.Hit(key)
	var in struct {
		Name          string   `json:"client_name"`
		Redirects     []string `json:"redirect_uris"`
		Method        string   `json:"token_endpoint_auth_method"`
		GrantTypes    []string `json:"grant_types"`
		ResponseTypes []string `json:"response_types"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Method != "" && in.Method != "none" {
		oauthError(w, 400, "invalid_client_metadata", "public clients use token_endpoint_auth_method=none")
		return
	}
	for _, g := range in.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			oauthError(w, 400, "invalid_client_metadata", "only authorization_code and refresh_token are supported")
			return
		}
	}
	for _, v := range in.ResponseTypes {
		if v != "code" {
			oauthError(w, 400, "invalid_client_metadata", "only the code response type is supported")
			return
		}
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		in.Name = "MCP client"
	}
	if len(in.Name) > 100 || len(in.Redirects) == 0 || len(in.Redirects) > 10 {
		oauthError(w, 400, "invalid_client_metadata", "provide a client name and 1–10 redirect URIs")
		return
	}
	for _, uri := range in.Redirects {
		if !validMCPRedirect(uri) {
			oauthError(w, 400, "invalid_redirect_uri", "redirect URIs must be HTTPS or an HTTP loopback callback, without a fragment or credentials")
			return
		}
	}
	id, err := auth.NewToken()
	if err != nil {
		oauthError(w, 500, "server_error", "registration failed")
		return
	}
	c := store.MCPClient{ID: "reet_client_" + id, Name: in.Name, RedirectURIs: in.Redirects}
	if err := s.st.SaveMCPClient(c); err != nil {
		oauthError(w, 500, "server_error", "registration failed")
		return
	}
	writeJSON(w, 201, map[string]any{"client_id": c.ID, "client_name": c.Name, "redirect_uris": c.RedirectURIs, "client_id_issued_at": time.Now().Unix(), "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}})
}

type mcpAuthorization struct {
	Client                                      store.MCPClient
	Redirect, State, Challenge, Resource, Scope string
}

func (s *server) parseMCPAuthorization(r *http.Request, q url.Values) (mcpAuthorization, error) {
	a := mcpAuthorization{Redirect: q.Get("redirect_uri"), State: q.Get("state"), Challenge: q.Get("code_challenge"), Resource: q.Get("resource"), Scope: "read"}
	if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || !pkceChallenge.MatchString(a.Challenge) || len(a.State) > 2048 {
		return a, fmt.Errorf("this connection requires an authorization code with PKCE S256")
	}
	client, err := s.st.MCPClient(q.Get("client_id"))
	if err != nil {
		return a, fmt.Errorf("this MCP client is not registered")
	}
	a.Client = client
	valid := false
	for _, uri := range client.RedirectURIs {
		if mcpRedirectMatches(uri, a.Redirect) {
			valid = true
			break
		}
	}
	if !valid {
		return a, fmt.Errorf("the callback does not match the registered client")
	}
	if a.Resource == "" {
		a.Resource = s.oauthBase(r) + "/mcp"
	}
	if a.Resource != s.oauthBase(r)+"/mcp" {
		return a, fmt.Errorf("the requested resource is not this MCP server")
	}
	for _, scope := range strings.Fields(q.Get("scope")) {
		switch scope {
		case "mcp:read":
		case "mcp:write":
			a.Scope = "write"
		default:
			return a, fmt.Errorf("unsupported scope")
		}
	}
	return a, nil
}

func (s *server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	if _, err := s.parseMCPAuthorization(r, r.URL.Query()); err != nil {
		oauthError(w, 400, "invalid_request", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, "/account/mcp/authorize?"+r.URL.RawQuery, http.StatusFound)
}

func (s *server) oauthConsentInfo(w http.ResponseWriter, r *http.Request) {
	a, err := s.parseMCPAuthorization(r, r.URL.Query())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u, _ := url.Parse(a.Redirect)
	enabled, err := s.st.MCPEnabled(s.uid(r))
	if err != nil {
		writeErr(w, 500, "could not read MCP access")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"clientName": a.Client.Name, "callbackHost": u.Host, "scope": a.Scope, "enabled": enabled, "canWrite": s.role(r) != "viewer", "account": s.currentUser(r).Email})
}

func (s *server) oauthConsent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query   string `json:"query"`
		Approve bool   `json:"approve"`
		Scope   string `json:"scope"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Query) > 8192 {
		writeErr(w, 400, "authorization request is too large")
		return
	}
	q, err := url.ParseQuery(in.Query)
	if err != nil {
		writeErr(w, 400, "invalid authorization request")
		return
	}
	a, err := s.parseMCPAuthorization(r, q)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	redirect, _ := url.Parse(a.Redirect)
	out := redirect.Query()
	out.Set("state", a.State)
	out.Set("iss", s.oauthBase(r))
	if !in.Approve {
		out.Set("error", "access_denied")
	} else {
		if in.Scope != "read" && in.Scope != "write" {
			writeErr(w, 400, "choose read or write access")
			return
		}
		if in.Scope == "write" && (a.Scope != "write" || s.role(r) == "viewer") {
			writeErr(w, 403, "write access is not available for this request")
			return
		}
		raw, err := auth.NewToken()
		if err != nil {
			writeErr(w, 500, "could not authorize connection")
			return
		}
		code, err := auth.NewToken()
		if err != nil {
			writeErr(w, 500, "could not authorize connection")
			return
		}
		c, err := s.st.CreateMCPConnection(s.uid(r), a.Client.Name, in.Scope, auth.HashToken(raw), time.Now().Add(30*24*time.Hour).UnixMilli())
		if err != nil {
			writeErr(w, 500, "could not authorize connection")
			return
		}
		if err = s.st.SetMCPEnabled(s.uid(r), true); err != nil {
			writeErr(w, 500, "could not enable MCP access")
			return
		}
		err = s.st.SaveMCPCode(store.MCPCode{Hash: auth.HashToken(code), ClientID: a.Client.ID, ConnectionID: c.ID, RedirectURI: a.Redirect, Challenge: a.Challenge, Resource: a.Resource, Scope: in.Scope, ExpiresAt: time.Now().Add(5 * time.Minute).UnixMilli()})
		if err != nil {
			writeErr(w, 500, "could not authorize connection")
			return
		}
		out.Set("code", code)
		s.eventRequest(r, "mcp-authorize", "", map[string]any{"name": a.Client.Name, "scope": in.Scope})
	}
	redirect.RawQuery = out.Encode()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{"redirect": redirect.String()})
}

func oauthScope(scope string) string {
	if scope == "write" {
		return "mcp:read mcp:write"
	}
	return "mcp:read"
}

func (s *server) oauthToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") || r.ParseForm() != nil {
		oauthError(w, 400, "invalid_request", "send a form-encoded token request")
		return
	}
	// Query strings never supply credentials or grant parameters.
	f := r.PostForm
	clientID := f.Get("client_id")
	if _, err := s.st.MCPClient(clientID); err != nil {
		oauthError(w, 401, "invalid_client", "unknown public client")
		return
	}
	resource := f.Get("resource")
	if resource == "" {
		resource = s.oauthBase(r) + "/mcp"
	}
	if resource != s.oauthBase(r)+"/mcp" {
		oauthError(w, 400, "invalid_target", "resource must identify this MCP endpoint")
		return
	}
	access, err := auth.NewToken()
	if err != nil {
		oauthError(w, 500, "server_error", "could not issue token")
		return
	}
	refresh, err := auth.NewToken()
	if err != nil {
		oauthError(w, 500, "server_error", "could not issue token")
		return
	}
	access = "reet_mcp_" + access
	refresh = "reet_refresh_" + refresh
	var scope string
	switch f.Get("grant_type") {
	case "authorization_code":
		if !pkceVerifier.MatchString(f.Get("code_verifier")) {
			oauthError(w, 400, "invalid_grant", "a valid PKCE verifier is required")
			return
		}
		scope, err = s.st.ExchangeMCPCode(auth.HashToken(f.Get("code")), clientID, f.Get("redirect_uri"), resource, f.Get("code_verifier"), auth.HashToken(access), auth.HashToken(refresh))
	case "refresh_token":
		requestedScope := ""
		for _, v := range strings.Fields(f.Get("scope")) {
			switch v {
			case "mcp:read":
				if requestedScope == "" {
					requestedScope = "read"
				}
			case "mcp:write":
				requestedScope = "write"
			default:
				oauthError(w, 400, "invalid_scope", "only mcp:read and mcp:write are supported")
				return
			}
		}
		scope, err = s.st.RefreshMCPGrant(auth.HashToken(f.Get("refresh_token")), clientID, resource, requestedScope, auth.HashToken(access), auth.HashToken(refresh))
	default:
		oauthError(w, 400, "unsupported_grant_type", "use authorization_code or refresh_token")
		return
	}
	if err != nil {
		oauthError(w, 400, "invalid_grant", "grant expired, was revoked, was already used, or does not match this client, redirect, resource or verifier")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, 200, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": 3600, "refresh_token": refresh, "scope": oauthScope(scope)})
}

func (s *server) oauthRevoke(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if r.ParseForm() != nil {
		oauthError(w, 400, "invalid_request", "send a form-encoded revocation request")
		return
	}
	if err := s.st.RevokeMCPOAuth(auth.HashToken(r.PostForm.Get("token")), r.PostForm.Get("client_id")); err != nil {
		oauthError(w, 500, "server_error", "could not revoke grant")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
}
