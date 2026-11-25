package skillfetch

import (
	"context"
	"testing"
)

func TestSkillName(t *testing.T) {
	for _, tc := range []struct{ name, md, want string }{
		{"plain", "---\nname: pdf-tools\ndescription: d\n---\nbody", "pdf-tools"},
		{"comment after the name", "---\nname: pdf-tools # main skill\n---\nbody", "pdf-tools"},
		{"double quotes", "---\nname: \"PDF Tools\"\n---\nbody", "PDF Tools"},
		{"single quotes", "---\nname: 'pdf-tools'\n---\nbody", "pdf-tools"},
		{"quoted name with a hash", "---\nname: \"pdf # tools\"\n---\nbody", "pdf # tools"},
		{"CRLF", "---\r\nname: pdf-tools\r\ndescription: d\r\n---\r\nbody\r\n", "pdf-tools"},
		{"name after other keys", "---\ndescription: d\nname: pdf-tools\n---\n", "pdf-tools"},
		{"front matter only, no trailing newline", "---\nname: pdf-tools\n---", "pdf-tools"},
		{"no front matter", "# pdf-tools\nname: pdf-tools\n", ""},
		{"name outside the front matter", "---\ndescription: d\n---\nname: pdf-tools\n", ""},
		{"front matter never closed", "---\nname: pdf-tools\n", ""},
		{"no name", "---\ndescription: d\n---\n", ""},
		{"not YAML", "---\nname: [unclosed\n---\n", ""},
		{"empty", "", ""},
	} {
		if got := SkillName([]byte(tc.md)); got != tc.want {
			t.Errorf("%s: SkillName = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A written name carries a comment as often as not; the folder is still the
// skill's.
func TestPickFolderReadsTheNameAsYAML(t *testing.T) {
	all := map[string]held{
		"tools/pdf/SKILL.md":   {data: []byte("---\nname: pdf-tools # main skill\n---\nx")},
		"tools/other/SKILL.md": {data: []byte("---\nname: other\n---\nx")},
	}
	dirs := map[string]bool{"tools/pdf": true, "tools/other": true}
	if got, ok := pickFolder(dirs, all, "pdf-tools"); !ok || got != "tools/pdf" {
		t.Fatalf("pickFolder = %q %v", got, ok)
	}
}

func TestFetchFindsAFolderByANameWithAComment(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{
		{name: "r-abc/tools/pdf/SKILL.md", body: "---\nname: pdf-tools # main skill\n---\nx"},
	}))
	res, err := f.Fetch(context.Background(), ghRef("pdf-tools"))
	if err != nil || len(res.Files) != 1 || res.Origin != "https://github.com/o/r/tree/HEAD/tools/pdf" {
		t.Fatalf("res = %+v err = %v", res, err)
	}
}
