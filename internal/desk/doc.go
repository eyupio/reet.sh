// Package desk defines a desk's document (tabs, a split tree and panes) and
// validates it. It depends on nothing else in the repository.
package desk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"unicode/utf8"
)

// Limits and the document schema.
const (
	Schema      = 1
	MaxTabs     = 20
	MaxPanes    = 9
	MaxName     = 80
	MaxStart    = 2000
	MaxDocBytes = 64 << 10
	MinRatio    = 0.1
	MaxRatio    = 0.9
)

// ErrInvalid is wrapped by every validation failure.
var ErrInvalid = errors.New("invalid desk")

// Doc is the document of one desk.
type Doc struct {
	Tabs   []Tab  `json:"tabs"`
	Active string `json:"active"`
}

// Tab is one tab: a split tree over a set of panes.
type Tab struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Colour string          `json:"colour"`
	Root   *Node           `json:"root"`
	Zoomed string          `json:"zoomed"`
	Active string          `json:"active"`
	Panes  map[string]Pane `json:"panes"`
}

// Node is a leaf (Pane set) or a split (Split, Ratio, A and B set). Ratio is
// the share of the space that A takes.
type Node struct {
	Pane  string  `json:"pane,omitempty"`
	Split string  `json:"split,omitempty"` // "right" or "down"
	Ratio float64 `json:"ratio,omitempty"`
	A     *Node   `json:"a,omitempty"`
	B     *Node   `json:"b,omitempty"`
}

// Pane is one place in a tab. Session is empty until a shell is started.
type Pane struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Colour  string `json:"colour"`
	Host    string `json:"host"`
	Session string `json:"session"`
	Kind    string `json:"kind"` // "shell" or "agent"
	Dir     string `json:"dir"`
	DirAt   int64  `json:"dirAt"`
	Start   string `json:"start"`
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Clamp limits a split ratio to MinRatio..MaxRatio; NaN becomes 0.5.
func Clamp(r float64) float64 {
	switch {
	case math.IsNaN(r):
		return 0.5
	case r < MinRatio:
		return MinRatio
	case r > MaxRatio:
		return MaxRatio
	}
	return r
}

