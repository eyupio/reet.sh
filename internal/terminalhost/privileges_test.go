package terminalhost

import "testing"

func TestNoNewPrivilegesInspectionPreservesUnknownState(t *testing.T) {
	for _, tc := range []struct {
		status                string
		restricted, wantError bool
	}{{"Name:\ttmux\nNoNewPrivs:\t1\n", true, false}, {"NoNewPrivs:\t0\n", false, false}, {"Name:\ttmux\n", false, true}, {"NoNewPrivs: unknown\n", false, true}} {
		got, err := parseNoNewPrivileges(tc.status)
		if got != tc.restricted || (err != nil) != tc.wantError {
			t.Fatal(tc, got, err)
		}
	}
}
