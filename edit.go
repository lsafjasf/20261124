package otio

import (
	"fmt"
	"time"
)

// NewRationalTime constructs an OTIO RationalTime.
func NewRationalTime(rate, value float64) RationalTime {
	return RationalTime{Schema: "RationalTime.1", Rate: rate, Value: value}
}

// NewTimeRange constructs an OTIO TimeRange.
func NewTimeRange(start, duration RationalTime) *TimeRange {
	return &TimeRange{
		Schema:   "TimeRange.1",
		Start:    start,
		Duration: duration,
	}
}

// NewTimeline constructs an empty timeline with a Stack named "tracks".
func NewTimeline(name string) *Timeline {
	timeline := &Timeline{Object: newObject("Timeline.1", name)}
	timeline.Tracks = &Stack{
		Object: newObject("Stack.1", "tracks"),
	}
	return timeline
}

// NewTrack constructs an empty track.
func NewTrack(name string) *Track {
	return &Track{Object: newObject("Track.1", name)}
}

// NewClip constructs a clip. A nil source range means use the media's full
// available range, which is allowed even when the media itself is missing.
func NewClip(name string, sourceRange *TimeRange) *Clip {
	clip := &Clip{Object: newObject("Clip.2", name)}
	clip.SourceRange = sourceRange
	clip.sourceRangeSet = sourceRange != nil
	return clip
}

// NewGap constructs a gap starting at source frame zero.
func NewGap(name string, duration RationalTime) *Gap {
	gap := &Gap{Object: newObject("Gap.1", name)}
	gap.SourceRange = NewTimeRange(NewRationalTime(duration.Rate, 0), duration)
	gap.sourceRangeSet = true
	return gap
}

// NewMarker constructs a marker. Color is usually an OTIO constant such as
// "RED" or "GREEN".
func NewMarker(name, color string, markedRange *TimeRange) Marker {
	marker := Marker{
		Schema:      "Marker.2",
		Name:        name,
		Color:       color,
		MarkedRange: markedRange,
		nameSet:     true,
		colorSet:    true,
		rangeSet:    markedRange != nil,
		rangeField:  "marked_range",
	}
	return marker
}

func newObject(schema, name string) Object {
	return Object{Schema: schema, Name: name, nameSet: true}
}

// AddMarker appends a marker to any timeline object.
func (object *Object) AddMarker(marker Marker) {
	if object.Markers == nil {
		object.Markers = make([]Marker, 0, 1)
	}
	object.Markers = append(object.Markers, marker)
	object.markersSet = true
	object.markersNull = false
}

// RemoveMarkers removes every marker with the given exact name and returns the
// number removed.
func (object *Object) RemoveMarkers(name string) int {
	kept := object.Markers[:0]
	removed := 0
	for _, marker := range object.Markers {
		if marker.Name == name {
			removed++
			continue
		}
		kept = append(kept, marker)
	}
	object.Markers = kept
	if removed > 0 {
		object.markersSet = true
		object.markersNull = false
	}
	return removed
}

// TrimSourceStart trims the source in-point by delta while keeping the
// start/duration rates unchanged.
func (object *Object) TrimSourceStart(delta time.Duration) error {
	if object.SourceRange == nil {
		return fmt.Errorf("object has no source_range")
	}
	return object.SourceRange.TrimStart(delta)
}

// TrimSourceEnd trims the source out-point by delta while keeping the duration
// rate unchanged.
func (object *Object) TrimSourceEnd(delta time.Duration) error {
	if object.SourceRange == nil {
		return fmt.Errorf("object has no source_range")
	}
	return object.SourceRange.TrimEnd(delta)
}

// AppendTrack adds a track to the timeline, creating the tracks Stack when
// necessary.
func (timeline *Timeline) AppendTrack(track *Track) error {
	if timeline == nil {
		return fmt.Errorf("timeline is nil")
	}
	if track == nil {
		return fmt.Errorf("track is nil")
	}
	if timeline.Tracks == nil {
		timeline.Tracks = &Stack{Object: newObject("Stack.1", "tracks")}
	}
	timeline.Tracks.Children = append(timeline.Tracks.Children, track)
	return nil
}

// Append adds a track or opaque child to a Stack.
func (stack *Stack) Append(item Item) error {
	if stack == nil {
		return fmt.Errorf("stack is nil")
	}
	return stack.Insert(len(stack.Children), item)
}

// Insert inserts a track or opaque child into a Stack.
func (stack *Stack) Insert(index int, item Item) error {
	if stack == nil {
		return fmt.Errorf("stack is nil")
	}
	if err := validateStackChild(item); err != nil {
		return err
	}
	if index < 0 || index > len(stack.Children) {
		return fmt.Errorf("stack index %d out of range", index)
	}
	stack.Children = append(stack.Children, nil)
	copy(stack.Children[index+1:], stack.Children[index:])
	stack.Children[index] = item
	return nil
}

