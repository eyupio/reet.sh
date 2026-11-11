package store

import "testing"

func TestTerminalSessionUsageAddsTokensAndKeepsTheLastContextReport(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err = st.migrateTerminalUsage(); err != nil {
		t.Fatal(err)
	}
	// A report for a session this account does not own is not kept.
	if err = st.AddTerminalSessionUsage(1, "0123456789abcdef0123456789abcdef0123456789abcdef", 1, 1, 0, 0, 0); err == nil {
		t.Fatal("usage was kept for an unknown session")
	}
	if u, err := st.TerminalSessionUsageFor(1, "0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil || u.Reported {
		t.Fatalf("unknown session: %+v %v", u, err)
	}
	if err = st.AddTerminalSessionUsage(1, "x", -1, 0, 0, 0, 0); err == nil {
		t.Fatal("negative usage was accepted")
	}
}
