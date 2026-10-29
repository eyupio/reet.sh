package api

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func favouritesPath() string { return "/api/v2/profile/model-favourites" }

func favouriteQuery(tool, model string) string {
	q := url.Values{}
	q.Set("tool", tool)
	q.Set("model", model)
	return favouritesPath() + "?" + q.Encode()
}

func favouriteList(t *testing.T, f *launchAPI) (float64, []any, string) {
	t.Helper()
	code, out, raw := f.do(t, f.cookie, "GET", favouritesPath(), nil)
	favourites, _ := out["favourites"].([]any)
	version, _ := out["version"].(float64)
	if code != 200 {
		t.Fatal("list", code, raw)
	}
	return version, favourites, raw
}

func TestModelFavouritesStarUnstarAndIdempotence(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	if version, favourites, raw := favouriteList(t, f); version != 0 || len(favourites) != 0 {
		t.Fatal("new account", version, raw)
	}
	code, out, raw := f.do(t, f.cookie, "POST", favouritesPath(), map[string]any{"tool": "opencode", "model": "model-one"})
	if code != 201 || out["version"] != float64(1) || len(out["favourites"].([]any)) != 1 {
		t.Fatal("star", code, raw)
	}
	if fav := out["favourites"].([]any)[0].(map[string]any); fav["tool"] != "opencode" || fav["model"] != "model-one" || fav["agentDefault"] != false {
		t.Fatal("star shape", raw)
	}
	code, out, raw = f.do(t, f.cookie, "POST", favouritesPath(), map[string]any{"tool": "opencode", "model": "model-one"})
	if code != 200 || out["version"] != float64(1) || len(out["favourites"].([]any)) != 1 {
		t.Fatal("a second star was not idempotent", code, raw)
	}
	code, _, raw = f.do(t, f.cookie, "DELETE", favouriteQuery("opencode", "model-one"), nil)
	if code != 204 || raw != "" {
		t.Fatal("unstar", code, raw)
	}
	code, _, raw = f.do(t, f.cookie, "DELETE", favouriteQuery("opencode", "model-one"), nil)
	if code != 204 {
		t.Fatal("a second unstar was not idempotent", code, raw)
	}
	if version, favourites, _ := favouriteList(t, f); version != 2 || len(favourites) != 0 {
		t.Fatal("after unstar", version, favourites)
	}
	added := projectEvents(t, f.st, f.uid, "model-favourite-added")
	if len(added) != 1 || !reflect.DeepEqual(added[0], map[string]any{"tool": "opencode", "model": "model-one"}) {
		t.Fatal("added audit", added)
	}
	removed := projectEvents(t, f.st, f.uid, "model-favourite-removed")
	if len(removed) != 1 || !reflect.DeepEqual(removed[0], map[string]any{"tool": "opencode", "model": "model-one"}) {
		t.Fatal("removed audit", removed)
	}
}

