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
	"time"

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

// Download only immutable account-owned revisions from the configured controller.
// Every path, byte count, digest and executable flag is checked before writing.
func installPrivateSkill(server, token string, skill store.SyncSkill, targetPath string) error {
	parts := privateSourcePattern.FindStringSubmatch(skill.Source)
	if parts == nil || !safeSlug.MatchString(skill.Slug) {
		return errors.New("invalid private skill source")
	}
	if err := validateSkillImportController(server); err != nil {
		return err
	}
	req, err := http.NewRequest("GET", strings.TrimRight(server, "/")+"/api/v2/skill-bundles/"+parts[1]+"/revisions/"+parts[2], nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Reet-Token", token)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("private skill download: controller replied %s", resp.Status)
	}
	var revision store.PrivateSkillRevision
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 16<<20))
	if err = decoder.Decode(&revision); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid private skill response")
	}
	if revision.BundleID != parts[1] || revision.ID != parts[2] || revision.ManifestDigest != parts[3] {
		return errors.New("private skill revision changed")
	}
	manifest, files, err := skillimport.Select(revision.Files, nil)
	if err != nil {
		return err
	}
	if manifest.Digest != parts[3] {
		return errors.New("private skill checksum mismatch")
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
	if _, err = os.Lstat(destination); err == nil {
		return errors.New("local skill appeared during transfer; preview again")
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(staging, destination)
}
