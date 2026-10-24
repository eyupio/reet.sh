package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillplan"
	"github.com/eyupio/reet.sh/internal/store"
	_ "modernc.org/sqlite"
)

const receiptRoot = "/api/v2/skill-content-receipts"

// capture runs one preview of the pinned slot to the point where the machine has
// reported a present, exclusive folder holding exactly the pinned revision.
func (h *previewHarness) capture(key string) string {
	h.t.Helper()
	w := h.browser("POST", previewRoot, h.createBody(key))
	var created store.SkillContentPreview
	if e := json.Unmarshal(w.Body.Bytes(), &created); w.Code != 200 || e != nil {
		h.t.Fatal(w.Code, w.Body.String(), e)
	}
	if w = h.connector("/work", `{"schemaVersion":1,"instanceId":"`+previewInstance+`"}`, h.secret); w.Code != 200 || !strings.Contains(w.Body.String(), created.ID) {
		h.t.Fatal(w.Code, w.Body.String())
	}
	if w = h.connector("/report", h.presentReport(created.ID, *h.bundle.Revision.Manifest), h.secret); w.Code != 200 {
		h.t.Fatal(w.Code, w.Body.String())
	}
	return created.ID
}

// captured is capture plus the review a person would be shown before adopting.
func (h *previewHarness) captured(key string) store.SkillContentPreview {
	h.t.Helper()
	w := h.browser("GET", previewRoot+"/"+h.capture(key), "")
	var p store.SkillContentPreview
	if e := json.Unmarshal(w.Body.Bytes(), &p); w.Code != 200 || e != nil || p.State != "captured" || p.ReviewHash == "" {
		h.t.Fatal(w.Code, w.Body.String(), e)
	}
	return p
}

func adoptJSON(key, previewID, reviewHash, revisionID string) string {
	return fmt.Sprintf(`{"requestKey":%q,"previewId":%q,"reviewHash":%q,"adoptAsRevisionId":%q}`, key, previewID, reviewHash, revisionID)
}

func (h *previewHarness) adoptBody(key string, p store.SkillContentPreview) string {
	return adoptJSON(key, p.ID, p.ReviewHash, h.bundle.Revision.ID)
}

func (h *previewHarness) adopt(key string, p store.SkillContentPreview) store.SkillContentReceipt {
	h.t.Helper()
	w := h.browser("POST", receiptRoot, h.adoptBody(key, p))
	var r store.SkillContentReceipt
	if e := json.Unmarshal(w.Body.Bytes(), &r); w.Code != 200 || e != nil || r.ID == "" {
		h.t.Fatal(w.Code, w.Body.String(), e)
	}
	return r
}

