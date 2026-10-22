package clientconfig

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Require resolves a missing command input without consuming a subsequent
// confirmation. Prompt output never shares the command's structured stdout.
func Require(value, label string, secret bool, in io.Reader, out io.Writer) (string, error) {
	if value != "" {
		return value, nil
	}
	if !Interactive(in) {
		return "", fmt.Errorf("missing %s; provide it explicitly or run interactively", label)
	}
	fmt.Fprintf(out, "%s: ", label)
	var answer string
	if secret {
		b, err := term.ReadPassword(int(in.(*os.File).Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", errors.New("could not read secret from terminal")
		}
		answer = string(b)
		for i := range b {
			b[i] = 0
		}
	} else {
		var b strings.Builder
		one := make([]byte, 1)
		for {
			n, err := in.Read(one)
			if n > 0 {
				if one[0] == '\n' {
					break
				}
				b.WriteByte(one[0])
				if b.Len() > 4096 {
					return "", errors.New("input exceeds size limit")
				}
			}
			if err != nil {
				return "", errors.New("input cancelled")
			}
		}
		answer = strings.TrimSpace(b.String())
	}
	if answer == "" || len(answer) > 4096 || strings.ContainsAny(answer, "\r\n\x00") {
		return "", fmt.Errorf("missing or invalid %s", label)
	}
	return answer, nil
}
