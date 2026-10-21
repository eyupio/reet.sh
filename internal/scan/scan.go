// Package scan finds locally installed agent skills so they can be imported as "used".
package scan

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Found is one installed skill in one place; Source is "owner/repo" when the skills CLI lock file knows it.
type Found struct {
	Name, Source, Host, Agent, Project, Path string
}

// ScannedEnv is one environment a scan looked at, including folders with no skills.
type ScannedEnv struct {
	Host    string   `json:"host"`
	Agent   string   `json:"agent"`
	Project string   `json:"project"`
	Path    string   `json:"path"`
	Skills  []string `json:"skills"`
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
	sources := map[string]string{}
	for _, lock := range []string{filepath.Join(home, ".agents", ".skill-lock.json"), filepath.Join(cwd, "skills-lock.json")} {
		readLock(lock, func(name, source string) {
			if source != "" {
				sources[name] = source
			}
		})
	}
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
			return
		}
		env := addEnv(agent, project, label)
		seen := map[string]bool{}
		for _, e := range entries {
			if st, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err == nil && !st.IsDir() {
				name := e.Name()
				found[host+"\x00"+agent+"\x00"+project+"\x00"+name] = Found{
					Name: name, Source: sources[name], Host: host, Agent: agent, Project: project, Path: label,
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
	project := filepath.Base(cwd)
	for _, d := range projectDirs {
		read(filepath.Join(cwd, d.Dir), d.Agent, project, filepath.Join(project, d.Dir))
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
		Source string `json:"source"`
	}
	if json.Unmarshal(v.Skills, &asMap) == nil {
		for name, e := range asMap {
			add(name, e.Source)
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

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(s string) error { *m = append(*m, s); return nil }

// Run implements `reet scan`.
func Run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(out)
	def := os.Getenv("REET_URL")
	if def == "" {
		def = "http://localhost:8080"
	}
	server := fs.String("server", def, "reet server URL (env REET_URL)")
	token := fs.String("token", os.Getenv("REET_TOKEN"), "access token (env REET_TOKEN)")
	dry := fs.Bool("dry-run", false, "list what would be imported without sending it")
	prune := fs.Bool("prune", false, "remove skills no longer found in each scanned environment")
	envName := fs.String("env", "", "environment name for this machine (default hostname)")
	var dirs multi
	fs.Var(&dirs, "dir", "extra skills folder to scan (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	host := strings.TrimSpace(*envName)
	if host == "" {
		host, _ = os.Hostname()
	}
	if host == "" {
		host = "unknown"
	}
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	scan := CollectScan(home, cwd, dirs, host)
	if len(scan.Found) == 0 && !*dry && !*prune {
		fmt.Fprintln(out, "no installed skills found")
		return nil
	}
	for _, f := range scan.Found {
		src := f.Source
		if src == "" {
			src = "(unknown source)"
		}
		place := f.Agent
		if f.Project != "" {
			place += " · " + f.Project
		}
		if place == "" {
			place = "(lock file)"
		}
		fmt.Fprintf(out, "%-40s %-26s %s\n", f.Name, src, place)
	}
	if *dry {
		fmt.Fprintf(out, "%d skills found in %d environments (dry run, nothing sent)\n", len(scan.Found), len(scan.Envs))
		return nil
	}
	type item struct {
		Name    string `json:"name"`
		Source  string `json:"source,omitempty"`
		Host    string `json:"host,omitempty"`
		Agent   string `json:"agent,omitempty"`
		Project string `json:"project,omitempty"`
		Path    string `json:"path,omitempty"`
	}
	items := make([]item, len(scan.Found))
	for i, f := range scan.Found {
		items[i] = item{f.Name, f.Source, f.Host, f.Agent, f.Project, f.Path}
	}
	body, _ := json.Marshal(map[string]any{"items": items, "environments": scan.Envs, "prune": *prune})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(*server, "/")+"/api/import", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reet-Source", "scan")
	if *token != "" {
		req.Header.Set("X-Reet-Token", *token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server replied %s", resp.Status)
	}
	var res struct {
		Imported int      `json:"imported"`
		Created  int      `json:"created"`
		Ignored  int      `json:"ignored"`
		Pruned   int      `json:"pruned"`
		Errors   []string `json:"errors"`
		User     string   `json:"user"`
	}
	if json.NewDecoder(resp.Body).Decode(&res) != nil {
		return errors.New("unreadable reply from server")
	}
	fmt.Fprintf(out, "%d new, %d ignored, %d pruned on %s\n", res.Created, res.Ignored, res.Pruned, host)
	if res.User != "" {
		fmt.Fprintf(out, "reported as %s\n", res.User)
	}
	fmt.Fprintf(out, "imported %d skills as used\n", res.Imported)
	for _, e := range res.Errors {
		fmt.Fprintf(out, "  skipped: %s\n", e)
	}
	return nil
}
