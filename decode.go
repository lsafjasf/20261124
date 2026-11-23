package otio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
)

var (
	timelineFields = fieldSet("OTIO_SCHEMA", "name", "metadata", "global_start_time", "tracks", "markers")
	stackFields    = fieldSet("OTIO_SCHEMA", "name", "metadata", "source_range", "children", "markers")
	trackFields    = fieldSet("OTIO_SCHEMA", "name", "metadata", "source_range", "children", "markers")
	clipFields     = fieldSet("OTIO_SCHEMA", "name", "metadata", "source_range", "markers")
	gapFields      = fieldSet("OTIO_SCHEMA", "name", "metadata", "source_range", "markers")
	markerFields   = fieldSet("OTIO_SCHEMA", "name", "color", "metadata", "marked_range", "range")
	rangeFields    = fieldSet("OTIO_SCHEMA", "start_time", "duration")
	timeFields     = fieldSet("OTIO_SCHEMA", "rate", "value")
)

func fieldSet(fields ...string) map[string]bool {
	set := make(map[string]bool, len(fields))
	for _, field := range fields {
		set[field] = true
	}
	return set
}

// Decode imports an OTIO JSON document. Unsupported fields and object schemas
// are retained, and the returned report names them.
func Decode(data []byte) (*Timeline, *Report, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, &Report{}, fmt.Errorf("decode OTIO JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, &Report{}, fmt.Errorf("decode OTIO JSON: multiple top-level values")
		}
		return nil, &Report{}, fmt.Errorf("decode OTIO JSON: %w", err)
	}
	root, ok := raw.(map[string]any)
	if !ok {
		return nil, &Report{}, fmt.Errorf("timeline must be a JSON object")
	}
	timeline, err := decodeTimeline(root, "")
	if err != nil {
		return nil, &Report{}, err
	}
	return timeline, Analyze(timeline), nil
}

// Load reads and imports an OTIO JSON file.
func Load(path string) (*Timeline, *Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &Report{}, fmt.Errorf("read %s: %w", path, err)
	}
	return Decode(data)
}

func decodeTimeline(raw map[string]any, path string) (*Timeline, error) {
	schema, err := requiredString(raw, "OTIO_SCHEMA", path)
	if err != nil {
		return nil, err
	}
	if schemaFamily(schema) != "Timeline" {
		return nil, fmt.Errorf("%s: expected Timeline schema, got %q", path, schema)
	}
	object, err := decodeObject(raw, path, timelineFields)
	if err != nil {
		return nil, err
	}
	timeline := &Timeline{Object: object}
	timeline.Schema = schema
	if value, ok := raw["global_start_time"]; ok {
		timeline.globalStartSet = true
		if value != nil {
			timeline.GlobalStartTime, err = decodeRationalTime(value, path+"/global_start_time")
			if err != nil {
				return nil, err
			}
		}
	}
	tracksValue, ok := raw["tracks"]
	if !ok || tracksValue == nil {
		return nil, fmt.Errorf("%s.tracks is required", path)
	}
	tracks, ok := tracksValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.tracks must be an object", path)
	}
	timeline.Tracks, err = decodeStack(tracks, path+"/tracks")
	if err != nil {
		return nil, err
	}
	return timeline, nil
}

func decodeStack(raw map[string]any, path string) (*Stack, error) {
	schema, err := requiredString(raw, "OTIO_SCHEMA", path)
	if err != nil {
		return nil, err
	}
	if schemaFamily(schema) != "Stack" {
		return nil, fmt.Errorf("%s: expected Stack schema, got %q", path, schema)
	}
	object, err := decodeObject(raw, path, stackFields)
	if err != nil {
		return nil, err
	}
	stack := &Stack{Object: object}
	stack.Schema = schema
	children, err := decodeChildren(raw["children"], path+"/children", func(child map[string]any, childPath string) (Item, error) {
		if family(child) == "Track" {
			return decodeTrack(child, childPath)
		}
		return decodeOpaque(child), nil
	})
	if err != nil {
		return nil, err
	}
	stack.Children = children
	return stack, nil
}