func receiptEvent(t *testing.T, st *store.Store, uid int64, action string) (store.Event, map[string]any) {
	t.Helper()
	events, err := st.ListEventsFor(uid, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	var found []store.Event
	for _, ev := range events {
		if ev.Action == action {
			found = append(found, ev)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected one %s event, got %+v", action, found)
	}
	detail := map[string]any{}
	if e := json.Unmarshal([]byte(found[0].Detail), &detail); e != nil {
		t.Fatal(found[0].Detail, e)
	}
	return found[0], detail
}

func TestSkillContentReceiptAPIAdoptsReleasesAndStaysMetadataOnly(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	var bodies []string
	seen := func(w *httptest.ResponseRecorder) *httptest.ResponseRecorder {
		t.Helper()
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private ownership record cached", w.Header())
		}
		bodies = append(bodies, w.Body.String())
		return w
	}

	if w := seen(h.browser("GET", receiptRoot, "")); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"receipts":[]}` {
		t.Fatal("an account with no ownership records should list an empty array", w.Code, w.Body.String())
	}

	// A folder equal to the pinned revision is offered that revision, never assigned it.
	id := h.capture("receipt_flow_preview_01")
	w := seen(h.browser("GET", previewRoot+"/"+id, ""))
	var p store.SkillContentPreview
	if e := json.Unmarshal(w.Body.Bytes(), &p); w.Code != 200 || e != nil || p.State != "captured" {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	if p.Adoption == nil || len(p.Adoption.Candidates) != 1 || p.Adoption.Candidates[0] != (store.AdoptionCandidate{RevisionID: h.bundle.Revision.ID, Matched: 2}) {
		t.Fatalf("the review should offer the pinned revision: %+v", p.Adoption)
	}
	if !strings.Contains(w.Body.String(), `"adoption":{"candidates":[`) {
		t.Fatal("the offer must travel on the preview", w.Body.String())
	}

	w = seen(h.browser("POST", receiptRoot, h.adoptBody("receipt_flow_adopt_0001", p)))
	var receipt store.SkillContentReceipt
	if e := json.Unmarshal(w.Body.Bytes(), &receipt); w.Code != 200 || e != nil {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	if receipt.ID == "" || receipt.Name != "demo" || receipt.BundleID != h.bundle.Bundle.ID || receipt.RevisionID != h.bundle.Revision.ID || receipt.FileCount != 2 ||
		receipt.Provenance != "adopted" || receipt.Verification != "controller_attested" || receipt.Managed == nil || len(receipt.Managed.Files) != 2 ||
		receipt.Evidence.PreviewID != p.ID || receipt.Evidence.PinID != h.pin.ID || receipt.Evidence.AdoptedBy != h.uid {
		t.Fatalf("%+v", receipt)
	}

	// The list is a summary; the item adds the managed file metadata.
	w = seen(h.browser("GET", receiptRoot, ""))
	var list struct {
		Receipts []store.SkillContentReceipt `json:"receipts"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &list); w.Code != 200 || e != nil || len(list.Receipts) != 1 || list.Receipts[0].ID != receipt.ID || list.Receipts[0].Version != receipt.Version || list.Receipts[0].Managed != nil || strings.Contains(w.Body.String(), `"managed"`) {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	w = seen(privateSkillHTTP(t, h.srv, "GET", receiptRoot+"/"+receipt.ID, "", "", "", h.token))
	var item store.SkillContentReceipt
	if e := json.Unmarshal(w.Body.Bytes(), &item); w.Code != 200 || e != nil || !reflect.DeepEqual(item, receipt) {
		t.Fatalf("the item read should return the adopted receipt: %d %s %v", w.Code, w.Body.String(), e)
	}
	if got := seen(h.browser("GET", previewRoot+"/"+id, "")); !strings.Contains(got.Body.String(), `"state":"superseded"`) {
		t.Fatal("adopting should consume its preview", got.Body.String())
	}

	// Release removes the claim and nothing else, and needs only the version it was shown.
	w = seen(privateSkillHTTP(t, h.srv, "DELETE", receiptRoot+"/"+receipt.ID, fmt.Sprintf(`{"expectedVersion":%q}`, receipt.Version), "", "", h.token))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"removed":true}` {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = seen(h.browser("GET", receiptRoot+"/"+receipt.ID, "")); w.Code != 404 {
		t.Fatal("a released record is still readable", w.Code, w.Body.String())
	}
	if w = seen(h.browser("GET", receiptRoot, "")); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"receipts":[]}` {
		t.Fatal(w.Code, w.Body.String())
	}
	if pin, e := h.st.SkillContentTargetFor(h.uid, h.pin.ID); e != nil || pin.Version != h.pin.Version {
		t.Fatal("releasing ownership changed the pin", pin, e)
	}
	if _, e := h.st.PrivateSkillRevisionFor(h.uid, h.bundle.Bundle.ID, h.bundle.Revision.ID); e != nil {
		t.Fatal("releasing ownership removed private bytes", e)
	}

	// Nothing on the wire carries bytes, secrets or a host path.
	absolute := regexp.MustCompile(`"(/|[A-Za-z]:\\)`)
	for _, body := range bodies {
		for _, leaked := range []string{"PRIVATE_PIN_CONTENT", `"data":`, h.secret, h.token, h.st.Path()} {
			if strings.Contains(body, leaked) {
				t.Fatalf("response leaks %q: %s", leaked, body)
			}
		}
		if absolute.MatchString(body) {
			t.Fatal("response carries an absolute path", body)
		}
	}

	// The audit trail names the skill and the record, and never a hash, path or byte.
	adopted, detail := receiptEvent(t, h.st, h.uid, "skill-content-adopt")
	if adopted.Source != "ui" || adopted.SkillID != "" || !reflect.DeepEqual(detail, map[string]any{"name": "demo", "files": float64(2), "receiptId": receipt.ID}) {
		t.Fatalf("%+v %v", adopted, detail)
	}
	released, detail := receiptEvent(t, h.st, h.uid, "skill-content-release")
	if released.Source != "ui" || released.SkillID != "" || !reflect.DeepEqual(detail, map[string]any{"name": "demo", "receiptId": receipt.ID}) {
		t.Fatalf("%+v %v", released, detail)
	}
}

func TestSkillContentReceiptAPIBoundaries(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	viewer, e := h.st.CreateUser("receipt-viewer@example.test", "Reader", "viewer", "hash")
	if e != nil {
		t.Fatal(e)
	}
	other, e := h.st.CreateUser("receipt-other@example.test", "Other", "member", "hash")
	if e != nil {
		t.Fatal(e)
	}
	viewerCookie, otherCookie := terminalLogin(t, h.st, viewer.ID), terminalLogin(t, h.st, other.ID)
	// A newer revision whose SKILL.md differs from the folder, so it proves nothing.
	w := privateSkillHTTP(t, h.srv, "POST", "/api/v2/skill-bundles/"+h.bundle.Bundle.ID+"/revisions", privateSkillBody("receipt_api_revision_002", h.bundle.Revision.ID, "A_DIFFERENT_SKILL_TEXT"), "", "", h.token)
	var newer store.PrivateSkillResult
	if e = json.Unmarshal(w.Body.Bytes(), &newer); w.Code != 200 || e != nil {
		t.Fatal(w.Code, w.Body.String(), e)
	}

	p := h.captured("receipt_boundary_preview_1")
	const origin, key = "https://reet.example", "receipt_boundary_adopt_01"
	body := h.adoptBody(key, p)
	unknownID := strings.Repeat("0", 48)
	type boundary struct {
		name, method, path, body, cookie, origin string
		status                                   int
		message                                  string
	}
	run := func(cases []boundary) {
		t.Helper()
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := privateSkillHTTP(t, h.srv, tc.method, tc.path, tc.body, tc.cookie, tc.origin, "")
				if r.Code != tc.status || !strings.Contains(r.Body.String(), tc.message) {
					t.Fatal(r.Code, r.Body.String())
				}
				if r.Header().Get("Cache-Control") != "no-store" && r.Code != 401 {
					t.Fatal("refusal cached")
				}
			})
		}
	}
	const (
		invalid  = "Supply a request key, a captured preview with its review hash, and the library revision it was installed from."
		missing  = "Ownership record or preview unavailable."
		conflict = "This ownership record is unavailable, blocked, changed or already exists. Preview again, and release existing ownership before adopting again."
		notClaim = "That revision does not match this folder's SKILL.md, so nothing can be claimed."
	)
	run([]boundary{
		{"anonymous", "GET", receiptRoot, "", "", "", 401, ""},
		{"viewer list", "GET", receiptRoot, "", viewerCookie, "", 403, ""},
		{"viewer adopt", "POST", receiptRoot, body, viewerCookie, origin, 403, ""},
		{"viewer item", "GET", receiptRoot + "/" + unknownID, "", viewerCookie, "", 403, ""},
		{"viewer release", "DELETE", receiptRoot + "/" + unknownID, `{"expectedVersion":"` + unknownID + `"}`, viewerCookie, origin, 403, ""},
		{"missing origin", "POST", receiptRoot, body, h.cookie, "", 403, ""},
		{"foreign origin", "POST", receiptRoot, body, h.cookie, "https://foreign.example", 403, ""},
		{"origin with a path", "POST", receiptRoot, body, h.cookie, origin + "/x", 403, ""},
		{"release without an origin", "DELETE", receiptRoot + "/" + unknownID, `{"expectedVersion":"` + unknownID + `"}`, h.cookie, "", 403, ""},
		{"query string", "GET", receiptRoot + "?x=1", "", h.cookie, "", 400, ""},
		{"empty query string", "GET", receiptRoot + "?", "", h.cookie, "", 400, ""},
		{"query string on an item", "GET", receiptRoot + "/" + unknownID + "?x=1", "", h.cookie, "", 400, ""},
		{"unknown owner field", "POST", receiptRoot, strings.TrimSuffix(body, "}") + `,"userId":9}`, h.cookie, origin, 400, ""},
		{"expected version on adoption", "POST", receiptRoot, strings.TrimSuffix(body, "}") + `,"expectedVersion":"` + unknownID + `"}`, h.cookie, origin, 400, ""},
		{"caller-supplied file list", "POST", receiptRoot, strings.TrimSuffix(body, "}") + `,"managed":{}}`, h.cookie, origin, 400, ""},
		{"trailing JSON", "POST", receiptRoot, body + `{}`, h.cookie, origin, 400, ""},
		{"oversized", "POST", receiptRoot, strings.Repeat(" ", 16<<10) + body, h.cookie, origin, 400, ""},
		{"empty body", "POST", receiptRoot, `{}`, h.cookie, origin, 400, invalid},
		{"short request key", "POST", receiptRoot, adoptJSON("short", p.ID, p.ReviewHash, h.bundle.Revision.ID), h.cookie, origin, 400, invalid},
		{"malformed preview ID", "POST", receiptRoot, adoptJSON(key, "not-an-id", p.ReviewHash, h.bundle.Revision.ID), h.cookie, origin, 400, invalid},
		{"short review hash", "POST", receiptRoot, adoptJSON(key, p.ID, p.ReviewHash[:63], h.bundle.Revision.ID), h.cookie, origin, 400, invalid},
		{"review hash in capitals", "POST", receiptRoot, adoptJSON(key, p.ID, strings.ToUpper(p.ReviewHash), h.bundle.Revision.ID), h.cookie, origin, 400, invalid},
		{"digest as revision ID", "POST", receiptRoot, adoptJSON(key, p.ID, p.ReviewHash, h.pin.ManifestDigest), h.cookie, origin, 400, invalid},
		{"wrong review hash", "POST", receiptRoot, adoptJSON(key, p.ID, strings.Repeat("0", 64), h.bundle.Revision.ID), h.cookie, origin, 409, conflict},
		{"unknown preview", "POST", receiptRoot, adoptJSON(key, unknownID, p.ReviewHash, h.bundle.Revision.ID), h.cookie, origin, 404, missing},
		{"unknown revision", "POST", receiptRoot, adoptJSON(key, p.ID, p.ReviewHash, strings.Repeat("0", 32)), h.cookie, origin, 404, ""},
		{"revision that does not match the folder", "POST", receiptRoot, adoptJSON(key, p.ID, p.ReviewHash, newer.Revision.ID), h.cookie, origin, 409, notClaim},
		{"foreign account's preview", "POST", receiptRoot, body, otherCookie, origin, 404, missing},
		{"unknown record", "GET", receiptRoot + "/" + unknownID, "", h.cookie, "", 404, missing},
		{"malformed record ID", "GET", receiptRoot + "/not-an-id", "", h.cookie, "", 404, missing},
		{"release of an unknown record", "DELETE", receiptRoot + "/" + unknownID, `{"expectedVersion":"` + unknownID + `"}`, h.cookie, origin, 404, missing},
		{"release of a malformed record ID", "DELETE", receiptRoot + "/not-an-id", `{"expectedVersion":"` + unknownID + `"}`, h.cookie, origin, 400, invalid},
	})
	if list, e := h.st.SkillContentReceiptsFor(h.uid); e != nil || len(list) != 0 {
		t.Fatal("a refused request recorded ownership", list, e)
	}

	receipt := h.adopt(key, p)
	path, release := receiptRoot+"/"+receipt.ID, fmt.Sprintf(`{"expectedVersion":%q}`, receipt.Version)
	run([]boundary{
		{"slot that already has a record", "POST", receiptRoot, h.adoptBody("receipt_boundary_adopt_02", p), h.cookie, origin, 409, conflict},
		{"foreign account's record", "GET", path, "", otherCookie, "", 404, missing},
		{"foreign account's release", "DELETE", path, release, otherCookie, origin, 404, missing},
		{"release without a version", "DELETE", path, `{}`, h.cookie, origin, 400, invalid},
		{"release with a null version", "DELETE", path, `{"expectedVersion":null}`, h.cookie, origin, 400, invalid},
		{"release without a body", "DELETE", path, "", h.cookie, origin, 400, ""},
		{"release with an unknown field", "DELETE", path, strings.TrimSuffix(release, "}") + `,"force":true}`, h.cookie, origin, 400, ""},
		{"release with trailing JSON", "DELETE", path, release + `{}`, h.cookie, origin, 400, ""},
		{"release with an oversized body", "DELETE", path, strings.Repeat(" ", 16<<10) + release, h.cookie, origin, 400, ""},
		{"release with a malformed version", "DELETE", path, `{"expectedVersion":"latest"}`, h.cookie, origin, 400, invalid},
		{"release with an empty version", "DELETE", path, `{"expectedVersion":""}`, h.cookie, origin, 400, invalid},
		{"release with a stale version", "DELETE", path, `{"expectedVersion":"` + strings.Repeat("e", 48) + `"}`, h.cookie, origin, 409, conflict},
	})
	if got, e := h.st.SkillContentReceiptFor(h.uid, receipt.ID); e != nil || !reflect.DeepEqual(got, receipt) {
		t.Fatal("a refused request changed the record", got, e)
	}

	// None of this is reachable by an MCP client.
	for _, p := range []string{receiptRoot, path} {
		for _, method := range []string{"GET", "POST", "DELETE"} {
			if mcpAPIPath(method, p) {
				t.Fatalf("MCP allowlist expanded: %s %s", method, p)
			}
		}
	}

	// A person demoted after adopting loses every route, by session and by token.
	if e = h.st.UpdateUserRole(h.uid, "viewer"); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ method, path, body string }{
		{"GET", receiptRoot, ""},
		{"POST", receiptRoot, body},
		{"GET", path, ""},
		{"DELETE", path, release},
	} {
		for name, as := range map[string][2]string{"session": {h.cookie, ""}, "token": {"", h.token}} {
			if r := privateSkillHTTP(t, h.srv, tc.method, tc.path, tc.body, as[0], origin, as[1]); r.Code != 403 {
				t.Fatalf("a demoted account reached %s %s by %s: %d %s", tc.method, tc.path, name, r.Code, r.Body.String())
			}
		}
	}
	if list, e := h.st.SkillContentReceiptsFor(h.uid); e != nil || len(list) != 0 {
		t.Fatal("the store should show a demoted account nothing", list, e)
	}
	if e = h.st.UpdateUserRole(h.uid, "owner"); e != nil {
		t.Fatal(e)
	}
	if got, e := h.st.SkillContentReceiptFor(h.uid, receipt.ID); e != nil || got.Version != receipt.Version {
		t.Fatal("demotion changed the record", got, e)
	}
}

