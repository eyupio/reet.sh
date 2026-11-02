package api

import (
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"testing"
)

func TestSessionInspectEndpointAccountIsolationAndStrictQuery(t *testing.T) {
	caps := browseCaps()
	caps.Inspect = true
	f := browseAPIFixture(t, caps)
	s, _, err := f.st.ReserveTerminalCreateFor(f.uid, f.agent, terminalbroker.ID(), f.peers[f.agent].Generation, terminalproto.Create{Title: "session", Cols: 80, Rows: 24, Tool: "claude", Dir: "/home/tester/repo"})
	if err != nil {
		t.Fatal(err)
	}
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		req := body.(terminalproto.Inspect)
		if req.Dir != "/home/tester/repo" {
			t.Errorf("wrong dir %+v", req)
		}
		return terminalproto.InspectResult{Branch: "main", Repo: true, Changed: 2}
	})
	path := "/api/v2/terminal/sessions/" + s.ID + "/inspect"
	if code, body, _ := f.get(t, f.cookie, path); code != 200 || body["changed"] != float64(2) {
		t.Fatal(code, body)
	}
	for _, q := range []string{"?view=exec", "?view=file&path=../secret", "?view=status&dir=/tmp", "?view=files&view=file", "?view=%zz"} {
		if code, body, _ := f.get(t, f.cookie, path+q); code != 400 {
			t.Fatal(q, code, body)
		}
	}
	other, err := f.st.CreateUser("inspect-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := f.get(t, terminalLogin(t, f.st, other.ID), path); code != 404 {
		t.Fatal("other account", code)
	}
	if code, _, _ := f.get(t, "", path); code != 401 {
		t.Fatal("anonymous", code)
	}
	if code, _, _ := f.get(t, f.cookie, path, "Authorization", "Bearer fixture-owner"); code != 403 {
		t.Fatal("API token", code)
	}
	if len(f.sentRequests()) != 1 {
		t.Fatal("refused request reached machine")
	}
}
