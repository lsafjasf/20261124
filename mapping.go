package edl

import "fmt"

// FrameMapping is the exact source frame behind one record-timeline frame.
// Event is the EDL event number, so repeated uses of the same reel remain
// distinguishable.
type FrameMapping struct {
	TimelineFrame       int64  `json:"timelineFrame"`
	AbsoluteRecordFrame int64  `json:"absoluteRecordFrame"`
	Track               string `json:"track"`
	Event               int    `json:"event"`
	Line                int    `json:"line"`
	Reel                string `json:"reel"`
	SourceFrame         int64  `json:"sourceFrame"`
	SourceLocalFrame    int64  `json:"sourceLocalFrame"`
	UnwrappedSourceTC   string `json:"unwrappedSourceTimecode"`
	SourceMode          string `json:"sourceMode"`
}

// FrameAt resolves a record-timeline frame to a source frame. If transition
// ranges overlap, the event entered most recently wins, matching an incoming
// edit. Empty track selects the first track in the file.
func (ft *FileTimeline) FrameAt(track string, timelineFrame int64) (FrameMapping, error) {
	if ft == nil {
		return FrameMapping{}, fmt.Errorf("nil timeline")
	}
	if timelineFrame < 0 || timelineFrame >= ft.TotalFrames {
		return FrameMapping{}, fmt.Errorf("timeline frame %d outside 0..%d", timelineFrame, ft.TotalFrames-1)
	}
	var chosen *Event
	var selectedTrack string
	for _, candidate := range ft.Tracks {
		if track != "" && candidate.Name != track {
			continue
		}
		selectedTrack = candidate.Name
		for _, index := range candidate.EventIndexes {
			ev := &ft.Events[index]
			if timelineFrame < ev.TimelineIn || timelineFrame >= ev.TimelineOut {
				continue
			}
			if chosen == nil ||
				ev.TimelineIn > chosen.TimelineIn ||
				(ev.TimelineIn == chosen.TimelineIn && ev.Number > chosen.Number) {
				chosen = ev
			}
		}
		if chosen != nil {
			break
		}
	}
	if chosen == nil {
		return FrameMapping{}, fmt.Errorf("timeline frame %d is not covered on track %q", timelineFrame, track)
	}
	offset := timelineFrame - chosen.TimelineIn
	if offset >= chosen.SourceDurationFrames {
		return FrameMapping{}, fmt.Errorf("event %d has no source frame for offset %d", chosen.Number, offset)
	}
	sourceFrame := chosen.SourceInAbs + offset
	sourceTC, err := FromFrames(sourceFrame, chosen.SourceIn.Rate, chosen.SourceIn.Mode)
	if err != nil {
		return FrameMapping{}, err
	}
	return FrameMapping{
		TimelineFrame:       timelineFrame,
		AbsoluteRecordFrame: ft.FirstCut.AbsoluteFrame + timelineFrame,
		Track:               selectedTrack,
		Event:               chosen.Number,
		Line:                chosen.Line,
		Reel:                chosen.Reel,
		SourceFrame:         sourceFrame,
		SourceLocalFrame:    offset,
		UnwrappedSourceTC:   sourceTC.String(),
		SourceMode:          string(chosen.SourceIn.Mode),
	}, nil
}