func TestSkillContentReceiptErrorMapping(t *testing.T) {
	const (
		invalid  = "Supply a request key, a captured preview with its review hash, and the library revision it was installed from."
		missing  = "Ownership record or preview unavailable."
		conflict = "This ownership record is unavailable, blocked, changed or already exists. Preview again, and release existing ownership before adopting again."
	)
	for name, tc := range map[string]struct {
		err    error
		status int
		want   string
	}{
		"invalid request":      {store.ErrSkillContentReceiptInvalid, 400, invalid},
		"missing record":       {store.ErrSkillContentReceiptMissing, 404, missing},
		"missing preview":      {store.ErrSkillContentPreviewMissing, 404, missing},
		"receipt conflict":     {store.ErrSkillContentReceipt, 409, conflict},
		"preview conflict":     {store.ErrSkillContentPreview, 409, conflict},
		"pin conflict":         {store.ErrSkillContentTargetConflict, 409, conflict},
		"scope":                {store.ErrSkillContentTargetScope, 409, conflict},
		"paused connector":     {store.ErrContentObservationStale, 409, conflict},
		"unknown connector":    {store.ErrRemoteNotFound, 409, conflict},
		"revision not matched": {skillplan.ErrNotClaimable, 409, "That revision does not match this folder's SKILL.md, so nothing can be claimed."},
		"capacity":             {store.ErrSkillContentReceiptCapacity, 409, "Ownership record limit reached (1024 per account)."},
		"wrapped sentinel":     {fmt.Errorf("adopt: %w", skillplan.ErrNotClaimable), 409, "That revision does not match this folder's SKILL.md, so nothing can be claimed."},
		"missing revision":     {store.ErrPrivateSkillMissing, 404, "Private skill unavailable."},
		"integrity fault":      {errors.New("stored ownership receipt failed integrity validation"), 500, "Could not access private skill storage."},
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			skillContentReceiptError(w, tc.err)
			var out map[string]string
			if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil || w.Code != tc.status || out["error"] != tc.want {
				t.Fatal(w.Code, w.Body.String(), e)
			}
		})
	}
}

