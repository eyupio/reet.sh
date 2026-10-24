package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillpreview"
	"github.com/eyupio/reet.sh/internal/store"
)

const (
	previewRoot      = "/api/v2/skill-content-previews"
	previewConnector = "/api/v2/sync/skill-content-preview"
	previewInstance  = "ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12"
)

type previewHarness struct {
	t      *testing.T
	srv    *Server
	st     *store.Store
	agent  store.SyncAgent
	secret string
	token  string
	uid    int64
	cookie string
	pin    store.SkillContentTarget
	bundle store.PrivateSkillResult
}

func newPreviewHarness(t *testing.T) *previewHarness {
	t.Helper()
	srv, st, a, secret, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	if e := st.TouchEnvironmentFor(uid, a.Host, "agents", "", ""); e != nil {
		t.Fatal(e)
	}
	env := store.EnvironmentIDFor(uid, a.Host, "agents", "")
	if e := st.RecordInventoryFor(uid, env, true, store.MachineInfo{}); e != nil {
		t.Fatal(e)
	}
	h := &previewHarness{t: t, srv: srv, st: st, agent: a, secret: secret, token: token, uid: uid, cookie: terminalLogin(t, st, uid)}
	w := privateSkillHTTP(t, srv, "POST", "/api/v2/skill-bundles", privateSkillBody("preview_api_bundle_001", "", "PRIVATE_PIN_CONTENT"), "", "", token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := json.Unmarshal(w.Body.Bytes(), &h.bundle); e != nil {
		t.Fatal(e)
	}
	empty := ""
	raw, _ := json.Marshal(store.SkillContentTargetWrite{EnvironmentID: env, AgentID: a.ID, BundleID: h.bundle.Bundle.ID, RevisionID: h.bundle.Revision.ID, Mode: "additive", ExpectedVersion: &empty})
	w = privateSkillHTTP(t, srv, "POST", "/api/v2/skill-content-targets", string(raw), "", "", token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := json.Unmarshal(w.Body.Bytes(), &h.pin); e != nil {
		t.Fatal(e)
	}
	return h
}

// browser acts as the signed-in person; connector as the enrolled machine.
func (h *previewHarness) browser(method, path, body string) *httptest.ResponseRecorder {
	return privateSkillHTTP(h.t, h.srv, method, path, body, h.cookie, "https://reet.example", "")
}

func (h *previewHarness) connector(path, body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "https://reet.example"+previewConnector+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+h.token)
	r.Header.Set("X-Reet-Agent", h.agent.ID)
	r.Header.Set("X-Reet-Agent-Key", key)
	w := httptest.NewRecorder()
	h.srv.ServeHTTP(w, r)
	return w
}

func (h *previewHarness) negotiate() {
	h.t.Helper()
	w := h.connector("/work", `{"schemaVersion":1,"instanceId":"`+previewInstance+`"}`, h.secret)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"work":null`) {
		h.t.Fatal(w.Code, w.Body.String())
	}
}

func (h *previewHarness) createBody(key string) string {
	return fmt.Sprintf(`{"requestKey":%q,"targetId":%q,"expectedTargetVersion":%q}`, key, h.pin.ID, h.pin.Version)
}

func (h *previewHarness) absentReport(id string) string {
	raw, _ := json.Marshal(map[string]any{"id": id, "instanceId": previewInstance, "capture": skillpreview.Capture{SchemaVersion: 1, Adapter: skillpreview.Adapter, RootID: strings.Repeat("d", 64), RootOwnership: "exclusive", Presence: "absent", Complete: true, Issues: []skillpreview.Issue{}}})
	return string(raw)
}

func TestSkillContentPreviewAPIReviewsOnlyMetadataAfterNegotiation(t *testing.T) {
	h := newPreviewHarness(t)

	// Nothing can be asked of a machine that has not said it can answer.
	if w := h.browser("POST", previewRoot, h.createBody("http_preview_000000001")); w.Code != 409 {
		t.Fatal("created work before negotiation", w.Code, w.Body.String())
	}
	if w := h.connector("/work", `{"schemaVersion":1,"instanceId":"`+previewInstance+`"}`, ""); w.Code != 403 {
		t.Fatal("negotiated without the enrolment secret", w.Code)
	}
	if w := h.connector("/work", `{"schemaVersion":2,"instanceId":"`+previewInstance+`"}`, h.secret); w.Code != 400 {
		t.Fatal("accepted a future schema", w.Code)
	}
	h.negotiate()

	w := h.browser("POST", previewRoot, h.createBody("http_preview_000000001"))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created store.SkillContentPreview
	if e := json.Unmarshal(w.Body.Bytes(), &created); e != nil {
		t.Fatal(e)
	}
	if created.State != "queued" || created.TargetVersion != h.pin.Version || created.Authority != "preview_only" {
		t.Fatalf("%+v", created)
	}
	if again := h.browser("POST", previewRoot, h.createBody("http_preview_000000001")); again.Code != 200 || !strings.Contains(again.Body.String(), created.ID) {
		t.Fatal("a repeated request key should return the original", again.Code, again.Body.String())
	}

	w = h.connector("/work", `{"schemaVersion":1,"instanceId":"`+previewInstance+`"}`, h.secret)
	if w.Code != 200 || !strings.Contains(w.Body.String(), created.ID) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, private := range []string{h.bundle.Bundle.ID, h.bundle.Revision.ID, h.pin.ID, h.pin.ManifestDigest, "additive", "PRIVATE_PIN_CONTENT"} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatalf("machine work exposes %q: %s", private, w.Body.String())
		}
	}

	report := h.absentReport(created.ID)
	for name, bad := range map[string]string{
		"unknown field":    strings.TrimSuffix(report, "}") + `,"files":[]}`,
		"trailing JSON":    report + `{}`,
		"oversized":        strings.Repeat(" ", 1<<20) + report,
		"no capture":       fmt.Sprintf(`{"id":%q,"instanceId":%q}`, created.ID, previewInstance),
		"forged presence":  strings.Replace(report, `"presence":"absent"`, `"presence":"present"`, 1),
		"incomplete":       strings.Replace(report, `"complete":true`, `"complete":false`, 1),
		"capture bytes":    strings.Replace(report, `"issues":[]`, `"issues":[],"data":"UFJJVkFURQ=="`, 1),
		"future schema":    strings.Replace(report, `"schemaVersion":1,"adapter"`, `"schemaVersion":2,"adapter"`, 1),
		"traversal in id":  strings.Replace(report, created.ID, "../"+created.ID, 1),
		"relative root id": strings.Replace(report, strings.Repeat("d", 64), "/home/someone/.agents/skills", 1),
	} {
		if r := h.connector("/report", bad, h.secret); r.Code != 400 && r.Code != 404 {
			t.Fatalf("%s: %d %s", name, r.Code, r.Body.String())
		}
	}
	if r := h.connector("/report", report, ""); r.Code != 403 {
		t.Fatal("report without the enrolment secret", r.Code)
	}
	if got := h.browser("GET", previewRoot+"/"+created.ID, ""); !strings.Contains(got.Body.String(), `"state":"capturing"`) {
		t.Fatal("a rejected report changed the request", got.Body.String())
	}
	if r := h.connector("/report", report, h.secret); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := h.connector("/report", report, h.secret); r.Code != 200 {
		t.Fatal("a lost acknowledgement must be safe to retry", r.Code, r.Body.String())
	}

	got := h.browser("GET", previewRoot+"/"+created.ID, "")
	body := got.Body.String()
	if got.Code != 200 || !strings.Contains(body, `"state":"captured"`) || !strings.Contains(body, `"authority":"preview_only"`) || !strings.Contains(body, `"reviewHash":"`) {
		t.Fatal(got.Code, body)
	}
	if !regexp.MustCompile(`"inventoryRevision":"[0-9]+"`).MatchString(body) {
		t.Fatal("the 63-bit inventory revision must travel as a string", body)
	}
	for _, leaked := range []string{"PRIVATE_PIN_CONTENT", `"data":`, h.secret, h.token} {
		if strings.Contains(body, leaked) {
			t.Fatalf("review leaks %q", leaked)
		}
	}
	var full store.SkillContentPreview
	if e := json.Unmarshal(got.Body.Bytes(), &full); e != nil || full.Plan == nil || !full.Plan.Ready || len(full.Plan.Actions) != 2 {
		t.Fatal("a new install of the two private files should be ready to review", full.Plan, e)
	}
	list := h.browser("GET", previewRoot, "")
	if list.Code != 200 || strings.Contains(list.Body.String(), `"plan":`) || strings.Contains(list.Body.String(), `"capture":`) || !strings.Contains(list.Body.String(), `"ready":true`) {
		t.Fatal("list must be bounded summaries plus readiness", list.Code, list.Body.String())
	}
	for _, r := range []*httptest.ResponseRecorder{got, list} {
		if r.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private review cached")
		}
	}

	// Nothing in this slice writes, queues or edits intent.
	if job, e := h.st.ClaimSyncJob(h.uid, h.agent); e != nil || job != nil {
		t.Fatal("a legacy poll received preview work", job, e)
	}
	if pin, e := h.st.SkillContentTargetFor(h.uid, h.pin.ID); e != nil || pin.Version != h.pin.Version {
		t.Fatal("the pin changed", pin, e)
	}
	for _, p := range []string{previewRoot, previewRoot + "/" + created.ID, previewRoot + "/" + created.ID + "/cancel", previewConnector + "/work", previewConnector + "/report", previewConnector + "/failure"} {
		for _, method := range []string{"GET", "POST", "DELETE"} {
			if mcpAPIPath(method, p) {
				t.Fatalf("MCP allowlist expanded: %s %s", method, p)
			}
		}
	}
}

