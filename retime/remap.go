package main

import (
	"fmt"
	"sort"
)

func candidatesOf(pts []frameRat, why string) []Candidate {
	frames := make([]int64, 0, len(pts))
	for _, p := range pts {
		frames = append(frames, p.roundNearest())
	}
	frames = dedupInt64(frames)
	out := make([]Candidate, 0, len(frames))
	for _, f := range frames {
		out = append(out, Candidate{Frame: f, Why: why})
	}
	return out
}

// Remap maps every event from old (source) positions to new timeline positions.
// Unique results are applied; ambiguous results are returned as pending candidates
// for manual confirmation.
func Remap(curve *Curve, events []Event) ([]MappedEvent, []MappedEvent, error) {
	var unique, pending []MappedEvent
	for _, ev := range events {
		m := MappedEvent{ID: ev.ID, Kind: ev.Kind}
		startPts, startWhy, err := curve.mapPoint(ev.Start)
		if err != nil {
			return nil, nil, fmt.Errorf("event %s: %v", ev.ID, err)
		}
		if ev.End == nil {
			finishPointEvent(&m, startPts, startWhy)
		} else {
			if *ev.End < ev.Start {
				return nil, nil, fmt.Errorf("event %s: end before start", ev.ID)
			}
			endPts, endWhy, err := curve.mapPoint(*ev.End)
			if err != nil {
				return nil, nil, fmt.Errorf("event %s: %v", ev.ID, err)
			}
			finishRangeEvent(&m, ev, startPts, endPts, startWhy, endWhy)
		}
		switch m.Status {
		case "unique":
			unique = append(unique, m)
		default:
			pending = append(pending, m)
		}
	}
	sortEvents(unique)
	sortEvents(pending)
	return unique, pending, nil
}

func finishPointEvent(m *MappedEvent, pts []frameRat, why string) {
	cands := candidatesOf(pts, why)
	if len(cands) == 1 {
		f := cands[0].Frame
		m.Status = "unique"
		m.NewStart = &f
		return
	}
	m.Status = "candidates"
	m.Candidates = cands
}

func finishRangeEvent(m *MappedEvent, ev Event, startPts, endPts []frameRat, startWhy, endWhy string) {
	startCands := candidatesOf(startPts, startWhy)
	endCands := candidatesOf(endPts, endWhy)
	warns := rangeWarnings(ev, startCands, endCands)
	if len(startCands) == 1 && len(endCands) == 1 {
		s, e := startCands[0].Frame, endCands[0].Frame
		m.Status = "unique"
		m.NewStart, m.NewEnd = &s, &e
		m.Warnings = warns
		return
	}
	m.Status = "candidates"
	m.Candidates = mergeCandidates(startCands, endCands)
	m.Warnings = warns
}

func rangeWarnings(ev Event, startCands, endCands []Candidate) []string {
	var warns []string
	if len(startCands) > 1 || len(endCands) > 1 {
		warns = append(warns, "endpoint lands in a freeze hold or reversal zone; position not unique")
	}
	if len(startCands) == 1 && len(endCands) == 1 && endCands[0].Frame < startCands[0].Frame {
		warns = append(warns, "range is time-reversed by negative speed and needs manual direction confirmation")
	}
	if ev.End != nil && *ev.End == ev.Start {
		warns = append(warns, "zero-length range treated as a point")
	}
	return warns
}

func mergeCandidates(a, b []Candidate) []Candidate {
	seen := map[int64]string{}
	for _, c := range a {
		seen[c.Frame] = "start:" + c.Why
	}
	for _, c := range b {
		if _, ok := seen[c.Frame]; !ok {
			seen[c.Frame] = "end:" + c.Why
		}
	}
	frames := make([]int64, 0, len(seen))
	for f := range seen {
		frames = append(frames, f)
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i] < frames[j] })
	out := make([]Candidate, 0, len(frames))
	for _, f := range frames {
		out = append(out, Candidate{Frame: f, Why: seen[f]})
	}
	return out
}

func sortEvents(es []MappedEvent) {
	sort.SliceStable(es, func(i, j int) bool {
		pi, pj := int64(0), int64(0)
		if es[i].NewStart != nil {
			pi = *es[i].NewStart
		} else if len(es[i].Candidates) > 0 {
			pi = es[i].Candidates[0].Frame
		}
		if es[j].NewStart != nil {
			pj = *es[j].NewStart
		} else if len(es[j].Candidates) > 0 {
			pj = es[j].Candidates[0].Frame
		}
		return pi < pj
	})
}
