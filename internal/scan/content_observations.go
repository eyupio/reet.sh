package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
)

var errObservationUnsupported = errors.New("content observations unsupported")

func observationRequest(ctx context.Context, c agentConfig, path string, body, result any, version string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if len(data) > contentobs.MaxBody {
		return contentobs.ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.Server+path, bytes.NewReader(data))
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
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 || resp.StatusCode == 405 {
		return errObservationUnsupported
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("content observation controller status %d", resp.StatusCode)
	}
	if result == nil {
		return nil
	}
	d := json.NewDecoder(io.LimitReader(resp.Body, 16<<10))
	if err = d.Decode(result); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return contentobs.ErrInvalid
	}
	return nil
}

func reportContentObservations(ctx context.Context, c agentConfig, version string) error {
	return reportContentObservationsWith(ctx, c, version, collectContentObservations)
}
func reportContentObservationsWith(ctx context.Context, c agentConfig, version string, collect func(context.Context, agentConfig, contentobs.Grant) contentobs.Report) error {
	var grant contentobs.Grant
	err := observationRequest(ctx, c, "/api/v2/sync/content-observations/prepare", struct{}{}, &grant, version)
	if errors.Is(err, errObservationUnsupported) {
		return nil
	}
	if err != nil {
		return err
	}
	// Negotiate before reading content. Old and future controllers do not trigger hashing.
	if grant.SchemaVersion != contentobs.Version {
		return nil
	}
	if !contentobs.ValidID(grant.ScanID) || grant.ExpiresAt <= time.Now().UnixMilli() {
		return contentobs.ErrInvalid
	}
	report := collect(ctx, c, grant)
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = report.Validate(); err != nil {
		return err
	}
	return observationRequest(ctx, c, "/api/v2/sync/content-observations", report, nil, version)
}

func collectContentObservations(ctx context.Context, c agentConfig, g contentobs.Grant) contentobs.Report {
	out := contentobs.Report{SchemaVersion: contentobs.Version, ScanID: g.ScanID, ObservedAt: time.Now().UnixMilli(), Scopes: []contentobs.Scope{}}
	seen := map[string]bool{}
	count := 0
	remaining := contentobs.MaxScanBytes
	for _, dir := range append([]string{c.Home}, c.Projects...) {
		if ctx.Err() != nil {
			break
		}
		scan := CollectScan(c.Home, dir, nil, c.Host)
		for _, env := range scan.Envs {
			key := env.Agent + "\x00" + env.Project
			if seen[key] || !contentobs.ValidScope(env.Agent, env.Project) {
				continue
			}
			seen[key] = true
			if len(out.Scopes) >= contentobs.MaxScopes {
				return out
			}
			// Resolve only the collector's known local roots, never controller input.
			root := ""
			if env.Project == "" {
				for _, d := range homeDirs {
					if d.Agent == env.Agent {
						root = filepath.Join(c.Home, d.Dir)
					}
				}
			} else {
				for _, d := range projectDirs {
					if d.Agent == env.Agent {
						root = filepath.Join(dir, d.Dir)
					}
				}
			}
			if root == "" {
				continue
			}
			scope := contentobs.Scope{Agent: env.Agent, Project: env.Project, Complete: len(env.Errors) == 0, Skills: []contentobs.Skill{}}
			for _, name := range env.Skills {
				if count >= contentobs.MaxSkills {
					scope.Complete = false
					break
				}
				count++
				v := contentobs.Skill{Name: name, SchemaVersion: skillbundle.SchemaVersion, Algorithm: skillbundle.Algorithm, ModeSemantics: skillbundle.ModeSemantics}
				if ctx.Err() != nil || remaining <= 1 {
					v.Issues = []string{"scan_limit"}
					scope.Complete = false
				} else {
					// Failed scans may read bytes excluded from the manifest. Reserve their
					// full allowance (plus the overflow probe); refund only complete scans.
					allowance := min(skillbundle.MaxBytes, remaining-1)
					remaining -= allowance + 1
					m, err := skillbundle.Scan(filepath.Join(root, name), skillbundle.Limits{TotalBytes: allowance, FileBytes: allowance})
					v.Complete = m.Complete
					v.Digest = m.Digest
					v.Files = len(m.Files)
					v.Bytes = m.TotalBytes
					if err == nil {
						remaining += allowance + 1 - m.TotalBytes
					} else {
						scope.Complete = false
					}
					codes := map[string]bool{}
					for _, issue := range m.Issues {
						if !codes[issue.Code] && len(v.Issues) < 8 {
							v.Issues = append(v.Issues, issue.Code)
							codes[issue.Code] = true
						}
					}
				}
				scope.Skills = append(scope.Skills, v)
			}
			out.Scopes = append(out.Scopes, scope)
		}
	}
	return out
}
