package chapters

import (
	"sort"

	"id3chap/internal/id3"
)

// Issue categories.
const (
	IssueCycle            = "cycle"
	IssueDuplicateChapter = "duplicate_chapter"
	IssueOutOfRange       = "out_of_audio_range"
	IssueDanglingRef      = "dangling_reference"
	IssueBadTiming        = "invalid_timing"
	IssueNestedOrder      = "nested_order"
	IssueDuplicateTitle   = "duplicate_title"
	IssueOrphan           = "multiple_roots"
)

// Issue is one validation finding.
type Issue struct {
	Category string `json:"category"`
	Element  string `json:"element"`
	Ref      string `json:"ref,omitempty"`
	Message  string `json:"message"`
}

// Report is the full inspect output.
type Report struct {
	Version       string         `json:"version"`
	AudioDuration uint32         `json:"audio_duration_ms"`
	DurationFrom  string         `json:"duration_source"`
	FrameCount    int            `json:"frame_count"`
	UnknownFrames []string       `json:"unknown_frames"`
	Tree          []*NodeView    `json:"tree"`
	Issues        []Issue        `json:"issues"`
	IssueCounts   map[string]int `json:"issue_counts"`
}

// NodeView is a JSON-friendly recursive view of a node.
type NodeView struct {
	Kind     string      `json:"kind"`
	ID       string      `json:"id"`
	Title    string      `json:"title,omitempty"`
	StartMS  uint32      `json:"start_ms,omitempty"`
	EndMS    uint32      `json:"end_ms,omitempty"`
	InCycle  bool        `json:"in_cycle,omitempty"`
	Children []*NodeView `json:"children,omitempty"`
}

var knownFrames = map[string]bool{
	"CHAP": true, "CTOC": true, "TIT2": true,
}

// Validate runs all checks and renders the report.
func Validate(tag *id3.Tag, durationMS uint32, durationFrom string) *Report {
	tree := Build(tag.Frames)
	r := &Report{
		Version:       tag.Version.String(),
		AudioDuration: durationMS,
		DurationFrom:  durationFrom,
		FrameCount:    len(tag.Frames),
		IssueCounts:   map[string]int{},
	}

	for _, f := range tag.Frames {
		if !knownFrames[f.ID] {
			r.UnknownFrames = append(r.UnknownFrames, f.ID)
		}
	}

	issues := r.checkReferences(tree)
	issues = append(issues, r.checkDuplicates(tag, tree)...)
	issues = append(issues, r.checkTiming(tree, durationMS)...)
	issues = append(issues, r.checkNesting(tree)...)

	visited := map[string]bool{}
	for _, root := range tree.Roots {
		r.Tree = append(r.Tree, renderNode(root, visited, 0))
	}
	// Nodes never reached from any root (e.g. inside a cycle with no entry).
	for _, n := range tree.Nodes {
		if !visited[n.ID] {
			r.Tree = append(r.Tree, renderNode(n, visited, 0))
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Category != issues[j].Category {
			return issues[i].Category < issues[j].Category
		}
		return issues[i].Element < issues[j].Element
	})
	for _, is := range issues {
		r.IssueCounts[is.Category]++
	}
	r.Issues = issues
	return r
}

func (r *Report) add(out *[]Issue, category, element, ref, msg string) {
	*out = append(*out, Issue{Category: category, Element: element, Ref: ref, Message: msg})
}

func (r *Report) checkReferences(tree *Tree) []Issue {
	var out []Issue
	for _, e := range tree.CycleEdges {
		r.add(&out, IssueCycle, e[0], e[1], "child reference forms a cycle")
	}
	for id, n := range tree.Nodes {
		if n.Kind != "toc" {
			continue
		}
		for _, cid := range n.Frame.ChildIDs {
			if _, ok := tree.Nodes[cid]; !ok {
				r.add(&out, IssueDanglingRef, id, cid, "CTOC references an unknown element id")
			}
		}
	}
	if len(tree.Roots) > 1 {
		for _, n := range tree.Roots[1:] {
			r.add(&out, IssueOrphan, n.ID, "", "element has no parent and is not the first root")
		}
	}
	return out
}

