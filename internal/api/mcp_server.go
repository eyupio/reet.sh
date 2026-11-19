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
	"unicode/utf8"

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

// A write has already committed by the time its response is formatted. Make
// that explicit if the result cannot fit, so clients do not retry the change.
func mcpResultJSON(data map[string]any, wrote bool) ([]byte, error) {
	raw, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		return nil, err
	}
	if utf8.RuneCount(raw) > mcpResponseLimit {
		if wrote {
			return nil, errors.New("the change succeeded, but its response exceeds 25,000 characters; read it with a filtered or paginated tool before making another change")
		}
		return nil, errors.New("response exceeds 25,000 characters; use a smaller page or narrower filters")
	}
	return raw, nil
}

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
		// The SDK applies object defaults before validating a null map. Guard
		// that input here; an omitted optional arguments object means {}.
		srv.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
					raw := bytes.TrimSpace(call.Params.Arguments)
					if bytes.Equal(raw, []byte("null")) {
						return nil, errors.New("tool arguments must be a JSON object; use {} when no arguments are needed")
					}
					if len(raw) == 0 {
						params := *call.Params
						params.Arguments = json.RawMessage(`{}`)
						copy := *call
						copy.Params = &params
						req = &copy
					}
				}
				return next(ctx, method, req)
			}
		})
		for _, tool := range mcpToolDefinitions {
			if mcpWorkerTool(tool.Name) && !access.canWrite() {
				continue
			}
			if mcpContextTool(tool.Name) && access.User.Role == "viewer" {
				continue
			}
			if mcpFactoryTool(tool.Name) && access.User.Role == "viewer" {
				continue
			}
			if !access.canWrite() && !tool.Annotations.ReadOnlyHint {
				continue
			}
			mcp.AddTool(srv, tool, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, map[string]any, error) {
				data, err := s.executeMCP(ctx, api, access, s.oauthBase(r), tool.Name, args)
				if err != nil {
					return nil, nil, err
				}
				out := map[string]any{"data": data}
				raw, err := mcpResultJSON(data, !tool.Annotations.ReadOnlyHint)
				if err != nil {
					return nil, nil, err
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
	validLimit := limit >= 1 && limit <= 100
	if !validLimit || offset < 0 {
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
		if utf8.RuneCount(b) < mcpResponseLimit-128 {
			return meta, nil
		}
		if len(selected) <= 1 {
			// One record that is too large on its own (a sync target with a big
			// library) keeps its fields and gives up its largest list for a count.
			if len(selected) == 1 {
				if shorter, ok := mcpCountLargestList(selected[0]); ok {
					selected = []any{shorter}
					meta["truncated"] = true
					meta["truncated_reason"] = "record_lists_counted"
					continue
				}
			}
			return nil, errors.New("one record exceeds the response limit; narrow filters or shorten notes")
		}
		selected = selected[:len(selected)-1]
		meta["truncated"] = true
		meta["truncated_reason"] = "response_character_limit"
	}
}

// mcpCountLargestList returns a copy of record with its largest list replaced
// by "<name>_count", and the list's name added to "lists_counted". It reports
// false when the record has no list left to give up.
func mcpCountLargestList(item any) (map[string]any, bool) {
	record, ok := item.(map[string]any)
	if !ok {
		return nil, false
	}
	largest, size := "", 0
	for key, value := range record {
		list, isList := value.([]any)
		if !isList || len(list) == 0 || key == "lists_counted" {
			continue
		}
		if b, _ := json.Marshal(list); len(b) > size || (len(b) == size && key < largest) {
			largest, size = key, len(b)
		}
	}
	if largest == "" {
		return nil, false
	}
	shorter := make(map[string]any, len(record)+1)
	for key, value := range record {
		if key != largest {
			shorter[key] = value
		}
	}
	shorter[largest+"_count"] = len(record[largest].([]any))
	counted, _ := record["lists_counted"].([]any)
	shorter["lists_counted"] = append(append([]any{}, counted...), largest)
	return shorter, true
}

// mcpDo sends one internal API request as the MCP caller.
func (s *server) mcpDo(ctx context.Context, api http.Handler, access mcpAccess, origin, method, path string, query url.Values, body map[string]any) (*mcpBuffer, error) {
	allowedPath := mcpAPIPath(method, path)
	needsWrite := method != "GET" && !access.canWrite()
	if !allowedPath || needsWrite {
		return nil, errors.New("this MCP connection is read-only")
	}
	// An MCP call is a POST whatever it does, so the fence is checked here, on
	// the request it makes: reads go through and writes are refused.
	if method != "GET" {
		if f, err := s.st.Fence(); err != nil || f.Fenced {
			if err != nil {
				return nil, errors.New("this controller cannot tell whether it has moved, so it is not accepting changes")
			}
			return nil, errors.New(fenceMessage(f))
		}
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(context.WithValue(ctx, userContextKey{}, access.User), method, origin+path+"?"+query.Encode(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reet-Source", "mcp")
	if access.Connection.Name != "" {
		req.Header.Set("X-Reet-Client", access.Connection.Name)
	}
	w := &mcpBuffer{header: make(http.Header)}
	api.ServeHTTP(w, req)
	if w.err != nil {
		return nil, w.err
	}
	if w.status >= 400 {
		var detail struct {
			Code, Field, Message, Hint, Error string
		}
		_ = json.Unmarshal(w.body.Bytes(), &detail)
		if detail.Code != "" {
			text := fmt.Sprintf("reet.sh returned HTTP %d [%s]", w.status, detail.Code)
			if detail.Field != "" {
				text += " field=" + detail.Field
			}
			msg := detail.Message
			if msg == "" {
				msg = detail.Error
			}
			if msg != "" {
				text += ": " + msg
			}
			if detail.Hint != "" {
				// A message that already ends a sentence is not given a
				// second full stop before the hint.
				if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "!") || strings.HasSuffix(text, "?") {
					text += " " + detail.Hint
				} else {
					text += ". " + detail.Hint
				}
			}
			return nil, errors.New(text)
		}
		return nil, fmt.Errorf("reet.sh returned HTTP %d; check resource IDs, account role and list permissions", w.status)
	}
	return w, nil
}

// mcpMachineEnvironments returns the scanned environments for each host, without their skill lists.
func (s *server) mcpMachineEnvironments(ctx context.Context, api http.Handler, access mcpAccess, origin string) (map[string][]map[string]any, error) {
	w, err := s.mcpDo(ctx, api, access, origin, "GET", "/api/environments", url.Values{}, nil)
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if json.Unmarshal(w.body.Bytes(), &data) != nil {
		return nil, errors.New("unexpected API response")
	}
	items, err := mcpItems(data, "environments")
	if err != nil {
		return nil, err
	}
	byHost := map[string][]map[string]any{}
	for _, item := range items {
		env, ok := item.(map[string]any)
		if !ok {
			continue
		}
		skills, _ := mcpItems(env, "skills")
		host, _ := env["host"].(string)
		byHost[host] = append(byHost[host], map[string]any{"id": env["id"], "name": env["name"], "agent": env["agent"], "skillCount": len(skills)})
	}
	return byHost, nil
}

func (s *server) executeMCPMachines(ctx context.Context, api http.Handler, access mcpAccess, origin, name string, args map[string]any) (map[string]any, error) {
	str := func(key string) string { v, _ := args[key].(string); return strings.TrimSpace(v) }
	byHost, err := s.mcpMachineEnvironments(ctx, api, access, origin)
	if err != nil {
		return nil, err
	}
	confirm := args["confirm"] == true
	if name == "reet_merge_machines" {
		from, to := str("from"), str("to")
		if _, ok := byHost[from]; !ok {
			return nil, errors.New("Both hosts must exist. Use reet_list_environments to find exact host names.")
		}
		if _, ok := byHost[to]; !ok {
			return nil, errors.New("Both hosts must exist. Use reet_list_environments to find exact host names.")
		}
		if from == to {
			return nil, errors.New("from and to must be different hosts.")
		}
		if !confirm {
			return map[string]any{"dry_run": true, "from": from, "to": to, "applied": false}, nil
		}
		if _, err = s.mcpDo(ctx, api, access, origin, "POST", "/api/v2/machines/merge", url.Values{}, map[string]any{"from": from, "to": to}); err != nil {
			return nil, err
		}
		s.event(access.User.ID, "mcp", "mcp-call", "", map[string]any{"connection": access.Connection.Name, "tool": name})
		return map[string]any{"dry_run": false, "from": from, "to": to, "applied": true}, nil
	}
	host := str("host")
	envs, ok := byHost[host]
	if !ok {
		return nil, errors.New("No environments for that host. Use reet_list_environments to find its exact host name.")
	}
	skillCount := 0
	for _, env := range envs {
		skillCount += env["skillCount"].(int)
	}
	result := map[string]any{"dry_run": !confirm, "host": host, "environments": envs, "skillCount": skillCount, "applied": false}
	if !confirm {
		return result, nil
	}
	if skillCount > 0 && args["force"] != true {
		return nil, fmt.Errorf("That machine still lists %d skills. Pass force=true to forget it anyway.", skillCount)
	}
	if _, err = s.mcpDo(ctx, api, access, origin, "DELETE", "/api/v2/machines", url.Values{"host": {host}}, nil); err != nil {
		return nil, err
	}
	s.event(access.User.ID, "mcp", "mcp-call", "", map[string]any{"connection": access.Connection.Name, "tool": name})
	result["applied"] = true
	return result, nil
}

func (s *server) executeMCP(ctx context.Context, api http.Handler, access mcpAccess, origin, name string, args map[string]any) (map[string]any, error) {
	if mcpWorkerTool(name) {
		return s.executeMCPWorker(ctx, access, name, args)
	}
	if mcpContextTool(name) {
		return s.executeMCPContext(access, name, args)
	}
	if mcpFactoryTool(name) {
		return s.executeMCPFactory(ctx, access, name, args)
	}
	if name == "reet_forget_machine" || name == "reet_merge_machines" {
		return s.executeMCPMachines(ctx, api, access, origin, name, args)
	}
	switch name {
	case "reet_update_skill", "reet_remove_skill", "reet_list_archived", "reet_restore_skill":
		return s.executeMCPLibrary(ctx, api, access, origin, name, args)
	}
	str := func(key string) string { v, _ := args[key].(string); return v }
	method, path := "GET", ""
	query := url.Values{}
	body := map[string]any{}
	for key, value := range args {
		knownKey := key == "response_format" || key == "limit" || key == "offset"
		if !knownKey {
			body[key] = value
		}
	}
	listPath := "/api/v2/lists/" + mcpInteger(args["list_id"])
	collectionKey := ""
	switch name {
	case "reet_list_skills", "reet_get_skill":
		path = "/api/skills"
		collectionKey = "skills"
		for _, key := range []string{"q", "tab", "status", "sort", "env", "machine", "only"} {
			if v := str(key); v != "" {
				query.Set(key, v)
			}
		}
		if tags, ok := args["tags"].([]any); ok {
			for _, tag := range tags {
				query.Add("tag", mcpInteger(tag))
			}
		}
		// The compact shape is always named field by field, so it carries
		// status and no retired stage flags.
		fields, _ := args["fields"].([]any)
		if args["compact"] == true || len(fields) > 0 {
			names := mcpCompactFields
			if len(fields) > 0 {
				names = make([]string, 0, len(fields))
				for _, field := range fields {
					names = append(names, fmt.Sprint(field))
				}
			}
			query.Set("fields", strings.Join(names, ","))
		}
		if name == "reet_get_skill" {
			query.Set("q", str("id"))
		}
	case "reet_add_skill":
		method, path = "POST", "/api/skills"
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
		if args["empty_only"] == true {
			query.Set("empty_only", "1")
		}
		if v := str("agent"); v != "" {
			query.Set("agent", v)
		}
	case "reet_delete_environment":
		method, path = "POST", "/api/v2/environments/delete"
	case "reet_merge_environments":
		method, path = "POST", "/api/v2/environments/merge"
	case "reet_list_lists":
		path, collectionKey = "/api/v2/lists", "lists"
	case "reet_get_list":
		path, collectionKey = listPath, "skills"
	case "reet_create_list":
		method, path = "POST", "/api/v2/lists"
		body["visibility"] = "personal"
	case "reet_update_list":
		method, path = "PATCH", listPath
		delete(body, "list_id")
	case "reet_delete_list":
		method, path = "POST", listPath+"/delete"
		delete(body, "list_id")
	case "reet_add_list_skill":
		method, path = "POST", listPath+"/skills"
		delete(body, "list_id")
		if (str("id") == "") == (str("input") == "") {
			return nil, errors.New("Supply exactly one of id (a skill already in the library) or input (an owner/repo/skill reference or skills.sh URL).")
		}
	case "reet_add_list_skills":
		method, path = "POST", listPath+"/skills/bulk"
		delete(body, "list_id")
	case "reet_remove_list_skill":
		method, path = "DELETE", listPath+"/skills"
		query.Set("id", str("id"))
		delete(body, "list_id")
		delete(body, "id")
	case "reet_bulk_update_skills":
		method, path = "POST", "/api/v2/skills/bulk"
	case "reet_find_duplicates":
		path = "/api/v2/skills/duplicates"
	case "reet_list_events":
		path = "/api/events"
		query.Set("limit", fmt.Sprint(args["limit"]))
		for _, key := range []string{"skill", "action", "source", "before", "after"} {
			if v := str(key); v != "" {
				query.Set(key, v)
			}
		}
		if list, ok := args["exclude"].([]any); ok {
			var sources []string
			for _, item := range list {
				if v, ok := item.(string); ok && v != "" {
					sources = append(sources, v)
				}
			}
			if len(sources) > 0 {
				query.Set("exclude", strings.Join(sources, ","))
			}
		}
	case "reet_create_tag":
		method, path = "POST", "/api/v2/tags"
	case "reet_rename_tag":
		method, path = "PATCH", "/api/v2/tags/"+mcpInteger(args["tag_id"])
		delete(body, "tag_id")
	case "reet_delete_tag":
		method, path = "POST", "/api/v2/tags/"+mcpInteger(args["tag_id"])+"/delete"
		delete(body, "tag_id")
	default:
		return nil, errors.New("unknown tool")
	}
	w, err := s.mcpDo(ctx, api, access, origin, method, path, query, body)
	if err != nil {
		return nil, err
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
		data["limit"] = args["limit"]
		return data, nil
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
		return nil, errors.New("skill not found in this account's library")
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

// mcpCompactFields is reet_list_skills' compact shape.
var mcpCompactFields = []string{"id", "name", "source", "sourceType", "status", "liked", "tags", "environment_count"}

// mcpGet reads one internal API GET as the MCP caller.
func (s *server) mcpGet(ctx context.Context, api http.Handler, access mcpAccess, origin, path string, query url.Values) (map[string]any, error) {
	w, err := s.mcpDo(ctx, api, access, origin, "GET", path, query, nil)
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if json.Unmarshal(w.body.Bytes(), &data) != nil {
		return nil, errors.New("unexpected API response")
	}
	return data, nil
}

// mcpShelfSkill is the shelf skill an exact ID names, from the listing
// reet_get_skill reads, or nil.
func (s *server) mcpShelfSkill(ctx context.Context, api http.Handler, access mcpAccess, origin, id string) (map[string]any, error) {
	data, err := s.mcpGet(ctx, api, access, origin, "/api/skills", url.Values{"q": {id}})
	if err != nil {
		return nil, err
	}
	items, err := mcpItems(data, "skills")
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if sk, ok := item.(map[string]any); ok && sk["id"] == id {
			return sk, nil
		}
	}
	return nil, nil
}

var errMCPSkillNotFound = errors.New("Skill not found in this account's library. Use reet_list_skills or reet_list_archived to find its ID.")

// mcpLibrarySkill is the library item a skill tool acts on, and its shelf ID
// ("" when it has none): id is a skill ID from reet_list_skills or a library
// id from reet_list_archived.
func (s *server) mcpLibrarySkill(ctx context.Context, api http.Handler, access mcpAccess, origin, id string) (item, shelf string, err error) {
	sk, err := s.mcpShelfSkill(ctx, api, access, origin, id)
	if err != nil {
		return "", "", err
	}
	if sk != nil {
		item, _ = sk["libraryItemId"].(string)
		if item == "" {
			return "", "", errors.New("That skill is not in your library, so it has no status to change. Add it again with reet_add_skill.")
		}
		return item, id, nil
	}
	for _, tab := range []string{"active", "archived"} {
		data, err := s.mcpGet(ctx, api, access, origin, "/api/v2/skills", url.Values{"status": {tab}})
		if err != nil {
			return "", "", err
		}
		rows, err := mcpItems(data, "skills")
		if err != nil {
			return "", "", err
		}
		for _, raw := range rows {
			if row, ok := raw.(map[string]any); ok && row["id"] == id {
				shelf, _ = row["shelfId"].(string)
				return id, shelf, nil
			}
		}
	}
	return "", "", errMCPSkillNotFound
}

// executeMCPLibrary runs the tools that archive, restore, list and delete
// library skills, through the Skills page's own routes.
func (s *server) executeMCPLibrary(ctx context.Context, api http.Handler, access mcpAccess, origin, name string, args map[string]any) (map[string]any, error) {
	if name == "reet_list_archived" {
		data, err := s.mcpGet(ctx, api, access, origin, "/api/v2/skills", url.Values{"status": {"archived"}})
		if err != nil {
			return nil, err
		}
		items, err := mcpItems(data, "skills")
		if err != nil {
			return nil, err
		}
		return mcpPage(items, args, map[string]any{"counts": data["counts"]})
	}
	id, _ := args["id"].(string)
	id = strings.TrimSpace(id)
	fields := map[string]any{}
	for _, key := range []string{"liked", "notes"} {
		if v, ok := args[key]; ok {
			fields[key] = v
		}
	}
	status, _ := args["status"].(string)
	if name == "reet_update_skill" && status == "" && len(fields) == 0 {
		return nil, errors.New("Supply at least one of liked, notes or status.")
	}
	item, shelf, err := s.mcpLibrarySkill(ctx, api, access, origin, id)
	if err != nil {
		return nil, err
	}
	wrote := false
	logged := func() {
		if !wrote {
			wrote = true
			s.event(access.User.ID, "mcp", "mcp-call", "", map[string]any{"connection": access.Connection.Name, "tool": name})
		}
	}
	if name == "reet_remove_skill" {
		w, err := s.mcpDo(ctx, api, access, origin, "POST", "/api/v2/skills/delete", url.Values{}, map[string]any{
			"ids": []string{item}, "noCopyConfirmed": args["noCopyConfirmed"] == true, "writtenConfirmed": args["writtenConfirmed"] == true,
		})
		if err != nil {
			return nil, err
		}
		logged()
		data := map[string]any{}
		if json.Unmarshal(w.body.Bytes(), &data) != nil {
			return nil, errors.New("unexpected API response")
		}
		return data, nil
	}
	if name == "reet_restore_skill" {
		status = "active"
	}
	if name == "reet_update_skill" && shelf == "" && len(fields) > 0 {
		return nil, errors.New("That skill has no likes or notes to change: it is not on your shelf. Only its status can change.")
	}
	var row map[string]any
	if status != "" {
		w, err := s.mcpDo(ctx, api, access, origin, "POST", "/api/v2/skills/status", url.Values{}, map[string]any{"ids": []string{item}, "status": status})
		if err != nil {
			return nil, err
		}
		logged()
		data := map[string]any{}
		if json.Unmarshal(w.body.Bytes(), &data) != nil {
			return nil, errors.New("unexpected API response")
		}
		rows, _ := mcpItems(data, "skills")
		for _, raw := range rows {
			if r, ok := raw.(map[string]any); ok && r["id"] == item {
				row = r
			}
		}
	}
	if name == "reet_restore_skill" {
		out := map[string]any{"id": item, "shelfId": shelf, "name": "", "status": "active"}
		if row != nil {
			out["shelfId"], out["name"] = row["shelfId"], row["name"]
		}
		return out, nil
	}
	if shelf == "" {
		// A skill with no shelf entry, such as one this account wrote and
		// never published, is its library row.
		if row == nil {
			return nil, errors.New("The status changed, but the skill could not be read back. Read it with reet_list_archived before changing it again.")
		}
		return row, nil
	}
	if len(fields) > 0 {
		fields["id"] = shelf
		w, err := s.mcpDo(ctx, api, access, origin, "PATCH", "/api/skill", url.Values{}, fields)
		if err != nil {
			return nil, err
		}
		logged()
		data := map[string]any{}
		if json.Unmarshal(w.body.Bytes(), &data) != nil {
			return nil, errors.New("unexpected API response")
		}
		return data, nil
	}
	sk, err := s.mcpShelfSkill(ctx, api, access, origin, shelf)
	if err != nil {
		return nil, err
	}
	if sk == nil {
		return nil, errMCPSkillNotFound
	}
	return sk, nil
}
