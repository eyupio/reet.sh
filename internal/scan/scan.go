// Package scan finds locally installed agent skills so they can be imported as "used".
package scan

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Found is one installed skill in one place; Source is "owner/repo" when the skills CLI lock file knows it.
type Found struct {
	Name, Source, Host, Agent, Project, Path string
	Digest                                   string
}

// ScannedEnv is one environment a scan looked at, including folders with no skills.
type ScannedEnv struct {
	Host    string   `json:"host"`
	Agent   string   `json:"agent"`
	Project string   `json:"project"`
	Path    string   `json:"path"`
	Skills  []string `json:"skills"`
	Errors  []string `json:"errors,omitempty"`
}

// Scan is everything a run found and every environment it looked at.
type Scan struct {
	Found []Found
	Envs  []ScannedEnv
}

var homeDirs = []struct{ Dir, Agent string }{
	{".claude/skills", "claude"},
	{".agents/skills", "agents"},
	{".cursor/skills", "cursor"},
	{".codex/skills", "codex"},
	{".config/opencode/skills", "opencode"},
}

var projectDirs = []struct{ Dir, Agent string }{
	{".claude/skills", "claude"},
	{".agents/skills", "agents"},
}

// CollectScan gathers installed skills and the environments they live in.
func CollectScan(home, cwd string, extraDirs []string, host string) Scan {

	sources, globalSources, projectSources := map[string]string{}, map[string]string{}, map[string]string{}
	readLock(filepath.Join(home, ".agents", ".skill-lock.json"), func(name, source string) {
		if source != "" {
			globalSources[name] = source
			sources[name] = source
		}
	})
	readLock(filepath.Join(cwd, "skills-lock.json"), func(name, source string) {
		if source != "" {
			projectSources[name] = source
			sources[name] = source
		}
	})
	found := map[string]Found{}
	envs := map[string]*ScannedEnv{}
	addEnv := func(agent, project, path string) *ScannedEnv {
		key := host + "\x00" + agent + "\x00" + project
		if env := envs[key]; env != nil {
			return env
		}
		env := &ScannedEnv{Host: host, Agent: agent, Project: project, Path: path}
		envs[key] = env
		return env
	}
	read := func(dir, agent, project, label string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// A missing skills folder under an existing agent configuration is an
			// empty inventory, not evidence that its old skills are still installed.
			if os.IsNotExist(err) {
				if parent, e := os.Stat(filepath.Dir(dir)); e == nil && parent.IsDir() {
					addEnv(agent, project, label)
				}
			}
			if !os.IsNotExist(err) {
				addEnv(agent, project, label).Errors = append(addEnv(agent, project, label).Errors, err.Error())
			}
			return
		}
		scopeSources := map[string]string{}
		for _, lock := range []string{filepath.Join(filepath.Dir(dir), ".skill-lock.json"), filepath.Join(filepath.Dir(dir), "skills-lock.json"), filepath.Join(dir, ".skill-lock.json")} {
			readLock(lock, func(name, source string) {
				if source != "" {
					scopeSources[name] = source
				}
			})
		}
		env := addEnv(agent, project, label)
		seen := map[string]bool{}
		for _, e := range entries {
			if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
				continue
			}
			st, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md"))
			if err != nil {
				if !os.IsNotExist(err) || e.Type()&os.ModeSymlink != 0 {
					env.Errors = append(env.Errors, fmt.Sprintf("%s: %v", e.Name(), err))
				}
				continue
			}
			if !st.IsDir() {
				name := e.Name()
				source := sourceForScope(globalSources, projectSources, project, name)
				if scoped := scopeSources[name]; scoped != "" {
					source = scoped
				}
				found[host+"\x00"+agent+"\x00"+project+"\x00"+name] = Found{
					Name: name, Source: source, Host: host, Agent: agent, Project: project, Path: label,
				}
				if !seen[name] {
					seen[name] = true
					env.Skills = append(env.Skills, name)
				}
			}
		}
	}
	for _, d := range homeDirs {
		read(filepath.Join(home, d.Dir), d.Agent, "", d.Dir)
	}
	project := filepath.Clean(cwd)
	if resolved, err := filepath.EvalSymlinks(project); err == nil {
		project = resolved
	}
	for _, d := range projectDirs {
		dir := filepath.Join(cwd, d.Dir)
		// Running from the home directory, or from a project whose agent folder
		// is linked to the home one, describes the same installs that the global
		// scan already reported. Treating them as a project duplicates every
		// skill and loses the lock-file source because the project lock is empty.
		if sameDirectory(dir, filepath.Join(home, d.Dir)) {
			continue
		}
		read(dir, d.Agent, project, filepath.Join(project, d.Dir))
	}
	for _, dir := range extraDirs {
		label := filepath.Base(dir)
		if label == "." || label == string(filepath.Separator) {
			label = "extra"
		}
		read(dir, "other", "", label)
	}
	anyEnv := map[string]bool{}
	for _, f := range found {
		anyEnv[f.Name] = true
	}
	for name, source := range sources {
		if !anyEnv[name] {
			found["\x00"+name] = Found{Name: name, Source: source}
		}
	}
	out := Scan{}
	for _, f := range found {
		out.Found = append(out.Found, f)
	}
	sort.Slice(out.Found, func(i, j int) bool {
		if out.Found[i].Name != out.Found[j].Name {
			return out.Found[i].Name < out.Found[j].Name
		}
		return out.Found[i].Agent < out.Found[j].Agent
	})
	for _, env := range envs {
		sort.Strings(env.Skills)
		out.Envs = append(out.Envs, *env)
	}
	sort.Slice(out.Envs, func(i, j int) bool {
		if out.Envs[i].Agent != out.Envs[j].Agent {
			return out.Envs[i].Agent < out.Envs[j].Agent
		}
		return out.Envs[i].Project < out.Envs[j].Project
	})
	return out
}

