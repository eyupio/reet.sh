package desk

import "math"

// leaves lists the pane ids of a tree, left to right and top to bottom.
func leaves(n *Node) []string {
	if n == nil {
		return nil
	}
	if n.Split == "" {
		return []string{n.Pane}
	}
	return append(leaves(n.A), leaves(n.B)...)
}

// Split puts a new leaf next to pane, as the B child at ratio 0.5. It only
// edits the tree; the caller adds newPane to Panes.
func (t *Tab) Split(pane, dir, newPane string) error {
	if dir != "right" && dir != "down" {
		return invalid("split %q", dir)
	}
	ids := leaves(t.Root)
	if len(ids) >= MaxPanes {
		return invalid("a tab holds at most %d panes", MaxPanes)
	}
	if newPane == "" {
		return invalid("new pane needs an id")
	}
	found := false
	for _, id := range ids {
		if id == newPane {
			return invalid("pane %q already in the tab", newPane)
		}
		if id == pane {
			found = true
		}
	}
	if !found {
		return invalid("pane %q not in the tab", pane)
	}
	n := findLeaf(t.Root, pane)
	old := *n
	*n = Node{Split: dir, Ratio: 0.5, A: &old, B: &Node{Pane: newPane}}
	return nil
}

func findLeaf(n *Node, pane string) *Node {
	if n == nil {
		return nil
	}
	if n.Split == "" {
		if n.Pane == pane {
			return n
		}
		return nil
	}
	if f := findLeaf(n.A, pane); f != nil {
		return f
	}
	return findLeaf(n.B, pane)
}

// Close removes a pane; its sibling takes the space. Closing the last pane
// leaves Root nil. The pane leaves Panes, and Zoomed and Active move off it.
func (t *Tab) Close(pane string) error {
	if findLeaf(t.Root, pane) == nil {
		return invalid("pane %q not in the tab", pane)
	}
	var sibling *Node
	t.Root, sibling = remove(t.Root, pane)
	delete(t.Panes, pane)
	if t.Zoomed == pane {
		t.Zoomed = ""
	}
	if t.Active == pane {
		t.Active = ""
		if sibling != nil {
			t.Active = leaves(sibling)[0]
		}
	}
	return nil
}

// remove returns the tree without the leaf, and the sibling that took its
// space (nil when the tree held only that leaf).
func remove(n *Node, pane string) (tree, sibling *Node) {
	if n.Split == "" {
		return nil, nil
	}
	if isLeaf(n.A, pane) {
		return n.B, n.B
	}
	if isLeaf(n.B, pane) {
		return n.A, n.A
	}
	if findLeaf(n.A, pane) != nil {
		n.A, sibling = remove(n.A, pane)
	} else {
		n.B, sibling = remove(n.B, pane)
	}
	return n, sibling
}

func isLeaf(n *Node, pane string) bool { return n.Split == "" && n.Pane == pane }

// Swap exchanges the places of two panes in the tree.
func (t *Tab) Swap(a, b string) error {
	na, nb := findLeaf(t.Root, a), findLeaf(t.Root, b)
	if na == nil || nb == nil {
		return invalid("pane not in the tab")
	}
	na.Pane, nb.Pane = nb.Pane, na.Pane
	return nil
}

// Preset rebuilds the tree from the panes in their present order. Names:
// "side" (columns), "stack" (rows), "grid" (ceil(sqrt n) columns), "main"
// (first pane left at 0.6, the rest stacked) and "even" (keep the shape, give
// every pane an equal share).
func (t *Tab) Preset(name string) error {
	ids := leaves(t.Root)
	switch name {
	case "side", "stack", "grid", "main", "even":
	default:
		return invalid("preset %q", name)
	}
	if len(ids) == 0 {
		return nil
	}
	switch name {
	case "side":
		t.Root = chain("right", ids)
	case "stack":
		t.Root = chain("down", ids)
	case "main":
		if len(ids) == 1 {
			t.Root = leaf1(ids[0])
		} else {
			t.Root = &Node{Split: "right", Ratio: 0.6, A: leaf1(ids[0]), B: chain("down", ids[1:])}
		}
	case "grid":
		cols := int(math.Ceil(math.Sqrt(float64(len(ids)))))
		base, extra := len(ids)/cols, len(ids)%cols
		var groups [][]string
		for c, at := 0, 0; c < cols; c++ {
			n := base
			if c < extra {
				n++
			}
			if n > 0 {
				groups = append(groups, ids[at:at+n])
			}
			at += n
		}
		nodes := make([]*Node, len(groups))
		for i, g := range groups {
			nodes[i] = chain("down", g)
		}
		t.Root = chainNodes("right", nodes)
	case "even":
		evenTree(t.Root)
	}
	return nil
}

func leaf1(id string) *Node { return &Node{Pane: id} }

func chain(dir string, ids []string) *Node {
	nodes := make([]*Node, len(ids))
	for i, id := range ids {
		nodes[i] = leaf1(id)
	}
	return chainNodes(dir, nodes)
}

// chainNodes nests nodes so each gets an equal share along dir.
func chainNodes(dir string, nodes []*Node) *Node {
	if len(nodes) == 1 {
		return nodes[0]
	}
	return &Node{Split: dir, Ratio: 1 / float64(len(nodes)), A: nodes[0], B: chainNodes(dir, nodes[1:])}
}

// evenTree sets each ratio to A's share of the panes below the split.
func evenTree(n *Node) {
	if n == nil || n.Split == "" {
		return
	}
	a, b := len(leaves(n.A)), len(leaves(n.B))
	n.Ratio = Clamp(float64(a) / float64(a+b))
	evenTree(n.A)
	evenTree(n.B)
}
