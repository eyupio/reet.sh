package api

import (
	"encoding/json"
	"fmt"
	"testing"
	"unicode/utf8"
)

// A sync target with a large library is one record bigger than the response
// limit. It used to fail with "one record exceeds the response limit", so the
// tool could not show most targets at all. The largest lists are now counted
// instead, starting with the biggest, and the record says which were left out.
func TestMCPPageSummarisesOneOversizedRecord(t *testing.T) {
	skills := func(n int) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = map[string]any{"id": fmt.Sprintf("owner/repository/skill-%04d", i), "name": fmt.Sprintf("skill-%04d", i), "source": "owner/repository", "sourceType": "github"}
		}
		return out
	}
	target := map[string]any{"revision": "one", "mode": "additive", "desired": skills(400), "baseline": skills(300), "missing": skills(2), "extra": skills(1)}
	page, err := mcpPage([]any{target}, map[string]any{}, nil)
	if err != nil {
		t.Fatalf("an oversized record must still be readable: %v", err)
	}
	raw, _ := json.Marshal(page)
	if utf8.RuneCount(raw) >= mcpResponseLimit {
		t.Fatalf("page is %d characters", utf8.RuneCount(raw))
	}
	item := page["items"].([]any)[0].(map[string]any)
	if item["desired_count"] != 400 || item["baseline_count"] != 300 {
		t.Fatalf("left-out lists were not counted: %v %v", item["desired_count"], item["baseline_count"])
	}
	if _, kept := item["desired"]; kept {
		t.Fatal("the oversized list is still in the record")
	}
	if len(item["missing"].([]any)) != 2 || len(item["extra"].([]any)) != 1 || item["revision"] != "one" {
		t.Fatalf("small fields must survive: %+v", item)
	}
	if page["truncated"] != true || page["truncated_reason"] != "record_lists_counted" || page["count"] != 1 {
		t.Fatalf("the page must say it was shortened: %+v", page["truncated_reason"])
	}
	if _, changed := target["desired_count"]; changed || len(target["desired"].([]any)) != 400 {
		t.Fatal("the caller's record was modified")
	}
	// A record with nothing to count is still refused, as before.
	if _, err := mcpPage([]any{map[string]any{"notes": string(make([]byte, mcpResponseLimit))}}, map[string]any{}, nil); err == nil {
		t.Fatal("a record that cannot be shortened must be refused")
	}
}
