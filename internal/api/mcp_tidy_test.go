package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillpreview"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpData(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned an error: %s", name, toolText(result))
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("%s: unexpected structured content %T", name, result.StructuredContent)
	}
	data, ok := structured["data"].(map[string]any)
	if !ok {
		t.Fatalf("%s: missing data", name)
	}
	return data
}

func mcpError(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error()
	}
	if !result.IsError {
		t.Fatalf("%s unexpectedly succeeded: %+v", name, result.StructuredContent)
	}
	return toolText(result)
}

func toolText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

func TestMCPTidyTools(t *testing.T) {
	ts, st, browser, uid := mcpAccounts(t)
	if _, err := st.UpsertLocalFor(uid, "seo-audit", store.Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertLocalFor(uid, "svg-logo-designer", store.Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertLocalFor(uid, "web-design-builder", store.Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMeta("local/svg-logo-designer", "SVG Logo Designer", "", 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMeta("local/web-design-builder", "Web Design Builder", "", 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"rknall/claude-skills/svg-logo-designer", "rknall/claude-skills/web-design-builder", "rknall/claude-skills/svg-logo-designer-copy"} {
		rf, err := ref.Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.UpsertFor(uid, rf, store.Patch{}); err != nil {
			t.Fatal(err)
		}
		name := "SVG Logo Designer"
		if strings.Contains(input, "web-design") {
			name = "Web Design Builder"
		}
		if err := st.SetMeta(rf.ID, name, "", 0, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, host := range []string{"container-a", "container-b"} {
		if err := st.TouchEnvironmentFor(uid, host, "reet-server", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.RecordEnvironmentFor(uid, "demo/skills/private", "busy-host", "claude", "", "/skills", ""); err != nil {
		t.Fatal(err)
	}

	token, _ := mcpManual(t, browser, ts, "write")
	session := mcpSession(t, ts, token)

	envs := mcpData(t, session, "reet_list_environments", map[string]any{"agent": "reet-server", "empty_only": true, "limit": 100})
	envItems := envs["items"].([]any)
	if len(envItems) != 2 {
		t.Fatalf("empty reet-server environments: %+v", envItems)
	}
	ids := []string{}
	for _, item := range envItems {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}

	preview := mcpData(t, session, "reet_delete_environment", map[string]any{"ids": ids})
	if preview["dry_run"] != true || len(preview["would_delete"].([]any)) != 2 {
		t.Fatalf("environment dry run: %+v", preview)
	}
	deleted := mcpData(t, session, "reet_delete_environment", map[string]any{"ids": ids, "dry_run": false, "confirm": true})
	if int(deleted["deleted"].(float64)) != 2 {
		t.Fatalf("environment delete: %+v", deleted)
	}
	busyEnvs := mcpData(t, session, "reet_list_environments", map[string]any{"agent": "claude", "empty_only": false, "limit": 100})
	busyID := busyEnvs["items"].([]any)[0].(map[string]any)["id"].(string)
	blocked := mcpError(t, session, "reet_delete_environment", map[string]any{"id": busyID, "dry_run": false, "confirm": true})
	if !strings.Contains(blocked, "environment_has_skills") {
		t.Fatalf("non-empty delete should be refused with a specific code: %s", blocked)
	}
	confirmRequired := mcpError(t, session, "reet_delete_environment", map[string]any{"id": busyID, "dry_run": false})
	if !strings.Contains(confirmRequired, "confirm_required") {
		t.Fatalf("missing confirmation error: %s", confirmRequired)
	}
	forced := mcpData(t, session, "reet_delete_environment", map[string]any{"id": busyID, "dry_run": false, "confirm": true, "force": true})
	if int(forced["deleted"].(float64)) != 1 {
		t.Fatalf("forced delete: %+v", forced)
	}

	// Merge two empty reports back together.
	for _, host := range []string{"merge-from", "merge-to"} {
		if err := st.TouchEnvironmentFor(uid, host, "claude", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	mergedEnvs := mcpData(t, session, "reet_list_environments", map[string]any{"host": "merge-from"})
	fromID := mergedEnvs["items"].([]any)[0].(map[string]any)["id"].(string)
	toEnvs := mcpData(t, session, "reet_list_environments", map[string]any{"host": "merge-to"})
	toID := toEnvs["items"].([]any)[0].(map[string]any)["id"].(string)
	mergePreview := mcpData(t, session, "reet_merge_environments", map[string]any{"from_id": fromID, "to_id": toID})
	if mergePreview["dry_run"] != true {
		t.Fatalf("merge preview: %+v", mergePreview)
	}
	mergeApplied := mcpData(t, session, "reet_merge_environments", map[string]any{"from_id": fromID, "to_id": toID, "dry_run": false, "confirm": true})
	if mergeApplied["dry_run"] != false {
		t.Fatalf("merge apply: %+v", mergeApplied)
	}

	list := mcpData(t, session, "reet_create_list", map[string]any{"name": "Tidy list"})
	listID := int(list["id"].(float64))
	addedLocal := mcpData(t, session, "reet_add_list_skill", map[string]any{"list_id": listID, "id": "local/seo-audit"})
	if int(addedLocal["skillCount"].(float64)) != 1 {
		t.Fatalf("local skill was not added: %+v", addedLocal)
	}
	ambiguous := mcpError(t, session, "reet_add_list_skill", map[string]any{"list_id": listID, "id": "SVG Logo Designer"})
	if !strings.Contains(ambiguous, "ambiguous_skill_id") {
		t.Fatalf("ambiguous id error: %s", ambiguous)
	}
	members := mcpJSON(t, browser, ts, "POST", "/api/v2/lists", map[string]any{"name": "Shared", "visibility": "members"}, 200)
	membersID := int(members["id"].(float64))
	localRefused := mcpError(t, session, "reet_add_list_skill", map[string]any{"list_id": membersID, "id": "local/seo-audit"})
	if !strings.Contains(localRefused, "local_skill_not_allowed_in_list") {
		t.Fatalf("local shared-list error: %s", localRefused)
	}
	bulkLocal := mcpData(t, session, "reet_bulk_update_skills", map[string]any{"ids": []string{"local/seo-audit"}, "add_to_list_id": membersID})
	bulkResults := bulkLocal["results"].([]any)
	if int(bulkLocal["failed"].(float64)) != 1 || bulkResults[0].(map[string]any)["code"] != "local_skill_not_allowed_in_list" {
		t.Fatalf("bulk local shared-list bypass: %+v", bulkLocal)
	}

	batch := mcpData(t, session, "reet_add_list_skills", map[string]any{"list_id": listID, "inputs": []map[string]any{
		{"id": "local/svg-logo-designer", "note": "local copy"},
		{"id": "missing/skill"},
		{"input": "rknall/claude-skills/svg-logo-designer"},
	}})
	if int(batch["added"].(float64)) != 2 || int(batch["failed"].(float64)) != 1 {
		t.Fatalf("batch list add: %+v", batch)
	}
	removed := mcpData(t, session, "reet_remove_list_skill", map[string]any{"list_id": listID, "id": "local/seo-audit"})
	if int(removed["skillCount"].(float64)) != 2 {
		t.Fatalf("remove list skill: %+v", removed)
	}
	updatedList := mcpData(t, session, "reet_update_list", map[string]any{"list_id": listID, "name": "Tidy renamed", "description": "changed"})
	if updatedList["name"] != "Tidy renamed" {
		t.Fatalf("update list: %+v", updatedList)
	}
	deletePreview := mcpData(t, session, "reet_delete_list", map[string]any{"list_id": listID})
	if deletePreview["dry_run"] != true {
		t.Fatalf("delete list preview: %+v", deletePreview)
	}
	mcpData(t, session, "reet_delete_list", map[string]any{"list_id": listID, "dry_run": false, "confirm": true})

	tag := mcpData(t, session, "reet_create_tag", map[string]any{"name": "Tidy tag"})
	tagID := int(tag["id"].(float64))
	bulk := mcpData(t, session, "reet_bulk_update_skills", map[string]any{
		"ids": []string{"local/seo-audit", "demo/skills/private"}, "add_tags": []string{"Bulk"}, "stage": "try",
	})
	if int(bulk["updated"].(float64)) != 2 || int(bulk["failed"].(float64)) != 0 {
		t.Fatalf("bulk update: %+v", bulk)
	}
	previewBulk := mcpData(t, session, "reet_bulk_update_skills", map[string]any{"filter": map[string]any{"untagged": true}, "add_tags": []string{"Tidy"}, "dry_run": true})
	if previewBulk["dry_run"] != true {
		t.Fatalf("bulk dry run: %+v", previewBulk)
	}
	renamed := mcpData(t, session, "reet_rename_tag", map[string]any{"tag_id": tagID, "name": "Tidy renamed tag"})
	if int(renamed["affected"].(float64)) != 0 {
		t.Fatalf("rename tag: %+v", renamed)
	}
	tagPreview := mcpData(t, session, "reet_delete_tag", map[string]any{"tag_id": tagID})
	if tagPreview["dry_run"] != true {
		t.Fatalf("delete tag preview: %+v", tagPreview)
	}
	tagDeleted := mcpData(t, session, "reet_delete_tag", map[string]any{"tag_id": tagID, "dry_run": false, "confirm": true})
	if tagDeleted["deleted"] != true {
		t.Fatalf("delete tag: %+v", tagDeleted)
	}

	duplicates := mcpData(t, session, "reet_find_duplicates", map[string]any{})
	groups := duplicates["groups"].([]any)
	foundSVG, foundWeb := false, false
	for _, raw := range groups {
		group := raw.(map[string]any)
		ids := []string{}
		for _, skill := range group["skills"].([]any) {
			ids = append(ids, skill.(map[string]any)["id"].(string))
		}
		joined := strings.Join(ids, " ")
		if strings.Contains(joined, "local/svg-logo-designer") && strings.Contains(joined, "rknall/claude-skills/svg-logo-designer") {
			foundSVG = true
		}
		if strings.Contains(joined, "local/web-design-builder") && strings.Contains(joined, "rknall/claude-skills/web-design-builder") {
			foundWeb = true
		}
	}
	if !foundSVG || !foundWeb {
		t.Fatalf("duplicate groups missing: %+v", groups)
	}

	events := mcpData(t, session, "reet_list_events", map[string]any{"source": "mcp", "action": "bulk-update", "limit": 10})
	if len(events["events"].([]any)) == 0 {
		t.Fatalf("no MCP audit events: %+v", events)
	}
	detail := events["events"].([]any)[0].(map[string]any)["detail"].(string)
	if !strings.Contains(detail, "Desktop test") {
		t.Fatalf("MCP client name missing from event detail: %s", detail)
	}

	readToken, _ := mcpManual(t, browser, ts, "read")
	readSession := mcpSession(t, ts, readToken)
	tools, err := readSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "reet_bulk_update_skills" || tool.Name == "reet_delete_environment" {
			t.Fatalf("read-only connection discovered write tool %s", tool.Name)
		}
	}
	if _, err := readSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "reet_bulk_update_skills", Arguments: map[string]any{"ids": []string{"local/seo-audit"}, "liked": true}}); err == nil {
		t.Fatal("read-only connection called a new write tool")
	}
}

func TestMCPCompactSkillsFitsOnePage(t *testing.T) {
	ts, st, browser, uid := mcpAccounts(t)
	for i := 0; i < 64; i++ {
		if _, err := st.UpsertLocalFor(uid, fmt.Sprintf("compact-skill-%03d", i), store.Patch{}); err != nil {
			t.Fatal(err)
		}
	}
	token, _ := mcpManual(t, browser, ts, "write")
	session := mcpSession(t, ts, token)
	data := mcpData(t, session, "reet_list_skills", map[string]any{"compact": true, "limit": 100})
	if int(data["count"].(float64)) != 65 || int(data["total_count"].(float64)) != 65 {
		t.Fatalf("compact page did not return every skill: count=%v total=%v", data["count"], data["total_count"])
	}
	if data["has_more"] != false {
		t.Fatalf("compact page still has more: %+v", data)
	}
	if _, ok := data["truncated"]; ok {
		t.Fatalf("compact page truncated: %+v", data)
	}
	first := data["items"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "name", "source", "sourceType", "stage", "liked", "bookmarked", "tags", "environment_count"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("compact item missing %s: %+v", key, first)
		}
	}
	if _, ok := first["notes"]; ok {
		t.Fatalf("compact item leaked notes: %+v", first)
	}
}

// An MCP client can neither list nor release ownership records, so the codes that
// refuse its delete and merge must name them and say what to do instead.
func TestMCPEnvironmentRefusalsNameOwnershipReceiptsAndPins(t *testing.T) {
	ts, st, browser, uid := mcpAccounts(t)
	agent, _, err := st.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	env := store.EnvironmentIDFor(uid, "laptop", "agents", "")
	if err = st.RecordInventoryFor(uid, env, true, store.MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	base, empty := "", ""
	bundle, err := st.SavePrivateSkillRevisionFor(uid, "", store.PrivateSkillWrite{RequestID: "mcp_receipt_bundle_01", ExpectedBase: &base, Name: "demo",
		Files: []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Private instructions\n---\nMCP_PRIVATE\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := st.SaveSkillContentTargetFor(uid, store.SkillContentTargetWrite{EnvironmentID: env, AgentID: agent.ID, BundleID: bundle.Bundle.ID, RevisionID: bundle.Revision.ID, Mode: "additive", ExpectedVersion: &empty})
	if err != nil {
		t.Fatal(err)
	}
	managed, _ := json.Marshal(bundle.Revision.Manifest)
	evidence, _ := json.Marshal(store.SkillContentReceiptEvidence{PinID: pin.ID, PinVersion: pin.Version, AdoptedBy: uid})
	db, err := sql.Open("sqlite", "file:"+filepath.Join(st.Path(), "reet.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := strings.Repeat("a", 48)
	if _, err = db.Exec(`INSERT INTO skill_content_receipts(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,root_id,adapter,schema_version,managed,provenance,evidence,request_key,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, uid, env, agent.ID, "demo", bundle.Bundle.ID, bundle.Revision.ID, strings.Repeat("d", 64), skillpreview.Adapter, 1, string(managed), "adopted", string(evidence), "mcp_receipt_request_1", strings.Repeat("b", 48), 1, 1); err != nil {
		t.Fatal(err)
	}
	if err = st.TouchEnvironmentFor(uid, "workstation", "claude", "", ""); err != nil {
		t.Fatal(err)
	}
	dest := store.EnvironmentIDFor(uid, "workstation", "claude", "")

	token, _ := mcpManual(t, browser, ts, "write")
	session := mcpSession(t, ts, token)

	// Merging is refused for the pin first, then for the record once the pin is gone,
	// and the preview the tool runs by default says so before anything is confirmed.
	if preview := mcpError(t, session, "reet_merge_environments", map[string]any{"from_id": env, "to_id": dest}); !strings.Contains(preview, "environment_has_pins") {
		t.Fatalf("preview of a merge of a pinned report: %s", preview)
	}
	refused := mcpError(t, session, "reet_merge_environments", map[string]any{"from_id": env, "to_id": dest, "dry_run": false, "confirm": true})
	if !strings.Contains(refused, "environment_has_pins") || !strings.Contains(refused, "Remove the private skill pin first.") {
		t.Fatalf("merge of a pinned report: %s", refused)
	}
	if err = st.DeleteSkillContentTargetFor(uid, pin.ID, pin.Version); err != nil {
		t.Fatal(err)
	}
	refused = mcpError(t, session, "reet_merge_environments", map[string]any{"from_id": env, "to_id": dest, "dry_run": false, "confirm": true})
	if !strings.Contains(refused, "environment_has_receipts") || !strings.Contains(refused, "Release the ownership record first.") {
		t.Fatalf("merge of a report that holds an ownership record: %s", refused)
	}

	if preview := mcpError(t, session, "reet_merge_environments", map[string]any{"from_id": env, "to_id": dest}); !strings.Contains(preview, "environment_has_receipts") {
		t.Fatalf("preview of a merge of a report that holds an ownership record: %s", preview)
	}

	// Deleting previews the refusal, applies it, and force forgets the record.
	preview := mcpData(t, session, "reet_delete_environment", map[string]any{"id": env})
	blocked, _ := preview["blocked"].([]any)
	if len(blocked) != 1 || blocked[0].(map[string]any)["code"] != "environment_has_receipts" || len(preview["would_delete"].([]any)) != 0 {
		t.Fatalf("delete preview: %+v", preview)
	}
	refused = mcpError(t, session, "reet_delete_environment", map[string]any{"id": env, "dry_run": false, "confirm": true})
	if !strings.Contains(refused, "environment_has_receipts") || !strings.Contains(refused, "force=true") {
		t.Fatalf("delete of a report that holds an ownership record: %s", refused)
	}
	forced := mcpData(t, session, "reet_delete_environment", map[string]any{"id": env, "dry_run": false, "confirm": true, "force": true})
	if int(forced["deleted"].(float64)) != 1 {
		t.Fatalf("forced delete: %+v", forced)
	}
}
