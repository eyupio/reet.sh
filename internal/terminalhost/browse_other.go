//go:build !linux && !darwin

package terminalhost

// dirOpenFlags is empty where the system has no O_DIRECTORY. Browse only
// reads a folder that ResolveDir found to be a directory (os.Stat, links
// followed), through an os.Root at the real home, and the read itself fails
// on anything that is not a directory; nothing here guarantees that a file
// swapped in at the last moment is not opened, though it is never read.
const dirOpenFlags = 0
