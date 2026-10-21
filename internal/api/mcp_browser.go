package api

import (
	"encoding/json"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/google/jsonschema-go/jsonschema"
)

var mcpInputSchemas = func() map[string]*jsonschema.Resolved {
	out := map[string]*jsonschema.Resolved{}
	for _, tool := range mcpToolDefinitions {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			panic(err)
		}
		var schema jsonschema.Schema
		if err = json.Unmarshal(raw, &schema); err != nil {
			panic(err)
		}
		resolved, err := schema.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
		if err != nil {
			panic(err)
		}
		out[tool.Name] = resolved
	}
	return out
}()

func (s *server) browserAccess(w http.ResponseWriter, r *http.Request) (mcpAccess, bool) {
	enabled, err := s.st.MCPEnabled(s.uid(r))
	if err != nil || !enabled {
		writeErr(w, 403, "MCP access is paused")
		return mcpAccess{}, false
	}
	browser, scope, err := s.st.MCPBrowser(s.uid(r))
	if err != nil || !browser {
		writeErr(w, 403, "browser WebMCP is not enabled")
		return mcpAccess{}, false
	}
	return mcpAccess{User: s.currentUser(r), Connection: store.MCPConnection{Name: "Browser WebMCP", Scope: scope}}, true
}

func (s *server) mcpBrowserTools(w http.ResponseWriter, r *http.Request) {
	access, ok := s.browserAccess(w, r)
	if !ok {
		return
	}
	tools := []any{}
	for _, tool := range mcpToolDefinitions {
		if access.canWrite() || tool.Annotations.ReadOnlyHint {
			tools = append(tools, tool)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"tools": tools})
}

func (s *server) mcpBrowserCall(w http.ResponseWriter, r *http.Request, api http.Handler) {
	access, ok := s.browserAccess(w, r)
	if !ok {
		return
	}
	var in struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Arguments == nil {
		in.Arguments = map[string]any{}
	}
	schema, ok := mcpInputSchemas[in.Name]
	if !ok {
		writeErr(w, 400, "unknown tool")
		return
	}
	if err := schema.ApplyDefaults(&in.Arguments); err != nil {
		writeErr(w, 400, "invalid tool arguments")
		return
	}
	if err := schema.Validate(in.Arguments); err != nil {
		writeErr(w, 400, "arguments do not match the tool schema")
		return
	}
	data, err := s.executeMCP(r.Context(), api, access, in.Name, in.Arguments)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out := map[string]any{"data": data}
	raw, _ := json.Marshal(out)
	if len(raw) > mcpResponseLimit {
		writeErr(w, 400, "response is too large; use a smaller page")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"content": []map[string]string{{"type": "text", "text": string(raw)}}, "structuredContent": out})
}
