//go:build !linux && !darwin

package terminalhost

// dirOpenFlags is empty where the system has no O_DIRECTORY; Browse has
// already found a real directory with lstat before it reads one.
const dirOpenFlags = 0
