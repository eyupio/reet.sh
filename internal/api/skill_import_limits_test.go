package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// importWhileLocked posts an import while the import lock is held elsewhere,
// and says what came back, or that it was left waiting for the lock.
func importWhileLocked(t *testing.T, s *server, body string, srvDo func(string) (int, string)) (code int, text string, waited bool) {
	t.Helper()
	s.importMu.Lock()
	type answer struct {
		code int
		text string
	}
	done := make(chan answer, 1)
	go func() {
		c, b := srvDo(body)
		done <- answer{c, b}
	}()
	select {
	case a := <-done:
		s.importMu.Unlock()
		return a.code, a.text, false
	case <-time.After(3 * time.Second):
		s.importMu.Unlock()
		a := <-done
		return a.code, a.text, true
	}
}

// An import has a limit on what it holds, and one over it is refused before
// the import lock is taken: each entry writes to the library, one at a time,
// while every other import, merge and forget waits.
func TestAnImportOverTheLimitIsRefusedWithoutTakingTheLock(t *testing.T) {
	s, _, srv, _ := auditServer(t, "")
	post := func(body string) (int, string) { return do(t, srv, "POST", "/api/import", body) }
	skills := func(n int) string { return `"` + strings.Join(make([]string, n), `","`) + `"` }
	over := importMaxEntries + 1
	for name, body := range map[string]string{
		"items":        `{"items":[` + strings.Repeat(`{"name":"x"},`, over-1) + `{"name":"x"}]}`,
		"text lines":   `{"text":"` + strings.Repeat(`x\n`, over-1) + `x"}`,
		"environments": `{"environments":[{"host":"h","agent":"claude","skills":[` + skills(over-1) + `]}]}`,
		"all three":    `{"items":[{"name":"x"}],"text":"x\ny","environments":[{"host":"h","agent":"claude","skills":[` + skills(over-4) + `]}]}`,
	} {
		code, text, waited := importWhileLocked(t, s, body, post)
		if waited {
			t.Errorf("%s: the import took the lock before refusing", name)
		}
		if code != http.StatusRequestEntityTooLarge || !strings.Contains(text, fmt.Sprint(importMaxEntries)) {
			t.Errorf("%s: %d %.200s", name, code, text)
		}
	}
	if code, text := do(t, srv, "GET", "/api/v2/skills?status=active", ""); code != 200 || strings.Contains(text, `"name":"x"`) {
		t.Fatalf("a refused import wrote: %d %.200s", code, text)
	}
}

// What a scan of a machine with a great many skills sends passes: an import
// at the limit is made.
func TestAnImportAtTheLimitPasses(t *testing.T) {
	_, _, srv, _ := auditServer(t, "")
	// One environment and the rest names in it: counted, not written one by one.
	skills := `"` + strings.Join(make([]string, importMaxEntries-1), `","`) + `"`
	body := `{"environments":[{"host":"h","agent":"claude","skills":[` + skills + `]}],"machine":{"host":"h"}}`
	if code, text := do(t, srv, "POST", "/api/import", body); code != 200 {
		t.Fatalf("at the limit: %d %.200s", code, text)
	}
}

// A name no library skill can have is skipped, like any name that cannot be
// told apart, and the rest of the import goes through.
func TestAnImportSkipsANameLongerThanALibrarySkillCanHave(t *testing.T) {
	_, st, srv, owner := auditServer(t, "")
	long := strings.Repeat("n", store.MaxLibrarySkillNameBytes+1)
	fits := strings.Repeat("m", store.MaxLibrarySkillNameBytes)
	body, _ := json.Marshal(map[string]any{
		"items": []map[string]string{{"name": long}, {"name": fits}, {"name": long, "source": "acme/tools"}},
		"text":  long + "\nshort-one\n",
	})
	code, text := do(t, srv, "POST", "/api/import", string(body))
	var out struct {
		Imported int
		Errors   []string
	}
	if code != 200 || json.Unmarshal([]byte(text), &out) != nil {
		t.Fatalf("import: %d %.200s", code, text)
	}
	if out.Imported != 2 || len(out.Errors) != 3 {
		t.Fatalf("imported %d with %d errors, want 2 and 3: %.300s", out.Imported, len(out.Errors), text)
	}
	for _, e := range out.Errors {
		if !strings.Contains(e, fmt.Sprint(store.MaxLibrarySkillNameBytes)) || len(e) > 200 {
			t.Errorf("error %q", e)
		}
	}
	items, err := st.LibraryFor(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, it := range items {
		names[it.Name] = true
	}
	if len(names) != 2 || !names[fits] || !names["short-one"] {
		t.Fatalf("library holds %d names, want the name that fits and short-one", len(names))
	}
}
