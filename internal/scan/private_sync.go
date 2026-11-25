package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
)

var privateSourcePattern = regexp.MustCompile(`^private:([a-f0-9]{32}):([a-f0-9]{32}):([a-f0-9]{64})$`)

func privateSkillDigest(root string) (string, error) {
	m, err := skillbundle.Scan(root, skillbundle.Limits{})
	if err != nil {
		return "", err
	}
	if !m.Complete {
		return "", errors.New("private skill contains unreadable or unsupported entries")
	}
	return m.Digest, nil
}

// privateSkillFiles reads and verifies an immutable account-owned revision.
func privateSkillFiles(c agentConfig, skill store.SyncSkill) ([]skillbundle.ContentFile, error) {
	server := c.Server
	parts := privateSourcePattern.FindStringSubmatch(skill.Source)
	if parts == nil || !safeSlug.MatchString(skill.Slug) {
		return nil, errors.New("invalid private skill source")
	}
	if err := validateSkillImportController(server); err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", strings.TrimRight(server, "/")+"/api/v2/skill-bundles/"+parts[1]+"/revisions/"+parts[2], nil)
	if err != nil {
		return nil, err
	}
	setAgentHeaders(req.Header, c, "")
	client := noRedirectClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("private skill download: controller replied %s", resp.Status)
	}
	var revision store.PrivateSkillRevision
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 16<<20))
	if err = decoder.Decode(&revision); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid private skill response")
	}
	same := revision.BundleID == parts[1] && revision.ID == parts[2] &&
		revision.ManifestDigest == parts[3]
	if !same {
		return nil, errors.New("private skill revision changed")
	}
	return checkSkillFiles(revision.Files, parts[3])
}

// checkSkillFiles returns files only when they make a valid skill whose
// digest is the one reet named. Nothing reaches disk without this check.
func checkSkillFiles(files []skillbundle.ContentFile, digest string) ([]skillbundle.ContentFile, error) {
	manifest, files, err := skillimport.Select(files, nil)
	if err != nil {
		return nil, err
	}
	if manifest.Digest != digest {
		return nil, errors.New("private skill checksum mismatch")
	}
	return files, nil
}

// makeStaging is os.MkdirTemp; tests replace it to stand for a folder whose
// parent reet cannot write in.
var makeStaging = os.MkdirTemp

// safeSlug is a skill folder name reet accepts from the controller.
var safeSlug = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// stageSkillFiles writes checked files into a new .reet-private-* folder in
// parent and returns it, for the caller to rename into place or remove.
func stageSkillFiles(parent string, files []skillbundle.ContentFile) (string, error) {
	staging, err := makeStaging(parent, ".reet-private-")
	if err != nil {
		return "", err
	}
	for _, file := range files {
		path := filepath.Join(staging, filepath.FromSlash(file.Path))
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			break
		}
		mode := os.FileMode(0644)
		if file.Executable {
			mode = 0755
		}
		if err = os.WriteFile(path, file.Data, mode); err != nil {
			break
		}
	}
	if err != nil {
		os.RemoveAll(staging)
		return "", err
	}
	return staging, nil
}
