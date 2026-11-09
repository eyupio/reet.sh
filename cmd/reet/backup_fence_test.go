package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func fenceState(t *testing.T, dir string) store.Fence {
	t.Helper()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f, err := st.Fence()
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestTheFenceCommandRaisesAndLiftsAndSaysWhatItDid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REET_DATA_DIR", dir)
	var out bytes.Buffer
	if err := backupFence([]string{"--moved-to", "https://new.example.com"}, &out); err != nil {
		t.Fatal(err)
	}
	if f := fenceState(t, dir); !f.Fenced || f.MovedTo != "https://new.example.com" || !strings.Contains(out.String(), "Fenced.") {
		t.Fatalf("after fencing: %+v %q", f, out.String())
	}
	// Fencing again changes nothing, including the address.
	out.Reset()
	if err := backupFence([]string{"--moved-to", "https://other.example.com"}, &out); err != nil {
		t.Fatal(err)
	}
	if f := fenceState(t, dir); f.MovedTo != "https://new.example.com" || !strings.Contains(out.String(), "nothing changed") {
		t.Fatalf("after fencing twice: %+v %q", f, out.String())
	}
	out.Reset()
	if err := backupFence([]string{"--lift"}, &out); err != nil {
		t.Fatal(err)
	}
	if f := fenceState(t, dir); f.Fenced || !strings.Contains(out.String(), "Lifted") {
		t.Fatalf("after lifting: %+v %q", f, out.String())
	}
	out.Reset()
	if err := backupFence([]string{"--lift"}, &out); err != nil || !strings.Contains(out.String(), "not fenced") {
		t.Fatalf("lifting when not fenced: %v %q", err, out.String())
	}
}

func TestTheFenceCommandRefusesABadAddressAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REET_DATA_DIR", dir)
	for _, args := range [][]string{{"--moved-to", "ftp://x.example"}, {"--moved-to", "https://u:p@x.example"}, {"--lift", "--moved-to", "https://x.example"}, {"stray"}} {
		if err := backupFence(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("%v was accepted", args)
		}
		if fenceState(t, dir).Fenced {
			t.Fatalf("%v fenced the controller", args)
		}
	}
}
