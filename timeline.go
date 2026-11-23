package fcpxml

import (
	"fmt"
	"sort"
)

// Timeline is the flattened final-picture view of a top-level sequence.
type Timeline struct {
	TopSequenceID   string
	TopSequenceName string
	FrameDuration   Time
	Duration        Time
	DurationFrames  int64
	PictureEvents   []PictureEvent
	Transitions     []TransitionEvent
	Cuts            []Cut
	Validation      *ValidationReport
	Unsupported     []Issue
}

// PictureEvent is a visible video, title, or compound-clip leaf mapped to the
// top-level sequence's zero-based frame timeline.
type PictureEvent struct {
	ID          string
	Kind        string
	Name        string
	Ref         string
	Lane        int
	SourceStart Time
	RawStart    Time
	RawEnd      Time
	Start       Time
	End         Time
	StartFrame  int64
	EndFrame    int64
	Path        string
	SequenceID  string
}

// TransitionEvent is a transition interval mapped to top-level frames.
type TransitionEvent struct {
	ID          string
	Name        string
	Ref         string
	Start       Time
	End         Time
	Center      Time
	Cut         Time
	StartFrame  int64
	EndFrame    int64
	CenterFrame int64
	CutFrame    int64
	HasCut      bool
	Path        string
	SequenceID  string
}

// Cut is a picture boundary or a transition edit point in the top timeline.
type Cut struct {
	Time   Time
	Frame  int64
	Kind   string
	Source string
}

// FrameSpan is a compact frame range used for invariant checks.
type FrameSpan struct {
	Start int64
	End   int64
}

// CutSignature returns all key cut points and picture spans in stable order.
// Round-trip tests compare this value before and after export/import.
func (t *Timeline) CutSignature() []string {
	if t == nil {
		return nil
	}
	result := make([]string, 0, len(t.Cuts)+len(t.PictureEvents))
	for _, cut := range t.Cuts {
		result = append(result, fmt.Sprintf("cut:%d:%s:%s", cut.Frame, cut.Time.String(), cut.Kind))
	}
	sortedEvents := append([]PictureEvent(nil), t.PictureEvents...)
	sort.SliceStable(sortedEvents, func(i, j int) bool {
		if sortedEvents[i].StartFrame != sortedEvents[j].StartFrame {
			return sortedEvents[i].StartFrame < sortedEvents[j].StartFrame
		}
		if sortedEvents[i].EndFrame != sortedEvents[j].EndFrame {
			return sortedEvents[i].EndFrame < sortedEvents[j].EndFrame
		}
		return sortedEvents[i].Path < sortedEvents[j].Path
	})
	for _, event := range sortedEvents {
		result = append(result, fmt.Sprintf("picture:%d:%d:%d:%d:%s",
			event.StartFrame, event.EndFrame, event.Start.Num, event.Start.Den, event.Path))
	}
	return result
}

// KeyFrames returns the unique top-level frame numbers at which picture or
// transition boundaries occur.
func (t *Timeline) KeyFrames() []int64 {
	if t == nil {
		return nil
	}
	seen := make(map[int64]bool, len(t.Cuts))
	frames := make([]int64, 0, len(t.Cuts))
	for _, cut := range t.Cuts {
		if seen[cut.Frame] {
			continue
		}
		seen[cut.Frame] = true
		frames = append(frames, cut.Frame)
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i] < frames[j] })
	return frames
}

type timeSpan struct {
	start Time
	end   Time
}

type itemTiming struct {
	node     *Node
	offset   Time
	duration Time
	end      Time
}

type analyzer struct {
	document      *Document
	top           *Node
	frameDuration Time
	topOrigin     Time
	topDuration   Time
	topVisible    *timeSpan
	events        []PictureEvent
	transitions   []TransitionEvent
	warnings      []Issue
}

// Analyze validates and flattens a document's project sequence. A partial
// timeline is returned with the error when validation detects a fatal issue.
func (d *Document) Analyze() (*Timeline, error) {
	return d.analyze("")
}

// AnalyzeSequence flattens the sequence selected by ID.
func (d *Document) AnalyzeSequence(sequenceID string) (*Timeline, error) {
	return d.analyze(sequenceID)
}