// Parse decodes a document strictly, clamps split ratios and validates it.
func Parse(data []byte) (Doc, error) {
	var d Doc
	if len(data) > MaxDocBytes {
		return Doc{}, invalid("document over %d bytes", MaxDocBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return Doc{}, invalid("%v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Doc{}, invalid("trailing data after document")
	}
	for i := range d.Tabs {
		clampTree(d.Tabs[i].Root)
	}
	if err := d.Validate(); err != nil {
		return Doc{}, err
	}
	return d, nil
}

func clampTree(n *Node) {
	if n == nil {
		return
	}
	if n.Split != "" {
		n.Ratio = Clamp(n.Ratio)
	}
	clampTree(n.A)
	clampTree(n.B)
}

func validColour(c string) bool {
	switch c {
	case "", "red", "amber", "green", "blue", "violet":
		return true
	}
	return false
}

// Validate checks the document against every limit and structural rule.
func (d Doc) Validate() error {
	if len(d.Tabs) > MaxTabs {
		return invalid("more than %d tabs", MaxTabs)
	}
	tabs := map[string]bool{}
	sessions := map[string]bool{}
	for i := range d.Tabs {
		t := &d.Tabs[i]
		if t.ID == "" || tabs[t.ID] {
			return invalid("tab id %q empty or repeated", t.ID)
		}
		tabs[t.ID] = true
		if err := t.validate(sessions); err != nil {
			return err
		}
	}
	if d.Active != "" && !tabs[d.Active] {
		return invalid("active tab %q does not exist", d.Active)
	}
	return nil
}

func (t *Tab) validate(sessions map[string]bool) error {
	if utf8.RuneCountInString(t.Name) > MaxName {
		return invalid("tab name over %d characters", MaxName)
	}
	if !validColour(t.Colour) {
		return invalid("tab colour %q", t.Colour)
	}
	if len(t.Panes) > MaxPanes {
		return invalid("more than %d panes", MaxPanes)
	}
	inTree := map[string]bool{}
	if err := checkNode(t.Root, 0, inTree); err != nil {
		return err
	}
	for id := range inTree {
		if _, ok := t.Panes[id]; !ok {
			return invalid("tree names missing pane %q", id)
		}
	}
	for id, p := range t.Panes {
		if !inTree[id] {
			return invalid("pane %q is not in the tree", id)
		}
		if p.ID != id {
			return invalid("pane %q listed under %q", p.ID, id)
		}
		if utf8.RuneCountInString(p.Name) > MaxName {
			return invalid("pane name over %d characters", MaxName)
		}
		if !validColour(p.Colour) {
			return invalid("pane colour %q", p.Colour)
		}
		if p.Kind != "shell" && p.Kind != "agent" {
			return invalid("pane kind %q", p.Kind)
		}
		if utf8.RuneCountInString(p.Start) > MaxStart {
			return invalid("start-up command over %d characters", MaxStart)
		}
		if p.Session != "" {
			if sessions[p.Session] {
				return invalid("session %q in more than one pane", p.Session)
			}
			sessions[p.Session] = true
		}
	}
	if t.Zoomed != "" && !inTree[t.Zoomed] {
		return invalid("zoomed pane %q does not exist", t.Zoomed)
	}
	if t.Active != "" && !inTree[t.Active] {
		return invalid("active pane %q does not exist", t.Active)
	}
	return nil
}

func checkNode(n *Node, depth int, seen map[string]bool) error {
	if n == nil {
		return nil
	}
	if depth > MaxPanes {
		return invalid("tree too deep")
	}
	if n.Split == "" {
		if n.Pane == "" || n.A != nil || n.B != nil {
			return invalid("leaf must name a pane and nothing else")
		}
		if seen[n.Pane] {
			return invalid("pane %q in the tree twice", n.Pane)
		}
		seen[n.Pane] = true
		return nil
	}
	if n.Split != "right" && n.Split != "down" {
		return invalid("split %q", n.Split)
	}
	if n.Pane != "" || n.A == nil || n.B == nil {
		return invalid("split must have two children and no pane")
	}
	if !(n.Ratio >= MinRatio && n.Ratio <= MaxRatio) {
		return invalid("ratio %v out of range", n.Ratio)
	}
	if err := checkNode(n.A, depth+1, seen); err != nil {
		return err
	}
	return checkNode(n.B, depth+1, seen)
}

// orderedPanes returns every pane in document order: tabs in order, panes in
// tree order, then any listed but not in the tree by id.
func (d *Doc) orderedPanes(visit func(t *Tab, id string)) {
	for i := range d.Tabs {
		t := &d.Tabs[i]
		seen := map[string]bool{}
		for _, id := range leaves(t.Root) {
			seen[id] = true
			if _, ok := t.Panes[id]; ok {
				visit(t, id)
			}
		}
		var rest []string
		for id := range t.Panes {
			if !seen[id] {
				rest = append(rest, id)
			}
		}
		sort.Strings(rest)
		for _, id := range rest {
			visit(t, id)
		}
	}
}

// Sessions lists the non-empty session ids in document order.
func (d Doc) Sessions() []string {
	var out []string
	d.orderedPanes(func(t *Tab, id string) {
		if s := t.Panes[id].Session; s != "" {
			out = append(out, s)
		}
	})
	return out
}

// ClearSession empties Session on every pane that has it, reporting whether
// anything changed.
func (d *Doc) ClearSession(session string) bool {
	if session == "" {
		return false
	}
	changed := false
	d.orderedPanes(func(t *Tab, id string) {
		if p := t.Panes[id]; p.Session == session {
			p.Session = ""
			t.Panes[id] = p
			changed = true
		}
	})
	return changed
}

// RepointHost moves every pane on machine from to machine to, reporting
// whether anything changed.
func (d *Doc) RepointHost(from, to string) bool {
	if from == to {
		return false
	}
	changed := false
	d.orderedPanes(func(t *Tab, id string) {
		if p := t.Panes[id]; p.Host == from {
			p.Host = to
			t.Panes[id] = p
			changed = true
		}
	})
	return changed
}
