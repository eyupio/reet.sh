package scan

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestPrivateSkillDownloadIsVerifiedBeforeAnythingIsStaged(t *testing.T) {
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: A private skill\n---\nUse this skill.\n")}, {Path: "scripts/run", Data: []byte("#!/bin/sh\necho demo\n"), Executable: true}}
	manifest, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	revision := store.PrivateSkillRevision{ID: strings.Repeat("a", 32), BundleID: strings.Repeat("b", 32), ManifestDigest: manifest.Digest, Files: files}
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("X-Reet-Token") != "account-token" || r.URL.Path != "/api/v2/skill-bundles/"+revision.BundleID+"/revisions/"+revision.ID {
			t.Error("wrong authenticated revision request")
		}
		json.NewEncoder(w).Encode(revision)
	}))
	defer srv.Close()
	config := agentConfig{Server: srv.URL, Token: "account-token"}
	skill := store.SyncSkill{ID: "local/demo", Slug: "demo", SourceType: "private", Source: "private:" + revision.BundleID + ":" + revision.ID + ":" + manifest.Digest}
	got, err := privateSkillFiles(config, skill)
	if err != nil {
		t.Fatal(err)
	}
	staging, err := stageSkillFiles(t.TempDir(), got)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := privateSkillDigest(staging)
	if err != nil || digest != manifest.Digest {
		t.Fatal(digest, err)
	}
	info, _ := os.Stat(filepath.Join(staging, "scripts/run"))
	if info.Mode()&0111 == 0 {
		t.Fatal("executable mode lost")
	}
	revision.Files = append([]skillbundle.ContentFile{}, files...)
	revision.Files[1].Data = []byte("changed")
	if _, err = privateSkillFiles(config, skill); err == nil {
		t.Fatal("accepted changed bytes")
	}
	if requests != 2 {
		t.Fatal(requests)
	}
	// Path validation rejects traversal before any file is created,
	// even when the response comes from the authenticated controller.
	revision.Files = append(append([]skillbundle.ContentFile{}, files...), skillbundle.ContentFile{Path: "../escape", Data: []byte("no")})
	if _, err = privateSkillFiles(config, skill); err == nil {
		t.Fatal("accepted traversal")
	}
}
func TestPrivateSkillDownloadNeverFollowsRedirectOrInvalidSource(t *testing.T) {
	leaked := false
	next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer next.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, next.URL, 302) }))
	defer srv.Close()
	config := agentConfig{Server: srv.URL, Token: "token"}
	skill := store.SyncSkill{Slug: "demo", SourceType: "private", Source: "private:" + strings.Repeat("a", 32) + ":" + strings.Repeat("b", 32) + ":" + strings.Repeat("c", 64)}
	if _, err := privateSkillFiles(config, skill); err == nil || leaked {
		t.Fatal("redirect followed", err)
	}
	skill.Source += "/../other"
	if _, err := privateSkillFiles(config, skill); err == nil {
		t.Fatal("invalid reference accepted")
	}
}
func TestCollectRecoversAgentSpecificSourceURLWithoutCrossingScopes(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	for _, root := range []string{filepath.Join(home, ".claude/skills"), filepath.Join(home, ".agents/skills"), filepath.Join(project, ".claude/skills")} {
		mkSkill(t, root, "demo", true)
	}
	locks := map[string]string{
		filepath.Join(home, ".agents/.skill-lock.json"): `{"skills":{"demo":{"source":"global/repository"}}}`,
		filepath.Join(home, ".claude/.skill-lock.json"): `{"skills":{"demo":{"sourceType":"github","sourceUrl":"https://github.com/claude/repository.git"}}}`,
		filepath.Join(project, "skills-lock.json"):      `{"skills":{"demo":{"source":"project/repository"}}}`,
	}
	for path, body := range locks {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	found := CollectScan(home, project, nil, "laptop").Found
	if len(found) != 3 {
		t.Fatal(found)
	}
	for _, item := range found {
		want := "global/repository"
		if item.Project != "" {
			want = "project/repository"
		} else if item.Agent == "claude" {
			want = "https://github.com/claude/repository.git"
		}
		if item.Source != want {
			t.Fatalf("source crosses scope: %+v want %s", item, want)
		}
	}
}
