package terminalbroker

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestInspectSessionOwnershipCapabilitiesAndPaths(t *testing.T) {
	caps := modelsCapabilities()
	caps.Inspect = true
	f := modelsFixture(t, caps)
	s, _, err := f.st.ReserveTerminalCreateFor(f.uid, f.agent, ID(), f.peer.Generation, terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "opencode", Dir: "/home/dev/project"})
	if err != nil {
		t.Fatal(err)
	}
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		req := body.(terminalproto.Inspect)
		if req.Dir != "/home/dev/project" || req.View != "file" || req.Path != "src/main.go" {
			t.Errorf("unbound request %+v", req)
		}
		return terminalproto.InspectResult{Text: "content"}
	})
	r, err := f.b.InspectSession(t.Context(), f.uid, s.ID, "file", "src/main.go")
	if err != nil || r.Text != "content" {
		t.Fatal(r, err)
	}
	if _, err = f.b.InspectSession(t.Context(), f.uid+1, s.ID, "file", "src/main.go"); !errors.Is(err, ErrDenied) {
		t.Fatal("other account", err)
	}
	if _, err = f.b.InspectSession(t.Context(), f.uid, s.ID, "file", "../secret"); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal("traversal", err)
	}
	if len(f.sentRequests()) != 1 {
		t.Fatal("refused operation reached machine")
	}
	old := modelsFixture(t, modelsCapabilities())
	s, _, err = old.st.ReserveTerminalCreateFor(old.uid, old.agent, ID(), old.peer.Generation, terminalproto.Create{Title: "old", Cols: 80, Rows: 24, Tool: "opencode", Dir: "/home/dev/project"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.b.InspectSession(t.Context(), old.uid, s.ID, "status", ""); !errors.Is(err, ErrInspectUnsupported) {
		t.Fatal("old connector", err)
	}
	if len(old.sentRequests()) != 0 {
		t.Fatal("sent unsupported request")
	}
}