func decodeTrack(raw map[string]any, path string) (*Track, error) {
	schema, err := requiredString(raw, "OTIO_SCHEMA", path)
	if err != nil {
		return nil, err
	}
	if schemaFamily(schema) != "Track" {
		return nil, fmt.Errorf("%s: expected Track schema, got %q", path, schema)
	}
	object, err := decodeObject(raw, path, trackFields)
	if err != nil {
		return nil, err
	}
	track := &Track{Object: object}
	track.Schema = schema
	children, err := decodeChildren(raw["children"], path+"/children", func(child map[string]any, childPath string) (Item, error) {
		switch family(child) {
		case "Clip":
			return decodeClip(child, childPath)
		case "Gap":
			return decodeGap(child, childPath)
		default:
			return decodeOpaque(child), nil
		}
	})
	if err != nil {
		return nil, err
	}
	track.Children = children
	return track, nil
}

func decodeChildren(raw any, path string, decode func(map[string]any, string) (Item, error)) ([]Item, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", path)
	}
	children := make([]Item, 0, len(values))
	for i, value := range values {
		childPath := indexPath(path, i)
		child, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", childPath)
		}
		item, err := decode(child, childPath)
		if err != nil {
			return nil, err
		}
		children = append(children, item)
	}
	return children, nil
}

func decodeClip(raw map[string]any, path string) (*Clip, error) {
	object, err := decodeObject(raw, path, clipFields)
	if err != nil {
		return nil, err
	}
	return &Clip{Object: object}, nil
}

func decodeGap(raw map[string]any, path string) (*Gap, error) {
	object, err := decodeObject(raw, path, gapFields)
	if err != nil {
		return nil, err
	}
	return &Gap{Object: object}, nil
}

func decodeOpaque(raw map[string]any) *Opaque {
	schema, _ := raw["OTIO_SCHEMA"].(string)
	return &Opaque{Schema: schema, Raw: raw}
}

func family(raw map[string]any) string {
	schema, _ := raw["OTIO_SCHEMA"].(string)
	return schemaFamily(schema)
}

func decodeObject(raw map[string]any, path string, known map[string]bool) (Object, error) {
	var object Object
	object.Schema, _ = raw["OTIO_SCHEMA"].(string)
	if value, ok := raw["name"]; ok {
		name, ok := value.(string)
		if !ok {
			return Object{}, fmt.Errorf("%s.name must be a string", path)
		}
		object.Name = name
		object.nameSet = true
	}
	if value, ok := raw["metadata"]; ok {
		metadata, err := decodeMap(value, path+"/metadata")
		if err != nil {
			return Object{}, err
		}
		object.Metadata = metadata
		object.metadataSet = true
	}
	if known["source_range"] {
		if value, ok := raw["source_range"]; ok {
			object.sourceRangeSet = true
			if value != nil {
				timeRange, err := decodeTimeRange(value, path+"/source_range")
				if err != nil {
					return Object{}, err
				}
				object.SourceRange = timeRange
			}
		}
	}
	if value, ok := raw["markers"]; ok {
		object.markersNull = value == nil
		markers, err := decodeMarkers(value, path+"/markers")
		if err != nil {
			return Object{}, err
		}
		object.Markers = markers
		object.markersSet = true
	}
	object.Extra = extraFields(raw, known)
	return object, nil
}

func decodeMarkers(raw any, path string) ([]Marker, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", path)
	}
	markers := make([]Marker, 0, len(values))
	for i, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", indexPath(path, i))
		}
		marker, err := decodeMarker(object, indexPath(path, i))
		if err != nil {
			return nil, err
		}
		markers = append(markers, marker)
	}
	return markers, nil
}

