//go:build !linux && !darwin

package terminalhost

// renameNoReplace renames from to to unless to is there.
func renameNoReplace(from, to string) error { return renameIfAbsent(from, to) }
