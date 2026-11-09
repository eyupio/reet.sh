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
func privateSkillFiles(server, token string, skill store.SyncSkill) ([]skillbundle.ContentFile, error) {
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
	req.Header.Set("X-Reet-Token", token)
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
	manifest, files, err := skillimport.Select(revision.Files, nil)
	if err != nil {
		return nil, err
	}
	if manifest.Digest != parts[3] {
		return nil, errors.New("private skill checksum mismatch")
	}
	return files, nil
}

// Download only immutable account-owned revisions from the configured controller.
// Every path, byte count, digest and executable flag is checked before writing.
func installPrivateSkill(server, token string, skill store.SyncSkill, targetPath string, beforeInstall ...func([]skillbundle.ContentFile) error) error {
	files, err := privateSkillFiles(server, token, skill)
	if err != nil {
		return err
	}

	staging, err := os.MkdirTemp(filepath.Dir(targetPath), ".reet-private-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	for _, file := range files {
		path := filepath.Join(staging, filepath.FromSlash(file.Path))
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if file.Executable {
			mode = 0755
		}
		if err = os.WriteFile(path, file.Data, mode); err != nil {
			return err
		}
	}
	// The caller has already backed up a reviewed replacement. Refuse a new
	// entry that appeared while downloading; never merge into an existing tree.
	destination := filepath.Join(targetPath, skill.Slug)
	_, err = os.Lstat(destination)
	if err == nil {
		return errors.New("local skill appeared during transfer; preview again")
	}
	if !os.IsNotExist(err) {
		return err
	}
	for _, record := range beforeInstall {
		if err = record(files); err != nil {
			return err
		}
	}
	return os.Rename(staging, destination)
}
