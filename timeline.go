package edl

import (
	"fmt"
	"sort"
	"strings"
)

// CutPoint is one record-timeline boundary. Frame is relative to the earliest
// record-in point in the file, which is always 0.
type CutPoint struct {
	Frame         int64  `json:"frame"`
	AbsoluteFrame int64  `json:"absoluteRecordFrame"`
	Timecode      string `json:"unwrappedRecordTimecode"`
	RawTimecode   string `json:"recordTimecode"`
	Event         int    `json:"event"`
	Edge          string `json:"edge"`
	Reel          string `json:"reel"`
	Track         string `json:"track"`
}

type FrameRange struct {
	In    int64 `json:"inFrame"`
	Out   int64 `json:"outFrame"`
	Frame int64 `json:"frames"`
}

type TrackTimeline struct {
	Name          string       `json:"name"`
	EventIndexes  []int        `json:"eventIndexes"`
	FirstCutFrame int64        `json:"firstCutFrame"`
	LastCutFrame  int64        `json:"lastCutFrame"`
	SpanFrames    int64        `json:"spanFrames"`
	SumEventFrame int64        `json:"sumEventFrames"`
	Gaps          []FrameRange `json:"gaps,omitempty"`
	Overlaps      []FrameRange `json:"overlaps,omitempty"`
	CutFrames     []int64      `json:"cutFrames"`
}

type LostTransition struct {
	File           string `json:"file"`
	Event          int    `json:"event"`
	Line           int    `json:"line"`
	Reel           string `json:"reel"`
	Track          string `json:"track"`
	Code           string `json:"code"`
	Class          string `json:"class"`
	DurationFrames int64  `json:"durationFrames,omitempty"`
	Reason         string `json:"reason"`
	Action         string `json:"action"`
}

type Validation struct {
	FirstCutAtZero        bool     `json:"firstCutAtZero"`
	LastCutAtTotal        bool     `json:"lastCutAtTotal"`
	SpanMatchesCutPoints  bool     `json:"spanMatchesCutPoints"`
	ContinuityWithinTrack bool     `json:"continuityWithinTrack"`
	SourceRecordDurations bool     `json:"sourceRecordDurationsMatch"`
	Issues                []string `json:"issues,omitempty"`
}

