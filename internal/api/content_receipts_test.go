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
	"slices"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
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

func TestSkillContentReceiptAPIReplayLogsOneAdoption(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	p := h.captured("receipt_replay_preview_01")
	body := h.adoptBody("receipt_replay_adopt_001", p)
	var first, again store.SkillContentReceipt
	for _, into := range []*store.SkillContentReceipt{&first, &again} {
		w := h.browser("POST", receiptRoot, body)
		if e := json.Unmarshal(w.Body.Bytes(), into); w.Code != 200 || e != nil || into.ID == "" {
			t.Fatal(w.Code, w.Body.String(), e)
		}
	}
	if !reflect.DeepEqual(again, first) {
		t.Fatalf("an identical retry should answer with the original record: %+v %+v", again, first)
	}
	// A retry that is refused leaves no trace either.
	changed := h.adoptBody("receipt_replay_adopt_001", p)
	changed = strings.Replace(changed, p.ReviewHash, strings.Repeat("0", 64), 1)
	if w := h.browser("POST", receiptRoot, changed); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, detail := receiptEvent(t, h.st, h.uid, "skill-content-adopt"); !reflect.DeepEqual(detail, map[string]any{"name": "demo", "files": float64(2), "receiptId": first.ID}) {
		t.Fatal(detail)
	}
}