func TestSkillContentPreviewAPIBoundariesAndIsolation(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	viewer, e := h.st.CreateUser("preview-viewer@example.test", "Reader", "viewer", "hash")
	if e != nil {
		t.Fatal(e)
	}
	other, e := h.st.CreateUser("preview-other@example.test", "Other", "member", "hash")
	if e != nil {
		t.Fatal(e)
	}
	viewerCookie, otherCookie := terminalLogin(t, h.st, viewer.ID), terminalLogin(t, h.st, other.ID)
	body := h.createBody("http_preview_boundary_1")
	for _, tc := range []struct {
		name, method, path, body, cookie, origin string
		status                                   int
	}{
		{"anonymous", "GET", previewRoot, "", "", "", 401},
		{"viewer read", "GET", previewRoot, "", viewerCookie, "", 403},
		{"viewer create", "POST", previewRoot, body, viewerCookie, "https://reet.example", 403},
		{"missing origin", "POST", previewRoot, body, h.cookie, "", 403},
		{"foreign origin", "POST", previewRoot, body, h.cookie, "https://foreign.example", 403},
		{"origin with a path", "POST", previewRoot, body, h.cookie, "https://reet.example/x", 403},
		{"query string", "GET", previewRoot + "?x=1", "", h.cookie, "", 400},
		{"empty query string", "GET", previewRoot + "?", "", h.cookie, "", 400},
		{"unknown owner field", "POST", previewRoot, strings.TrimSuffix(body, "}") + `,"userId":9}`, h.cookie, "https://reet.example", 400},
		{"caller-supplied revision", "POST", previewRoot, strings.TrimSuffix(body, "}") + `,"revisionId":"` + h.pin.RevisionID + `"}`, h.cookie, "https://reet.example", 400},
		{"caller-supplied planner snapshot", "POST", previewRoot, strings.TrimSuffix(body, "}") + `,"snapshot":{}}`, h.cookie, "https://reet.example", 400},
		{"trailing JSON", "POST", previewRoot, body + `{}`, h.cookie, "https://reet.example", 400},
		{"oversized", "POST", previewRoot, strings.Repeat(" ", 16<<10) + body, h.cookie, "https://reet.example", 400},
		{"short request key", "POST", previewRoot, strings.Replace(body, "http_preview_boundary_1", "short", 1), h.cookie, "https://reet.example", 400},
		{"missing version", "POST", previewRoot, fmt.Sprintf(`{"requestKey":"http_preview_boundary_1","targetId":%q}`, h.pin.ID), h.cookie, "https://reet.example", 400},
		{"stale pin version", "POST", previewRoot, strings.Replace(body, h.pin.Version, strings.Repeat("1", 48), 1), h.cookie, "https://reet.example", 409},
		{"unknown pin", "POST", previewRoot, strings.Replace(body, h.pin.ID, strings.Repeat("0", 48), 1), h.cookie, "https://reet.example", 404},
		{"foreign account's pin", "POST", previewRoot, body, otherCookie, "https://reet.example", 404},
		{"cancel body with fields", "POST", previewRoot + "/" + strings.Repeat("a", 48) + "/cancel", `{"x":1}`, h.cookie, "https://reet.example", 400},
		{"unknown request", "GET", previewRoot + "/" + strings.Repeat("a", 48), "", h.cookie, "", 404},
		{"malformed request id", "GET", previewRoot + "/not-an-id", "", h.cookie, "", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := privateSkillHTTP(t, h.srv, tc.method, tc.path, tc.body, tc.cookie, tc.origin, "")
			if r.Code != tc.status {
				t.Fatal(r.Code, r.Body.String())
			}
		})
	}

	w := h.browser("POST", previewRoot, body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created store.SkillContentPreview
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	// A second review of the same machine waits for the first.
	if r := h.browser("POST", previewRoot, h.createBody("http_preview_boundary_2")); r.Code != 409 {
		t.Fatal("two active previews for one machine", r.Code)
	}
	for _, tc := range []struct{ method, path, cookie string }{
		{"GET", previewRoot + "/" + created.ID, otherCookie},
		{"POST", previewRoot + "/" + created.ID + "/cancel", otherCookie},
		{"DELETE", previewRoot + "/" + created.ID, otherCookie},
	} {
		body := ""
		if strings.HasSuffix(tc.path, "/cancel") {
			body = `{}`
		}
		if r := privateSkillHTTP(t, h.srv, tc.method, tc.path, body, tc.cookie, "https://reet.example", ""); r.Code != 404 {
			t.Fatal("foreign account reached a private review", tc.method, r.Code)
		}
	}
	// The browser's own credentials are not an enrolment: it cannot report a capture.
	if r := privateSkillHTTP(t, h.srv, "POST", previewConnector+"/report", h.absentReport(created.ID), h.cookie, "https://reet.example", ""); r.Code == 200 {
		t.Fatal("a browser session recorded a machine capture")
	}
	// A different enrolled machine cannot report for this one.
	second, secondSecret, e := h.st.EnrollSyncAgent(h.uid, "second-machine", "test")
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "https://reet.example"+previewConnector+"/report", strings.NewReader(h.absentReport(created.ID)))
	r.Header.Set("Authorization", "Bearer "+h.token)
	r.Header.Set("X-Reet-Agent", second.ID)
	r.Header.Set("X-Reet-Agent-Key", secondSecret)
	rec := httptest.NewRecorder()
	h.srv.ServeHTTP(rec, r)
	if rec.Code == 200 {
		t.Fatal("another machine recorded a capture for this request", rec.Body.String())
	}

	// Cancelling revokes the work; the machine cannot then report.
	if r := h.browser("POST", previewRoot+"/"+created.ID+"/cancel", `{}`); r.Code != 200 || !strings.Contains(r.Body.String(), `"state":"cancelled"`) {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := h.connector("/report", h.absentReport(created.ID), h.secret); r.Code != 409 {
		t.Fatal("recorded after cancellation", r.Code, r.Body.String())
	}
	if r := h.browser("DELETE", previewRoot+"/"+created.ID, ""); r.Code != 200 {
		t.Fatal(r.Code)
	}
	if r := h.browser("GET", previewRoot+"/"+created.ID, ""); r.Code != 404 {
		t.Fatal("dismissed history still readable", r.Code)
	}
	if _, e := h.st.PrivateSkillRevisionFor(h.uid, h.bundle.Bundle.ID, h.bundle.Revision.ID); e != nil {
		t.Fatal("dismissing history removed private bytes", e)
	}
}