type FileTimeline struct {
	Path            string           `json:"path"`
	Title           string           `json:"title"`
	SourceLines     int              `json:"sourceLines"`
	FrameRate       string           `json:"frameRate"`
	FPS             string           `json:"fps"`
	Mode            TimecodeMode     `json:"mode"`
	MixedModes      bool             `json:"mixedModes"`
	FirstCut        CutPoint         `json:"firstCut"`
	LastCut         CutPoint         `json:"lastCut"`
	TotalFrames     int64            `json:"totalFrames"`
	SumEventFrames  int64            `json:"sumEventFrames"`
	TotalDuration   DurationInfo     `json:"totalDuration"`
	CutFrames       []int64          `json:"cutFrames"`
	RepeatedReels   []string         `json:"repeatedReels,omitempty"`
	Tracks          []TrackTimeline  `json:"tracks"`
	Events          []Event          `json:"events"`
	LostTransitions []LostTransition `json:"unsupportedTransitions,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
	Validation      Validation       `json:"validation"`
}

type DurationInfo struct {
	Rational string `json:"rational"`
	Seconds  string `json:"seconds"`
}

type BatchSummary struct {
	FileCount             int          `json:"fileCount"`
	TotalFrames           int64        `json:"totalFrames"`
	TotalDuration         DurationInfo `json:"totalDuration"`
	UnsupportedTransition int          `json:"unsupportedTransitions"`
	ValidationIssues      int          `json:"validationIssues"`
	RepeatedReels         []string     `json:"repeatedReels,omitempty"`
}

type BatchTimeline struct {
	Version int            `json:"version"`
	Files   []FileTimeline `json:"files"`
	Summary BatchSummary   `json:"summary"`
}

// BuildTimeline expands wrapped labels to absolute frames and places each
// event on the record timeline.
func BuildTimeline(f *EDLFile) (*FileTimeline, error) {
	if f == nil || len(f.Events) == 0 {
		return nil, fmt.Errorf("empty EDL")
	}
	events := append([]Event(nil), f.Events...)
	rate := events[0].RecordIn.Rate
	previousRecordOut := int64(0)
	for i := range events {
		ev := &events[i]
		if ev.RecordIn.Rate != rate || ev.SourceIn.Rate != rate {
			return nil, fmt.Errorf("event %d: mixed playback frame rates are not supported in one timeline", ev.Number)
		}
		ev.SourceInAbs = ev.SourceIn.Frames
		ev.SourceOutAbs = ev.SourceOut.Frames
		if ev.SourceOutAbs <= ev.SourceInAbs {
			ev.SourceOutAbs = unwrapForward(ev.SourceOutAbs, ev.SourceInAbs, ev.SourceOut.Mode, rate)
		}
		ev.SourceDurationFrames = ev.SourceOutAbs - ev.SourceInAbs
		if ev.SourceDurationFrames <= 0 {
			return nil, fmt.Errorf("event %d: source duration is not positive", ev.Number)
		}

		rawIn := ev.RecordIn.Frames
		if i > 0 && rawIn < previousRecordOut {
			day := framesPerDay(ev.RecordIn.Mode, rate)
			if previousRecordOut-rawIn > day/2 {
				for rawIn < previousRecordOut {
					rawIn += day
				}
			}
		}
		ev.RecordInAbs = rawIn
		ev.RecordOutAbs = ev.RecordOut.Frames
		if ev.RecordOutAbs <= ev.RecordInAbs {
			ev.RecordOutAbs = unwrapForward(ev.RecordOutAbs, ev.RecordInAbs, ev.RecordOut.Mode, rate)
		}
		ev.RecordDurationFrames = ev.RecordOutAbs - ev.RecordInAbs
		if ev.RecordDurationFrames <= 0 {
			return nil, fmt.Errorf("event %d: record duration is not positive", ev.Number)
		}
		ev.SourceRecordOffset = ev.RecordIn.Frames - ev.SourceIn.Frames
		ev.AbsoluteSRCOffset = ev.RecordInAbs - ev.SourceInAbs
		previousRecordOut = ev.RecordOutAbs
		if ev.Transition.DurationFrames < 0 {
			return nil, fmt.Errorf("event %d: negative transition duration", ev.Number)
		}
	}

	origin := events[0].RecordInAbs
	for _, ev := range events {
		if ev.RecordInAbs < origin {
			origin = ev.RecordInAbs
		}
	}
	var lastFrame int64
	for i := range events {
		ev := &events[i]
		ev.TimelineIn = ev.RecordInAbs - origin
		ev.TimelineOut = ev.RecordOutAbs - origin
		if ev.TimelineOut > lastFrame {
			lastFrame = ev.TimelineOut
		}
	}

	ft := &FileTimeline{
		Path:          f.Path,
		Title:         f.Title,
		SourceLines:   f.SourceLines,
		FrameRate:     rate.String(),
		FPS:           rate.Decimal(),
		Mode:          f.Mode,
		MixedModes:    f.MixedModes,
		TotalFrames:   lastFrame,
		TotalDuration: durationInfo(lastFrame, rate),
		Events:        events,
		RepeatedReels: repeatedReels(events),
		Warnings:      append([]string(nil), f.Warnings...),
	}
	ft.CutFrames, ft.FirstCut, ft.LastCut = collectCutPoints(events, rate)
	ft.Tracks = buildTracks(events)
	for _, track := range ft.Tracks {
		ft.SumEventFrames += track.SumEventFrame
	}
	ft.LostTransitions = lostTransitions(f.Path, events)
	ft.Validation = validateFile(ft, events)
	return ft, nil
}

func durationInfo(frames int64, rate Rate) DurationInfo {
	f := rate.SecondsPerFrame().Mul(NewFractionNoErr(frames, 1))
	return DurationInfo{
		Rational: f.String(),
		Seconds:  f.Decimal(9),
	}
}

func repeatedReels(events []Event) []string {
	counts := make(map[string]int)
	for _, ev := range events {
		counts[ev.Reel]++
	}
	var out []string
	for reel, count := range counts {
		if count > 1 {
			out = append(out, fmt.Sprintf("%s (%d)", reel, count))
		}
	}
	sort.Strings(out)
	return out
}

func collectCutPoints(events []Event, rate Rate) ([]int64, CutPoint, CutPoint) {
	type candidate struct {
		cut   CutPoint
		order int
	}
	var candidates []candidate
	seen := make(map[int64]bool)
	for _, ev := range events {
		inTC, _ := FromFrames(ev.RecordInAbs, rate, ev.RecordIn.Mode)
		if !seen[ev.TimelineIn] {
			candidates = append(candidates, candidate{
				order: len(candidates),
				cut: CutPoint{
					Frame:         ev.TimelineIn,
					AbsoluteFrame: ev.RecordInAbs,
					Timecode:      inTC.String(),
					RawTimecode:   ev.RecordIn.String(),
					Event:         ev.Number,
					Edge:          "in",
					Reel:          ev.Reel,
					Track:         ev.Track,
				},
			})
			seen[ev.TimelineIn] = true
		}
		outTC, _ := FromFrames(ev.RecordOutAbs, rate, ev.RecordOut.Mode)
		if !seen[ev.TimelineOut] {
			candidates = append(candidates, candidate{
				order: len(candidates),
				cut: CutPoint{
					Frame:         ev.TimelineOut,
					AbsoluteFrame: ev.RecordOutAbs,
					Timecode:      outTC.String(),
					RawTimecode:   ev.RecordOut.String(),
					Event:         ev.Number,
					Edge:          "out",
					Reel:          ev.Reel,
					Track:         ev.Track,
				},
			})
			seen[ev.TimelineOut] = true
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].cut.Frame != candidates[j].cut.Frame {
			return candidates[i].cut.Frame < candidates[j].cut.Frame
		}
		return candidates[i].order < candidates[j].order
	})
	frames := make([]int64, 0, len(candidates))
	for _, c := range candidates {
		frames = append(frames, c.cut.Frame)
	}
	if len(candidates) == 0 {
		return frames, CutPoint{}, CutPoint{}
	}
	return frames, candidates[0].cut, candidates[len(candidates)-1].cut
}

func buildTracks(events []Event) []TrackTimeline {
	indexes := make(map[string][]int)
	var names []string
	for i, ev := range events {
		track := strings.TrimSpace(ev.Track)
		if track == "" {
			track = "?"
		}
		if _, ok := indexes[track]; !ok {
			names = append(names, track)
		}
		indexes[track] = append(indexes[track], i)
	}
	tracks := make([]TrackTimeline, 0, len(names))
	for _, name := range names {
		idx := append([]int(nil), indexes[name]...)
		sort.SliceStable(idx, func(i, j int) bool {
			left, right := events[idx[i]], events[idx[j]]
			if left.TimelineIn != right.TimelineIn {
				return left.TimelineIn < right.TimelineIn
			}
			return left.TimelineOut < right.TimelineOut
		})
		tt := TrackTimeline{Name: name, EventIndexes: idx}
		seenCuts := make(map[int64]bool)
		for pos, eventIndex := range idx {
			ev := events[eventIndex]
			if pos == 0 {
				tt.FirstCutFrame = ev.TimelineIn
			}
			if ev.TimelineOut > tt.LastCutFrame {
				tt.LastCutFrame = ev.TimelineOut
			}
			tt.SumEventFrame += ev.RecordDurationFrames
			if !seenCuts[ev.TimelineIn] {
				tt.CutFrames = append(tt.CutFrames, ev.TimelineIn)
				seenCuts[ev.TimelineIn] = true
			}
			if !seenCuts[ev.TimelineOut] {
				tt.CutFrames = append(tt.CutFrames, ev.TimelineOut)
				seenCuts[ev.TimelineOut] = true
			}
			if pos > 0 {
				prev := events[idx[pos-1]]
				switch {
				case ev.TimelineIn > prev.TimelineOut:
					tt.Gaps = append(tt.Gaps, FrameRange{
						In:    prev.TimelineOut,
						Out:   ev.TimelineIn,
						Frame: ev.TimelineIn - prev.TimelineOut,
					})
				case ev.TimelineIn < prev.TimelineOut:
					tt.Overlaps = append(tt.Overlaps, FrameRange{
						In:    ev.TimelineIn,
						Out:   prev.TimelineOut,
						Frame: prev.TimelineOut - ev.TimelineIn,
					})
				}
			}
		}
		tt.SpanFrames = tt.LastCutFrame - tt.FirstCutFrame
		sort.Slice(tt.CutFrames, func(i, j int) bool { return tt.CutFrames[i] < tt.CutFrames[j] })
		tracks = append(tracks, tt)
	}
	return tracks
}

func lostTransitions(path string, events []Event) []LostTransition {
	var lost []LostTransition
	for _, ev := range events {
		if ev.Transition.Supported {
			continue
		}
		reason := ""
		switch ev.Transition.Class {
		case TransitionDissolve:
			reason = "dissolve mixing and source/target handles are not represented by the cut-only timeline"
		case TransitionWipe:
			reason = "wipe pattern, direction, and edge softness are not represented by the cut-only timeline"
		case TransitionKey:
			reason = "key/composite parameters are not represented by the cut-only timeline"
		case TransitionFade:
			reason = "fade curve and opacity are not represented by the cut-only timeline"
		default:
			reason = "transition semantics are outside the supported cut-only EDL subset"
		}
		lost = append(lost, LostTransition{
			File:           path,
			Event:          ev.Number,
			Line:           ev.Line,
			Reel:           ev.Reel,
			Track:          ev.Track,
			Code:           ev.Transition.Code,
			Class:          string(ev.Transition.Class),
			DurationFrames: ev.Transition.DurationFrames,
			Reason:         reason,
			Action:         "exported as a hard cut; record cut point and source range are preserved",
		})
	}
	return lost
}

func validateFile(ft *FileTimeline, events []Event) Validation {
	v := Validation{
		FirstCutAtZero:        ft.FirstCut.Frame == 0,
		LastCutAtTotal:        ft.LastCut.Frame == ft.TotalFrames,
		SpanMatchesCutPoints:  ft.LastCut.Frame-ft.FirstCut.Frame == ft.TotalFrames,
		ContinuityWithinTrack: true,
		SourceRecordDurations: true,
	}
	if !v.FirstCutAtZero {
		v.Issues = append(v.Issues, fmt.Sprintf("first cut is %d, want 0", ft.FirstCut.Frame))
	}
	if !v.LastCutAtTotal {
		v.Issues = append(v.Issues, fmt.Sprintf("last cut is %d, total is %d", ft.LastCut.Frame, ft.TotalFrames))
	}
	if !v.SpanMatchesCutPoints {
		v.Issues = append(v.Issues, "record span does not match last-first cut frames")
	}
	for _, track := range ft.Tracks {
		if len(track.Gaps) > 0 || len(track.Overlaps) > 0 {
			v.ContinuityWithinTrack = false
		}
	}
	for _, ev := range events {
		if ev.SourceDurationFrames != ev.RecordDurationFrames {
			v.SourceRecordDurations = false
			v.Issues = append(v.Issues, fmt.Sprintf(
				"event %d source duration %d != record duration %d",
				ev.Number, ev.SourceDurationFrames, ev.RecordDurationFrames,
			))
		}
		if ev.Transition.DurationFrames > ev.RecordDurationFrames {
			v.Issues = append(v.Issues, fmt.Sprintf(
				"event %d transition duration %d exceeds event %d frames",
				ev.Number, ev.Transition.DurationFrames, ev.RecordDurationFrames,
			))
		}
	}
	return v
}

// BuildBatch analyzes files without combining their record timelines.
func BuildBatch(files []*EDLFile) (*BatchTimeline, error) {
	batch := &BatchTimeline{Version: 1}
	total := NewFractionNoErr(0, 1)
	var repeated []string
	for _, file := range files {
		ft, err := BuildTimeline(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.Path, err)
		}
		batch.Files = append(batch.Files, *ft)
		batch.Summary.FileCount++
		batch.Summary.TotalFrames += ft.TotalFrames
		rate, _ := ParseRate(ft.FrameRate)
		total = total.Add(rate.SecondsPerFrame().Mul(NewFractionNoErr(ft.TotalFrames, 1)))
		batch.Summary.UnsupportedTransition += len(ft.LostTransitions)
		batch.Summary.ValidationIssues += len(ft.Validation.Issues)
		for _, reel := range ft.RepeatedReels {
			repeated = append(repeated, ft.Path+":"+reel)
		}
	}
	batch.Summary.TotalDuration = DurationInfo{Rational: total.String(), Seconds: total.Decimal(9)}
	sort.Strings(repeated)
	batch.Summary.RepeatedReels = repeated
	return batch, nil
}
