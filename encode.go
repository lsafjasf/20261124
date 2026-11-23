package otio

import (
	"encoding/json"
	"fmt"
	"os"
)

// Encode re-exports the timeline as indented OTIO JSON. The returned report
// lists retained unknown data and missing media in the edited model.
func Encode(timeline *Timeline) ([]byte, *Report, error) {
	raw, err := encodeTimeline(timeline)
	if err != nil {
		return nil, Analyze(timeline), err
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, Analyze(timeline), fmt.Errorf("encode OTIO JSON: %w", err)
	}
	data = append(data, '\n')
	return data, Analyze(timeline), nil
}

// Save writes an OTIO JSON file.
func Save(path string, timeline *Timeline) (*Report, error) {
	data, report, err := Encode(timeline)
	if err != nil {
		return report, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return report, fmt.Errorf("write %s: %w", path, err)
	}
	return report, nil
}

// MarshalJSON permits the model to be used with encoding/json while preserving
// unknown fields. Use Encode when the report is also needed.
func (timeline *Timeline) MarshalJSON() ([]byte, error) {
	raw, err := encodeTimeline(timeline)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON imports an OTIO document. Use Decode when the report is also
// needed.
func (timeline *Timeline) UnmarshalJSON(data []byte) error {
	parsed, _, err := Decode(data)
	if err != nil {
		return err
	}
	*timeline = *parsed
	return nil
}

func encodeTimeline(timeline *Timeline) (map[string]any, error) {
	if timeline == nil {
		return nil, fmt.Errorf("timeline is nil")
	}
	if err := validateForEncode(timeline); err != nil {
		return nil, err
	}
	raw := encodeObject(&timeline.Object, "Timeline.1")
	if timeline.GlobalStartTime != nil {
		raw["global_start_time"] = encodeRationalTime(timeline.GlobalStartTime)
	} else if timeline.globalStartSet {
		raw["global_start_time"] = nil
	}
	if timeline.Tracks == nil {
		return nil, fmt.Errorf("/tracks is required")
	}
	raw["tracks"] = encodeStack(timeline.Tracks)
	return raw, nil
}

func encodeStack(stack *Stack) map[string]any {
	raw := encodeObject(&stack.Object, "Stack.1")
	children := make([]any, 0, len(stack.Children))
	for _, child := range stack.Children {
		switch item := child.(type) {
		case *Track:
			children = append(children, encodeTrack(item))
		case *Opaque:
			children = append(children, encodeOpaque(item))
		case nil:
			children = append(children, nil)
		default:
			children = append(children, encodeUnsupportedItem(item))
		}
	}
	raw["children"] = children
	return raw
}

func encodeTrack(track *Track) map[string]any {
	raw := encodeObject(&track.Object, "Track.1")
	children := make([]any, 0, len(track.Children))
	for _, child := range track.Children {
		switch item := child.(type) {
		case *Clip:
			children = append(children, encodeClip(item))
		case *Gap:
			children = append(children, encodeGap(item))
		case *Opaque:
			children = append(children, encodeOpaque(item))
		case nil:
			children = append(children, nil)
		default:
			children = append(children, encodeUnsupportedItem(item))
		}
	}
	raw["children"] = children
	return raw
}

func encodeClip(clip *Clip) map[string]any {
	return encodeObject(&clip.Object, "Clip.2")
}

func encodeGap(gap *Gap) map[string]any {
	return encodeObject(&gap.Object, "Gap.1")
}

func encodeOpaque(item *Opaque) map[string]any {
	raw := cloneMap(item.Raw)
	if raw == nil {
		raw = make(map[string]any)
	}
	if _, ok := raw["OTIO_SCHEMA"]; !ok && item.Schema != "" {
		raw["OTIO_SCHEMA"] = item.Schema
	}
	return raw
}

func encodeUnsupportedItem(item Item) map[string]any {
	return map[string]any{
		"OTIO_SCHEMA": item.ItemSchema(),
		"name":        item.ItemName(),
	}
}

func encodeObject(object *Object, fallbackSchema string) map[string]any {
	raw := cloneMap(object.Extra)
	raw["OTIO_SCHEMA"] = defaultSchema(object.Schema, fallbackSchema)
	if object.Name != "" || object.nameSet {
		raw["name"] = object.Name
	}
	if object.Metadata != nil || object.metadataSet {
		raw["metadata"] = object.Metadata
	}
	if object.SourceRange != nil {
		raw["source_range"] = encodeTimeRange(object.SourceRange)
	} else if object.sourceRangeSet {
		raw["source_range"] = nil
	}
	if len(object.Markers) > 0 || object.markersSet {
		if object.markersNull && len(object.Markers) == 0 {
			raw["markers"] = nil
		} else {
			markers := make([]any, 0, len(object.Markers))
			for i := range object.Markers {
				markers = append(markers, encodeMarker(&object.Markers[i]))
			}
			raw["markers"] = markers
		}
	}
	return raw
}

func encodeMarker(marker *Marker) map[string]any {
	raw := cloneMap(marker.Extra)
	raw["OTIO_SCHEMA"] = defaultSchema(marker.Schema, "Marker.2")
	if marker.Name != "" || marker.nameSet {
		raw["name"] = marker.Name
	}
	if marker.Color != "" || marker.colorSet {
		raw["color"] = marker.Color
	}
	if marker.Metadata != nil || marker.metadataSet {
		raw["metadata"] = marker.Metadata
	}
	rangeField := marker.rangeField
	if rangeField == "" {
		rangeField = "marked_range"
	}
	if marker.MarkedRange != nil {
		raw[rangeField] = encodeTimeRange(marker.MarkedRange)
	} else if marker.rangeSet {
		raw[rangeField] = nil
	}
	return raw
}

func encodeTimeRange(timeRange *TimeRange) map[string]any {
	raw := cloneMap(timeRange.Extra)
	raw["OTIO_SCHEMA"] = defaultSchema(timeRange.Schema, "TimeRange.1")
	raw["start_time"] = encodeRationalTime(&timeRange.Start)
	raw["duration"] = encodeRationalTime(&timeRange.Duration)
	return raw
}

func encodeRationalTime(value *RationalTime) map[string]any {
	raw := cloneMap(value.Extra)
	raw["OTIO_SCHEMA"] = defaultSchema(value.Schema, "RationalTime.1")
	raw["rate"] = value.Rate
	raw["value"] = value.Value
	return raw
}

func cloneMap(source map[string]any) map[string]any {
	if source == nil {
		return make(map[string]any)
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
