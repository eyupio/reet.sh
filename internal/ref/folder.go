package ref

import (
	"regexp"
	"strings"
)

var notFolderSafe = regexp.MustCompile(`[^a-z0-9._]+`)

// InstalledFolder is the folder the skills CLI installs a skill into: the
// skill's own name, lower-cased, with every run of other characters made one
// hyphen. "UI/UX Design Review" is installed as ui-ux-design-review, whatever
// the skill is called in the catalogue (uiux-design-review) or in its
// repository (ui-design-review).
func InstalledFolder(name string) string {
	folder := strings.Trim(notFolderSafe.ReplaceAllString(strings.ToLower(name), "-"), ".-")
	if len(folder) > 255 {
		folder = folder[:255]
	}
	if folder == "" {
		return "unnamed-skill"
	}
	return folder
}
