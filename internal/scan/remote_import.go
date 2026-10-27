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
	"runtime"
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

type controllerHTTPError int

func (e controllerHTTPError) Error() string { return fmt.Sprintf("controller status %d", int(e)) }
func remoteImportPost(ctx context.Context, c agentConfig, path string, in, out any, version string) error {
	return controllerPost(ctx, c, "/api/v2/sync/skill-import/"+path, in, out, version, 16<<20)
}

// controllerPost is the shared versioned-channel transport: enrolment headers, no
// redirects (they would forward the credentials), an HTTPS or loopback origin and
// a bounded, strictly decoded response. A non-200 answer is a controllerHTTPError.
func controllerPost(ctx context.Context, c agentConfig, route string, in, out any, version string, limit int) error {
	if err := validateSkillImportController(c.Server); err != nil {
		return err
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if len(raw) > limit {
		return errors.New("controller transport limit exceeded")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.Server+route, bytes.NewReader(raw))
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
		return controllerHTTPError(response.StatusCode)
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return errors.New("controller response too large")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err = d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid controller response")
	}
	return nil
}
func processRemoteImport(ctx context.Context, c agentConfig, version string) (bool, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return false, nil
	}
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
	if err == controllerHTTPError(404) || err == controllerHTTPError(405) || err == controllerHTTPError(403) {
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
		if err == controllerHTTPError(409) || err == controllerHTTPError(404) {
			err = nil
		}
		return true, err
	}
	if work.Instance != instance {
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
		if err == controllerHTTPError(409) || err == controllerHTTPError(404) {
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
	// Recheck authorisation after local capture, before sending contents. This
	// catches cancellation/expiry during disk reads; the write transaction still
	// arbitrates a cancellation racing an already in-flight transfer.
	var current struct {
		SchemaVersion int                 `json:"schemaVersion"`
		Work          *store.RemoteImport `json:"work"`
	}
	if err = remoteImportPost(ctx, c, "work", map[string]any{"schemaVersion": skillimport.RemoteVersion, "instanceId": instance}, &current, version); err != nil {
		return true, err
	}
	if current.SchemaVersion != skillimport.RemoteVersion || current.Work == nil || current.Work.ID != work.ID || current.Work.State != "uploading" || current.Work.Digest != m.Digest {
		return true, nil
	}
	var result store.PrivateSkillResult
	err = remoteImportPost(ctx, c, "transfer", map[string]any{"id": work.ID, "instanceId": instance, "sourceDigest": work.Preview.SourceDigest, "files": selected}, &result, version)
	if err == controllerHTTPError(409) || err == controllerHTTPError(400) {
		return failure("transfer_failed")
	}
	// An ambiguous response is retried using the same controller-owned request,
	// never a new revision. Successful receipts are durable in the same transaction.
	return true, err
}
