package api

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpWriteSession connects to the environment's hosted MCP server with a
// management connection.
func (e reconcileEnv) mcpWriteSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	if err := e.st.SetMCPEnabled(e.uid, true); err != nil {
		t.Fatal(err)
	}
	secret := "reet_mcp_status_test"
	if _, err := e.st.CreateMCPConnection(e.uid, "Status test", "write", auth.HashToken(secret), time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	return mcpSession(t, e.srv, secret)
}

// status reads one library item's status, "" once it is deleted.
func (e reconcileEnv) status(t *testing.T, item string) string {
	t.Helper()
	if !e.live(t, item) {
		return ""
	}
	items, err := e.st.SkillItemsFor(e.uid, []string{item})
	if err != nil {
		t.Fatal(err)
	}
	return items[0].Status
}

// The skill tools archive, list, restore and delete library skills through
// the Skills page's own routes, and the ignore list and sync plan tools are
// gone.
func TestMCPSkillToolsArchiveRestoreAndDelete(t *testing.T) {
	e := newReconcileEnv(t)
	session := e.mcpWriteSession(t)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	for _, gone := range []string{"reet_list_ignored", "reet_unignore_skill", "reet_list_sync_targets"} {
		if slices.Contains(names, gone) {
			t.Fatalf("%s is still offered", gone)
		}
		if text := mcpError(t, session, gone, map[string]any{}); !strings.Contains(text, "tool") {
			t.Fatalf("%s still answers: %s", gone, text)
		}
	}
	for _, added := range []string{"reet_list_archived", "reet_restore_skill"} {
		if !slices.Contains(names, added) {
			t.Fatalf("%s is not offered: %v", added, names)
		}
	}

	item := e.linkedSkill(t, findSkills)
	shelf := ""
	for _, raw := range mcpData(t, session, "reet_list_skills", map[string]any{"status": "active"})["items"].([]any) {
		sk := raw.(map[string]any)
		if sk["name"] == "find-skills" {
			shelf, _ = sk["id"].(string)
			if sk["status"] != "active" {
				t.Fatalf("listed status: %+v", sk)
			}
		}
	}
	if shelf == "" {
		t.Fatal("the active skill is not listed as active")
	}

	archived := mcpData(t, session, "reet_update_skill", map[string]any{"id": shelf, "status": "archived"})
	if archived["status"] != "archived" || archived["id"] != shelf || e.status(t, item) != "archived" {
		t.Fatalf("archive: %+v, stored %q", archived, e.status(t, item))
	}
	if got := mcpData(t, session, "reet_list_skills", map[string]any{"status": "active"})["total_count"]; got != float64(0) {
		t.Fatalf("an archived skill is listed as active: %v", got)
	}
	list := mcpData(t, session, "reet_list_archived", map[string]any{})
	rows := list["items"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != item || rows[0].(map[string]any)["shelfId"] != shelf || rows[0].(map[string]any)["status"] != "archived" {
		t.Fatalf("archived list: %+v", list)
	}

	restored := mcpData(t, session, "reet_restore_skill", map[string]any{"id": item})
	if restored["id"] != item || restored["shelfId"] != shelf || restored["name"] != "find-skills" || restored["status"] != "active" || e.status(t, item) != "active" {
		t.Fatalf("restore: %+v, stored %q", restored, e.status(t, item))
	}
	if text := mcpError(t, session, "reet_restore_skill", map[string]any{"id": "demo/skills/never-added"}); !strings.Contains(text, "not found") {
		t.Fatalf("restore of an unknown skill: %s", text)
	}

	// A skill the person wrote is deleted only with writtenConfirmed, as on
	// the Skills page.
	written := e.writtenSkill(t, "alpha")
	text := mcpError(t, session, "reet_remove_skill", map[string]any{"id": written})
	if !strings.Contains(text, "HTTP 409 [written]: You wrote alpha. Deleting it deletes your writing too.") || !strings.Contains(text, "writtenConfirmed") || !e.live(t, written) {
		t.Fatalf("delete of a written skill: %s", text)
	}
	if got := mcpData(t, session, "reet_remove_skill", map[string]any{"id": written, "writtenConfirmed": true}); got["deleted"] != float64(1) || e.live(t, written) {
		t.Fatalf("confirmed delete of a written skill: %+v", got)
	}
	if got := mcpData(t, session, "reet_remove_skill", map[string]any{"id": shelf}); got["deleted"] != float64(1) || e.live(t, item) {
		t.Fatalf("delete: %+v", got)
	}
}

// Archiving many skills at once does what the Skills page's Archive does: a
// skill reet has no copy of is copied in before it leaves any machine, and
// the event says which status they went to.
func TestBulkStatusStartsTheCopyPassAndSaysTheStatus(t *testing.T) {
	e := newReconcileEnv(t)
	r, err := ref.Parse("vercel-labs/skills/find-skills")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.UpsertFor(e.uid, r, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	skills, _, err := e.st.ListFor(e.uid, store.Filter{})
	if err != nil || len(skills) != 1 {
		t.Fatal(skills, err)
	}
	item := skills[0].LibraryItemID
	if items, err := e.st.SkillItemsFor(e.uid, []string{item}); err != nil || items[0].HasCopy {
		t.Fatalf("the skill started with a copy: %+v %v", items, err)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/bulk", `{"ids":["`+skills[0].ID+`"],"status":"archived","dry_run":true}`); code != 200 {
		t.Fatalf("dry run: %d %s", code, raw)
	}
	e.settle()
	if items, _ := e.st.SkillItemsFor(e.uid, []string{item}); items[0].HasCopy || items[0].Status != "active" {
		t.Fatalf("a dry run changed something: %+v", items[0])
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/bulk", `{"ids":["`+skills[0].ID+`"],"status":"archived"}`); code != 200 {
		t.Fatalf("bulk archive: %d %s", code, raw)
	}
	e.settle()
	items, err := e.st.SkillItemsFor(e.uid, []string{item})
	if err != nil || items[0].Status != "archived" || !items[0].HasCopy {
		t.Fatalf("bulk archive did not copy the skill in: %+v %v", items, err)
	}
	code, raw := e.owner(t, "GET", "/api/events?action=bulk-update&limit=5", "")
	if code != 200 || !strings.Contains(raw, `\"status\":\"archived\"`) {
		t.Fatalf("the bulk event does not say the status: %d %s", code, raw)
	}
}

// Ruling R47: reet_list_skills also lists library skills with no shelf row,
// such as one this account wrote and never published, by their library id,
// so an agent can find what it can archive or delete.
func TestMCPListSkillsIncludesLibrarySkillsWithNoShelfRow(t *testing.T) {
	e := newReconcileEnv(t)
	session := e.mcpWriteSession(t)
	written := e.writtenSkill(t, "alpha")
	find := func(args map[string]any) map[string]any {
		t.Helper()
		for _, raw := range mcpData(t, session, "reet_list_skills", args)["items"].([]any) {
			if sk := raw.(map[string]any); sk["id"] == written {
				return sk
			}
		}
		return nil
	}
	sk := find(map[string]any{"status": "active"})
	if sk == nil || sk["name"] != "alpha" || sk["status"] != "active" {
		t.Fatalf("the written skill is not listed as active: %+v", sk)
	}
	if sk = find(map[string]any{"q": "alph", "compact": true}); sk == nil || sk["status"] != "active" || sk["sourceType"] != "written" {
		t.Fatalf("a search does not find the written skill: %+v", sk)
	}
	if find(map[string]any{"status": "archived"}) != nil || find(map[string]any{"tab": "liked"}) != nil {
		t.Fatal("the written skill is listed under a filter it does not match")
	}
	if got := mcpData(t, session, "reet_get_skill", map[string]any{"id": written}); got["name"] != "alpha" {
		t.Fatalf("get_skill by library id: %+v", got)
	}
	if got := mcpData(t, session, "reet_update_skill", map[string]any{"id": written, "status": "archived"}); got["status"] != "archived" {
		t.Fatalf("archive by library id: %+v", got)
	}
	if find(map[string]any{"status": "archived"}) == nil || find(map[string]any{"status": "active"}) != nil {
		t.Fatal("the archived written skill is not listed as archived")
	}
}

// reet_bulk_update_skills sets a status through the Skills page's own store
// path, so a library skill with no shelf row, such as one written and never
// published, is archived and restored by its library id and counted, as a
// shelf skill is by its shelf id. A filter on status finds both.
func TestBulkStatusTakesLibraryIDsWithNoShelfRow(t *testing.T) {
	e := newReconcileEnv(t)
	written := e.writtenSkill(t, "alpha")
	linked := e.linkedSkill(t, "vercel-labs/skills/find-skills")
	skills, _, err := e.st.ListFor(e.uid, store.Filter{})
	if err != nil || len(skills) != 1 {
		t.Fatal(skills, err)
	}
	shelfID := skills[0].ID
	bulk := func(body string) map[string]any {
		t.Helper()
		code, raw := e.owner(t, "POST", "/api/v2/skills/bulk", body)
		if code != 200 {
			t.Fatalf("bulk %s: %d %s", body, code, raw)
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := bulk(`{"ids":["` + shelfID + `","` + written + `"],"status":"archived","dry_run":true}`)
	if out["updated"] != float64(2) || out["failed"] != float64(0) {
		t.Fatalf("dry run: %+v", out)
	}
	if e.status(t, written) != "active" || e.status(t, linked) != "active" {
		t.Fatal("a dry run changed a status")
	}
	out = bulk(`{"ids":["` + shelfID + `","` + written + `"],"status":"archived"}`)
	if out["updated"] != float64(2) || out["failed"] != float64(0) {
		t.Fatalf("archive by shelf and library id: %+v", out)
	}
	e.settle()
	if e.status(t, written) != "archived" || e.status(t, linked) != "archived" {
		t.Fatalf("archive: written %s, linked %s", e.status(t, written), e.status(t, linked))
	}
	out = bulk(`{"filter":{"status":"archived"},"status":"active"}`)
	if out["updated"] != float64(2) || out["failed"] != float64(0) {
		t.Fatalf("restore by filter: %+v", out)
	}
	if e.status(t, written) != "active" || e.status(t, linked) != "active" {
		t.Fatalf("restore: written %s, linked %s", e.status(t, written), e.status(t, linked))
	}
	// A like or a tag needs a shelf row: the library-only skill says why and
	// is left as it was.
	out = bulk(`{"ids":["` + written + `"],"add_tags":["Later"],"status":"archived"}`)
	if out["updated"] != float64(0) || out["failed"] != float64(1) || e.status(t, written) != "active" {
		t.Fatalf("tag on a library-only skill: %+v, status %s", out, e.status(t, written))
	}
	out = bulk(`{"ids":["no-such-skill"],"status":"archived"}`)
	if out["updated"] != float64(0) || out["failed"] != float64(1) {
		t.Fatalf("unknown id: %+v", out)
	}
}

// reet_list_skills counts the library skills with no shelf row with the rest,
// by status.
func TestMCPListSkillsCountsLibrarySkillsWithNoShelfRow(t *testing.T) {
	e := newReconcileEnv(t)
	session := e.mcpWriteSession(t)
	written := e.writtenSkill(t, "alpha")
	e.linkedSkill(t, "vercel-labs/skills/find-skills")
	if code, raw := e.owner(t, "POST", "/api/v2/skills/status", `{"ids":["`+written+`"],"status":"archived"}`); code != 200 {
		t.Fatalf("archive: %d %s", code, raw)
	}
	counts, _ := mcpData(t, session, "reet_list_skills", map[string]any{})["counts"].(map[string]any)
	want := map[string]any{"all": float64(2), "liked": float64(0), "active": float64(1), "archived": float64(1)}
	if len(counts) != len(want) {
		t.Fatalf("counts %+v, want %+v", counts, want)
	}
	for k, v := range want {
		if counts[k] != v {
			t.Fatalf("counts %+v, want %+v", counts, want)
		}
	}
}
