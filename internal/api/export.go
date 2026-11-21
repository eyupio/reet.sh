package api

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) export(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	// From the Skills page (a tab named): the rows that page shows.
	if stage := r.URL.Query().Get("stage"); (format == "md" || format == "markdown" || format == "csv") && (stage == store.SkillActive || stage == store.SkillArchived) {
		rows, err := s.skillsPageExport(r, stage)
		if err != nil {
			skillsPageError(w, err)
			return
		}
		s.eventRequest(r, "export", "", map[string]any{"format": format, "stage": stage})
		if format == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="reet-skills.csv"`)
			w.Write([]byte(skillRowsCSV(rows)))
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="reet-skills.md"`)
		w.Write([]byte(skillRowsMarkdown(rows, stage)))
		return
	}
	switch format {
	case "md", "markdown":
		skills, _, err := s.st.ListFor(s.uid(r), filterFromRequest(r))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.eventRequest(r, "export", "", map[string]any{"format": "markdown"})
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="reet-shelf.md"`)
		w.Write([]byte(markdown(skills)))
		return
	case "csv":
		skills, _, err := s.st.ListFor(s.uid(r), filterFromRequest(r))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.eventRequest(r, "export", "", map[string]any{"format": "csv"})
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="reet-shelf.csv"`)
		w.Write([]byte(csvExport(skills)))
		return
	}
	includeEvents := r.URL.Query().Get("events") != "0"
	snap, err := s.st.ExportFor(s.uid(r), includeEvents)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "export", "", map[string]any{"format": "json", "events": includeEvents})
	w.Header().Set("Content-Disposition", `attachment; filename="reet-export.json"`)
	writeJSON(w, 200, snap)
}

func installCommand(s store.Skill) string {
	var source string
	switch s.SourceType {
	case "github":
		source = "https://github.com/" + s.Source
	case "hosted":
		source = s.URL
	case "well-known":
		source = "https://" + s.Source
	default:
		return ""
	}
	// The skills CLI matches a skill by its own name, which is not always its
	// folder name, so a differing name is offered as well.
	selector := s.Slug
	name := strings.TrimSpace(s.Name)
	plain := name != "" && len(name) <= 200 && !strings.HasPrefix(name, "-") && !strings.Contains(name, "*") && !strings.ContainsFunc(name, unicode.IsControl)
	if plain && !strings.EqualFold(name, s.Slug) {
		selector += " '" + strings.ReplaceAll(name, "'", `'\''`) + "'"
	}
	return fmt.Sprintf("npx %s add %s --skill %s", ref.SkillsCLI, source, selector)
}

// skillsPageExport is what the Skills page shows for the tab status, filtered
// as the page filters it (web/src/lib/skills/skillsPage.ts matchesSearch, and
// its Liked and tag filters), in name order: each library skill once,
// whatever shelf rows name it, with the description the page shows. Case is
// folded beyond ASCII, as the page folds it.
func (s *server) skillsPageExport(r *http.Request, status string) ([]store.SkillPageRow, error) {
	page, err := s.skillsPageData(r, status)
	if err != nil {
		return nil, err
	}
	q := r.URL.Query()
	liked := q.Get("tab") == "liked"
	var tagIDs []int64
	for _, raw := range q["tag"] {
		for _, part := range strings.Split(raw, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				tagIDs = append(tagIDs, id)
			}
		}
	}
	term, tagNames := store.SplitQuery(q.Get("q"))
	term = strings.ToLower(term)
	out := []store.SkillPageRow{}
	for _, row := range page.Skills {
		if liked && !row.Liked || !skillRowHasTags(row, tagIDs, tagNames) {
			continue
		}
		if term != "" && !slices.ContainsFunc([]string{row.Name, row.ShelfID, row.Description, row.Notes}, func(f string) bool { return strings.Contains(strings.ToLower(f), term) }) {
			continue
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// skillRowHasTags says whether the row carries every tag named by id and by
// (lowercased) name.
func skillRowHasTags(row store.SkillPageRow, ids []int64, names []string) bool {
	for _, id := range ids {
		if !slices.ContainsFunc(row.Tags, func(t store.Tag) bool { return t.ID == id }) {
			return false
		}
	}
	for _, name := range names {
		if !slices.ContainsFunc(row.Tags, func(t store.Tag) bool { return strings.ToLower(t.Name) == name }) {
			return false
		}
	}
	return true
}

// skillRowLink is where a row's skill can be read: its public page when
// published, else the link it was imported from.
func skillRowLink(row store.SkillPageRow) string {
	if row.PublicURL != "" {
		return row.PublicURL
	}
	if strings.HasPrefix(row.Origin, "https://") || strings.HasPrefix(row.Origin, "http://") {
		return row.Origin
	}
	return ""
}

func skillRowsMarkdown(rows []store.SkillPageRow, status string) string {
	var b strings.Builder
	b.WriteString("# reet.sh skills\n")
	if len(rows) == 0 {
		return b.String()
	}
	title := "Active"
	if status == store.SkillArchived {
		title = "Archived"
	}
	fmt.Fprintf(&b, "\n## %s\n\n", title)
	for _, row := range rows {
		b.WriteString(markdownItem(row.Name, skillRowLink(row), row.Description))
		b.WriteString("\n")
	}
	return b.String()
}

// markdownItem is one skill as a Markdown list item, "- [name](link) — description", which stays one item whatever
// the three hold: the name is escaped, the link percent-encodes what would end it (and is left out when it is not a
// web address), and the description is markdownText.
func markdownItem(name, link, desc string) string {
	item := "- " + markdownEscape(name)
	if strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://") {
		item = "- [" + markdownEscape(name) + "](" + markdownURL(link) + ")"
	}
	if desc = markdownText(desc); desc != "" {
		item += " — " + desc
	}
	return item
}

var (
	markdownLinkRe = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	htmlTagRe      = regexp.MustCompile(`<!--.*?-->|</?[A-Za-z][A-Za-z0-9-]*(?:\s[^<>]*)?/?>`)
	markdownEscape = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, `<`, `\<`, `>`, `\>`, `*`, `\*`, `_`, `\_`, "`", "\\`", `#`, `\#`).Replace
)

