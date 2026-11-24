package clientconfig

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// EnrolCodePrefix starts every enrolment code a controller makes.
const EnrolCodePrefix = "reet_enrol_"

// Enrolment is the credential a machine enrols with: a single-use code, or
// (the older way) a personal API token. Both empty means none, for an
// instance that needs none.
type Enrolment struct {
	Code   string
	Token  string
	Source Source
}

// ResolveEnrolment finds what to enrol with: the --code value, then
// REET_ENROL_CODE, then a personal token from where the CLI has always read
// one (never prompted for), then a hidden prompt for a code. Nothing is saved.
func ResolveEnrolment(code, explicitToken, envFile, server string, in io.Reader, out io.Writer) (Enrolment, error) {
	if code == "" {
		code = strings.TrimSpace(os.Getenv("REET_ENROL_CODE"))
	}
	if code != "" {
		return enrolCode(code)
	}
	token, source, err := resolveTokenSource(explicitToken, envFile, in, out, false)
	if err != nil {
		return Enrolment{}, err
	}
	if token != "" {
		fmt.Fprintf(out, "Enrolling with a personal API token from %s. A machine no longer needs one: make an enrolment code under Machines, Add a machine, and pass it with --code.\n", source)
		return Enrolment{Token: token, Source: source}, nil
	}
	if !interactiveFn(in) {
		return Enrolment{}, nil
	}
	where := "your reet site"
	if server != "" {
		where = server
	}
	fmt.Fprintf(out, "Enrolment code (Machines, Add a machine on %s; hidden, Enter if the site needs none): ", where)
	entered, err := readSecret(in)
	fmt.Fprintln(out)
	if err != nil || entered == "" {
		return Enrolment{}, err
	}
	return enrolCode(strings.TrimSpace(entered))
}

func enrolCode(code string) (Enrolment, error) {
	if !strings.HasPrefix(code, EnrolCodePrefix) || len(code) > 256 || strings.ContainsAny(code, " \t\r\n\x00") {
		return Enrolment{}, errors.New("that is not an enrolment code; copy the code shown under Machines, Add a machine (it starts " + EnrolCodePrefix + ")")
	}
	return Enrolment{Code: code, Source: "the enrolment code"}, nil
}
