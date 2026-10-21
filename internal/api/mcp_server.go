package api

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Generated from the stdio server by `cd mcp && npm run export-tools`.
//
//go:embed mcp_tools.json
var mcpToolJSON []byte

var mcpToolDefinitions = func() []*mcp.Tool {
	var tools []*mcp.Tool
	if err := json.Unmarshal(mcpToolJSON, &tools); err != nil {
		panic(err)
	}
	return tools
}()

const mcpResponseLimit = 25000

func mcpInteger(value any) string {
	if n, ok := value.(float64); ok {
		return strconv.FormatFloat(n, 'f', 0, 64)
	}
	return fmt.Sprint(value)
}

func (s *server) hostedMCP(api http.Handler) http.Handler {
	cache := mcp.NewSchemaCache()
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		access, ok := r.Context().Value(mcpAccessKey{}).(mcpAccess)
		if !ok {
			return nil
		}
		srv := mcp.NewServer(&mcp.Implementation{Name: "reet-mcp-server", Version: s.version}, &mcp.ServerOptions{SchemaCache: cache})
		for _, tool := range mcpToolDefinitions {
			if !access.canWrite() && !tool.Annotations.ReadOnlyHint {
				continue
			}
			mcp.AddTool(srv, tool, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, map[string]any, error) {
				data, err := s.executeMCP(ctx, api, access, tool.Name, args)
				if err != nil {
					return nil, nil, err
				}
				out := map[string]any{"data": data}
				raw, _ := json.Marshal(out)
				if len(raw) > mcpResponseLimit {
					return nil, nil, errors.New("response exceeds 25,000 characters; use a smaller page or narrower filters")
				}
				text := string(raw)
				if args["response_format"] == "markdown" {
					text = "```json\n" + string(raw) + "\n```"
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
			})
		}
		return srv
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !mcpSameOrigin(r) {
			writeErr(w, 403, "cross-site requests are not allowed")
			return
		}
		// Only header bearer credentials are accepted; browser cookies and query tokens cannot authenticate MCP.
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		access, valid := s.mcpCredential(raw, s.oauthBase(r)+"/mcp")
		if !ok || !valid {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource/mcp"`, s.oauthBase(r)))
			writeErr(w, 401, "connect with OAuth or an active MCP bearer token from Account → MCP access")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		ctx := context.WithValue(r.Context(), mcpAccessKey{}, access)
		transport.ServeHTTP(w, r.WithContext(ctx))
	})
}

type mcpBuffer struct {
	header http.Header
	body   bytes.Buffer
	status int
	err    error
}

func (b *mcpBuffer) Header() http.Header { return b.header }
func (b *mcpBuffer) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *mcpBuffer) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	if b.body.Len()+len(p) > 8<<20 {
		b.err = errors.New("API response exceeds 8 MiB; narrow the search")
		return 0, b.err
	}
	return b.body.Write(p)
}

func mcpItems(data map[string]any, key string) ([]any, error) {
	if data[key] == nil {
		return []any{}, nil
	}
	a, ok := data[key].([]any)
	if !ok {
		return nil, errors.New("unexpected API collection")
	}
	return a, nil
}

func mcpPage(items []any, args, meta map[string]any) (map[string]any, error) {
	limit, offset := 20, 0
	if n, ok := args["limit"].(float64); ok {
		limit = int(n)
	}
	if n, ok := args["offset"].(float64); ok {
		offset = int(n)
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, errors.New("use limit 1–100 and a nonnegative offset")
	}
	start := min(offset, len(items))
	end := min(start+limit, len(items))
	selected := items[start:end]
	if meta == nil {
		meta = map[string]any{}
	}
	for {
		meta["items"] = selected
		meta["count"] = len(selected)
		meta["total_count"] = len(items)
		meta["offset"] = offset
		meta["has_more"] = offset+len(selected) < len(items)
		meta["next_offset"] = nil
		if meta["has_more"] == true {
			meta["next_offset"] = offset + len(selected)
		}
		b, _ := json.Marshal(meta)
		if len(b) < mcpResponseLimit-128 {
			return meta, nil
		}
		if len(selected) <= 1 {
			return nil, errors.New("one record exceeds the response limit; narrow filters or shorten notes")
		}
		selected = selected[:len(selected)-1]
		meta["truncated"] = true
	}
}

