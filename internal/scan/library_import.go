package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
)

// CollectLibrarySkillLocations resolves machine-local read roots. It does not
// translate them into destination paths or broaden the legacy apply adapters.
// A configuration error returns the valid locations as well; callers must keep
// processing them so one malformed app override cannot block unrelated roots.
func CollectLibrarySkillLocations(home string, projects, custom []string, application string, getenv func(string) string) ([]store.LibraryLocation, error) {
	if application != "shared" && application != "claude" && application != "codex" && application != "opencode" && application != "agy" {
		return nil, errors.New("unsupported application for custom skill folders")
	}
	var out []store.LibraryLocation
	var configurationErrors []error
	seen := map[string]bool{}
	add := func(app, project, root string) error {
		if !filepath.IsAbs(root) || (project != "" && !filepath.IsAbs(project)) {
			configurationErrors = append(configurationErrors, errors.New("skill folders and projects must use absolute paths"))
			return nil
		}
		root = filepath.Clean(root)
		key := app + "\x00" + project + "\x00" + root
		if seen[key] {
			return nil
		}
		seen[key] = true
		sum := sha256.Sum256([]byte(key))
		out = append(out, store.LibraryLocation{ID: hex.EncodeToString(sum[:16]), Application: app, Project: project, Path: root})
		return nil
	}
	configured := func(variable, fallback string) string {
		if value := getenv(variable); value != "" {
			return value
		}
		return filepath.Join(home, fallback)
	}
	claude := configured("CLAUDE_CONFIG_DIR", ".claude")
	codex := configured("CODEX_HOME", ".codex")
	xdg := configured("XDG_CONFIG_HOME", ".config")
	opencode := getenv("OPENCODE_CONFIG_DIR")
	if opencode == "" {
		opencode = filepath.Join(xdg, "opencode")
	}
	for _, root := range []struct{ app, path string }{
		{"shared", filepath.Join(home, ".agents", "skills")},
		{"claude", filepath.Join(claude, "skills")},
		{"codex", filepath.Join(codex, "skills")},
		{"opencode", filepath.Join(opencode, "skills")},
	} {
		if err := add(root.app, "", root.path); err != nil {
			return nil, err
		}
	}
	for _, project := range projects {
		for _, root := range []struct{ app, dir string }{{"shared", ".agents"}, {"claude", ".claude"}, {"codex", ".codex"}, {"opencode", ".opencode"}} {
			if err := add(root.app, project, filepath.Join(project, root.dir, "skills")); err != nil {
				return nil, err
			}
		}
	}
	for _, root := range custom {
		if err := add(application, "", root); err != nil {
			return nil, err
		}
	}
	for i := range out {
		location := &out[i]
		if location.Project != "" {
			if location.Application == "shared" {
				location.LegacyAgent = "agents"
			} else if location.Application == "claude" || location.Application == "codex" {
				location.LegacyAgent = location.Application
			}
			continue
		}
		defaults := map[string]struct{ agent, dir string }{"shared": {"agents", ".agents/skills"}, "claude": {"claude", ".claude/skills"}, "codex": {"codex", ".codex/skills"}, "opencode": {"opencode", ".config/opencode/skills"}}
		if expected, ok := defaults[location.Application]; ok && location.Path == filepath.Join(home, filepath.FromSlash(expected.dir)) {
			location.LegacyAgent = expected.agent
		}
	}
	if len(out) > 64 {
		configurationErrors = append(configurationErrors, errors.New("at most 64 skill folders can be imported at once"))
		out = out[:64]
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, errors.Join(configurationErrors...)
}

type libraryImportSummary struct {
	Imported  int                  `json:"imported"`
	Unchanged int                  `json:"unchanged"`
	Issues    []libraryImportIssue `json:"issues"`
}
type libraryImportIssue struct {
	LocationID string `json:"locationId"`
	Name       string `json:"name,omitempty"`
	Code       string `json:"code"`
}

func runLibraryImport(args []string, out io.Writer, version string) error {
	fs := flag.NewFlagSet("agent library-import", flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	app := fs.String("application", "shared", "application for explicitly added skill folders")
	var folders multi
	fs.Var(&folders, "folder", "absolute skill directory to import (repeatable; also imports known shared/project folders)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	c, err := loadAgentConfig(*config)
	if err != nil {
		return err
	}
	locations, configurationErr := CollectLibrarySkillLocations(c.Home, c.Projects, folders, *app, os.Getenv)
	if configurationErr != nil && len(locations) == 0 {
		return configurationErr
	}
	result := importLibraryLocations(context.Background(), c, locations, folders, version)
	if configurationErr != nil {
		result.Issues = append(result.Issues, libraryImportIssue{Code: "location_configuration_invalid"})
	}
	if err := json.NewEncoder(out).Encode(result); err != nil {
		return err
	}
	if len(result.Issues) > 0 {
		return errors.New("some library content could not be captured; complete imports were retained")
	}
	return nil
}

func importLibraryLocations(ctx context.Context, c agentConfig, locations []store.LibraryLocation, explicit []string, version string) libraryImportSummary {
	result := libraryImportSummary{Issues: []libraryImportIssue{}}
	for _, location := range locations {
		issue := func(name, code string) {
			result.Issues = append(result.Issues, libraryImportIssue{location.ID, name, code})
		}
		before, err := os.Lstat(location.Path)
		if os.IsNotExist(err) {
			for _, root := range explicit {
				if filepath.Clean(root) == location.Path {
					issue("", "folder_missing")
					break
				}
			}
			continue
		}
		if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
			issue("", "folder_unavailable")
			continue
		}
		var prepared struct {
			SchemaVersion int                          `json:"schemaVersion"`
			Items         []store.LibraryLocationEntry `json:"items"`
		}
		// Negotiate and authenticate the registered root before reading contents.
		if err = agentRequestContext(ctx, c, "/api/v2/library/locations", location, &prepared, version); err != nil || prepared.SchemaVersion != 1 {
			issue("", "location_unavailable")
			continue
		}
		known := map[string]store.LibraryLocationEntry{}
		for _, entry := range prepared.Items {
			known[entry.Name] = entry
		}
		root, err := os.OpenRoot(location.Path)
		if err != nil {
			issue("", "folder_unavailable")
			continue
		}
		dir, err := root.Open(".")
		if err != nil {
			root.Close()
			issue("", "folder_unavailable")
			continue
		}
		entries, err := dir.ReadDir(1025)
		dir.Close()
		root.Close()
		if err != nil && err != io.EOF || len(entries) > 1024 {
			issue("", "folder_incomplete")
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 {
				issue(name, "symlink")
				continue
			}
			if !entry.IsDir() {
				continue
			}
			if !store.ValidHostedName(name) {
				issue(name, "invalid_name")
				continue
			}
			path := filepath.Join(location.Path, name)
			if _, err := os.Lstat(filepath.Join(path, "SKILL.md")); err != nil {
				if !os.IsNotExist(err) {
					issue(name, "capture_incomplete")
				}
				continue
			}
			manifest, err := skillbundle.Scan(path, skillbundle.Limits{})
			if err != nil {
				issue(name, "capture_incomplete")
				continue
			}
			prior := known[name]
			if prior.Digest == manifest.Digest {
				result.Unchanged++
				continue
			}
			_, files, err := skillbundle.ReadContents(path, manifest.Digest)
			if err != nil {
				issue(name, "capture_changed")
				continue
			}
			if _, _, err = skillimport.Select(files, nil); err != nil {
				issue(name, "blocked_content")
				continue
			}
			after, err := os.Lstat(location.Path)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				issue(name, "folder_changed")
				continue
			}
			sum := sha256.Sum256([]byte(c.ID + "\x00" + location.ID + "\x00" + name + "\x00" + prior.RevisionID + "\x00" + manifest.Digest))
			request := store.LibraryLocationImport{LocationID: location.ID, Name: name, RequestID: "root_" + hex.EncodeToString(sum[:]), ExpectedBase: &prior.RevisionID, Files: files}
			var receipt store.PrivateSkillResult
			if err = agentRequestContext(ctx, c, "/api/v2/library/locations/content", request, &receipt, version); err != nil {
				issue(name, "import_failed")
				continue
			}
			if receipt.Revision.ManifestDigest != manifest.Digest {
				issue(name, "receipt_mismatch")
				continue
			}
			result.Imported++
		}
	}
	return result
}
