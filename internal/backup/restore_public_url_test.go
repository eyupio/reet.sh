package backup

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/lifecycle"
)

func TestARestoreWithAPublicURLSaysWhatChangedAndEndsWithTheChecklist(t *testing.T) {
	svc, st, dir, id := liveController(t)
	res, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{AllowLiveSource: true, PublicURL: "https://new.example.com/"}, st.Close)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Rewrote the saved addresses to https://new.example.com:", "mail base URL", "sync library origin", "MCP grants", "Not in the copy; set these by hand", "REET_OIDC_REDIRECT_URL", "terminal-control/state.json", "REET_BACKUP_S3_BUCKET"} {
		if !strings.Contains(res.Message, want) {
			t.Fatalf("the report does not say %q:\n%s", want, res.Message)
		}
	}
	// The checklist ends the report, after what was held back.
	if i, j := strings.Index(res.Message, "Rewrote the saved"), strings.Index(res.Message, "Not in the copy"); i < 0 || j < i {
		t.Fatal("the checklist does not come last")
	}
}

func TestARestoreWithNoPublicURLSaysTheAddressesWereLeft(t *testing.T) {
	svc, st, dir, id := liveController(t)
	res, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{AllowLiveSource: true}, st.Close)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Message, "Saved addresses were left as they were") || strings.Contains(res.Message, "Rewrote the saved") {
		t.Fatalf("report:\n%s", res.Message)
	}
}

func TestABadPublicURLIsRefusedBeforeAnythingChanges(t *testing.T) {
	for _, bad := range []string{"https://x.example/path", "ftp://x.example", "https://u:p@x.example", "not a url"} {
		svc, st, dir, id := liveController(t)
		before := liveBytes(t, dir)
		if _, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{AllowLiveSource: true, PublicURL: bad}, st.Close); err == nil || !strings.Contains(err.Error(), "plain http or https origin") {
			t.Fatalf("%q: %v", bad, err)
		}
		if !bytes.Equal(before, liveBytes(t, dir)) {
			t.Fatalf("%q changed reet.db", bad)
		}
		noAsideCopy(t, dir)
		raw, _ := movable(t)
		dest, ddir := destination(t)
		if _, err := dest.StageImport(context.Background(), ddir, raw, copyPassword, RestoreOptions{AllowLiveSource: true, PublicURL: bad}); err == nil {
			t.Fatalf("%q was staged", bad)
		}
		if p, _ := PendingImport(ddir); p != nil {
			t.Fatalf("%q left a stage", bad)
		}
	}
}

func TestAnImportWithAPublicURLReportsItAndAppliesIt(t *testing.T) {
	raw, _ := movable(t)
	svc, dir := destination(t)
	rep, err := svc.StageImport(context.Background(), dir, raw, copyPassword, RestoreOptions{AllowLiveSource: true, PublicURL: "https://new.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.PublicURL != "https://new.example.com" || !strings.Contains(strings.Join(rep.Lines(), "\n"), "Rewrote the saved addresses") {
		t.Fatalf("report: %+v", rep)
	}
}

// The checklist is derived from the code: a setting added to what the server
// reads, and not to the checklist, fails here.
func TestTheChecklistNamesEverySettingTheServerReads(t *testing.T) {
	named := map[string]bool{}
	for _, it := range Checklist() {
		for _, n := range it.Names {
			named[n] = true
		}
	}
	pattern := regexp.MustCompile(`REET_[A-Z0-9_]+|SKILLS_SH_TOKEN`)
	found := map[string]string{}
	read := func(path, from, to string) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		if from != "" {
			i := strings.Index(src, from)
			if i < 0 {
				t.Fatalf("%s no longer has %q; update this test", path, from)
			}
			src = src[i:]
			if j := strings.Index(src, to); to != "" && j > 0 {
				src = src[:j]
			}
		}
		for _, name := range pattern.FindAllString(src, -1) {
			found[name] = path
		}
	}
	// What `serve` and its helpers read, what single sign-on reads, what the
	// backup reader and the mail outbox read, and what `reet install` saves.
	read("../../cmd/reet/main.go", "func serve() error", "// Server identity belongs")
	read("../oidc/config.go", "", "")
	read("config.go", "", "")
	read("../mail/config.go", "", "")
	for _, name := range lifecycle.ServerEnv() {
		found[name] = "lifecycle.ServerEnv"
	}
	// Not settings of a controller: a prefix used to build names, and the
	// environment-file pointer only a client reads.
	skip := map[string]bool{"REET_OIDC_": true, "REET_ENV_FILE": true, "REET_BACKUP_S3_": true, "REET_SETUP_S3_": true}
	for name, where := range found {
		if skip[name] || strings.HasSuffix(name, "_") {
			continue
		}
		if !named[name] {
			t.Errorf("%s (read in %s) is not in the checklist; add it to Checklist()", name, where)
		}
	}
	if len(found) < 30 {
		t.Fatalf("only %d settings found; the source patterns have drifted", len(found))
	}
}