func (s *server) executeMCP(ctx context.Context, api http.Handler, access mcpAccess, name string, args map[string]any) (map[string]any, error) {
	str := func(key string) string { v, _ := args[key].(string); return v }
	method, path := "GET", ""
	query := url.Values{}
	body := map[string]any{}
	for key, value := range args {
		if key != "response_format" && key != "limit" && key != "offset" {
			body[key] = value
		}
	}
	listPath := "/api/v2/lists/" + mcpInteger(args["list_id"])
	collectionKey := ""
	switch name {
	case "reet_list_skills", "reet_get_skill":
		path = "/api/skills"
		collectionKey = "skills"
		for _, key := range []string{"q", "tab", "stage", "sort", "env", "machine", "only"} {
			if v := str(key); v != "" {
				query.Set(key, v)
			}
		}
		if tags, ok := args["tags"].([]any); ok {
			for _, tag := range tags {
				query.Add("tag", mcpInteger(tag))
			}
		}
		if name == "reet_get_skill" {
			query.Set("q", str("id"))
		}
	case "reet_add_skill":
		method, path = "POST", "/api/skills"
	case "reet_update_skill":
		method, path = "PATCH", "/api/skill"
		if len(body) <= 1 {
			return nil, errors.New("supply liked, bookmarked, used or notes")
		}
	case "reet_remove_skill":
		method, path = "DELETE", "/api/skill"
		query.Set("id", str("id"))
		if args["ignore"] == true {
			query.Set("ignore", "1")
		}
	case "reet_list_tags":
		path, collectionKey = "/api/tags", "tags"
	case "reet_tag_skill":
		method, path = "POST", "/api/skill/tags"
		add, _ := args["add"].([]any)
		remove, _ := args["remove"].([]any)
		if len(add)+len(remove) == 0 {
			return nil, errors.New("supply tags to add or remove")
		}
	case "reet_list_environments", "reet_get_environment":
		path, collectionKey = "/api/environments", "environments"
	case "reet_list_sync_targets":
		path, collectionKey = "/api/v2/sync", "targets"
	case "reet_list_lists":
		path, collectionKey = "/api/v2/lists", "lists"
	case "reet_get_list":
		path, collectionKey = listPath, "skills"
	case "reet_create_list":
		method, path = "POST", "/api/v2/lists"
		body["visibility"] = "personal"
	case "reet_add_list_skill":
		method, path = "POST", listPath+"/skills"
		delete(body, "list_id")
	case "reet_list_events":
		path = "/api/events"
		query.Set("limit", fmt.Sprint(args["limit"]))
		if str("skill") != "" {
			query.Set("skill", str("skill"))
		}
	case "reet_list_ignored":
		path, collectionKey = "/api/ignored", "ignored"
	case "reet_unignore_skill":
		method, path = "DELETE", "/api/skill/ignore"
		query.Set("id", str("id"))
	default:
		return nil, errors.New("unknown tool")
	}
	if !mcpAPIPath(method, path) || (method != "GET" && !access.canWrite()) {
		return nil, errors.New("this MCP connection is read-only")
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(context.WithValue(ctx, userContextKey{}, access.User), method, path+"?"+query.Encode(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reet-Source", "mcp")
	w := &mcpBuffer{header: make(http.Header)}
	api.ServeHTTP(w, req)
	if w.err != nil {
		return nil, w.err
	}
	if w.status >= 400 {
		return nil, fmt.Errorf("reet.sh returned HTTP %d; check resource IDs, account role and list permissions", w.status)
	}
	if method != "GET" {
		s.event(access.User.ID, "mcp", "mcp-call", "", map[string]any{"connection": access.Connection.Name, "tool": name})
	}
	if w.status == 204 {
		return map[string]any{"ok": true}, nil
	}
	data := map[string]any{}
	if json.Unmarshal(w.body.Bytes(), &data) != nil {
		return nil, errors.New("unexpected API response")
	}
	if name == "reet_add_list_skill" {
		return map[string]any{"id": data["id"], "name": data["name"], "skillCount": data["skillCount"]}, nil
	}
	if name == "reet_list_events" {
		return map[string]any{"events": data["events"], "limit": args["limit"]}, nil
	}
	if collectionKey == "" {
		return data, nil
	}
	items, err := mcpItems(data, collectionKey)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{}
	switch name {
	case "reet_get_skill":
		for _, item := range items {
			sk, ok := item.(map[string]any)
			if ok && sk["id"] == str("id") {
				return sk, nil
			}
		}
		return nil, errors.New("skill not found on this account's shelf")
	case "reet_list_skills":
		meta["counts"] = data["counts"]
	case "reet_list_environments":
		filtered := []any{}
		for _, item := range items {
			env := item.(map[string]any)
			if host := str("host"); host != "" && env["host"] != host {
				continue
			}
			skills, _ := mcpItems(env, "skills")
			delete(env, "skills")
			env["skillCount"] = len(skills)
			filtered = append(filtered, env)
		}
		items = filtered
	case "reet_get_environment":
		found := false
		for _, item := range items {
			env := item.(map[string]any)
			if env["id"] == str("id") {
				items, err = mcpItems(env, "skills")
				delete(env, "skills")
				meta["environment"] = env
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("environment not found; list environments to find its ID")
		}
		if err != nil {
			return nil, err
		}
	case "reet_list_lists":
		if q := strings.ToLower(str("q")); q != "" {
			filtered := []any{}
			for _, item := range items {
				l := item.(map[string]any)
				if strings.Contains(strings.ToLower(fmt.Sprint(l["name"])+" "+fmt.Sprint(l["description"])), q) {
					filtered = append(filtered, l)
				}
			}
			items = filtered
		}
	case "reet_get_list":
		delete(data, "skills")
		delete(data, "members")
		delete(data, "audience")
		meta["list"] = data
	}
	return mcpPage(items, args, meta)
}