func decodeMarker(raw map[string]any, path string) (Marker, error) {
	schema, err := requiredString(raw, "OTIO_SCHEMA", path)
	if err != nil {
		return Marker{}, err
	}
	if schemaFamily(schema) != "Marker" {
		return Marker{}, fmt.Errorf("%s: expected Marker schema, got %q", path, schema)
	}
	marker := Marker{Schema: schema}
	if value, ok := raw["name"]; ok {
		name, ok := value.(string)
		if !ok {
			return Marker{}, fmt.Errorf("%s.name must be a string", path)
		}
		marker.Name = name
		marker.nameSet = true
	}
	if value, ok := raw["color"]; ok {
		color, ok := value.(string)
		if !ok {
			return Marker{}, fmt.Errorf("%s.color must be a string", path)
		}
		marker.Color = color
		marker.colorSet = true
	}
	if value, ok := raw["metadata"]; ok {
		metadata, err := decodeMap(value, path+"/metadata")
		if err != nil {
			return Marker{}, err
		}
		marker.Metadata = metadata
		marker.metadataSet = true
	}
	marked, markedOK := raw["marked_range"]
	legacy, legacyOK := raw["range"]
	if markedOK && legacyOK {
		return Marker{}, fmt.Errorf("%s has both marked_range and range; refusing to discard either", path)
	}
	switch {
	case markedOK:
		marker.rangeField = "marked_range"
		marker.rangeSet = true
		if marked != nil {
			marker.MarkedRange, err = decodeTimeRange(marked, path+"/marked_range")
		}
	case legacyOK:
		marker.rangeField = "range"
		marker.rangeSet = true
		if legacy != nil {
			marker.MarkedRange, err = decodeTimeRange(legacy, path+"/range")
		}
	}
	if err != nil {
		return Marker{}, err
	}
	marker.Extra = extraFields(raw, markerFields)
	return marker, nil
}

func decodeTimeRange(raw any, path string) (*TimeRange, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", path)
	}
	schema, err := requiredString(object, "OTIO_SCHEMA", path)
	if err != nil {
		return nil, err
	}
	if schemaFamily(schema) != "TimeRange" {
		return nil, fmt.Errorf("%s: expected TimeRange schema, got %q", path, schema)
	}
	startValue, ok := object["start_time"]
	if !ok {
		return nil, fmt.Errorf("%s.start_time is required", path)
	}
	durationValue, ok := object["duration"]
	if !ok {
		return nil, fmt.Errorf("%s.duration is required", path)
	}
	start, err := decodeRationalTime(startValue, path+"/start_time")
	if err != nil {
		return nil, err
	}
	duration, err := decodeRationalTime(durationValue, path+"/duration")
	if err != nil {
		return nil, err
	}
	return &TimeRange{
		Schema:   schema,
		Start:    *start,
		Duration: *duration,
		Extra:    extraFields(object, rangeFields),
	}, nil
}

func decodeRationalTime(raw any, path string) (*RationalTime, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", path)
	}
	schema, err := requiredString(object, "OTIO_SCHEMA", path)
	if err != nil {
		return nil, err
	}
	if schemaFamily(schema) != "RationalTime" {
		return nil, fmt.Errorf("%s: expected RationalTime schema, got %q", path, schema)
	}
	rate, err := requiredNumber(object, "rate", path)
	if err != nil {
		return nil, err
	}
	value, err := requiredNumber(object, "value", path)
	if err != nil {
		return nil, err
	}
	time := &RationalTime{
		Schema: schema,
		Rate:   rate,
		Value:  value,
		Extra:  extraFields(object, timeFields),
	}
	if err := time.validate(path); err != nil {
		return nil, err
	}
	return time, nil
}

func decodeMap(raw any, path string) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", path)
	}
	return object, nil
}

func extraFields(raw map[string]any, known map[string]bool) map[string]any {
	var extra map[string]any
	for key, value := range raw {
		if known[key] {
			continue
		}
		if extra == nil {
			extra = make(map[string]any)
		}
		extra[key] = value
	}
	return extra
}

func requiredString(raw map[string]any, key, path string) (string, error) {
	value, ok := raw[key]
	if !ok {
		return "", fmt.Errorf("%s.%s is required", path, key)
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("%s.%s must be a non-empty string", path, key)
	}
	return text, nil
}

func requiredNumber(raw map[string]any, key, path string) (float64, error) {
	value, ok := raw[key]
	if !ok {
		return 0, fmt.Errorf("%s.%s is required", path, key)
	}
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseFloat(string(number), 64)
		if err != nil {
			return 0, fmt.Errorf("%s.%s must be a number: %w", path, key, err)
		}
		return parsed, nil
	case float64:
		return number, nil
	default:
		return 0, fmt.Errorf("%s.%s must be a number", path, key)
	}
}
