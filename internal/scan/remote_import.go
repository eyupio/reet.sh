package scan

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
)

var importInstanceOnce sync.Once
var importInstance string

func remoteImportInstance() string {
	importInstanceOnce.Do(func() {
		var raw [24]byte
		if _, err := rand.Read(raw[:]); err == nil {
			importInstance = hex.EncodeToString(raw[:])
		}
	})
	return importInstance
}

type importHTTPError int

func (e importHTTPError) Error() string { return fmt.Sprintf("import controller status %d", int(e)) }
func remoteImportPost(ctx context.Context, c agentConfig, path string, in, out any, version string) error {
	if err := validateSkillImportController(c.Server); err != nil {
		return err
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if len(raw) > 16<<20 {
		return errors.New("import transport limit exceeded")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.Server+"/api/v2/sync/skill-import/"+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reet-Token", c.Token)
	req.Header.Set("X-Reet-Agent", c.ID)
	req.Header.Set("X-Reet-Agent-Key", c.Secret)
	req.Header.Set("X-Reet-Version", version)
	req.Header.Set("X-Reet-Commit", buildCommit)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return importHTTPError(response.StatusCode)
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return errors.New("import response too large")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err = d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid import response")
	}
	return nil
}
func processRemoteImport(ctx context.Context, c agentConfig, version string) (bool, error) {
	return processRemoteImportWith(ctx, c, version, remoteImportInstance())
}
func processRemoteImportWith(ctx context.Context, c agentConfig, version, instance string) (bool, error) {
	if !contentobs.ValidID(instance) {
		return false, errors.New("cannot create import instance")
	}
	var response struct {
		SchemaVersion int                 `json:"schemaVersion"`
		Work          *store.RemoteImport `json:"work"`
	}
	err := remoteImportPost(ctx, c, "work", map[string]any{"schemaVersion": skillimport.RemoteVersion, "instanceId": instance}, &response, version)
	if err == importHTTPError(404) || err == importHTTPError(405) || err == importHTTPError(403) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if response.SchemaVersion != skillimport.RemoteVersion || response.Work == nil {
		return false, nil
	}
	work := response.Work
	failure := func(code string) (bool, error) {
		var ack any
		err := remoteImportPost(ctx, c, "failure", map[string]string{"id": work.ID, "instanceId": instance, "code": code}, &ack, version)
		if err == importHTTPError(409) || err == importHTTPError(404) {
			err = nil
		}
		return true, err
	}
	if work.Instance != instance || work.ExpiresAt < time.Now().UnixMilli() {
		return true, nil
	}
	root, _, err := skillImportRoot(c, work.Agent, work.Project, work.Name)
	if err != nil {
		return failure("scope_unavailable")
	}
	if work.State == "previewing" {
		m, err := skillbundle.Scan(root, skillbundle.Limits{})
		if err != nil {
			return failure("capture_failed")
		}
		_, files, err := skillbundle.ReadContents(root, m.Digest)
		if err != nil {
			return failure("source_changed")
		}
		p := skillimport.Preview{SchemaVersion: skillimport.RemoteVersion, SourceDigest: m.Digest, SkillValid: false, Files: []skillimport.PreviewFile{}}
		byPath := map[string]skillbundle.ContentFile{}
		for _, f := range files {
			byPath[f.Path] = f
			if f.Path == "SKILL.md" {
				h := store.HostedSkill{Slug: work.Name, Content: string(f.Data)}
				p.SkillValid = store.ValidateHostedSkill(&h) == nil
			}
		}
		for _, f := range m.Files {
			p.Files = append(p.Files, skillimport.PreviewFile{File: f, Blocked: skillimport.BlockedReason(byPath[f.Path])})
		}
		var ack any
		err = remoteImportPost(ctx, c, "preview", map[string]any{"id": work.ID, "instanceId": instance, "preview": p}, &ack, version)
		if err == importHTTPError(409) || err == importHTTPError(404) {
			err = nil
		}
		return true, err
	}
	if work.State != "uploading" || work.Preview == nil || !work.Preview.Allowed(work.Excluded) {
		return failure("transfer_failed")
	}
	_, files, err := skillbundle.ReadContents(root, work.Preview.SourceDigest)
	if err != nil {
		return failure("source_changed")
	}
	m, selected, err := skillimport.Select(files, work.Excluded)
	if err != nil || m.Digest != work.Digest {
		return failure("source_changed")
	}
	var result store.PrivateSkillResult
	err = remoteImportPost(ctx, c, "transfer", map[string]any{"id": work.ID, "instanceId": instance, "sourceDigest": work.Preview.SourceDigest, "files": selected}, &result, version)
	if err == importHTTPError(409) || err == importHTTPError(400) {
		return failure("transfer_failed")
	}
	// An ambiguous response is retried using the same controller-owned request,
	// never a new revision. Successful receipts are durable in the same transaction.
	return true, err
}
