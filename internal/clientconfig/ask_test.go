package clientconfig

import (
	"bytes"
	"strings"
	"testing"
)

func TestAskSupportsSetupLabelsAndKeepsFollowingAnswers(t *testing.T) {
	in := strings.NewReader("\nyes\n")
	var out bytes.Buffer
	answer, err := Ask(in, &out, "Enable local shell access?", "no")
	if err != nil || answer != "no" {
		t.Fatal(answer, err)
	}
	answer, err = Ask(in, &out, "Install services?", "yes")
	if err != nil || answer != "yes" || !strings.Contains(out.String(), "Install services?") {
		t.Fatal(answer, err, out.String())
	}
}