func TestSkillContentReceiptAPIListsAndReleasesADamagedReceipt(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	receipt := h.adopt("receipt_damaged_adopt_01", h.captured("receipt_damaged_preview_1"))

	// Break the stored manifest from outside the store, as disk damage would.
	db, e := sql.Open("sqlite", "file:"+filepath.Join(h.st.Path(), "reet.db")+"?_pragma=busy_timeout(5000)")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(`UPDATE skill_content_receipts SET managed='not json' WHERE id=?`, receipt.ID); e != nil {
		t.Fatal(e)
	}

	w := h.browser("GET", receiptRoot, "")
	var list struct {
		Receipts []store.SkillContentReceipt `json:"receipts"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &list); w.Code != 200 || e != nil || len(list.Receipts) != 1 {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	got := list.Receipts[0]
	if !got.Damaged || got.ID != receipt.ID || got.Name != "demo" || got.Version != receipt.Version || got.Provenance != "" || got.Verification != "" || got.Managed != nil ||
		got.ManagedDigest != "" || got.FileCount != 0 || got.Evidence != (store.SkillContentReceiptEvidence{}) || !strings.Contains(w.Body.String(), `"damaged":true`) {
		t.Fatalf("a damaged record should list with only its identity and version: %s", w.Body.String())
	}

	// Nothing that proves ownership accepts it: an item read is a fault, not a miss or a conflict.
	if r := h.browser("GET", receiptRoot+"/"+receipt.ID, ""); r.Code != 500 || !strings.Contains(r.Body.String(), "Could not access private skill storage.") {
		t.Fatal(r.Code, r.Body.String())
	}

	// The version it was listed with is enough to release it.
	if r := h.browser("DELETE", receiptRoot+"/"+receipt.ID, fmt.Sprintf(`{"expectedVersion":%q}`, strings.Repeat("e", 48))); r.Code != 409 {
		t.Fatal("a stale version released a damaged record", r.Code, r.Body.String())
	}
	if r := h.browser("DELETE", receiptRoot+"/"+receipt.ID, fmt.Sprintf(`{"expectedVersion":%q}`, got.Version)); r.Code != 200 || strings.TrimSpace(r.Body.String()) != `{"removed":true}` {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := h.browser("GET", receiptRoot, ""); r.Code != 200 || strings.TrimSpace(r.Body.String()) != `{"receipts":[]}` {
		t.Fatal(r.Code, r.Body.String())
	}
	if _, detail := receiptEvent(t, h.st, h.uid, "skill-content-release"); !reflect.DeepEqual(detail, map[string]any{"name": "demo", "receiptId": receipt.ID}) {
		t.Fatal("a damaged record should still be released by name", detail)
	}
}

func TestSkillContentReceiptAPIRejectsLegacyAuthModes(t *testing.T) {
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		t.Run(string(mode), func(t *testing.T) {
			srv, _, _, _, _ := privateFixture(t, mode, "legacy-token")
			for _, tc := range []struct{ method, path string }{
				{"GET", receiptRoot},
				{"POST", receiptRoot},
				{"GET", receiptRoot + "/" + strings.Repeat("a", 48)},
				{"DELETE", receiptRoot + "/" + strings.Repeat("a", 48)},
			} {
				w := privateSkillHTTP(t, srv, tc.method, tc.path, `{}`, "", "https://reet.example", "legacy-token")
				if w.Code != 403 {
					t.Fatal(tc.method, tc.path, w.Code, w.Body.String())
				}
			}
		})
	}
}
