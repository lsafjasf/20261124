package main

import (
	"reflect"
	"testing"
)

func i64p(v int64) *int64 { return &v }

func sampleCurve() []Segment {
	return []Segment{
		{Name: "normal", SrcStart: 0, Hold: 10, Speed: "1"},
		{Name: "fast", SrcStart: 10, Hold: 20, Speed: "2"},
		{Name: "hold", SrcStart: 30, Hold: 5, Speed: "0"},
		{Name: "reverse", SrcStart: 35, Hold: 10, Speed: "-1"},
		{Name: "slow", SrcStart: 45, Hold: 4, Speed: "1/2"},
	}
}

func TestBuildCurveDurations(t *testing.T) {
	c, err := BuildCurve(sampleCurve())
	if err != nil {
		t.Fatal(err)
	}
	// 10 + 10 + 5 + 10 + 8 = 43 frames
	if got := c.DurationFrames(); got != 43 {
		t.Fatalf("duration = %d, want 43", got)
	}
	// Exact boundary check: fast segment ends at 20, freeze 20..25.
	if got := c.segs[2].tlStart.r.RatString(); got != "20" {
		t.Fatalf("freeze start = %s, want 20", got)
	}
}

func TestVariableSpeedMapping(t *testing.T) {
	c, _ := BuildCurve(sampleCurve())
	// src 15 sits inside 2x segment: 10 + (15-10)/2 = 12.5 -> 13 frames.
	pts, why, err := c.mapPoint(15)
	if err != nil {
		t.Fatal(err)
	}
	if why != "variable-speed" || len(pts) != 1 {
		t.Fatalf("unexpected pts=%v why=%s", pts, why)
	}
	if got := pts[0].roundNearest(); got != 13 {
		t.Fatalf("mapped frame = %d, want 13", got)
	}
	// Fractional speed 1/2: src 47 -> timeline 35 + (47-45)/(1/2) = 39.
	pts, _, _ = c.mapPoint(47)
	if got := pts[0].roundNearest(); got != 39 {
		t.Fatalf("slow seg mapped = %d, want 39", got)
	}
}

func TestFreezeProducesCandidates(t *testing.T) {
	c, _ := BuildCurve(sampleCurve())
	// Freeze half-open covers source [30,35) and occupies timeline [20,25].
	for _, src := range []int64{30, 32, 34} {
		pts, why, err := c.mapPoint(src)
		if err != nil {
			t.Fatal(err)
		}
		if why != "freeze-hold-start" || len(pts) != 2 {
			t.Fatalf("src %d: pts=%v why=%s", src, pts, why)
		}
		got := []int64{pts[0].roundNearest(), pts[1].roundNearest()}
		if !reflect.DeepEqual(got, []int64{20, 25}) {
			t.Fatalf("src %d candidates = %v, want [20 25]", src, got)
		}
	}
}

func TestNegativeSpeedMapping(t *testing.T) {
	c, _ := BuildCurve(sampleCurve())
	// Reverse segment src 35..45 on timeline 25..35. src 40 -> 25 + (45-40) = 30.
	pts, why, err := c.mapPoint(40)
	if err != nil {
		t.Fatal(err)
	}
	if why != "negative-speed" {
		t.Fatalf("why = %s", why)
	}
	if got := pts[0].roundNearest(); got != 30 {
		t.Fatalf("reverse mapped = %d, want 30", got)
	}
}

func TestRemapStatuses(t *testing.T) {
	c, _ := BuildCurve(sampleCurve())
	events := []Event{
		{ID: "sub1", Kind: "subtitle", Start: 5, End: i64p(9)},   // unique
		{ID: "mk1", Kind: "marker", Start: 33},                   // freeze -> candidates
		{ID: "sub2", Kind: "subtitle", Start: 40, End: i64p(44)}, // unique reversed
		{ID: "mk2", Kind: "marker", Start: 15},                   // unique 12.5->13
	}
	unique, pending, err := Remap(c, events)
	if err != nil {
		t.Fatal(err)
	}
	if len(unique) != 3 || len(pending) != 1 {
		t.Fatalf("unique=%d pending=%d", len(unique), len(pending))
	}
	p := pending[0]
	if p.ID != "mk1" || len(p.Candidates) != 2 ||
		p.Candidates[0].Frame != 20 || p.Candidates[1].Frame != 25 {
		t.Fatalf("bad pending: %+v", p)
	}
	byID := map[string]MappedEvent{}
	for _, u := range unique {
		byID[u.ID] = u
	}
	if *byID["mk2"].NewStart != 13 {
		t.Fatalf("mk2 = %+v", byID["mk2"])
	}
	// Reversed range: start 40 -> 30, end 44 -> 26, warning emitted.
	s2 := byID["sub2"]
	if *s2.NewStart != 30 || *s2.NewEnd != 26 || len(s2.Warnings) == 0 {
		t.Fatalf("sub2 = %+v", s2)
	}
}

func TestUndoRestoresEverything(t *testing.T) {
	p := Project{Sequence: "seq1", SpeedCurve: sampleCurve(), Events: []Event{
		{ID: "mk1", Kind: "marker", Start: 33},
		{ID: "sub1", Kind: "subtitle", Start: 0, End: i64p(10)},
	}}
	m, err := ApplyProject(p)
	if err != nil {
		t.Fatal(err)
	}
	restored := Undo(m)
	if !reflect.DeepEqual(restored.SpeedCurve, p.SpeedCurve) {
		t.Fatal("speed curve not restored exactly")
	}
	if !reflect.DeepEqual(restored.Events, p.Events) {
		t.Fatal("events not restored exactly")
	}
}

func TestConfirmResolvesPending(t *testing.T) {
	p := Project{Sequence: "seq1", SpeedCurve: sampleCurve(), Events: []Event{
		{ID: "mk1", Kind: "marker", Start: 33},
	}}
	m, _ := ApplyProject(p)
	if len(m.Pending) != 1 {
		t.Fatalf("pending = %d", len(m.Pending))
	}
	m2, err := Confirm(m, []Confirmation{{ID: "mk1", NewStart: 25}})
	if err != nil {
		t.Fatal(err)
	}
	if len(m2.Pending) != 0 || len(m2.UniqueMapped) != 1 ||
		*m2.UniqueMapped[0].NewStart != 25 {
		t.Fatalf("confirm result wrong: %+v", m2)
	}
	// Reject a frame that was not a candidate.
	if _, err := Confirm(m, []Confirmation{{ID: "mk1", NewStart: 99}}); err == nil {
		t.Fatal("expected rejection of invalid candidate")
	}
}

func TestOutsideCurveRejected(t *testing.T) {
	c, _ := BuildCurve(sampleCurve())
	if _, _, err := c.mapPoint(999); err == nil {
		t.Fatal("expected error for source frame outside curve")
	}
}

func TestRejectsBadCurve(t *testing.T) {
	bad := []Segment{
		{Name: "x", SrcStart: 0, Hold: 0, Speed: "1"},
	}
	if _, err := BuildCurve(bad); err == nil {
		t.Fatal("expected zero-hold rejection")
	}
	bad[0].Hold = 10
	bad[0].Speed = "nope"
	if _, err := BuildCurve(bad); err == nil {
		t.Fatal("expected bad-speed rejection")
	}
}
