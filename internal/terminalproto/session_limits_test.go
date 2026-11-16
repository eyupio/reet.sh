package terminalproto

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// The browser enforces the live session limits, so its copy must not drift.
func TestSessionLimitsMatchTheWeb(t *testing.T) {
	src, err := os.ReadFile("../../web/src/lib/terminal/api.ts")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{"SESSIONS_PER_MACHINE": MaxSessionsPerMachine, "SESSIONS_PER_ACCOUNT": MaxSessionsPerAccount} {
		m := regexp.MustCompile(`export const ` + name + ` = (\d+)`).FindSubmatch(src)
		if m == nil {
			t.Fatalf("%s is not exported from api.ts", name)
		}
		if got, _ := strconv.Atoi(string(m[1])); got != want {
			t.Errorf("%s = %d in the web, %d in terminalproto", name, got, want)
		}
	}
	if MaxSessionsPerMachine != 24 || MaxSessionsPerAccount != 64 {
		t.Error("session limits changed from 24 and 64")
	}
}