// Collect merges the scan into one row per name for callers that only need the list.
func Collect(home, cwd string, extraDirs []string) []Found {
	scan := CollectScan(home, cwd, extraDirs, "")
	out := []Found{}
	seen := map[string]bool{}
	for _, f := range scan.Found {
		if seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func readLock(path string, add func(name, source string)) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var v struct {
		Skills json.RawMessage `json:"skills"`
	}
	if json.Unmarshal(b, &v) != nil {
		return
	}
	var asMap map[string]struct {
		Source        string `json:"source"`
		SourceType    string `json:"sourceType"`
		SourceBaseURL string `json:"sourceBaseUrl"`
		SourceURL     string `json:"sourceUrl"`
		SkillPath     string `json:"skillPath"`
	}
	if json.Unmarshal(v.Skills, &asMap) == nil {
		for key, e := range asMap {
			name := lockSkillName(key, e.SkillPath)
			source := e.Source
			if source == "" && e.SourceType == "github" {
				source = e.SourceURL
			}
			if e.SourceType == "well-known" {
				base := e.SourceBaseURL
				if base == "" {
					base = e.SourceURL
				}
				if u, err := url.Parse(base); err == nil && u.Host != "" && u.User == nil {
					parts := strings.Split(strings.Trim(u.Path, "/"), "/")
					if len(parts) == 2 && (parts[1] == name || parts[1] == key) {
						source = u.Scheme + "://" + u.Host + "/" + parts[0]
					}
				}
			}
			add(name, source)
		}
		return
	}
	var asList []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	if json.Unmarshal(v.Skills, &asList) == nil {
		for _, e := range asList {
			add(e.Name, e.Source)
		}
	}
}

// lockSkillName prefers the installed folder name from the lock entry's skillPath.
// The skills CLI may key entries by a display name (for example "SVG Logo
// Designer"), while scans see the folder name ("svg-logo-designer"); matching on
// the folder keeps sources attached and avoids phantom lock-only skills.
func lockSkillName(key, skillPath string) string {
	if skillPath == "" {
		return key
	}
	base := path.Base(path.Dir(strings.ReplaceAll(skillPath, "\\", "/")))
	if base == "." || base == "/" || base == "" {
		return key
	}
	return base
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(s string) error { *m = append(*m, s); return nil }

// Run implements `reet scan`.
func Run(args []string, out io.Writer) error { return RunVersion(args, out, "dev") }

func RunVersion(args []string, out io.Writer, version string) error {
	return runVersionAt("", args, out, version)
}

// runVersionAt scans an explicit directory instead of the process working
// directory. Background collectors must never change global process state.
func runVersionAt(dir string, args []string, out io.Writer, version string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(out)
	def := os.Getenv("REET_URL")
	server := fs.String("server", def, "reet server URL (env REET_URL)")
	token := fs.String("token", "", "access token (env REET_TOKEN)")
	tokenEnvFile := fs.String("env-file", "", "read REET_TOKEN from an explicitly selected .env file; flags/exported token win")
	dry := fs.Bool("dry-run", false, "list what would be imported without sending it")
	reconcile := fs.Bool("reconcile", false, "repair hostname/--env aliases before refreshing inventories; never changes installed skills")
	prune := fs.Bool("prune", false, "remove stale inventory reports for scanned environments; keeps installed files and shelf entries")
	serverHost := fs.Bool("server-host", false, "register this process as a reet server")
	envName := fs.String("env", os.Getenv("REET_ENV"), "environment name for this machine (default hostname)")
	var dirs multi
	fs.Var(&dirs, "dir", "extra skills folder to scan (repeatable)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments; use --help")
	}
	r, err := RunReport(ReportOptions{
		Server:      *server,
		Token:       *token,
		TokenFile:   *tokenEnvFile,
		Host:        *envName,
		Dir:         dir,
		Dirs:        dirs,
		Prune:       *prune,
		Reconcile:   *reconcile,
		ServerHost:  *serverHost,
		DryRun:      *dry,
		Version:     version,
		Interactive: true,
		In:          os.Stdin,
		Out:         os.Stderr,
		OnScan:      func(found Scan) { RenderFindings(out, found.Found) },
		OnReconciled: func(from, to string) {
			fmt.Fprintf(out, "Reet, hostname %s now reports as %s. Refreshing inventories.\n", from, to)
		},
	})
	if err == nil || r.DryRun || r.Sent {
		RenderResult(out, r)
	}
	return err
}

func sourceForScope(global, project map[string]string, scope, name string) string {
	if scope != "" {
		return project[name]
	}
	return global[name]
}

// sameDirectory reports whether two paths point at the same folder, including
// through symlinks. Missing paths still compare by their cleaned absolute name.
func sameDirectory(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	if filepath.Clean(absA) == filepath.Clean(absB) {
		return true
	}
	resolvedA, errA := filepath.EvalSymlinks(absA)
	resolvedB, errB := filepath.EvalSymlinks(absB)
	return errA == nil && errB == nil && filepath.Clean(resolvedA) == filepath.Clean(resolvedB)
}

// Err reports incomplete scopes without converting unreadable files to absence.
func (s Scan) Err() error {
	var problems []string
	for _, env := range s.Envs {
		for _, problem := range env.Errors {
			problems = append(problems, env.Path+": "+problem)
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