func (r *Report) checkDuplicates(tag *id3.Tag, tree *Tree) []Issue {
	var out []Issue
	for _, id := range tree.DuplicateID {
		r.add(&out, IssueDuplicateChapter, id, "", "duplicate element id; later frame ignored")
	}
	seenTitle := map[string][]string{}
	for _, n := range tree.Nodes {
		if n.Kind == "chapter" && n.Title != "" {
			seenTitle[n.Title] = append(seenTitle[n.Title], n.ID)
		}
	}
	titles := make([]string, 0, len(seenTitle))
	for t := range seenTitle {
		titles = append(titles, t)
	}
	sort.Strings(titles)
	for _, t := range titles {
		if len(seenTitle[t]) > 1 {
			r.add(&out, IssueDuplicateTitle, seenTitle[t][0], t, "multiple chapters share the same TIT2")
		}
	}
	return out
}

func (r *Report) checkTiming(tree *Tree, durationMS uint32) []Issue {
	var out []Issue
	for _, n := range tree.Nodes {
		f := n.Frame
		if n.Kind == "chapter" {
			if f.Start > f.End {
				r.add(&out, IssueBadTiming, n.ID, "", "start_ms is greater than end_ms")
			}
			n.SpanStart, n.SpanEnd = f.Start, f.End
		} else {
			computeSpan(n, map[string]bool{})
		}
		if durationMS > 0 && n.SpanEnd > durationMS {
			r.add(&out, IssueOutOfRange, n.ID, "", "timing extends beyond audio duration")
		}
	}
	return out
}

func computeSpan(n *Node, active map[string]bool) {
	if active[n.ID] || n.InCycle {
		return
	}
	active[n.ID] = true
	var lo, hi uint32
	first := true
	for _, c := range n.Children {
		if c.Kind != "toc" {
			if c.Frame.Start == 0 && c.Frame.End == 0 {
				continue
			}
		}
		if c.Kind == "toc" && c.SpanEnd == 0 && c.SpanStart == 0 {
			computeSpan(c, active)
		}
		s, e := c.effectiveSpan()
		if first {
			lo, hi = s, e
			first = false
			continue
		}
		if s < lo {
			lo = s
		}
		if e > hi {
			hi = e
		}
	}
	n.SpanStart, n.SpanEnd = lo, hi
}

func (n *Node) effectiveSpan() (uint32, uint32) {
	if n.Kind == "chapter" {
		return n.Frame.Start, n.Frame.End
	}
	return n.SpanStart, n.SpanEnd
}

func (r *Report) checkNesting(tree *Tree) []Issue {
	var out []Issue
	for _, n := range tree.Nodes {
		if n.Kind != "toc" || n.InCycle {
			continue
		}
		var prevEnd uint32
		havePrev := false
		for _, c := range n.Children {
			s, e := c.effectiveSpan()
			if havePrev && s < prevEnd {
				r.add(&out, IssueNestedOrder, n.ID, c.ID, "ordered children overlap or are out of order")
			}
			if e > prevEnd {
				prevEnd = e
			}
			havePrev = true
			// Nested containment: child must lie inside parent span.
			if (n.SpanStart != 0 || n.SpanEnd != 0) && (s < n.SpanStart || e > n.SpanEnd) {
				r.add(&out, IssueNestedOrder, c.ID, n.ID, "child timing is not contained within parent TOC span")
			}
		}
	}
	return out
}

func renderNode(n *Node, visited map[string]bool, depth int) *NodeView {
	v := &NodeView{Kind: n.Kind, ID: n.ID, Title: n.Title, InCycle: n.InCycle}
	if n.Kind == "chapter" {
		v.StartMS, v.EndMS = n.Frame.Start, n.Frame.End
	} else {
		v.StartMS, v.EndMS = n.SpanStart, n.SpanEnd
	}
	if visited[n.ID] || depth > 64 {
		return v
	}
	visited[n.ID] = true
	for _, c := range n.Children {
		v.Children = append(v.Children, renderNode(c, visited, depth+1))
	}
	return v
}
