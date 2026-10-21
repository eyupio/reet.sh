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

// Found is one installed skill; Source is "owner/repo" when the skills CLI lock file knows it.
type Found struct{ Name, Source string }

var homeDirs = []string{".claude/skills", ".agents/skills", ".cursor/skills", ".codex/skills", ".config/opencode/skills"}
var projectDirs = []string{".claude/skills", ".agents/skills"}

// Collect gathers installed skills from the usual folders plus extraDirs, and sources from lock files.
// Unreadable folders and malformed lock files are skipped.
func Collect(home, cwd string, extraDirs []string) []Found {
	byName := map[string]string{}
	add := func(name, source string) {
		if name == "" {
			return
		}
		if cur, ok := byName[name]; !ok || (cur == "" && source != "") {
			byName[name] = source
		}
	}
	var dirs []string
	for _, d := range homeDirs {
		dirs = append(dirs, filepath.Join(home, d))
	}
	for _, d := range projectDirs {
		dirs = append(dirs, filepath.Join(cwd, d))
	}
	dirs = append(dirs, extraDirs...)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if st, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err == nil && !st.IsDir() {
				add(e.Name(), "")
			}
		}
	}
	for _, lock := range []string{filepath.Join(home, ".agents", ".skill-lock.json"), filepath.Join(cwd, "skills-lock.json")} {
		readLock(lock, add)
	}
	out := make([]Found, 0, len(byName))
	for n, s := range byName {
		out = append(out, Found{Name: n, Source: s})
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
	var dirs multi
	fs.Var(&dirs, "dir", "extra skills folder to scan (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	found := Collect(home, cwd, dirs)
	if len(found) == 0 {
		fmt.Fprintln(out, "no installed skills found")
		return nil
	}
	for _, f := range found {
		src := f.Source
		if src == "" {
			src = "(unknown source)"
		}
		fmt.Fprintf(out, "%-40s %s\n", f.Name, src)
	}
	if *dry {
		fmt.Fprintf(out, "%d skills found (dry run, nothing sent)\n", len(found))
		return nil
	}
	type item struct {
		Name   string `json:"name"`
		Source string `json:"source,omitempty"`
	}
	items := make([]item, len(found))
	for i, f := range found {
		items[i] = item{f.Name, f.Source}
	}
	body, _ := json.Marshal(map[string]any{"items": items})
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
		Errors   []string `json:"errors"`
	}
	if json.NewDecoder(resp.Body).Decode(&res) != nil {
		return errors.New("unreadable reply from server")
	}
	fmt.Fprintf(out, "imported %d skills as used\n", res.Imported)
	for _, e := range res.Errors {
		fmt.Fprintf(out, "  skipped: %s\n", e)
	}
	return nil
}
