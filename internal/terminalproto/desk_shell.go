package terminalproto

import "strings"

// DeskShellFeature lets a machine report each running session's live working
// folder (SessionSnapshot.Dir, SessionChanged.Dir), and lets a controller
// (re)open a plain shell for a desk pane (Create.Desk). A connection without it
// never carries either: an older controller's strict decoder would refuse the
// folder, an older machine the desk reference.
const DeskShellFeature = "desk-shell-v1"

// FolderGone is the detail of a launch_failed answer to a create with a desk
// reference whose folder is missing, not a directory or cannot be entered. It
// is sent only in answer to such a create, which only a desk-shell-v1
// controller sends, so an older controller never sees it.
const FolderGone = "folder_gone"

// SupportsDeskShell reports whether the peer advertised desk-shell-v1.
func SupportsDeskShell(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == DeskShellFeature {
			return true
		}
	}
	return false
}

// DeskRef ties a plain shell to the desk pane it is opened for. Pane is the
// pane's id (a ValidID, so it is safe in a path on the machine). Dir is a
// folder the shell starts in that may lie outside home: the controller sends
// it only for a folder that machine itself reported for that pane, and the
// machine then opens any folder its user can enter. A create carries Dir or
// Desk.Dir, never both; Create.Dir keeps the home rule.
type DeskRef struct {
	Pane string `json:"pane"`
	Dir  string `json:"dir,omitempty"`
}

// validDesk accepts no desk reference, or one for a plain shell with a valid
// pane id and at most one folder.
func (c Create) validDesk() bool {
	if c.Desk == (DeskRef{}) {
		return true
	}
	plainShell := c.Tool == "" && !c.Structured && c.Worktree == "" && c.Project == ""
	validRef := ValidID(c.Desk.Pane) && validLiveDir(c.Desk.Dir)
	oneFolder := c.Desk.Dir == "" || c.Dir == ""
	return plainShell && validRef && oneFolder
}

// validLiveDir accepts an absent folder or an absolute, plain one.
func validLiveDir(dir string) bool { return dir == "" || validDir(dir) }