func (d *Document) analyze(sequenceID string) (*Timeline, error) {
	report, validationErr := d.validate(sequenceID)
	if validationErr != nil {
		return &Timeline{Validation: report}, validationErr
	}
	a := &analyzer{
		document:      d,
		top:           report.TopSequence,
		frameDuration: report.TopFrameDuration,
	}

	origin, err := parseNodeTime(a.top, "tcStart", ZeroTime)
	if err != nil {
		return &Timeline{Validation: report}, err
	}
	a.topOrigin = origin
	duration, err := parseNodeTime(a.top, "duration", ZeroTime)
	if err != nil {
		return &Timeline{Validation: report}, err
	}
	if duration.Num <= 0 {
		duration, err = a.measureTopDuration()
		if err != nil {
			return &Timeline{Validation: report}, err
		}
	}
	a.topDuration = duration
	if duration.Num > 0 {
		visible := &timeSpan{start: ZeroTime, end: duration}
		a.topVisible = visible
	}

	if err := a.analyzeSequence(a.top, timeNeg(origin), a.topVisible, nil); err != nil {
		return &Timeline{Validation: report, Unsupported: a.warnings}, err
	}

	timeline := &Timeline{
		TopSequenceID:   a.top.Attr("id"),
		TopSequenceName: a.top.Attr("name"),
		FrameDuration:   a.frameDuration,
		PictureEvents:   a.events,
		Transitions:     a.transitions,
		Validation:      report,
		Unsupported:     a.warnings,
	}
	sort.SliceStable(timeline.PictureEvents, func(i, j int) bool {
		left, right := timeline.PictureEvents[i], timeline.PictureEvents[j]
		if cmp := left.Start.Cmp(right.Start); cmp != 0 {
			return cmp < 0
		}
		if cmp := left.End.Cmp(right.End); cmp != 0 {
			return cmp < 0
		}
		return left.Path < right.Path
	})
	sort.SliceStable(timeline.Transitions, func(i, j int) bool {
		return timeline.Transitions[i].Start.Cmp(timeline.Transitions[j].Start) < 0
	})
	timeline.Duration = a.effectiveDuration()
	if timeline.Duration.Num > 0 {
		timeline.DurationFrames, err = FrameAt(timeline.Duration, a.frameDuration, RoundNearest)
		if err != nil {
			return timeline, err
		}
	}
	timeline.Cuts = buildCuts(timeline.PictureEvents, timeline.Transitions, a.frameDuration)
	return timeline, nil
}

func (a *analyzer) measureTopDuration() (Time, error) {
	spine := a.top.Child("spine")
	if spine == nil {
		return ZeroTime, nil
	}
	origin, err := parseNodeTime(a.top, "tcStart", ZeroTime)
	if err != nil {
		return Time{}, err
	}
	cursor := origin
	maxEnd := origin
	for _, item := range spine.Children {
		offset, err := parseNodeTime(item, "offset", cursor)
		if err != nil {
			return Time{}, err
		}
		duration, err := a.itemDuration(item)
		if err != nil {
			return Time{}, err
		}
		end, err := offset.Add(duration)
		if err != nil {
			return Time{}, err
		}
		if item.Name.Local != "transition" && duration.Num > 0 && end.Cmp(cursor) > 0 {
			cursor = end
		}
		if end.Cmp(maxEnd) > 0 {
			maxEnd = end
		}
	}
	return maxEnd.Sub(origin)
}

func timeNeg(value Time) Time {
	return Time{Num: -value.Num, Den: value.Den}
}

func (a *analyzer) effectiveDuration() Time {
	duration := a.topDuration
	for _, event := range a.events {
		if event.End.Cmp(duration) > 0 {
			duration = event.End
		}
	}
	for _, transition := range a.transitions {
		if transition.End.Cmp(duration) > 0 {
			duration = transition.End
		}
	}
	if duration.Num < 0 {
		return ZeroTime
	}
	return duration
}

func (a *analyzer) analyzeSequence(sequence *Node, shift Time, visible *timeSpan, stack []*Node) error {
	for _, active := range stack {
		if active == sequence {
			return fmt.Errorf("circular sequence nesting at %s", sequence.Path())
		}
	}
	stack = append(stack, sequence)
	spine := sequence.Child("spine")
	if spine == nil {
		a.warn("missing-spine", sequence.Path(), "sequence has no spine")
		return nil
	}
	tcStart, err := parseNodeTime(sequence, "tcStart", ZeroTime)
	if err != nil {
		return err
	}
	sequenceVisible := visible
	sequenceDuration, err := parseNodeTime(sequence, "duration", ZeroTime)
	if err != nil {
		return err
	}
	if sequenceDuration.Num > 0 {
		sequenceStart, err := tcStart.Add(shift)
		if err != nil {
			return err
		}
		sequenceEnd, err := sequenceStart.Add(sequenceDuration)
		if err != nil {
			return err
		}
		sequenceVisible = intersectSpans(visible,
			&timeSpan{start: sequenceStart, end: sequenceEnd})
		if sequenceVisible == nil {
			return nil
		}
	}
	return a.analyzeItems(spine.Children, shift, sequenceVisible, tcStart, sequence, stack)
}

