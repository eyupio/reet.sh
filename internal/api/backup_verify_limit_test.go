package api

import (
	"net/http"
	"strings"
	"testing"
)

// A verify of a copy kept only off-site downloads and opens the whole copy, and
// a wrong password is a guess; so an account gets a few an hour, then waits.
func TestVerifyingCopiesIsLimitedPerAccount(t *testing.T) {
	t.Parallel()
	tg := newTarget(t, true)
	run := runRequest(t, tg, "owner-token", nil)
	id, _ := run.v["id"].(string)
	if run.code != 200 || id == "" {
		t.Fatalf("run: %d %s", run.code, run.text)
	}
	verify := func(token, body string) reply {
		req, _ := http.NewRequest("POST", tg.url+"/api/v2/admin/backups/items/"+id+"/verify", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		return doImport(t, req)
	}
	// A monitoring script verifies with an owner's token; wrong guesses count too.
	if r := verify("owner-token", `{}`); r.code != 200 || r.v["ok"] != true {
		t.Fatalf("first verify: %d %s", r.code, r.text)
	}
	for i := range 5 {
		if r := verify("owner-token", `{"password":"a wrong guess at the password"}`); r.code != 200 {
			t.Fatalf("guess %d: %d %s", i, r.code, r.text)
		}
	}
	req, _ := http.NewRequest("POST", tg.url+"/api/v2/admin/backups/items/"+id+"/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer owner-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("the seventh verify in an hour: %d, Retry-After %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if r := verify("owner-token", `{}`); !strings.Contains(r.text, "try again later") {
		t.Fatalf("the refusal: %d %s", r.code, r.text)
	}
	// The budget is the account's: another account is not held back by it.
	req, _ = http.NewRequest("POST", tg.url+"/api/v2/backups/items/"+id+"/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer member-token")
	if r := doImport(t, req); r.code != 200 {
		t.Fatalf("another account was limited: %d %s", r.code, r.text)
	}
}
