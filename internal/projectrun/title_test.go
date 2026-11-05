package projectrun

import "testing"

// A member of a start in several projects is named as a single launch would be:
// from what the prompt asks for, else from the project.
func TestSelectionTitleNamesTheWorkNotTheGreeting(t *testing.T) {
	for _, c := range []struct{ prompt, project, want string }{
		{"Please fix the build", "Shop", "Fix the build"},
		{"Hi,\nadd a footer link", "Shop", "Add a footer link"},
		{"Use the brainstorming skill.\nDesign a retry queue", "Shop", "Design a retry queue"},
		{"hi", "Shop", "Shop"},
		{"", "Shop", "Shop"},
		{"...", "Shop", "Shop"},
	} {
		if got := SelectionTitle(c.prompt, c.project); got != c.want {
			t.Errorf("SelectionTitle(%q, %q) = %q, want %q", c.prompt, c.project, got, c.want)
		}
	}
}