func TestSkillContentPreviewAPIFailureCodesAreBounded(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	w := h.browser("POST", previewRoot, h.createBody("http_preview_failure_01"))
	var created store.SkillContentPreview
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	h.connector("/work", `{"schemaVersion":1,"instanceId":"`+previewInstance+`"}`, h.secret)
	failure := func(code string) *httptest.ResponseRecorder {
		return h.connector("/failure", fmt.Sprintf(`{"id":%q,"instanceId":%q,"code":%q}`, created.ID, previewInstance, code), h.secret)
	}
	for _, bad := range []string{"open /home/someone/.claude/skills/demo: permission denied", "", "CAPTURE_FAILED", strings.Repeat("a", 65), "unsupported_platform"} {
		if r := failure(bad); r.Code != 400 {
			t.Fatalf("accepted free-form failure text %q: %d", bad, r.Code)
		}
	}
	if r := failure("scope_unavailable"); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	got := h.browser("GET", previewRoot+"/"+created.ID, "")
	if !strings.Contains(got.Body.String(), `"state":"failed"`) || !strings.Contains(got.Body.String(), `"error":"scope_unavailable"`) {
		t.Fatal(got.Body.String())
	}
}

func TestSkillContentPreviewAPIRejectsLegacyAuthModes(t *testing.T) {
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		t.Run(string(mode), func(t *testing.T) {
			srv, _, _, _, _ := privateFixture(t, mode, "legacy-token")
			for _, method := range []string{"GET", "POST", "DELETE"} {
				for _, path := range []string{previewRoot, previewRoot + "/" + strings.Repeat("a", 48), previewConnector + "/work"} {
					w := privateSkillHTTP(t, srv, method, path, `{}`, "", "https://reet.example", "legacy-token")
					if w.Code != 403 && w.Code != 404 && w.Code != 405 {
						t.Fatal(method, path, w.Code, w.Body.String())
					}
					if w.Code == 200 {
						t.Fatal("legacy auth reached private previews")
					}
				}
			}
		})
	}
}
