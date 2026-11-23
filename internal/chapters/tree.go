// Package chapters builds and validates the CHAP/CTOC hierarchy of an ID3 tag.
package chapters

import "id3chap/internal/id3"

// Node is either a chapter (CHAP) or a table-of-contents (CTOC).
type Node struct {
	Frame     *id3.Frame
	Kind      string // "chapter" or "toc"
	ID        string
	Title     string
	Children  []*Node
	InCycle   bool
	Orphan    bool
	SpanStart uint32
	SpanEnd   uint32
}

// Tree holds the element graph and its derived structure.
type Tree struct {
	Nodes       map[string]*Node
	Roots       []*Node
	DuplicateID []string
	CycleEdges  [][2]string
}

// Build constructs the element graph from CHAP/CTOC frames in their tag order.
func Build(frames []*id3.Frame) *Tree {
	t := &Tree{Nodes: map[string]*Node{}}
	seen := map[string]bool{}

	for _, f := range frames {
		if f.ID != "CHAP" && f.ID != "CTOC" {
			continue
		}
		id := f.ElementID
		if id == "" {
			continue
		}
		if seen[id] {
			if !contains(t.DuplicateID, id) {
				t.DuplicateID = append(t.DuplicateID, id)
			}
			continue
		}
		seen[id] = true
		kind := "chapter"
		if f.ID == "CTOC" {
			kind = "toc"
		}
		t.Nodes[id] = &Node{Frame: f, Kind: kind, ID: id, Title: f.Title}
	}

	hasParent := map[string]bool{}
	for _, n := range t.Nodes {
		if n.Kind != "toc" {
			continue
		}
		for _, cid := range n.Frame.ChildIDs {
			child, ok := t.Nodes[cid]
			if !ok {
				continue // reported as dangling reference during validation
			}
			n.Children = append(n.Children, child)
			hasParent[cid] = true
		}
	}

	t.detectCycles()

	for _, f := range frames {
		id := f.ElementID
		if id == "" {
			continue
		}
		if n, ok := t.Nodes[id]; ok && !hasParent[id] {
			t.Roots = append(t.Roots, n)
		}
	}
	return t
}

func (t *Tree) detectCycles() {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var dfs func(id string)
	dfs = func(id string) {
		color[id] = gray
		stack = append(stack, id)
		n := t.Nodes[id]
		for _, cid := range n.Frame.ChildIDs {
			child, ok := t.Nodes[cid]
			if !ok {
				continue
			}
			switch color[cid] {
			case white:
				dfs(cid)
			case gray:
				t.CycleEdges = append(t.CycleEdges, [2]string{id, cid})
				child.InCycle = true
				n.InCycle = true
			}
		}
		color[id] = black
		stack = stack[:len(stack)-1]
	}
	for id := range t.Nodes {
		if color[id] == white {
			dfs(id)
		}
	}
	// Mark every node participating in a back-edge closure.
	for _, e := range t.CycleEdges {
		t.markCyclePath(e[0], e[1], map[string]bool{})
	}
}

func (t *Tree) markCyclePath(from, to string, visited map[string]bool) {
	if visited[from] {
		return
	}
	visited[from] = true
	if n, ok := t.Nodes[from]; ok {
		n.InCycle = true
		if from != to {
			for _, cid := range n.Frame.ChildIDs {
				if _, ok := t.Nodes[cid]; ok {
					t.markCyclePath(cid, to, visited)
				}
			}
		}
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