func TestModelFavouritesOrderStaleAndSuccess(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	for _, model := range []string{"model-a", "model-b", "model-c"} {
		if code, _, raw := f.do(t, f.cookie, "POST", favouritesPath(), map[string]any{"tool": "opencode", "model": model}); code != 201 {
			t.Fatal("star", code, raw)
		}
	}
	version, _, _ := favouriteList(t, f)
	order := []map[string]any{
		{"tool": "opencode", "model": "model-c"},
		{"tool": "opencode", "model": "model-a"},
		{"tool": "opencode", "model": "model-b"},
	}
	code, out, raw := f.do(t, f.cookie, "PUT", favouritesPath()+"/order", map[string]any{"version": version + 1, "items": order})
	if code != 409 || out["code"] != "favourites_stale" {
		t.Fatal("stale version", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PUT", favouritesPath()+"/order", map[string]any{"version": version, "items": order[:2]})
	if code != 409 || out["code"] != "favourites_stale" {
		t.Fatal("missing item", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PUT", favouritesPath()+"/order", map[string]any{"version": version, "items": order})
	if code != 200 || out["version"] != version+1 {
		t.Fatal("order", code, raw)
	}
	got := out["favourites"].([]any)
	if got[0].(map[string]any)["model"] != "model-c" || got[1].(map[string]any)["model"] != "model-a" || got[2].(map[string]any)["model"] != "model-b" {
		t.Fatal("order result", raw)
	}
	if code, out, raw = f.do(t, f.cookie, "PUT", favouritesPath()+"/order", map[string]any{"version": version, "items": order}); code != 409 || out["code"] != "favourites_stale" {
		t.Fatal("old version after a good order", code, raw)
	}
	if v, list, _ := favouriteList(t, f); v != version+1 || len(list) != 3 || list[0].(map[string]any)["model"] != "model-c" {
		t.Fatal("a refused order changed the list", v, list)
	}
}

func TestModelFavouriteDefaultSetClearAndNotFound(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	for _, fav := range []map[string]any{{"tool": "opencode", "model": "model-a"}, {"tool": "opencode", "model": "model-b"}, {"tool": "claude", "model": "model-c"}} {
		if code, _, raw := f.do(t, f.cookie, "POST", favouritesPath(), fav); code != 201 {
			t.Fatal("star", code, raw)
		}
	}
	code, out, raw := f.do(t, f.cookie, "PUT", favouritesPath()+"/default", map[string]any{"tool": "opencode", "model": "model-b"})
	if code != 200 || !favouriteIsDefault(out, "opencode", "model-b") {
		t.Fatal("set default", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PUT", favouritesPath()+"/default", map[string]any{"tool": "opencode", "model": "model-a"})
	if code != 200 || favouriteIsDefault(out, "opencode", "model-b") || !favouriteIsDefault(out, "opencode", "model-a") {
		t.Fatal("switch default", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PUT", favouritesPath()+"/default", map[string]any{"tool": "opencode", "model": ""})
	if code != 200 || favouriteIsDefault(out, "opencode", "model-a") {
		t.Fatal("clear default", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PUT", favouritesPath()+"/default", map[string]any{"tool": "opencode", "model": "model-c"})
	if code != 404 || out["code"] != "favourite_not_found" {
		t.Fatal("not a favourite", code, raw)
	}
	rows := projectEvents(t, f.st, f.uid, "model-favourite-default")
	if len(rows) != 3 {
		t.Fatal("default audit", rows)
	}
	for _, row := range rows {
		if len(row) != 2 || row["tool"] == nil || row["model"] == nil {
			t.Fatal("default audit carries more than tool and model", row)
		}
	}
}

func favouriteIsDefault(out map[string]any, tool, model string) bool {
	list, _ := out["favourites"].([]any)
	for _, raw := range list {
		fav, _ := raw.(map[string]any)
		if fav["tool"] == tool && fav["model"] == model {
			return fav["agentDefault"] == true
		}
	}
	return false
}

func TestModelFavouritesInvalidAndFull(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	for name, c := range map[string]struct {
		method, path string
		body         any
		code, field  string
	}{
		"bad tool":          {"POST", favouritesPath(), map[string]any{"tool": "bash", "model": "model-one"}, "invalid_tool", "tool"},
		"bad model":         {"POST", favouritesPath(), map[string]any{"tool": "opencode", "model": "-rm"}, "invalid_model", "model"},
		"delete bad tool":   {"DELETE", favouriteQuery("bash", "model-one"), nil, "invalid_tool", "tool"},
		"default bad tool":  {"PUT", favouritesPath() + "/default", map[string]any{"tool": "bash", "model": ""}, "invalid_tool", "tool"},
		"default bad model": {"PUT", favouritesPath() + "/default", map[string]any{"tool": "opencode", "model": "-rm"}, "invalid_model", "model"},
		"order bad tool":    {"PUT", favouritesPath() + "/order", map[string]any{"version": 0, "items": []map[string]any{{"tool": "bash", "model": "model-one"}}}, "invalid_tool", "tool"},
	} {
		code, out, raw := f.do(t, f.cookie, c.method, c.path, c.body)
		if code != 422 || out["code"] != c.code || out["field"] != c.field {
			t.Fatal(name, code, raw)
		}
	}
	if code, out, raw := f.do(t, f.cookie, "POST", favouritesPath(), `{"tool":"opencode","model":"model-one","extra":true}`); code != 400 || out["error"] == nil {
		t.Fatal("strict decode", code, raw)
	}
	for i := 0; i < store.MaxModelFavourites; i++ {
		if _, _, err := f.st.AddModelFavouriteFor(f.uid, "opencode", fmt.Sprintf("model-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	code, out, raw := f.do(t, f.cookie, "POST", favouritesPath(), map[string]any{"tool": "opencode", "model": "model-over"})
	if code != 409 || out["code"] != "favourites_full" {
		t.Fatal("full", code, raw)
	}
}

func TestModelFavouritesViewerCanReadButNotMutate(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	if code, _, raw := f.do(t, f.cookie, "POST", favouritesPath(), map[string]any{"tool": "opencode", "model": "model-one"}); code != 201 {
		t.Fatal("star", code, raw)
	}
	viewer, err := f.st.CreateUser("favourites-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	cookie := terminalLogin(t, f.st, viewer.ID)
	if code, out, raw := f.do(t, cookie, "GET", favouritesPath(), nil); code != 200 || out["version"] == nil {
		t.Fatal("viewer read", code, raw)
	}
	for name, c := range map[string]struct {
		method, path string
		body         any
	}{
		"star":    {"POST", favouritesPath(), map[string]any{"tool": "opencode", "model": "model-two"}},
		"unstar":  {"DELETE", favouriteQuery("opencode", "model-one"), nil},
		"order":   {"PUT", favouritesPath() + "/order", map[string]any{"version": 1, "items": []any{}}},
		"default": {"PUT", favouritesPath() + "/default", map[string]any{"tool": "opencode", "model": "model-one"}},
	} {
		if code, _, raw := f.do(t, cookie, c.method, c.path, c.body); code != 403 {
			t.Fatal(name, code, raw)
		}
	}
	if version, list, _ := favouriteList(t, f); version != 1 || len(list) != 1 {
		t.Fatal("a viewer changed the owner's favourites", version, list)
	}
}

func TestProfileDefaultLaunchFieldsAndInvalidCombinations(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	code, out, raw := f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{
		"defaultTool": "cline", "defaultModel": "model-one", "defaultEffort": "high",
	})
	profile, _ := out["profile"].(map[string]any)
	if code != 200 || profile["defaultTool"] != "cline" || profile["defaultModel"] != "model-one" || profile["defaultEffort"] != "high" {
		t.Fatal("save defaults", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"defaultEffort": "low"})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["defaultTool"] != "cline" || profile["defaultModel"] != "model-one" || profile["defaultEffort"] != "low" {
		t.Fatal("partial save", code, raw)
	}
	for name, body := range map[string]map[string]any{
		"bad tool":             {"defaultTool": "bash"},
		"bad model":            {"defaultModel": "-rm"},
		"bad effort":           {"defaultEffort": "max"},
		"effort on claude":     {"defaultTool": "claude", "defaultModel": "", "defaultEffort": "high"},
		"model without tool":   {"defaultTool": "", "defaultModel": "model-one", "defaultEffort": ""},
		"effort without tool":  {"defaultTool": "", "defaultModel": "", "defaultEffort": "high"},
		"retain model no tool": {"defaultTool": ""},
	} {
		code, _, raw := f.do(t, f.cookie, "PATCH", "/api/v2/profile", body)
		if code != 400 {
			t.Fatal(name, code, raw)
		}
	}
	if p, err := f.st.ProfileFor(f.uid); err != nil || p.DefaultTool != "cline" || p.DefaultModel != "model-one" || p.DefaultEffort != "low" {
		t.Fatal("a refused profile save changed something", p, err)
	}
	events := projectEvents(t, f.st, f.uid, "profile")
	var detail map[string]any
	for _, row := range events {
		if row["defaultEffort"] == "high" {
			detail = row
		}
	}
	if detail == nil || detail["defaultTool"] != "cline" || detail["defaultModel"] != "model-one" {
		t.Fatal("profile audit detail", events)
	}
}

func TestProfileThemeEditIgnoresALegacyModelWithoutTool(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	profile, err := f.st.ProfileFor(f.uid)
	if err != nil {
		t.Fatal(err)
	}
	profile.DefaultModel = "legacy-model"
	profile.DefaultTool = ""
	if err := f.st.SaveProfileFor(f.uid, profile); err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"theme": "dark"})
	saved, _ := out["profile"].(map[string]any)
	if code != 200 || saved["theme"] != "dark" || saved["defaultModel"] != "legacy-model" {
		t.Fatal("a theme edit was blocked by the legacy pairing", code, raw)
	}
	code, _, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"defaultModel": "another-model"})
	if code != 400 {
		t.Fatal("a launch-default edit kept the legacy pairing", code, raw)
	}
}

func TestFolderProjectClineAndEffort(t *testing.T) {
	f := projectFixture(t)
	created := f.fpCreate(t, map[string]any{"defaultTool": "cline", "defaultModel": "model-one", "defaultEffort": "high"})
	if created["defaultTool"] != "cline" || created["defaultModel"] != "model-one" || created["defaultEffort"] != "high" {
		t.Fatal("cline project", created)
	}
	id := created["id"].(string)
	res := f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"defaultEffort": "low"}, `"1"`)
	if res.code != 200 || res.out["defaultEffort"] != "low" || res.out["defaultTool"] != "cline" {
		t.Fatal("effort update", res.code, res.raw)
	}
	res = f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"defaultTool": "claude"}, `"2"`)
	if res.code != 422 || res.out["code"] != "invalid_effort" || res.out["field"] != "defaultEffort" {
		t.Fatal("effort left on an agent that takes none", res.code, res.raw)
	}
	if got, _ := f.st.FolderProjectFor(f.uid, id); got.DefaultEffort != "low" || got.DefaultTool != "cline" {
		t.Fatal("a refused update changed the project", got)
	}
	for name, body := range map[string]map[string]any{
		"model without tool":  {"defaultTool": "", "defaultModel": "model-one"},
		"effort without tool": {"defaultTool": "", "defaultModel": "", "defaultEffort": "high"},
		"effort on claude":    {"defaultTool": "claude", "defaultEffort": "high"},
		"unknown effort":      {"defaultTool": "cline", "defaultEffort": "max"},
		"unknown tool":        {"defaultTool": "bash"},
	} {
		f.resetFolderWrites()
		res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(body), "")
		if res.code != 422 {
			t.Fatal(name, res.code, res.raw)
		}
	}
	// An existing row keeps the defaults it had when another field changes.
	plain := f.fpCreate(t, map[string]any{"name": "Plain", "dir": "/home/tester/plain", "defaultEffort": nil})
	plainID := plain["id"].(string)
	res = f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+plainID, map[string]any{"name": "Renamed"}, `"1"`)
	if res.code != 200 || res.out["defaultEffort"] != "" || res.out["defaultTool"] != "claude" || res.out["defaultModel"] != "sonnet" {
		t.Fatal("existing row changed", res.code, res.raw)
	}
	if strings.Contains(res.raw, "defaultEffort\":\"high") {
		t.Fatal("unexpected effort", res.raw)
	}
}
