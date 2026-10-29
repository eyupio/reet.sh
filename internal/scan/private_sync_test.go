package scan

import (
	"bytes"
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

func TestPrivateSkillInstallVerifiedFilesAndNoOverwrite(t *testing.T) {
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
	skill := store.SyncSkill{ID: "local/demo", Slug: "demo", SourceType: "private", Source: "private:" + revision.BundleID + ":" + revision.ID + ":" + manifest.Digest}
	dir := filepath.Join(t.TempDir(), "skills")
	if err = os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err = installPrivateSkill(srv.URL, "account-token", skill, dir); err != nil {
		t.Fatal(err)
	}
	got, err := privateSkillDigest(filepath.Join(dir, "demo"))
	if err != nil || got != manifest.Digest {
		t.Fatal(got, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "demo/scripts/run"))
	if info.Mode()&0111 == 0 {
		t.Fatal("executable mode lost")
	}
	if err = installPrivateSkill(srv.URL, "account-token", skill, dir); err == nil {
		t.Fatal("overwrote installed skill")
	}
	revision.Files[1].Data = []byte("changed")
	if err = installPrivateSkill(srv.URL, "account-token", skill, dir); err == nil {
		t.Fatal("accepted changed bytes")
	}
	if requests != 3 {
		t.Fatal(requests)
	}
	// Path validation rejects traversal before any file is created,
	// even when the response comes from the authenticated controller.
	revision.Files = append(files, skillbundle.ContentFile{Path: "../escape", Data: []byte("no")})
	if err = installPrivateSkill(srv.URL, "account-token", skill, dir); err == nil {
		t.Fatal("accepted traversal")
	}
}
func TestPrivateSkillDownloadNeverFollowsRedirectOrInvalidSource(t *testing.T) {
	leaked := false
	next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer next.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, next.URL, 302) }))
	defer srv.Close()
	skill := store.SyncSkill{Slug: "demo", SourceType: "private", Source: "private:" + strings.Repeat("a", 32) + ":" + strings.Repeat("b", 32) + ":" + strings.Repeat("c", 64)}
	if err := installPrivateSkill(srv.URL, "token", skill, t.TempDir()); err == nil || leaked {
		t.Fatal("redirect followed", err)
	}
	skill.Source += "/../other"
	if err := installPrivateSkill(srv.URL, "token", skill, t.TempDir()); err == nil {
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

func TestPrivateSkillSyncApplyAndSubsequentPreview(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Transfer test\n---\nPrivate instructions.\n")}}
	manifest, _ := skillbundle.FromContents(files)
	revision := store.PrivateSkillRevision{ID: strings.Repeat("a", 32), BundleID: strings.Repeat("b", 32), ManifestDigest: manifest.Digest, Files: files}
	skill := store.SyncSkill{ID: "local/demo", Name: "demo", Slug: "demo", SourceType: "private", Source: "private:" + revision.BundleID + ":" + revision.ID + ":" + manifest.Digest}
	target := store.SyncTarget{Target: store.Environment{ID: "target", Name: "Desktop", Host: "desktop", Agent: "agents"}, Mode: "additive", Revision: "revision", Desired: []store.SyncSkill{skill}}
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v2/skill-bundles/"):
			downloads++
			json.NewEncoder(w).Encode(revision)
		case r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{target}})
		default:
			w.Write([]byte(`{"errors":[]}`))
		}
	}))
	defer srv.Close()
	args := []string{"--server", srv.URL, "--token", "token", "--env", "desktop", "--target", "target", "--apply"}
	execute := func([]string, string) error { t.Fatal("private sync invoked public installer"); return nil }
	var out bytes.Buffer
	if err := runSync(args, strings.NewReader("Desktop\n"), &out, execute, "test"); err != nil {
		t.Fatal(err, out.String())
	}
	out.Reset()
	if err := runSync(args, strings.NewReader("Desktop\n"), &out, execute, "test"); err != nil {
		t.Fatal(err, out.String())
	}
	if downloads != 1 {
		t.Fatal("already matching private copy downloaded again", downloads, out.String())
	}
}
