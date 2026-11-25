package skillfetch

import (
	"archive/tar"
	"context"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
)

// A link inside the skill's folder is not copied, and a skill saved without it
// would look complete, so the fetch says the skill has one.
func TestFetchSaysWhenTheSkillFolderHasALink(t *testing.T) {
	for _, tc := range []struct {
		name string
		link entry
		want string
	}{
		{"symlink", entry{name: "r-abc/skills/pdf/references", typeflag: tar.TypeSymlink, link: "../../shared/refs"}, "(references)"},
		{"hard link", entry{name: "r-abc/skills/pdf/scripts/run.sh", typeflag: tar.TypeLink, link: "skills/pdf/SKILL.md"}, "(scripts/run.sh)"},
	} {
		f := githubServer(t, buildTar(t, []entry{
			{name: "r-abc/skills/pdf/SKILL.md", body: "---\nname: pdf\n---\nx"},
			tc.link,
		}))
		res, err := f.Fetch(context.Background(), ghRef("pdf"))
		if err == nil || !strings.Contains(err.Error(), "the skill has a link "+tc.want+" that reet does not copy") || len(res.Files) != 0 {
			t.Errorf("%s: files %v err %v", tc.name, paths(res.Files), err)
		}
	}
}

// A link elsewhere in the repository is not part of the skill.
func TestFetchIgnoresLinksOutsideTheSkillFolder(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "---\nname: pdf\n---\nx"},
		{name: "r-abc/skills/pdf-extra/refs", typeflag: tar.TypeSymlink, link: "../pdf"},
		{name: "r-abc/docs/link", typeflag: tar.TypeSymlink, link: "/etc/passwd"},
		{name: "r-abc/link", typeflag: tar.TypeLink, link: "README.md"},
	}))
	res, err := f.Fetch(context.Background(), ghRef("pdf"))
	if err != nil || len(res.Files) != 1 || res.Files[0].Path != "SKILL.md" {
		t.Fatalf("files %v err %v", paths(res.Files), err)
	}
}

// Of several skills fetched from one archive, only the one whose folder has a
// link is refused.
func TestFetchManyRefusesOnlyTheSkillWithALink(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "---\nname: pdf\n---\nx"},
		{name: "r-abc/skills/pdf/refs", typeflag: tar.TypeSymlink, link: "../../shared"},
		{name: "r-abc/skills/other/SKILL.md", body: "---\nname: other\n---\nx"},
	}))
	out := f.FetchMany(context.Background(), []ref.Ref{ghRef("pdf"), ghRef("other")})
	if out[0].Err == nil || !strings.Contains(out[0].Err.Error(), "(refs)") {
		t.Errorf("pdf: %v", out[0].Err)
	}
	if out[1].Err != nil || len(out[1].Files) != 1 {
		t.Errorf("other: %v %v", paths(out[1].Files), out[1].Err)
	}
}
