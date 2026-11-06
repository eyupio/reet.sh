package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
)

func mcpWorkerTool(name string) bool {
	return name == "reet_list_worker_access" || name == "reet_send_worker_message" || name == "reet_get_worker_message"
}
func (s *server) registerWorkerRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{accounts: "Worker messaging requires a browser account.", signedOut: "Sign in to manage worker messages.", readOnly: "A read-only account cannot grant worker messaging."}, h)
	}
	for _, path := range []string{"/api/v2/gaffer/conversations/{id}/worker", "/api/v2/mcp/connections/{id}/worker", "/api/v2/mcp/browser/worker"} {
		mux.HandleFunc("GET "+path, access(s.workerSettings))
		mux.HandleFunc("PUT "+path, access(s.workerConfigure))
		mux.HandleFunc("POST "+path+"/messages/{messageId}/interrupt", access(s.workerInterrupt))
	}
}
func (s *server) workerSettings(w http.ResponseWriter, r *http.Request) {
	kind, subject, session, err := s.contextTarget(r)
	if err != nil {
		gafferError(w, err)
		return
	}
	var v gaffer.WorkerSettings
	if session != "" {
		v, err = s.gafferProviders.WorkerSettings(s.uid(r), kind, subject, session)
	} else if kind == "gaffer" {
		v.Reason = "General chat has no worker. Open a structured session to relay an instruction."
	} else {
		_, err = s.st.CheckContextManagementSubject(s.uid(r), kind, subject)
	}
	if err != nil {
		gafferError(w, err)
		return
	}
	messages := []store.WorkerMessage{}
	if session != "" {
		messages, err = s.st.WorkerMessagesFor(s.uid(r), kind, subject, session)
		if err != nil {
			gafferError(w, err)
			return
		}
	}
	out := map[string]any{"worker": v, "messages": messages}
	if kind != "gaffer" {
		choices, err := s.st.ContextSessionChoicesFor(s.uid(r))
		if err != nil {
			gafferError(w, err)
			return
		}
		out["sessions"] = choices
		// Paused clients remain manageable in the browser. Their grants are still
		// enforced by fresh active/scope checks on every actual tool call.
		scopes, err := s.st.WorkerGrantScopesFor(s.uid(r), kind, subject)
		if err != nil {
			gafferError(w, err)
			return
		}
		out["scopes"] = scopes
	}
	writeJSON(w, 200, out)
}
func (s *server) workerConfigure(w http.ResponseWriter, r *http.Request) {
	kind, subject, session, err := s.contextTarget(r)
	if err != nil {
		gafferError(w, err)
		return
	}
	var in store.WorkerGrant
	if !decodeStrictJSON(w, r, 2048, &in, "Reload worker permission before saving.") {
		return
	}
	v, err := s.gafferProviders.ConfigureWorker(s.uid(r), kind, subject, session, in)
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "worker-messaging-changed", "", map[string]any{"kind": kind, "subject": subject, "session": session, "enabled": v.Grant.Enabled, "revision": v.Grant.Revision})
	writeJSON(w, 200, map[string]any{"worker": v})
}
func (s *server) workerInterrupt(w http.ResponseWriter, r *http.Request) {
	kind, subject, session, err := s.contextTarget(r)
	if err != nil {
		gafferError(w, err)
		return
	}
	var in struct {
		Revision int64 `json:"revision"`
		Confirm  bool  `json:"confirm"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "Review the queued instruction before interrupting.") {
		return
	}
	if !in.Confirm {
		writeErr(w, 422, "Explicit confirmation is required to stop the worker and send.")
		return
	}
	m, err := s.gafferProviders.InterruptWorker(s.uid(r), kind, subject, session, r.PathValue("messageId"), in.Revision)
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "worker-message-interrupt-requested", "", map[string]any{"kind": kind, "subject": subject, "session": session, "message": m.ID, "queue": m.QueueID})
	writeJSON(w, 200, map[string]any{"message": m})
}
func (s *server) executeMCPWorker(ctx context.Context, access mcpAccess, name string, args map[string]any) (map[string]any, error) {
	if !access.canWrite() {
		return nil, gaffer.ErrContextForbidden
	}
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
	if name == "reet_list_worker_access" {
		scopes, err := s.st.WorkerScopesFor(access.User.ID, kind, subject)
		return map[string]any{"scopes": scopes}, err
	}
	session, _ := args["session_id"].(string)
	id, _ := args["request_id"].(string)
	if name == "reet_get_worker_message" {
		m, err := s.gafferProviders.WorkerMessageReceipt(access.User.ID, kind, subject, session, id)
		m.Text = ""
		return map[string]any{"message": m}, err
	}
	text, _ := args["text"].(string)
	m, err := s.gafferProviders.WorkerMessage(ctx, access.User.ID, kind, subject, session, id, "", text)
	if err == nil {
		s.event(access.User.ID, "mcp", "worker-message-requested", "", map[string]any{"session": session, "message": m.ID, "queue": m.QueueID, "state": m.State})
	}
	m.Text = ""
	return map[string]any{"message": m}, err
}
