package scan

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
)

func skillImportRoot(c agentConfig, agent, project, name string) (string, string, error) {
	if !contentobs.ValidScope(agent, project) || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || !contentobs.ValidText(name, 255) {
		return "", "", errors.New("choose a supported agent and one skill folder name")
	}
	if project == "" {
		for _, d := range homeDirs {
			if d.Agent == agent {
				return filepath.Join(c.Home, d.Dir, name), "", nil
			}
		}
	}
	if project != "" {
		canonical, err := filepath.EvalSymlinks(project)
		if err != nil {
			return "", "", errors.New("project unavailable")
		}
		canonical, err = filepath.Abs(canonical)
		if err != nil {
			return "", "", err
		}
		allowed := false
		for _, p := range c.Projects {
			if sameDirectory(p, canonical) {
				allowed = true
			}
		}
		if !allowed {
			return "", "", errors.New("project is not enrolled locally")
		}
		for _, d := range projectDirs {
			if d.Agent == agent {
				return filepath.Join(canonical, d.Dir, name), canonical, nil
			}
		}
	}
	return "", "", errors.New("this agent scope is not supported for import")
}
func validateSkillImportController(server string) error {
	u, err := url.Parse(server)
	if err != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return errors.New("skill import requires a controller HTTPS origin (or local loopback HTTP)")
	}
	return nil
}
func skillImportRequest(ctx context.Context, c agentConfig, method string, body, result any, version string) error {
	if err := validateSkillImportController(c.Server); err != nil {
		return err
	}
	var err error
	var data []byte
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
		if len(data) > 16<<20 {
			return errors.New("skill import request too large")
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Server, "/")+"/api/v2/sync/skill-import", bytes.NewReader(data))
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
		return errors.New("controller does not support machine skill import; update it before uploading")
	}
	if resp.StatusCode != 200 {
		message := "upload was not acknowledged; keep the original request ID when resolving or retrying it"
		switch resp.StatusCode {
		case 400:
			message = "invalid skill contents or import fields"
		case 401, 403:
			message = "an active write-role account and its owned machine enrollment are required"
		case 409:
			message = "check the machine pause state, reported scope, expected bundle revision and original request ID"
		}
		return fmt.Errorf("skill import controller status %d: %s", resp.StatusCode, message)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return errors.New("oversized import response")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err = d.Decode(result); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid skill import reply")
	}
	return nil
}
func runAgentSkillImport(args []string, in io.Reader, out io.Writer, version string) error {
	if len(args) == 0 || args[0] != "import" {
		return errors.New("usage: reet agent skill import --agent agents --name NAME [--project DIR] [--exclude FILE] [--upload] [--yes] [--request-id ID] [--bundle ID --expected-base REV]")
	}
	fs := flag.NewFlagSet("agent skill import", flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private enrollment configuration")
	agent := fs.String("agent", "agents", "installed agent scope")
	name := fs.String("name", "", "skill folder name (must match SKILL.md)")
	project := fs.String("project", "", "enrolled project directory; omit for home scope")
	upload := fs.Bool("upload", false, "upload after preview and explicit confirmation; default is local preview only")
	yes := fs.Bool("yes", false, "confirm the displayed upload for automation; requires --upload")
	requestID := fs.String("request-id", "", "reuse this ID after an uncertain upload response")
	bundle := fs.String("bundle", "", "append to this owned private bundle; default creates a new private copy")
	base := fs.String("expected-base", "", "current private revision when appending")
	var excludes multi
	fs.Var(&excludes, "exclude", "exact relative file path to exclude; repeatable, SKILL.md required")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || (*yes && !*upload) || (*bundle != "" && *base == "") || (*bundle == "" && *base != "") {
		return errors.New("invalid import arguments; append needs --bundle and --expected-base, confirmation needs --upload")
	}
	if *requestID != "" && !regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`).MatchString(*requestID) {
		return errors.New("request ID must contain 16–80 ASCII letters, digits, underscores or hyphens")
	}
	c, err := loadAgentConfig(*config)
	if err != nil {
		return err
	}
	if err = validateSkillImportController(c.Server); err != nil {
		return err
	}
	root, scope, err := skillImportRoot(c, *agent, *project, *name)
	if err != nil {
		return err
	}
	source, err := skillbundle.Scan(root, skillbundle.Limits{})
	if err != nil {
		return errors.New("skill capture is incomplete or unsafe; resolve local scan issues before importing")
	}
	_, files, err := skillbundle.ReadContents(root, source.Digest)
	if err != nil {
		return errors.New("skill changed while reading; preview again")
	}
	manifest, selected, err := skillimport.Select(files, excludes)
	if err != nil {
		return err
	}
	// Match the storage validation now, before prompting or sending any payload.
	for _, f := range selected {
		if f.Path == "SKILL.md" {
			h := store.HostedSkill{Slug: *name, Content: string(f.Data)}
			if store.ValidateHostedSkill(&h) != nil {
				return errors.New("SKILL.md needs valid matching name, description and instructions")
			}
		}
	}
	fmt.Fprintf(out, "Private skill import preview\nMachine: %q\nAgent: %s\nProject: %q\nSkill: %s\nController: %s\n", c.Host, *agent, scope, *name, c.Server)
	if *bundle == "" {
		fmt.Fprintln(out, "Destination: new private bundle")
	} else {
		fmt.Fprintf(out, "Destination: bundle %s at base %s\n", *bundle, *base)
	}
	for _, f := range manifest.Files {
		fmt.Fprintf(out, "  %s · %d bytes · executable=%t\n", f.Path, f.Bytes, f.Executable)
	}
	for _, v := range excludes {
		fmt.Fprintf(out, "  Excluded: %s\n", v)
	}
	fmt.Fprintf(out, "%d files · %d bytes\nReviewed digest: %s\nNo publication, installation or shell execution.\n", len(manifest.Files), manifest.TotalBytes, manifest.Digest)
	if !*upload {
		fmt.Fprintln(out, "Preview only. Review the files, then repeat with --upload to import privately.")
		return nil
	}
	// Probe the typed endpoint before payload dispatch. An older controller never
	// receives raw file bytes as a speculative request to an unknown route.
	var capability struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err = skillImportRequest(context.Background(), c, "GET", nil, &capability, version); err != nil {
		return err
	}
	if capability.SchemaVersion != 1 {
		return errors.New("unsupported machine skill import schema")
	}
	if !*yes {
		fmt.Fprint(out, "Type IMPORT to upload these selected files privately, or Enter to cancel: ")
		answer, readErr := bufio.NewReader(io.LimitReader(in, 1024)).ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if strings.TrimSpace(answer) != "IMPORT" {
			fmt.Fprintln(out, "Import cancelled. No files uploaded.")
			return nil
		}
	}
	// The displayed file set cannot change while the operator reviews it.
	_, files, err = skillbundle.ReadContents(root, source.Digest)
	if err != nil {
		return errors.New("skill changed after preview; no upload was sent")
	}
	current, selected, err := skillimport.Select(files, excludes)
	if err != nil {
		return err
	}
	if current.Digest != manifest.Digest {
		return errors.New("selected contents changed; preview again")
	}
	if *requestID == "" {
		var raw [24]byte
		if _, err = rand.Read(raw[:]); err != nil {
			return err
		}
		*requestID = hex.EncodeToString(raw[:])
	}
	fmt.Fprintf(out, "Upload request ID: %s\nIf the response is lost, repeat unchanged with --request-id %s.\n", *requestID, *requestID)
	body := store.MachineSkillImport{SchemaVersion: 1, RequestID: *requestID, BundleID: *bundle, ExpectedBase: base, Agent: *agent, Project: scope, Name: *name, Digest: manifest.Digest, Files: selected}
	var result store.PrivateSkillResult
	if err = skillImportRequest(context.Background(), c, "POST", body, &result, version); err != nil {
		return err
	}
	if result.Revision.ManifestDigest != manifest.Digest || result.Bundle.Head != result.Revision.ID || result.Bundle.ID == "" || result.Revision.ID == "" {
		return errors.New("invalid import receipt; resolve the original request ID before another import")
	}
	fmt.Fprintf(out, "Imported privately. Bundle: %s\nRevision: %s\n", result.Bundle.ID, result.Revision.ID)
	return nil
}