// A receipt row so damaged that even the list cannot read it is still released,
// and its event then carries no name for the label to guess at.
func TestSkillContentReceiptAPIReleaseEventOmitsAnUnknownName(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	receipt := h.adopt("receipt_noname_adopt_01", h.captured("receipt_noname_preview_1"))
	if _, e := h.rawDB().Exec(`UPDATE skill_content_receipts SET schema_version='not a number' WHERE id=?`, receipt.ID); e != nil {
		t.Fatal(e)
	}
	if w := h.browser("GET", receiptRoot, ""); w.Code != 500 {
		t.Fatal("the fixture should make the list itself fail", w.Code, w.Body.String())
	}
	if w := h.browser("DELETE", receiptRoot+"/"+receipt.ID, fmt.Sprintf(`{"expectedVersion":%q}`, receipt.Version)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, detail := receiptEvent(t, h.st, h.uid, "skill-content-release"); !reflect.DeepEqual(detail, map[string]any{"receiptId": receipt.ID}) {
		t.Fatal("an unknown name should be left out, not recorded as empty", detail)
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
		{"preview superseded by an existing record", "POST", receiptRoot, h.adoptBody("receipt_boundary_adopt_02", p), h.cookie, origin, 409, conflict},
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

// breakdownPath is the read of one check against one library revision.
func breakdownPath(previewID, revisionID string) string {
	return previewRoot + "/" + previewID + "/adoption/" + revisionID
}

// breakdownRow is the metadata one manifest holds for path, listed with status.
func breakdownRow(t *testing.T, m *skillbundle.Manifest, path, status string) store.AdoptionFile {
	t.Helper()
	for _, f := range m.Files {
		if f.Path == path {
			return store.AdoptionFile{Path: f.Path, Bytes: f.Bytes, Digest: f.Digest, Executable: f.Executable, Status: status}
		}
	}
	t.Fatalf("%s is not in the manifest", path)
	return store.AdoptionFile{}
}

func TestSkillContentAdoptionBreakdownAPI(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	viewer, e := h.st.CreateUser("breakdown-viewer@example.test", "Reader", "viewer", "hash")
	if e != nil {
		t.Fatal(e)
	}
	other, e := h.st.CreateUser("breakdown-other@example.test", "Other", "member", "hash")
	if e != nil {
		t.Fatal(e)
	}
	viewerCookie, otherCookie := terminalLogin(t, h.st, viewer.ID), terminalLogin(t, h.st, other.ID)

	// The folder holds exactly the pinned revision. A sibling shares its SKILL.md but
	// not the rest; another keeps the name and differs in SKILL.md; a third bundle
	// has the pinned bytes; and another account has a revision of its own.
	skill := []byte("---\nname: demo\ndescription: Private instructions\n---\nPRIVATE_PIN_CONTENT\n")
	save := func(owner int64, key, base, bundle string, files ...skillbundle.ContentFile) store.PrivateSkillResult {
		t.Helper()
		out, e := h.st.SavePrivateSkillRevisionFor(owner, bundle, store.PrivateSkillWrite{RequestID: key, ExpectedBase: &base, Name: "demo", Files: files})
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	pinned := h.bundle.Revision
	sibling := save(h.uid, "breakdown_api_sibling_01", pinned.ID, h.bundle.Bundle.ID,
		skillbundle.ContentFile{Path: "SKILL.md", Data: skill}, skillbundle.ContentFile{Path: "data.bin", Data: []byte{9}}, skillbundle.ContentFile{Path: "extra.txt", Data: []byte("hello")})
	differs := save(h.uid, "breakdown_api_differs_01", sibling.Revision.ID, h.bundle.Bundle.ID, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Private instructions\n---\nA_DIFFERENT_SKILL_TEXT\n")})
	another := save(h.uid, "breakdown_api_bundle_02", "", "", skillbundle.ContentFile{Path: "SKILL.md", Data: skill}, skillbundle.ContentFile{Path: "data.bin", Data: []byte{0, 255, 1}})
	theirs := save(other.ID, "breakdown_api_foreign_01", "", "", skillbundle.ContentFile{Path: "SKILL.md", Data: skill})

	p := h.captured("breakdown_api_preview_01")
	unknownID := strings.Repeat("0", 48)
	events := func() int {
		t.Helper()
		list, e := h.st.ListEventsFor(h.uid, 0, "")
		if e != nil {
			t.Fatal(e)
		}
		return len(list)
	}
	eventsBefore := events()

	const (
		missing  = "Ownership record or preview unavailable."
		noSkill  = "Private skill unavailable."
		conflict = "This ownership record is unavailable, blocked, changed or already exists. Preview again, and release existing ownership before adopting again."
		notClaim = "That revision does not match this folder's SKILL.md, so nothing can be claimed."
	)
	for _, tc := range []struct {
		name, path, cookie string
		status             int
		message            string
	}{
		{"anonymous", breakdownPath(p.ID, pinned.ID), "", 401, ""},
		{"viewer", breakdownPath(p.ID, pinned.ID), viewerCookie, 403, ""},
		{"query string", breakdownPath(p.ID, pinned.ID) + "?x=1", h.cookie, 400, ""},
		{"empty query string", breakdownPath(p.ID, pinned.ID) + "?", h.cookie, 400, ""},
		{"foreign account's preview", breakdownPath(p.ID, theirs.Revision.ID), otherCookie, 404, missing},
		{"foreign account's revision", breakdownPath(p.ID, theirs.Revision.ID), h.cookie, 404, noSkill},
		{"unknown preview", breakdownPath(unknownID, pinned.ID), h.cookie, 404, missing},
		{"malformed preview ID", breakdownPath("not-an-id", pinned.ID), h.cookie, 404, missing},
		{"unknown revision", breakdownPath(p.ID, strings.Repeat("0", 32)), h.cookie, 404, noSkill},
		{"malformed revision ID", breakdownPath(p.ID, "not-an-id"), h.cookie, 404, noSkill},
		{"revision of another bundle", breakdownPath(p.ID, another.Revision.ID), h.cookie, 409, conflict},
		{"revision that does not match the folder", breakdownPath(p.ID, differs.Revision.ID), h.cookie, 409, notClaim},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := privateSkillHTTP(t, h.srv, "GET", tc.path, "", tc.cookie, "", "")
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.message) {
				t.Fatal(w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" && w.Code != 401 {
				t.Fatal("refusal cached")
			}
		})
	}

	t.Run("legacy auth modes", func(t *testing.T) {
		for _, mode := range []AuthMode{AuthNone, AuthToken} {
			srv, _, _, _, _ := privateFixture(t, mode, "legacy-token")
			if w := privateSkillHTTP(t, srv, "GET", breakdownPath(strings.Repeat("a", 48), strings.Repeat("a", 32)), "", "", "", "legacy-token"); w.Code != 403 {
				t.Fatal(mode, w.Code, w.Body.String())
			}
		}
	})

	// None of this is reachable by an MCP client.
	for _, method := range []string{"GET", "POST", "DELETE"} {
		if mcpAPIPath(method, breakdownPath(p.ID, pinned.ID)) {
			t.Fatalf("MCP allowlist expanded: %s", method)
		}
	}

	// A sibling that shares only SKILL.md: the differing data.bin appears once for
	// each side, extra.txt only in the library, and the counts are the offer's.
	var bodies []string
	read := func(revisionID, cookie, token string) (store.SkillContentAdoptionBreakdown, string) {
		t.Helper()
		w := privateSkillHTTP(t, h.srv, "GET", breakdownPath(p.ID, revisionID), "", cookie, "", token)
		var out store.SkillContentAdoptionBreakdown
		if e := json.Unmarshal(w.Body.Bytes(), &out); w.Code != 200 || e != nil || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String(), e, w.Header())
		}
		bodies = append(bodies, w.Body.String())
		return out, w.Body.String()
	}
	got, raw := read(sibling.Revision.ID, h.cookie, "")
	want := store.SkillContentAdoptionBreakdown{RevisionID: sibling.Revision.ID, Matched: 1, ObservedOnly: 1, RevisionOnly: 2, Files: []store.AdoptionFile{
		breakdownRow(t, pinned.Manifest, "SKILL.md", "claimed"),
		breakdownRow(t, pinned.Manifest, "data.bin", "stays_yours"),
		breakdownRow(t, sibling.Revision.Manifest, "data.bin", "library_only"),
		breakdownRow(t, sibling.Revision.Manifest, "extra.txt", "library_only"),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s\nwant %+v", raw, want)
	}
	if !strings.HasPrefix(raw, `{"revisionId":"`+sibling.Revision.ID+`","files":[{"path":"SKILL.md","bytes":`) || !strings.Contains(raw, `"matched":1,"observedOnly":1,"revisionOnly":2}`) {
		t.Fatalf("the wire shape is the contract: %s", raw)
	}
	if offer := (store.AdoptionCandidate{RevisionID: got.RevisionID, Matched: got.Matched, ObservedOnly: got.ObservedOnly, RevisionOnly: got.RevisionOnly}); !slices.Contains(p.Adoption.Candidates, offer) {
		t.Fatalf("the breakdown's counts should be the offer's: %+v", p.Adoption)
	}

	// The pinned revision claims everything, and a personal token reads it too.
	claimedAll, _ := read(pinned.ID, h.cookie, "")
	viaToken, _ := read(pinned.ID, "", h.token)
	if !reflect.DeepEqual(claimedAll, viaToken) || claimedAll.Matched != 2 || claimedAll.ObservedOnly != 0 || claimedAll.RevisionOnly != 0 || len(claimedAll.Files) != 2 {
		t.Fatalf("%+v %+v", claimedAll, viaToken)
	}

	// Nothing on the wire carries bytes, private text, a secret or a host path.
	absolute := regexp.MustCompile(`"(/|[A-Za-z]:\\)`)
	for _, body := range bodies {
		for _, leaked := range []string{"PRIVATE_PIN_CONTENT", "Private instructions", "hello", `"data"`, h.secret, h.token, h.st.Path()} {
			if strings.Contains(body, leaked) {
				t.Fatalf("response leaks %q: %s", leaked, body)
			}
		}
		if absolute.MatchString(body) {
			t.Fatal("response carries an absolute path", body)
		}
	}
	if events() != eventsBefore {
		t.Fatal("a read was audited")
	}
	if again := h.browser("GET", previewRoot+"/"+p.ID, ""); !strings.Contains(again.Body.String(), `"state":"captured"`) {
		t.Fatal("reading a breakdown changed the check", again.Body.String())
	}

	// What the screen showed as claimed is what the receipt that follows holds.
	claimed := []string{}
	for _, f := range claimedAll.Files {
		if f.Status == "claimed" {
			claimed = append(claimed, f.Path)
		}
	}
	receipt := h.adopt("breakdown_api_adopt_01", p)
	managed := []string{}
	for _, f := range receipt.Managed.Files {
		managed = append(managed, f.Path)
	}
	if !reflect.DeepEqual(claimed, managed) || claimedAll.Matched != receipt.FileCount {
		t.Fatalf("claimed %v (%d), receipt holds %v (%d)", claimed, claimedAll.Matched, managed, receipt.FileCount)
	}
	// Adoption consumed the check, so there is nothing left to explain.
	if w := privateSkillHTTP(t, h.srv, "GET", breakdownPath(p.ID, pinned.ID), "", h.cookie, "", ""); w.Code != 409 || !strings.Contains(w.Body.String(), conflict) {
		t.Fatal(w.Code, w.Body.String())
	}

	// A person demoted since loses the read, by session and by token.
	if e = h.st.UpdateUserRole(h.uid, "viewer"); e != nil {
		t.Fatal(e)
	}
	for name, as := range map[string][2]string{"session": {h.cookie, ""}, "token": {"", h.token}} {
		if w := privateSkillHTTP(t, h.srv, "GET", breakdownPath(p.ID, pinned.ID), "", as[0], "", as[1]); w.Code != 403 {
			t.Fatalf("a demoted account reached the breakdown by %s: %d %s", name, w.Code, w.Body.String())
		}
	}
}

// rawDB is a second connection, for rows the API cannot create (a persisted
// duplicate report that predates consolidation).
func (h *previewHarness) rawDB() *sql.DB {
	h.t.Helper()
	db, e := sql.Open("sqlite", "file:"+filepath.Join(h.st.Path(), "reet.db")+"?_pragma=busy_timeout(5000)")
	if e != nil {
		h.t.Fatal(e)
	}
	h.t.Cleanup(func() { db.Close() })
	return db
}

func eventsOf(t *testing.T, st *store.Store, uid int64, action string) int {
	t.Helper()
	events, err := st.ListEventsFor(uid, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range events {
		if ev.Action == action {
			n++
		}
	}
	return n
}

func TestMachineMergeAnswersASlotCollisionWith409(t *testing.T) {
	for name, tc := range map[string]struct {
		want error
		seed func(h *previewHarness, db *sql.DB, dest string)
	}{
		"receipts": {store.ErrSkillContentReceiptMerge, func(h *previewHarness, db *sql.DB, dest string) {
			receipt := h.adopt("merge_machine_adopt_001", h.captured("merge_machine_preview_01"))
			id := strings.Repeat("a", 48)
			if _, e := db.Exec(`INSERT INTO skill_content_receipts(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,root_id,adapter,schema_version,managed,provenance,evidence,request_key,version,created_at,updated_at) SELECT ?,user_id,?,agent_id,name,bundle_id,revision_id,root_id,adapter,schema_version,managed,provenance,evidence,?,?,created_at,updated_at FROM skill_content_receipts WHERE id=?`,
				id, dest, "seed_"+id, strings.Repeat("b", 48), receipt.ID); e != nil {
				h.t.Fatal(e)
			}
		}},
		"pins": {store.ErrSkillContentTargetMerge, func(h *previewHarness, db *sql.DB, dest string) {
			if _, e := db.Exec(`INSERT INTO skill_content_targets(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,mode,version,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
				strings.Repeat("c", 48), h.uid, dest, h.agent.ID, h.pin.Name, h.pin.BundleID, h.pin.RevisionID, "managed", strings.Repeat("d", 48), 1); e != nil {
				h.t.Fatal(e)
			}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newPreviewHarness(t)
			h.negotiate()
			if e := h.st.TouchEnvironmentFor(h.uid, "preferred", "agents", "", ""); e != nil {
				t.Fatal(e)
			}
			tc.seed(h, h.rawDB(), store.EnvironmentIDFor(h.uid, "preferred", "agents", ""))

			w := h.browser("POST", "/api/v2/machines/merge", fmt.Sprintf(`{"from":%q,"to":"preferred"}`, h.agent.Host))
			var out map[string]string
			if e := json.Unmarshal(w.Body.Bytes(), &out); w.Code != 409 || e != nil || out["error"] != tc.want.Error() {
				t.Fatalf("a slot collision should be a conflict the person can act on, not a fault: %d %s %v", w.Code, w.Body.String(), e)
			}
			if n := eventsOf(t, h.st, h.uid, "merge-machines"); n != 0 {
				t.Fatal("a refused merge was audited as done", n)
			}
			if pin, e := h.st.SkillContentTargetFor(h.uid, h.pin.ID); e != nil || pin.EnvironmentID != h.pin.EnvironmentID {
				t.Fatal("a refused merge moved the pin", pin, e)
			}
			if aliases, e := h.st.MachineAliasesFor(h.uid); e != nil || len(aliases) != 0 {
				t.Fatal("a refused merge recorded an alias", aliases, e)
			}
		})
	}
}

func TestEnvironmentMergeRefusesPinsAndReceiptsWith409(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	if e := h.st.TouchEnvironmentFor(h.uid, "laptop", "claude", "", ""); e != nil {
		t.Fatal(e)
	}
	dest := store.EnvironmentIDFor(h.uid, "laptop", "claude", "")
	mergeBody := func(from string) string {
		return fmt.Sprintf(`{"from_id":%q,"to_id":%q,"confirm":true}`, from, dest)
	}
	const path = "/api/v2/environments/merge"
	refused := func(w *httptest.ResponseRecorder, code, message, hint string) {
		t.Helper()
		var out map[string]string
		want := map[string]string{"error": message, "code": code, "message": message, "field": "from_id", "hint": hint}
		if e := json.Unmarshal(w.Body.Bytes(), &out); w.Code != 409 || e != nil || !reflect.DeepEqual(out, want) {
			t.Fatalf("%d %s %v, want 409 %v", w.Code, w.Body.String(), e, want)
		}
	}
	reportListed := func() {
		t.Helper()
		envs, e := h.st.ListEnvironmentsFor(h.uid)
		if e != nil {
			t.Fatal(e)
		}
		for _, env := range envs {
			if env.ID == h.pin.EnvironmentID {
				return
			}
		}
		t.Fatal("a refused merge removed the source report")
	}

	// The pin stands in the way first, and removing it never touches ownership.
	refused(h.browser("POST", path, mergeBody(h.pin.EnvironmentID)), "environment_has_pins",
		"remove private skill pins before merging this report into a different scope", "Remove the private skill pin first.")
	if pin, e := h.st.SkillContentTargetFor(h.uid, h.pin.ID); e != nil || !reflect.DeepEqual(pin, h.pin) {
		t.Fatal("a refused merge changed the pin", pin, e)
	}
	reportListed()

	receipt := h.adopt("merge_env_adopt_000001", h.captured("merge_env_preview_0001"))
	if e := h.st.DeleteSkillContentTargetFor(h.uid, h.pin.ID, h.pin.Version); e != nil {
		t.Fatal(e)
	}
	refused(h.browser("POST", path, mergeBody(h.pin.EnvironmentID)), "environment_has_receipts",
		"release private skill ownership receipts before merging this report into a different scope", "Release the ownership record first.")
	if got, e := h.st.SkillContentReceiptFor(h.uid, receipt.ID); e != nil || !reflect.DeepEqual(got, receipt) {
		t.Fatal("a refused merge changed the receipt", got, e)
	}
	reportListed()
	if n := eventsOf(t, h.st, h.uid, "environment-merge"); n != 0 {
		t.Fatal("a refused merge was audited as done", n)
	}

	// An unknown report is still a 404 and releasing the receipt still unblocks the merge.
	var out map[string]string
	if w := h.browser("POST", path, mergeBody("no-such-report")); w.Code != 404 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out["code"] != "environment_not_found" {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := h.st.ReleaseSkillContentReceiptFor(h.uid, receipt.ID, receipt.Version); e != nil {
		t.Fatal(e)
	}
	if w := h.browser("POST", path, mergeBody(h.pin.EnvironmentID)); w.Code != 200 || !strings.Contains(w.Body.String(), `"skills_moved":0`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

// A preview must not promise a merge the confirmed request will refuse.
func TestEnvironmentMergeDryRunPreviewsTheSameRefusal(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	if e := h.st.TouchEnvironmentFor(h.uid, "laptop", "claude", "", ""); e != nil {
		t.Fatal(e)
	}
	dest := store.EnvironmentIDFor(h.uid, "laptop", "claude", "")
	const path = "/api/v2/environments/merge"
	body := func(from string, dry bool) string {
		return fmt.Sprintf(`{"from_id":%q,"to_id":%q,"dry_run":%t,"confirm":%t}`, from, dest, dry, !dry)
	}
	source := h.pin.EnvironmentID
	// refused expects the dry run and the confirmed merge to answer with one and the same 409.
	refused := func(code, message, hint string) {
		t.Helper()
		dry, real := h.browser("POST", path, body(source, true)), h.browser("POST", path, body(source, false))
		var out map[string]string
		want := map[string]string{"error": message, "code": code, "message": message, "field": "from_id", "hint": hint}
		if e := json.Unmarshal(dry.Body.Bytes(), &out); dry.Code != 409 || e != nil || !reflect.DeepEqual(out, want) {
			t.Fatalf("dry run: %d %s %v, want 409 %v", dry.Code, dry.Body.String(), e, want)
		}
		if real.Code != 409 || real.Body.String() != dry.Body.String() {
			t.Fatalf("the preview and the merge disagree: %d %s vs %d %s", dry.Code, dry.Body.String(), real.Code, real.Body.String())
		}
	}
	const (
		pinMessage     = "remove private skill pins before merging this report into a different scope"
		receiptMessage = "release private skill ownership receipts before merging this report into a different scope"
	)

	// A report holding a pin.
	refused("environment_has_pins", pinMessage, "Remove the private skill pin first.")

	// A pin and a receipt: the pin is answered first, as the merge does.
	receipt := h.adopt("merge_dry_adopt_00001", h.captured("merge_dry_preview_0001"))
	refused("environment_has_pins", pinMessage, "Remove the private skill pin first.")

	// Only a receipt.
	if e := h.st.DeleteSkillContentTargetFor(h.uid, h.pin.ID, h.pin.Version); e != nil {
		t.Fatal(e)
	}
	refused("environment_has_receipts", receiptMessage, "Release the ownership record first.")

	// An unknown source or destination is still a 404.
	for name, b := range map[string]string{
		"source":      body("no-such-report", true),
		"destination": fmt.Sprintf(`{"from_id":%q,"to_id":"no-such-report","dry_run":true}`, source),
	} {
		var out map[string]string
		if w := h.browser("POST", path, b); w.Code != 404 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out["code"] != "environment_not_found" {
			t.Fatal("unknown "+name, w.Code, w.Body.String())
		}
	}

	// Neither: the preview is the unchanged 200 and changes nothing.
	if e := h.st.ReleaseSkillContentReceiptFor(h.uid, receipt.ID, receipt.Version); e != nil {
		t.Fatal(e)
	}
	w := h.browser("POST", path, body(source, true))
	var preview struct {
		DryRun     bool              `json:"dry_run"`
		From       store.Environment `json:"from"`
		To         store.Environment `json:"to"`
		SkillsMove *int              `json:"skills_moved"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &preview); w.Code != 200 || e != nil || !preview.DryRun || preview.From.ID != source || preview.To.ID != dest || preview.SkillsMove == nil || *preview.SkillsMove != 0 {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	envs, e := h.st.ListEnvironmentsFor(h.uid)
	if e != nil {
		t.Fatal(e)
	}
	listed := 0
	for _, env := range envs {
		if env.ID == source || env.ID == dest {
			listed++
		}
	}
	if listed != 2 || eventsOf(t, h.st, h.uid, "environment-merge") != 0 {
		t.Fatal("a preview changed something", listed)
	}
}

// envChange is the part of a blocked entry the person acts on.
type envChange struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func TestEnvironmentDeleteKeepsOwnershipReceiptsUnlessForced(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	receipt := h.adopt("delete_env_adopt_00001", h.captured("delete_env_preview_0001"))
	id := receipt.EnvironmentID
	const path = "/api/v2/environments/delete"
	const message = "environment still holds ownership receipts; release them first"
	preview := func(body string) (would []store.Environment, blocked []envChange) {
		t.Helper()
		w := h.browser("POST", path, body)
		var out struct {
			DryRun      bool                `json:"dry_run"`
			WouldDelete []store.Environment `json:"would_delete"`
			Blocked     []envChange         `json:"blocked"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &out); w.Code != 200 || e != nil || !out.DryRun {
			t.Fatal(w.Code, w.Body.String(), e)
		}
		return out.WouldDelete, out.Blocked
	}

	// The dry run says what the real delete would do.
	if would, blocked := preview(fmt.Sprintf(`{"ids":[%q],"dry_run":true}`, id)); len(would) != 0 || !reflect.DeepEqual(blocked, []envChange{{id, "environment_has_receipts", message}}) {
		t.Fatal(would, blocked)
	}
	if would, blocked := preview(fmt.Sprintf(`{"ids":[%q],"dry_run":true,"force":true}`, id)); len(would) != 1 || would[0].ID != id || len(blocked) != 0 {
		t.Fatal("force should preview the delete it would make", would, blocked)
	}

	// The real delete answers in the existing blocked shape and removes nothing.
	w := h.browser("POST", path, fmt.Sprintf(`{"id":%q,"confirm":true}`, id))
	var refusal struct {
		Error, Code, Message, Hint string
		Blocked                    []envChange
	}
	if e := json.Unmarshal(w.Body.Bytes(), &refusal); w.Code != 400 || e != nil ||
		refusal.Error != message || refusal.Code != "environment_has_receipts" || refusal.Message != message ||
		refusal.Hint != "Release the ownership records first, or use force=true to forget them with the report." ||
		!reflect.DeepEqual(refusal.Blocked, []envChange{{id, "environment_has_receipts", message}}) {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	if got, e := h.st.SkillContentReceiptFor(h.uid, receipt.ID); e != nil || !reflect.DeepEqual(got, receipt) {
		t.Fatal("a refused delete changed the receipt", got, e)
	}
	if n := eventsOf(t, h.st, h.uid, "environment-delete"); n != 0 {
		t.Fatal("a refused delete was audited as done", n)
	}

	// Forcing forgets the record with the report; the private bytes are not touched.
	w = h.browser("POST", path, fmt.Sprintf(`{"id":%q,"confirm":true,"force":true}`, id))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"deleted":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if list, e := h.st.SkillContentReceiptsFor(h.uid); e != nil || len(list) != 0 {
		t.Fatal("the receipt outlived a forced delete", list, e)
	}
	if _, e := h.st.PrivateSkillRevisionFor(h.uid, h.bundle.Bundle.ID, h.bundle.Revision.ID); e != nil {
		t.Fatal("forgetting ownership removed private bytes", e)
	}
	if n := eventsOf(t, h.st, h.uid, "environment-delete"); n != 1 {
		t.Fatal("the forced delete should be audited once", n)
	}
}

func TestEnvironmentDeleteSucceedsOnceOwnershipIsReleased(t *testing.T) {
	h := newPreviewHarness(t)
	h.negotiate()
	receipt := h.adopt("delete_env_adopt_00002", h.captured("delete_env_preview_0002"))
	if e := h.st.ReleaseSkillContentReceiptFor(h.uid, receipt.ID, receipt.Version); e != nil {
		t.Fatal(e)
	}
	w := h.browser("POST", "/api/v2/environments/delete", fmt.Sprintf(`{"id":%q,"confirm":true}`, receipt.EnvironmentID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"deleted":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
