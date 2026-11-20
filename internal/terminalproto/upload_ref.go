package terminalproto

import (
	"regexp"
	"strings"
)

var uploadReferenceSafe = regexp.MustCompile(`^[A-Za-z0-9_./:@%+-]+$`)

// UploadReference is the text a browser or a launch writes for an uploaded
// file: the absolute path unchanged unless it needs POSIX single quoting.
func UploadReference(path string) string {
	if uploadReferenceSafe.MatchString(path) {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
