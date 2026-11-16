package desk

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func leaf(id string) *Node { return &Node{Pane: id} }

func pane(id string) Pane { return Pane{ID: id, Kind: "shell", Host: "m1"} }

func oneTab() Doc {
	return Doc{
		Active: "t1",
		Tabs: []Tab{{
			ID: "t1", Name: "main", Root: leaf("p1"), Active: "p1",
			Panes: map[string]Pane{"p1": pane("p1")},
		}},
	}
}

func TestValidate(t *testing.T) {
	mut := func(f func(d *Doc)) Doc { d := oneTab(); f(&d); return d }
	twoPanes := func(d *Doc) {
		d.Tabs[0].Root = &Node{Split: "right", Ratio: 0.5, A: leaf("p1"), B: leaf("p2")}
		d.Tabs[0].Panes["p2"] = pane("p2")
	}
	many := func(n int) Doc {
		d := Doc{}
		for i := 0; i < n; i++ {
			id := string(rune('a'+i%26)) + string(rune('a'+i/26))
			d.Tabs = append(d.Tabs, Tab{ID: id, Panes: map[string]Pane{}})
		}
		return d
	}
	nPanes := func(n int) Doc {
		d := oneTab()
		tab := &d.Tabs[0]
		tab.Root = leaf("p0")
		tab.Panes = map[string]Pane{"p0": pane("p0")}
		tab.Active = ""
		for i := 1; i < n; i++ {
			id := "p" + string(rune('0'+i))
			tab.Root = &Node{Split: "right", Ratio: 0.5, A: tab.Root, B: leaf(id)}
			tab.Panes[id] = pane(id)
		}
		return d
	}
	tests := []struct {
		name string
		doc  Doc
		ok   bool
	}{
		{"empty", Doc{}, true},
		{"one tab one pane", oneTab(), true},
		{"two panes", mut(twoPanes), true},
		{"20 tabs", many(20), true},
		{"21 tabs", many(21), false},
		{"9 panes", nPanes(9), true},
		{"10 panes", nPanes(10), false},
		{"leaf names missing pane", mut(func(d *Doc) { d.Tabs[0].Root = leaf("zz") }), false},
		{"pane not in tree", mut(func(d *Doc) { d.Tabs[0].Panes["p2"] = pane("p2") }), false},
		{"pane in tree twice", mut(func(d *Doc) {
			d.Tabs[0].Root = &Node{Split: "right", Ratio: 0.5, A: leaf("p1"), B: leaf("p1")}
		}), false},
		{"ratio 0", mut(func(d *Doc) { twoPanes(d); d.Tabs[0].Root.Ratio = 0 }), false},
		{"ratio 1", mut(func(d *Doc) { twoPanes(d); d.Tabs[0].Root.Ratio = 1 }), false},
		{"ratio NaN", mut(func(d *Doc) { twoPanes(d); d.Tabs[0].Root.Ratio = math.NaN() }), false},
		{"bad split", mut(func(d *Doc) { twoPanes(d); d.Tabs[0].Root.Split = "left" }), false},
		{"split missing child", mut(func(d *Doc) {
			d.Tabs[0].Root = &Node{Split: "right", Ratio: 0.5, A: leaf("p1")}
		}), false},
		{"colour pink", mut(func(d *Doc) { d.Tabs[0].Colour = "pink" }), false},
		{"colour blue", mut(func(d *Doc) { d.Tabs[0].Colour = "blue" }), true},
		{"pane colour pink", mut(func(d *Doc) {
			p := d.Tabs[0].Panes["p1"]
			p.Colour = "pink"
			d.Tabs[0].Panes["p1"] = p
		}), false},
		{"name 80 runes", mut(func(d *Doc) { d.Tabs[0].Name = strings.Repeat("é", 80) }), true},
		{"name 81 runes", mut(func(d *Doc) { d.Tabs[0].Name = strings.Repeat("é", 81) }), false},
		{"start 2000", mut(func(d *Doc) {
			p := d.Tabs[0].Panes["p1"]
			p.Start = strings.Repeat("x", 2000)
			d.Tabs[0].Panes["p1"] = p
		}), true},
		{"start 2001", mut(func(d *Doc) {
			p := d.Tabs[0].Panes["p1"]
			p.Start = strings.Repeat("x", 2001)
			d.Tabs[0].Panes["p1"] = p
		}), false},
		{"duplicate tab id", mut(func(d *Doc) { d.Tabs = append(d.Tabs, Tab{ID: "t1", Panes: map[string]Pane{}}) }), false},
		{"duplicate session", mut(func(d *Doc) {
			twoPanes(d)
			for _, id := range []string{"p1", "p2"} {
				p := d.Tabs[0].Panes[id]
				p.Session = "s1"
				d.Tabs[0].Panes[id] = p
			}
		}), false},
		{"active names no tab", mut(func(d *Doc) { d.Active = "nope" }), false},
		{"zoomed names no pane", mut(func(d *Doc) { d.Tabs[0].Zoomed = "nope" }), false},
		{"tab active names no pane", mut(func(d *Doc) { d.Tabs[0].Active = "nope" }), false},
		{"bad kind", mut(func(d *Doc) {
			p := d.Tabs[0].Panes["p1"]
			p.Kind = "vim"
			d.Tabs[0].Panes["p1"] = p
		}), false},
		{"map key mismatch", mut(func(d *Doc) {
			d.Tabs[0].Panes["p1"] = Pane{ID: "other", Kind: "shell"}
		}), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.doc.Validate()
			if tc.ok && err != nil {
				t.Fatalf("want ok, got %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestParse(t *testing.T) {
	good := `{"tabs":[{"id":"t1","name":"a","colour":"","root":{"split":"right","ratio":0,"a":{"pane":"p1"},"b":{"pane":"p2"}},"zoomed":"","active":"p1","panes":{"p1":{"id":"p1","kind":"shell","host":"m","dirAt":5,"start":"ls"},"p2":{"id":"p2","kind":"agent","host":"m"}}}],"active":"t1"}`
	d, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if r := d.Tabs[0].Root.Ratio; r != MinRatio {
		t.Fatalf("ratio not clamped: %v", r)
	}
	if d.Tabs[0].Panes["p1"].DirAt != 5 || d.Tabs[0].Panes["p1"].Start != "ls" {
		t.Fatalf("camel case fields lost: %+v", d.Tabs[0].Panes["p1"])
	}
	big := `{"tabs":[],"active":"` + strings.Repeat("x", 65<<10) + `"}`
	for name, in := range map[string]string{
		"unknown field": `{"tabs":[],"active":"","extra":1}`,
		"trailing":      `{} {}`,
		"not json":      `nope`,
		"too big":       big,
		"bad pane kind": `{"tabs":[{"id":"t","panes":{"p":{"id":"p","kind":"x"}},"root":{"pane":"p"}}]}`,
	} {
		if _, err := Parse([]byte(in)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	if _, err := Parse([]byte(`{}`)); err != nil {
		t.Fatalf("empty doc: %v", err)
	}
	out, _ := json.Marshal(d.Tabs[0].Panes["p1"])
	if !strings.Contains(string(out), `"dirAt"`) {
		t.Fatalf("marshal: %s", out)
	}
}

func TestClamp(t *testing.T) {
	for in, want := range map[float64]float64{0: 0.1, 1: 0.9, 0.5: 0.5, -3: 0.1, math.Inf(1): 0.9, math.Inf(-1): 0.1, math.NaN(): 0.5} {
		if got := Clamp(in); got != want {
			t.Errorf("Clamp(%v)=%v want %v", in, got, want)
		}
	}
}

func twoTabs() Doc {
	p := func(id, sess, host string) Pane { return Pane{ID: id, Kind: "shell", Session: sess, Host: host} }
	return Doc{Tabs: []Tab{
		{ID: "t1", Root: &Node{Split: "right", Ratio: 0.5, A: leaf("a"), B: leaf("b")},
			Panes: map[string]Pane{"a": p("a", "s1", "m1"), "b": p("b", "", "m2")}},
		{ID: "t2", Root: leaf("c"), Panes: map[string]Pane{"c": p("c", "s2", "m1")}},
	}}
}

func TestSessions(t *testing.T) {
	got := twoTabs().Sessions()
	if len(got) != 2 || got[0] != "s1" || got[1] != "s2" {
		t.Fatalf("got %v", got)
	}
}

func TestClearSession(t *testing.T) {
	d := twoTabs()
	if !d.ClearSession("s2") {
		t.Fatal("expected change")
	}
	if d.Tabs[1].Panes["c"].Session != "" {
		t.Fatal("not cleared")
	}
	if d.ClearSession("s2") || d.ClearSession("") {
		t.Fatal("expected no change")
	}
	if d.Tabs[0].Panes["a"].Session != "s1" {
		t.Fatal("other session touched")
	}
}

func TestRepointHost(t *testing.T) {
	d := twoTabs()
	if !d.RepointHost("m1", "m9") {
		t.Fatal("expected change")
	}
	if d.Tabs[0].Panes["a"].Host != "m9" || d.Tabs[1].Panes["c"].Host != "m9" || d.Tabs[0].Panes["b"].Host != "m2" {
		t.Fatalf("wrong repoint: %+v", d)
	}
	if d.RepointHost("m1", "m9") {
		t.Fatal("expected no change")
	}
}
