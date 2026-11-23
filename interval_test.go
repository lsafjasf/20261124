package subtime

import "testing"

func TestMapIntervalTailUnderOneFrame(t *testing.T) {
	interval := Interval{
		ID:    "short-tail",
		Start: Point{Rate: Rate25, Position: NewRational(100, 1)},
		End:   Point{Rate: Rate25, Position: NewRational(201, 2)},
	}
	mapping, err := MapInterval(interval, Rate2997DF)
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(mapping.Issues, IssueTailUnderOneFrame) {
		t.Fatalf("issues = %+v", mapping.Issues)
	}
	if mapping.Start.IsFrame() || mapping.End.IsFrame() {
		t.Fatalf("cross-frame points must stay fractional: %s -> %s", mapping.Start.Position, mapping.End.Position)
	}
	if len(mapping.StartCandidates) != 2 || len(mapping.EndCandidates) != 2 {
		t.Fatalf("expected candidates for both points")
	}
}

func TestResolveExplainsCollapseAndExplicitMinimum(t *testing.T) {
	interval := Interval{
		Start: Point{Rate: Rate25, Position: NewRational(100, 1)},
		End:   Point{Rate: Rate25, Position: NewRational(201, 2)},
	}
	mapping, err := MapInterval(interval, Rate2997DF)
	if err != nil {
		t.Fatal(err)
	}
	policy := IntervalPolicy{Start: SnapCeil, End: SnapFloor}
	if _, err := mapping.Resolve(policy); err == nil {
		t.Fatal("collapsing snap should require an explicit decision")
	}
	policy.EnsureOneFrame = true
	resolved, err := mapping.Resolve(policy)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.AdjustedEnd || resolved.End.Number != resolved.Start.Number+1 {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestDuplicateIntervalsArePreserved(t *testing.T) {
	start := Point{Rate: Rate23976, Position: NewRational(24000, 1)}
	end := Point{Rate: Rate23976, Position: NewRational(24024, 1)}
	input := []Interval{
		{ID: "a", Start: start, End: end},
		{ID: "b", Start: start, End: end},
	}
	output, err := MapIntervals(input, Rate2997DF)
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 2 || output[0].Source.ID != "a" || output[1].Source.ID != "b" {
		t.Fatalf("duplicates were not preserved: %+v", output)
	}
	if output[0].Start.Position.Cmp(output[1].Start.Position) != 0 ||
		output[0].End.Position.Cmp(output[1].End.Position) != 0 {
		t.Fatalf("identical inputs produced different exact outputs")
	}
}

func TestIntervalAcrossWholeHour(t *testing.T) {
	start := Frame{Number: 107891, Rate: Rate2997DF}
	end := Frame{Number: 107892, Rate: Rate2997DF}
	startPoint, _ := PointFromFrame(start)
	endPoint, _ := PointFromFrame(end)
	mapping, err := MapInterval(Interval{Start: startPoint, End: endPoint}, Rate25)
	if err != nil {
		t.Fatal(err)
	}
	startBack, _ := mapping.Start.At(Rate2997DF)
	endBack, _ := mapping.End.At(Rate2997DF)
	gotStart, ok := startBack.ExactFrame()
	if !ok || gotStart.Number != start.Number {
		t.Fatalf("hour start round trip = %s", startBack.Position)
	}
	gotEnd, ok := endBack.ExactFrame()
	if !ok || gotEnd.Number != end.Number {
		t.Fatalf("hour end round trip = %s", endBack.Position)
	}
}

func TestZeroLengthDuplicatePointIsReported(t *testing.T) {
	point := Point{Rate: Rate25, Position: NewRational(90000, 1)}
	mapping, err := MapInterval(Interval{Start: point, End: point}, Rate2997DF)
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(mapping.Issues, IssueZeroLength) {
		t.Fatalf("issues = %+v", mapping.Issues)
	}
}

func hasIssue(issues []IntervalIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
