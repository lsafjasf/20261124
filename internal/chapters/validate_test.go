package chapters

import (
	"testing"

	"id3chap/internal/id3"
)

func frame(id string) *id3.Frame {
	return &id3.Frame{ID: id, Version: id3.Version{Major: 4}}
}

func chap(id, title string, start, end uint32) *id3.Frame {
	f := frame("CHAP")
	f.ElementID, f.Title, f.Start, f.End = id, title, start, end
	return f
}

func toc(id string, children ...string) *id3.Frame {
	f := frame("CTOC")
	f.ElementID, f.ChildIDs = id, children
	return f
}

func issueMap(r *Report) map[string][]Issue {
	m := map[string][]Issue{}
	for _, is := range r.Issues {
		m[is.Category] = append(m[is.Category], is)
	}
	return m
}

func TestHealthyTree(t *testing.T) {
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		toc("toc", "c1", "c2"),
		chap("c1", "Intro", 0, 500),
		chap("c2", "Main", 500, 1500),
	}}
	r := Validate(tag, 2000, "test")
	if len(r.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", r.Issues)
	}
	if len(r.Tree) != 1 || r.Tree[0].ID != "toc" || len(r.Tree[0].Children) != 2 {
		t.Fatalf("tree shape wrong: %+v", r.Tree)
	}
}

func TestCycleDetected(t *testing.T) {
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		toc("a", "b"),
		toc("b", "a"),
	}}
	r := Validate(tag, 1000, "test")
	m := issueMap(r)
	if len(m[IssueCycle]) == 0 {
		t.Fatalf("cycle not detected: %+v", r.Issues)
	}
}

func TestDuplicatesAndOutOfRange(t *testing.T) {
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		toc("toc", "c1", "c2"),
		chap("c1", "Same", 0, 500),
		chap("c2", "Same", 500, 5000), // dup title + past duration
		chap("c2", "Other", 0, 10),    // duplicate element id
	}}
	r := Validate(tag, 2000, "test")
	m := issueMap(r)
	if len(m[IssueDuplicateChapter]) != 1 || m[IssueDuplicateChapter][0].Element != "c2" {
		t.Fatalf("dup chapter: %+v", m[IssueDuplicateChapter])
	}
	if len(m[IssueDuplicateTitle]) != 1 {
		t.Fatalf("dup title: %+v", m[IssueDuplicateTitle])
	}
	if len(m[IssueOutOfRange]) == 0 {
		t.Fatalf("out of range not detected: %+v", r.Issues)
	}
}

func TestDanglingAndNesting(t *testing.T) {
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		toc("toc", "c1", "ghost", "c2"),
		chap("c1", "A", 100, 500),
		chap("c2", "B", 300, 800), // overlaps c1 -> ordering issue
	}}
	r := Validate(tag, 2000, "test")
	m := issueMap(r)
	if len(m[IssueDanglingRef]) != 1 || m[IssueDanglingRef][0].Ref != "ghost" {
		t.Fatalf("dangling: %+v", m[IssueDanglingRef])
	}
	if len(m[IssueNestedOrder]) == 0 {
		t.Fatalf("nesting order not detected: %+v", r.Issues)
	}
}

func TestInvalidTiming(t *testing.T) {
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		chap("c1", "A", 900, 100),
	}}
	r := Validate(tag, 2000, "test")
	m := issueMap(r)
	if len(m[IssueBadTiming]) != 1 {
		t.Fatalf("bad timing: %+v", r.Issues)
	}
}

func TestUnknownFramesListed(t *testing.T) {
	u := frame("TXXX")
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		u, chap("c1", "A", 0, 10),
	}}
	r := Validate(tag, 2000, "test")
	if len(r.UnknownFrames) != 1 || r.UnknownFrames[0] != "TXXX" {
		t.Fatalf("unknown frames: %v", r.UnknownFrames)
	}
}

func TestNestedTOCSpanAndContainment(t *testing.T) {
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		toc("root", "inner", "c3"),
		toc("inner", "c1", "c2"),
		chap("c1", "A", 0, 400),
		chap("c2", "B", 400, 800),
		chap("c3", "C", 800, 1200),
	}}
	r := Validate(tag, 5000, "test")
	if len(r.Issues) != 0 {
		t.Fatalf("healthy nested tree flagged: %+v", r.Issues)
	}
	root := r.Tree[0]
	if root.ID != "root" || root.StartMS != 0 || root.EndMS != 1200 {
		t.Fatalf("root span: %+v", root)
	}
	if root.Children[0].ID != "inner" || root.Children[0].EndMS != 800 {
		t.Fatalf("inner span: %+v", root.Children[0])
	}
}

func TestChildEscapesParentSpan(t *testing.T) {
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		toc("root", "c1", "c2"),
		chap("c1", "A", 0, 100),
		chap("c2", "B", 50, 900), // overlaps c1 and widens beyond sibling order
	}}
	r := Validate(tag, 5000, "test")
	m := issueMap(r)
	if len(m[IssueNestedOrder]) == 0 {
		t.Fatalf("expected nested-order issue: %+v", r.Issues)
	}
}

func TestZeroTimingNotOutOfRange(t *testing.T) {
	tag := &id3.Tag{Version: id3.Version{Major: 4}, Frames: []*id3.Frame{
		chap("c1", "A", 0, 0),
	}}
	r := Validate(tag, 1000, "test")
	if _, ok := issueMap(r)[IssueOutOfRange]; ok {
		t.Fatalf("zero-length chapter wrongly flagged: %+v", r.Issues)
	}
}