// markdownText is a description as one line of Markdown text. It comes from a skill's SKILL.md, often someone else's,
// so its links become their text, its HTML tags and comments go, and what is left is escaped so it makes no link or
// HTML of its own.
func markdownText(desc string) string {
	desc = strings.Join(strings.Fields(desc), " ")
	desc = markdownLinkRe.ReplaceAllString(desc, "$1")
	desc = htmlTagRe.ReplaceAllString(desc, "")
	return markdownEscape(strings.Join(strings.Fields(desc), " "))
}

// markdownURL is a link as a Markdown link's destination: spaces, control characters, parentheses, angle brackets
// and backslashes are percent-encoded, so the link ends where it should.
func markdownURL(link string) string {
	var b strings.Builder
	for i := 0; i < len(link); i++ {
		if c := link[i]; c <= ' ' || c == 0x7f || strings.IndexByte(`()<>\`, c) >= 0 {
			fmt.Fprintf(&b, "%%%02X", c)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// csvCell keeps a spreadsheet from reading an exported cell as a formula (CSV injection): a cell that starts with =,
// +, -, @, a tab or a carriage return gets a quote in front.
func csvCell(cell string) string {
	if cell != "" && strings.IndexByte("=+-@\t\r", cell[0]) >= 0 {
		return "'" + cell
	}
	return cell
}

// csvRow is a row of cells, each one csvCell.
func csvRow(cells ...string) []string {
	for i, cell := range cells {
		cells[i] = csvCell(cell)
	}
	return cells
}

func skillRowsCSV(rows []store.SkillPageRow) string {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"id", "name", "status", "origin", "link", "liked", "notes", "tags"})
	for _, row := range rows {
		var tags []string
		for _, t := range row.Tags {
			tags = append(tags, t.Name)
		}
		_ = w.Write(csvRow(row.ID, row.Name, row.Status, row.OriginKind, skillRowLink(row), strconv.FormatBool(row.Liked), row.Notes, strings.Join(tags, "; ")))
	}
	w.Flush()
	return b.String()
}

func markdown(skills []store.Skill) string {
	var b strings.Builder
	b.WriteString("# reet.sh library\n")
	groups := []struct {
		title string
		match func(store.Skill) bool
	}{
		{"In use", func(s store.Skill) bool { return s.Used }},
		{"To test", func(s store.Skill) bool { return !s.Used && s.Bookmarked }},
		{"Parked", func(s store.Skill) bool { return !s.Used && !s.Bookmarked }},
	}
	for _, group := range groups {
		var items []store.Skill
		for _, s := range skills {
			if group.match(s) {
				items = append(items, s)
			}
		}
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", group.title)
		for _, s := range items {
			name := s.Name
			if name == "" {
				name = s.Slug
			}
			b.WriteString(markdownItem(name, s.URL, s.Description))
			b.WriteString("\n")
			if cmd := installCommand(s); cmd != "" {
				fmt.Fprintf(&b, "  ```\n  %s\n  ```\n", cmd)
			}
		}
	}
	return b.String()
}

func csvExport(skills []store.Skill) string {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"id", "name", "stage", "source", "url", "installs", "installs_week", "liked", "bookmarked", "used", "notes", "tags", "environments"})
	for _, s := range skills {
		var tags, envs []string
		for _, t := range s.Tags {
			tags = append(tags, t.Name)
		}
		for _, e := range s.Environments {
			envs = append(envs, e.Name)
		}
		_ = w.Write(csvRow(
			s.ID, s.Name, stageLabel(s), s.Source, s.URL,
			strconv.FormatInt(s.Installs, 10), strconv.FormatInt(s.InstallsWeek, 10),
			strconv.FormatBool(s.Liked), strconv.FormatBool(s.Bookmarked), strconv.FormatBool(s.Used),
			s.Notes, strings.Join(tags, "; "), strings.Join(envs, "; "),
		))
	}
	w.Flush()
	return b.String()
}

func (s *server) restore(w http.ResponseWriter, r *http.Request) {
	var in store.Snapshot
	if !decode(w, r, &in) {
		return
	}
	if in.Skills == nil {
		writeErr(w, 400, "backup has no skills")
		return
	}
	if err := s.st.RestoreSnapshotFor(s.uid(r), in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.eventRequest(r, "restore", "", map[string]any{"restored": len(in.Skills), "ignored": len(in.Ignored)})
	writeJSON(w, 200, map[string]int{"restored": len(in.Skills)})
}