// Remove removes a child from a Stack.
func (stack *Stack) Remove(index int) (Item, error) {
	if stack == nil {
		return nil, fmt.Errorf("stack is nil")
	}
	if index < 0 || index >= len(stack.Children) {
		return nil, fmt.Errorf("stack index %d out of range", index)
	}
	item := stack.Children[index]
	copy(stack.Children[index:], stack.Children[index+1:])
	stack.Children[len(stack.Children)-1] = nil
	stack.Children = stack.Children[:len(stack.Children)-1]
	return item, nil
}

// Append adds a clip, gap, or opaque child to a Track.
func (track *Track) Append(item Item) error {
	if track == nil {
		return fmt.Errorf("track is nil")
	}
	return track.Insert(len(track.Children), item)
}

// Insert inserts a clip, gap, or opaque child into a Track.
func (track *Track) Insert(index int, item Item) error {
	if track == nil {
		return fmt.Errorf("track is nil")
	}
	if err := validateTrackChild(item); err != nil {
		return err
	}
	if index < 0 || index > len(track.Children) {
		return fmt.Errorf("track index %d out of range", index)
	}
	track.Children = append(track.Children, nil)
	copy(track.Children[index+1:], track.Children[index:])
	track.Children[index] = item
	return nil
}

// Remove removes a child from a Track.
func (track *Track) Remove(index int) (Item, error) {
	if track == nil {
		return nil, fmt.Errorf("track is nil")
	}
	if index < 0 || index >= len(track.Children) {
		return nil, fmt.Errorf("track index %d out of range", index)
	}
	item := track.Children[index]
	copy(track.Children[index:], track.Children[index+1:])
	track.Children[len(track.Children)-1] = nil
	track.Children = track.Children[:len(track.Children)-1]
	return item, nil
}

// ClipByName finds the first clip with the exact name on this track.
func (track *Track) ClipByName(name string) *Clip {
	if track == nil {
		return nil
	}
	for _, child := range track.Children {
		if clip, ok := child.(*Clip); ok && clip != nil && clip.Name == name {
			return clip
		}
	}
	return nil
}

// Overlaps reports whether two marker ranges overlap. Touching endpoints do
// not count as overlap, and mixed frame rates are resolved in seconds.
func (marker Marker) Overlaps(other Marker) (bool, error) {
	if marker.MarkedRange == nil || other.MarkedRange == nil {
		return false, fmt.Errorf("both markers need a marked range")
	}
	aStart := marker.MarkedRange.Start
	aEnd, err := marker.MarkedRange.End()
	if err != nil {
		return false, err
	}
	bStart := other.MarkedRange.Start
	bEnd, err := other.MarkedRange.End()
	if err != nil {
		return false, err
	}
	aEndBeforeB, err := aEnd.Compare(bStart)
	if err != nil {
		return false, err
	}
	bEndBeforeA, err := bEnd.Compare(aStart)
	if err != nil {
		return false, err
	}
	return aEndBeforeB > 0 && bEndBeforeA > 0, nil
}

// OverlappingMarkers returns every overlapping marker pair without rejecting
// or merging any markers.
func (object *Object) OverlappingMarkers() ([][2]int, error) {
	var pairs [][2]int
	for i := 0; i < len(object.Markers); i++ {
		for j := i + 1; j < len(object.Markers); j++ {
			overlaps, err := object.Markers[i].Overlaps(object.Markers[j])
			if err != nil {
				return nil, err
			}
			if overlaps {
				pairs = append(pairs, [2]int{i, j})
			}
		}
	}
	return pairs, nil
}

func validateStackChild(item Item) error {
	switch child := item.(type) {
	case *Track:
		if child == nil {
			return fmt.Errorf("stack child is a nil Track")
		}
	case *Opaque:
		if child == nil {
			return fmt.Errorf("stack child is a nil Opaque item")
		}
	case nil:
		return fmt.Errorf("stack child is nil")
	default:
		return fmt.Errorf("Stack cannot contain %s items", item.Kind())
	}
	return nil
}

func validateTrackChild(item Item) error {
	switch child := item.(type) {
	case *Clip:
		if child == nil {
			return fmt.Errorf("track child is a nil Clip")
		}
	case *Gap:
		if child == nil {
			return fmt.Errorf("track child is a nil Gap")
		}
	case *Opaque:
		if child == nil {
			return fmt.Errorf("track child is a nil Opaque item")
		}
	case nil:
		return fmt.Errorf("track child is nil")
	default:
		return fmt.Errorf("Track cannot contain %s items", item.Kind())
	}
	return nil
}
