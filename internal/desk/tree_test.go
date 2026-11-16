package desk

import (
	"errors"
	"reflect"
	"testing"
)

func tabOf(ids ...string) *Tab {
	t := &Tab{ID: "t", Panes: map[string]Pane{}}
	for i, id := range ids {
		t.Panes[id] = pane(id)
		if i == 0 {
			t.Root = leaf(id)
			continue
		}
		t.Root = &Node{Split: "right", Ratio: 0.5, A: t.Root, B: leaf(id)}
	}
	return t
}

func TestSplit(t *testing.T) {
	tab := tabOf("p1")
	if err := tab.Split("p1", "down", "p2"); err != nil {
		t.Fatal(err)
	}
	want := &Node{Split: "down", Ratio: 0.5, A: leaf("p1"), B: leaf("p2")}
	if !reflect.DeepEqual(tab.Root, want) {
		t.Fatalf("got %+v", tab.Root)
	}
	for _, bad := range []struct{ p, d, n string }{
		{"zz", "right", "p3"}, {"p1", "left", "p3"}, {"p1", "right", "p2"}, {"p1", "right", ""},
	} {
		if err := tab.Split(bad.p, bad.d, bad.n); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: got %v", bad, err)
		}
	}
}

func TestSplitAtLimit(t *testing.T) {
	tab := tabOf("a", "b", "c", "d", "e", "f", "g", "h")
	if err := tab.Split("a", "right", "i"); err != nil {
		t.Fatalf("9th pane: %v", err)
	}
	if err := tab.Split("a", "right", "j"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("10th pane: %v", err)
	}
	if n := len(leaves(tab.Root)); n != MaxPanes {
		t.Fatalf("panes %d", n)
	}
}

func TestCloseGivesSpaceToSibling(t *testing.T) {
	tab := tabOf("a", "b", "c") // ((a|b)|c)
	tab.Active, tab.Zoomed = "b", "b"
	if err := tab.Close("b"); err != nil {
		t.Fatal(err)
	}
	want := &Node{Split: "right", Ratio: 0.5, A: leaf("a"), B: leaf("c")}
	if !reflect.DeepEqual(tab.Root, want) {
		t.Fatalf("got %+v", tab.Root)
	}
	if _, ok := tab.Panes["b"]; ok {
		t.Fatal("pane not removed from map")
	}
	if tab.Zoomed != "" || tab.Active != "a" {
		t.Fatalf("zoomed %q active %q", tab.Zoomed, tab.Active)
	}
	if err := tab.Close("zz"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	d := Doc{Active: "t", Tabs: []Tab{*tab}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseLastPane(t *testing.T) {
	tab := tabOf("a")
	tab.Active = "a"
	if err := tab.Close("a"); err != nil {
		t.Fatal(err)
	}
	if tab.Root != nil || len(tab.Panes) != 0 || tab.Active != "" {
		t.Fatalf("%+v", tab)
	}
}

func TestSwap(t *testing.T) {
	tab := tabOf("a", "b", "c")
	if err := tab.Swap("a", "c"); err != nil {
		t.Fatal(err)
	}
	if got := leaves(tab.Root); !reflect.DeepEqual(got, []string{"c", "b", "a"}) {
		t.Fatalf("got %v", got)
	}
	if err := tab.Swap("a", "a"); err != nil {
		t.Fatal(err)
	}
	if err := tab.Swap("a", "zz"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestPresetGridOfThree(t *testing.T) {
	tab := tabOf("a", "b", "c")
	if err := tab.Preset("grid"); err != nil {
		t.Fatal(err)
	}
	want := &Node{Split: "right", Ratio: 0.5,
		A: &Node{Split: "down", Ratio: 0.5, A: leaf("a"), B: leaf("b")},
		B: leaf("c")}
	if !reflect.DeepEqual(tab.Root, want) {
		t.Fatalf("got %+v", tab.Root)
	}
}

func TestPresets(t *testing.T) {
	tab := tabOf("a", "b", "c")
	if err := tab.Preset("main"); err != nil {
		t.Fatal(err)
	}
	want := &Node{Split: "right", Ratio: 0.6, A: leaf("a"),
		B: &Node{Split: "down", Ratio: 0.5, A: leaf("b"), B: leaf("c")}}
	if !reflect.DeepEqual(tab.Root, want) {
		t.Fatalf("main: %+v", tab.Root)
	}
	if err := tab.Preset("side"); err != nil {
		t.Fatal(err)
	}
	if tab.Root.Split != "right" || tab.Root.Ratio != 1.0/3 || tab.Root.B.Ratio != 0.5 {
		t.Fatalf("side: %+v", tab.Root)
	}
	if err := tab.Preset("stack"); err != nil {
		t.Fatal(err)
	}
	if got := leaves(tab.Root); tab.Root.Split != "down" || !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("stack: %+v", tab.Root)
	}
	tab = tabOf("a", "b", "c")
	tab.Root = &Node{Split: "right", Ratio: 0.8, A: leaf("a"), B: &Node{Split: "down", Ratio: 0.2, A: leaf("b"), B: leaf("c")}}
	if err := tab.Preset("even"); err != nil {
		t.Fatal(err)
	}
	if tab.Root.Ratio != 1.0/3 || tab.Root.B.Ratio != 0.5 {
		t.Fatalf("even: %+v", tab.Root)
	}
	if err := tab.Preset("nope"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	for _, n := range []int{1, 2, 4, 5, 9} {
		ids := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}[:n]
		for _, p := range []string{"side", "stack", "grid", "main", "even"} {
			x := tabOf(ids...)
			if err := x.Preset(p); err != nil {
				t.Fatal(err)
			}
			if got := leaves(x.Root); !reflect.DeepEqual(got, ids) {
				t.Fatalf("%s/%d: %v", p, n, got)
			}
			if err := (Doc{Tabs: []Tab{*x}}).Validate(); err != nil {
				t.Fatalf("%s/%d: %v", p, n, err)
			}
		}
	}
}