func intersectSpans(left, right *timeSpan) *timeSpan {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	start := timeMax(left.start, right.start)
	end := timeMin(left.end, right.end)
	if start.Cmp(end) >= 0 {
		return nil
	}
	return &timeSpan{start: start, end: end}
}

func (a *analyzer) analyzeItems(items []*Node, shift Time, visible *timeSpan,
	cursor Time, owner *Node, stack []*Node) error {
	timings := make([]itemTiming, len(items))
	for index, item := range items {
		offset, err := parseNodeTime(item, "offset", cursor)
		if err != nil {
			return err
		}
		timings[index] = itemTiming{node: item, offset: offset}
		if item.Name.Local == "spine" {
			continue
		}
		duration, err := a.itemDuration(item)
		if err != nil {
			return err
		}
		end, err := offset.Add(duration)
		if err != nil {
			return err
		}
		timings[index].duration = duration
		timings[index].end = end
		// A transition overlaps its neighbours and does not move the serial
		// cursor. A following item with no explicit offset still starts at the
		// preceding story element's end (the edit point).
		if item.Name.Local != "transition" && duration.Num > 0 && end.Cmp(cursor) > 0 {
			cursor = end
		}
	}

	for index, item := range items {
		timing := timings[index]
		offset, duration := timing.offset, timing.duration
		if item.Name.Local == "spine" {
			storylineShift, err := shift.Add(offset)
			if err != nil {
				return err
			}
			if err := a.analyzeItems(item.Children, storylineShift, visible, ZeroTime, owner, stack); err != nil {
				return err
			}
			continue
		}
		if item.Attr("enabled") == "0" {
			continue
		}

		switch item.Name.Local {
		case "transition":
			cut, hasCut := transitionCut(timings, index)
			if err := a.addTransition(item, offset, duration, cut, hasCut, shift, visible, owner); err != nil {
				return err
			}
			continue
		case "gap":
			if containsTimelineChildren(item) {
				rawVisible, err := clipSpan(offset, duration, shift, visible)
				if err != nil {
					return err
				}
				if rawVisible != nil {
					gapStart, err := parseNodeTime(item, "start", ZeroTime)
					if err != nil {
						return err
					}
					childShift, err := shift.Add(offset)
					if err != nil {
						return err
					}
					childShift, err = childShift.Sub(gapStart)
					if err != nil {
						return err
					}
					if err := a.analyzeItems(item.Children, childShift, rawVisible,
						gapStart, owner, stack); err != nil {
						return err
					}
				}
			}
			continue
		case "marker", "chapter-marker", "rating", "keyword",
			"analysis-marker", "note", "metadata", "filter-video", "filter-audio":
			continue
		case "audition":
			a.warn("unsupported-audition", item.Path(),
				"audition alternatives are outside the supported subset; no picture emitted")
			continue
		}

		rawVisible, err := clipSpan(offset, duration, shift, visible)
		if err != nil {
			return err
		}
		if rawVisible == nil {
			continue
		}
		clipStart, err := parseNodeTime(item, "start", ZeroTime)
		if err != nil {
			return err
		}
		childShift, err := shift.Add(offset)
		if err != nil {
			return err
		}
		childShift, err = childShift.Sub(clipStart)
		if err != nil {
			return err
		}

		nested := a.findNestedSequence(item)
		if nested != nil {
			if err := a.analyzeSequence(nested, childShift, rawVisible, stack); err != nil {
				return err
			}
			for _, child := range item.Children {
				if child.Name.Local == "sequence" || !isTimelineChild(child) {
					continue
				}
				if err := a.analyzeItems([]*Node{child}, childShift, rawVisible,
					clipStart, owner, stack); err != nil {
					return err
				}
			}
			continue
		}
		if containsTimelineChildren(item) {
			if err := a.analyzeItems(item.Children, childShift, rawVisible, clipStart, owner, stack); err != nil {
				return err
			}
			switch item.Name.Local {
			case "clip", "sync-clip", "mc-clip":
				continue
			}
		}
		if !isPictureElement(item) {
			if item.Attr("ref") != "" || item.Name.Local == "clip" {
				a.warn("unsupported-element", item.Path(),
					fmt.Sprintf("<%s> is outside the supported picture subset", item.Name.Local))
			}
			continue
		}
		if err := a.addPicture(item, offset, duration, clipStart, shift, rawVisible, owner); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzer) itemDuration(item *Node) (Time, error) {
	if item.HasAttr("duration") {
		return parseNodeTime(item, "duration", ZeroTime)
	}
	if nested := a.findNestedSequence(item); nested != nil {
		return parseNodeTime(nested, "duration", ZeroTime)
	}
	if ref := item.Attr("ref"); ref != "" {
		if target := a.document.IDs[ref]; target != nil && target.HasAttr("duration") {
			return parseNodeTime(target, "duration", ZeroTime)
		}
	}
	return ZeroTime, nil
}

func (a *analyzer) findNestedSequence(item *Node) *Node {
	if child := item.Child("sequence"); child != nil {
		return child
	}
	ref := item.Attr("ref")
	if ref == "" {
		return nil
	}
	target := a.document.IDs[ref]
	if target == nil {
		return nil
	}
	if target.Name.Local == "sequence" {
		return target
	}
	if child := target.Child("sequence"); child != nil {
		return child
	}
	return nil
}

func transitionCut(timings []itemTiming, index int) (Time, bool) {
	if index < 0 || index >= len(timings) {
		return ZeroTime, false
	}
	transition := timings[index]
	var previousEnd, nextStart Time
	havePrevious, haveNext := false, false
	for cursor := index - 1; cursor >= 0; cursor-- {
		if timings[cursor].node.Name.Local == "transition" {
			continue
		}
		previousEnd = timings[cursor].end
		havePrevious = timings[cursor].duration.Num > 0
		break
	}
	for cursor := index + 1; cursor < len(timings); cursor++ {
		if timings[cursor].node.Name.Local == "transition" {
			continue
		}
		nextStart = timings[cursor].offset
		haveNext = true
		break
	}
	inside := func(value Time) bool {
		return value.Cmp(transition.offset) >= 0 && value.Cmp(transition.end) <= 0
	}
	if havePrevious && inside(previousEnd) {
		return previousEnd, true
	}
	if haveNext && inside(nextStart) {
		return nextStart, true
	}
	if havePrevious && haveNext && previousEnd.Cmp(nextStart) == 0 {
		return previousEnd, true
	}
	return Time{}, false
}

func clipSpan(offset, duration, shift Time, parent *timeSpan) (*timeSpan, error) {
	start, err := offset.Add(shift)
	if err != nil {
		return nil, err
	}
	end := start
	if duration.Num > 0 {
		end, err = start.Add(duration)
		if err != nil {
			return nil, err
		}
	}
	span := &timeSpan{start: start, end: end}
	if parent == nil {
		if span.start.Cmp(ZeroTime) < 0 {
			span.start = ZeroTime
		}
		if span.end.Cmp(span.start) <= 0 {
			return nil, nil
		}
		return span, nil
	}
	if span.start.Cmp(parent.start) < 0 {
		span.start = parent.start
	}
	if span.end.Cmp(parent.end) > 0 {
		span.end = parent.end
	}
	if span.start.Cmp(span.end) >= 0 {
		return nil, nil
	}
	return span, nil
}

func containsTimelineChildren(node *Node) bool {
	for _, child := range node.Children {
		if isTimelineChild(child) {
			return true
		}
	}
	return false
}

func isTimelineChild(node *Node) bool {
	switch node.Name.Local {
	case "sequence", "spine", "clip", "ref-clip", "sync-clip", "mc-clip",
		"asset-clip", "video", "title", "caption", "audition", "gap":
		return true
	default:
		return false
	}
}

func isPictureElement(node *Node) bool {
	switch node.Name.Local {
	case "asset-clip", "video", "title", "caption", "clip", "ref-clip":
		return true
	default:
		return false
	}
}

func (a *analyzer) addPicture(item *Node, offset, duration, sourceStart, shift Time,
	visible *timeSpan, owner *Node) error {
	startFrame, err := FrameAt(visible.start, a.frameDuration, RoundNearest)
	if err != nil {
		return err
	}
	endFrame, err := FrameAt(visible.end, a.frameDuration, RoundNearest)
	if err != nil {
		return err
	}
	if endFrame < startFrame {
		endFrame = startFrame
	}
	rawStart, err := offset.Add(shift)
	if err != nil {
		return err
	}
	rawEnd, err := rawStart.Add(duration)
	if err != nil {
		return err
	}
	event := PictureEvent{
		ID:          item.Attr("id"),
		Kind:        item.Name.Local,
		Name:        item.Attr("name"),
		Ref:         item.Attr("ref"),
		Lane:        parseLane(item),
		SourceStart: sourceStart,
		RawStart:    rawStart,
		RawEnd:      rawEnd,
		Start:       visible.start,
		End:         visible.end,
		StartFrame:  startFrame,
		EndFrame:    endFrame,
		Path:        item.Path(),
	}
	if owner != nil {
		event.SequenceID = owner.Attr("id")
	}
	a.events = append(a.events, event)
	return nil
}

func (a *analyzer) addTransition(item *Node, offset, duration, localCut Time, hasCut bool,
	shift Time, visible *timeSpan, owner *Node) error {
	if duration.Num <= 0 {
		a.warn("zero-transition", item.Path(), "transition has no positive duration")
		return nil
	}
	start, err := clipSpan(offset, duration, shift, visible)
	if err != nil {
		return err
	}
	if start == nil {
		return nil
	}
	half, err := duration.Div(Time{Num: 2, Den: 1})
	if err != nil {
		return err
	}
	center, err := start.start.Add(half)
	if err != nil {
		return err
	}
	if center.Cmp(start.start) < 0 {
		center = start.start
	}
	if center.Cmp(start.end) > 0 {
		center = start.end
	}
	startFrame, err := FrameAt(start.start, a.frameDuration, RoundNearest)
	if err != nil {
		return err
	}
	endFrame, err := FrameAt(start.end, a.frameDuration, RoundNearest)
	if err != nil {
		return err
	}
	centerFrame, err := FrameAt(center, a.frameDuration, RoundNearest)
	if err != nil {
		return err
	}
	cut := Time{}
	var cutFrame int64
	if hasCut {
		cut, err = localCut.Add(shift)
		if err != nil {
			return err
		}
		if cut.Cmp(start.start) >= 0 && cut.Cmp(start.end) <= 0 {
			cutFrame, err = FrameAt(cut, a.frameDuration, RoundNearest)
			if err != nil {
				return err
			}
		} else {
			hasCut = false
		}
	}
	transition := TransitionEvent{
		ID:          item.Attr("id"),
		Name:        item.Attr("name"),
		Ref:         transitionRef(item),
		Start:       start.start,
		End:         start.end,
		Center:      center,
		Cut:         cut,
		StartFrame:  startFrame,
		EndFrame:    endFrame,
		CenterFrame: centerFrame,
		CutFrame:    cutFrame,
		HasCut:      hasCut,
		Path:        item.Path(),
	}
	if owner != nil {
		transition.SequenceID = owner.Attr("id")
	}
	a.transitions = append(a.transitions, transition)
	return nil
}

func transitionRef(item *Node) string {
	if ref := item.Attr("ref"); ref != "" {
		return ref
	}
	for _, child := range item.Children {
		if (child.Name.Local == "filter-video" || child.Name.Local == "filter-audio") &&
			child.Attr("ref") != "" {
			return child.Attr("ref")
		}
	}
	return ""
}

func (a *analyzer) warn(code, path, message string) {
	a.warnings = append(a.warnings, Issue{
		Severity: SeverityWarning, Code: code, Path: path, Message: message,
	})
}

func buildCuts(events []PictureEvent, transitions []TransitionEvent, frameDuration Time) []Cut {
	type candidate struct {
		time   Time
		kind   string
		source string
	}
	candidates := make([]candidate, 0, len(events)*2+len(transitions)*3)
	for _, event := range events {
		candidates = append(candidates,
			candidate{time: event.Start, kind: "picture-start", source: event.Path},
			candidate{time: event.End, kind: "picture-end", source: event.Path})
	}
	for _, transition := range transitions {
		candidates = append(candidates,
			candidate{time: transition.Start, kind: "transition-start", source: transition.Path},
			candidate{time: transition.End, kind: "transition-end", source: transition.Path})
		if transition.HasCut {
			candidates = append(candidates,
				candidate{time: transition.Cut, kind: "transition-cut", source: transition.Path})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if cmp := candidates[i].time.Cmp(candidates[j].time); cmp != 0 {
			return cmp < 0
		}
		if candidates[i].kind != candidates[j].kind {
			return candidates[i].kind < candidates[j].kind
		}
		return candidates[i].source < candidates[j].source
	})
	cuts := make([]Cut, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, item := range candidates {
		key := item.time.String() + "|" + item.kind + "|" + item.source
		if seen[key] {
			continue
		}
		seen[key] = true
		frame, err := FrameAt(item.time, frameDuration, RoundNearest)
		if err != nil {
			continue
		}
		cuts = append(cuts, Cut{Time: item.time, Frame: frame, Kind: item.kind, Source: item.source})
	}
	return cuts
}
