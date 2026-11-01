package api

import (
	"strings"
	"testing"
)

func groupID(t *testing.T, out map[string]any) string {
	t.Helper()
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("no group id in %+v", out)
	}
	return id
}

func TestProjectGroupsCreateMoveAndRemove(t *testing.T) {
	f := projectFixture(t)
	first := f.fpDo(t, f.cookie, "POST", "/api/v2/project-groups", map[string]any{"name": "Client work", "colour": "blue"}, "")
	if first.code != 201 || first.out["name"] != "Client work" || first.out["colour"] != "blue" || first.out["position"] != float64(0) || first.header.Get("ETag") != `"1"` {
		t.Fatal("create group", first.code, first.raw)
	}
	g1 := groupID(t, first.out)
	second := f.fpDo(t, f.cookie, "POST", "/api/v2/project-groups", map[string]any{"name": "Personal"}, "")
	if second.code != 201 {
		t.Fatal("second group", second.code, second.raw)
	}
	g2 := groupID(t, second.out)
	if dup := f.fpDo(t, f.cookie, "POST", "/api/v2/project-groups", map[string]any{"name": "client work"}, ""); dup.code != 409 || dup.out["code"] != "group_name_taken" {
		t.Fatal("duplicate name", dup.code, dup.raw)
	}

	project := f.fpCreate(t, nil)
	pid := project["id"].(string)
	moved := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/"+pid+"/group", map[string]any{"groupId": g1}, "")
	if moved.code != 200 {
		t.Fatal("move to group", moved.code, moved.raw)
	}
	list := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, "")
	got := list.out["projects"].([]any)[0].(map[string]any)
	if got["groupId"] != g1 {
		t.Fatal("list group", got)
	}

	// The whole rail order and each project's group settle together.
	order := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/reorder", map[string]any{
		"ids": []string{pid}, "versions": []int64{int64(got["version"].(float64))}, "groupIds": []string{g2},
	}, "")
	if order.code != 200 {
		t.Fatal("arrange", order.code, order.raw)
	}
	if moved := order.out["projects"].([]any)[0].(map[string]any); moved["groupId"] != g2 {
		t.Fatal("arrange group", moved)
	}

	groups := f.fpDo(t, f.cookie, "GET", "/api/v2/project-groups", nil, "")
	if groups.code != 200 || len(groups.out["groups"].([]any)) != 2 {
		t.Fatal("groups", groups.code, groups.raw)
	}
	// Remove the group holding the project: the project stays and is ungrouped.
	if gone := f.fpDo(t, f.cookie, "DELETE", "/api/v2/project-groups/"+g2, nil, `"1"`); gone.code != 200 {
		t.Fatal("remove group", gone.code, gone.raw)
	}
	list = f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, "")
	if got := list.out["projects"].([]any)[0].(map[string]any); got["groupId"] != nil {
		t.Fatal("project left a removed group", got)
	}
}

func TestProjectGroupStaleVersionAndUnknownGroup(t *testing.T) {
	f := projectFixture(t)
	created := f.fpDo(t, f.cookie, "POST", "/api/v2/project-groups", map[string]any{"name": "Work"}, "")
	gid := groupID(t, created.out)
	stale := f.fpDo(t, f.cookie, "PATCH", "/api/v2/project-groups/"+gid, map[string]any{"name": "Work 2"}, `"9"`)
	if stale.code != 409 || stale.out["code"] != "group_changed" {
		t.Fatal("stale group", stale.code, stale.raw)
	}
	pid := f.fpCreate(t, nil)["id"].(string)
	bad := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/"+pid+"/group", map[string]any{"groupId": "grp_missing"}, "")
	if bad.code != 422 || !strings.Contains(bad.raw, "invalid_group") {
		t.Fatal("unknown group", bad.code, bad.raw)
	}
}
