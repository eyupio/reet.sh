//go:build !linux && !darwin

package terminalcontrol

import "errors"

func checkPath(string, bool, bool) error {
	return errors.New("terminal controller commands require Linux or macOS")
}
func checkSocket(string) error {
	return errors.New("terminal controller commands require Linux or macOS")
}
