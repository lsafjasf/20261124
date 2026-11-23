package otio

import (
	"fmt"
)

// Validate checks structural requirements and all RationalTime rates. Missing
// media and overlapping markers are valid; Analyze reports the former.
func (timeline *Timeline) Validate() error {
	return validateForEncode(timeline)
}

func validateForEncode(timeline *Timeline) error {
	if timeline == nil {
		return fmt.Errorf("timeline is nil")
	}
	if err := validateSchema(timeline.Schema, "Timeline", "Timeline.1", "/"); err != nil {
		return err
	}
	if err := validateObject(&timeline.Object, "/"); err != nil {
		return err
	}
	if timeline.GlobalStartTime != nil {
		if err := timeline.GlobalStartTime.validate("/global_start_time"); err != nil {
			return err
		}
	}
	if timeline.Tracks == nil {
		return fmt.Errorf("/tracks is required")
	}
	return validateStack(timeline.Tracks, "/tracks")
}

func validateStack(stack *Stack, path string) error {
	if stack == nil {
		return fmt.Errorf("%s is nil", path)
	}
	if err := validateSchema(stack.Schema, "Stack", "Stack.1", path); err != nil {
		return err
	}
	if err := validateObject(&stack.Object, path); err != nil {
		return err
	}
	for i, child := range stack.Children {
		childPath := indexPath(path+"/children", i)
		switch item := child.(type) {
		case *Track:
			if item == nil {
				return fmt.Errorf("%s is a nil Track", childPath)
			}
			if err := validateTrack(item, childPath); err != nil {
				return err
			}
		case *Opaque:
			if item == nil {
				return fmt.Errorf("%s is a nil Opaque item", childPath)
			}
		case nil:
			return fmt.Errorf("%s is nil", childPath)
		default:
			return fmt.Errorf("%s cannot contain %s items", childPath, item.Kind())
		}
	}
	return nil
}

func validateTrack(track *Track, path string) error {
	if track == nil {
		return fmt.Errorf("%s is nil", path)
	}
	if err := validateSchema(track.Schema, "Track", "Track.1", path); err != nil {
		return err
	}
	if err := validateObject(&track.Object, path); err != nil {
		return err
	}
	for i, child := range track.Children {
		childPath := indexPath(path+"/children", i)
		switch item := child.(type) {
		case *Clip:
			if item == nil {
				return fmt.Errorf("%s is a nil Clip", childPath)
			}
			if err := validateSchema(item.Schema, "Clip", "Clip.2", childPath); err != nil {
				return err
			}
			if err := validateObject(&item.Object, childPath); err != nil {
				return err
			}
		case *Gap:
			if item == nil {
				return fmt.Errorf("%s is a nil Gap", childPath)
			}
			if err := validateSchema(item.Schema, "Gap", "Gap.1", childPath); err != nil {
				return err
			}
			if err := validateObject(&item.Object, childPath); err != nil {
				return err
			}
		case *Opaque:
			if item == nil {
				return fmt.Errorf("%s is a nil Opaque item", childPath)
			}
		case nil:
			return fmt.Errorf("%s is nil", childPath)
		default:
			return fmt.Errorf("%s cannot contain %s items", childPath, item.Kind())
		}
	}
	return nil
}

func validateObject(object *Object, path string) error {
	if object.SourceRange != nil {
		if err := object.SourceRange.Validate(path + "/source_range"); err != nil {
			return err
		}
	}
	for i := range object.Markers {
		markerPath := indexPath(path+"/markers", i)
		marker := &object.Markers[i]
		if err := validateSchema(marker.Schema, "Marker", "Marker.2", markerPath); err != nil {
			return err
		}
		if marker.MarkedRange != nil {
			if err := marker.MarkedRange.Validate(markerPath + "/marked_range"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSchema(schema, family, fallback, path string) error {
	actual := defaultSchema(schema, fallback)
	if schemaFamily(actual) != family {
		return fmt.Errorf("%s: expected %s schema, got %q", path, family, actual)
	}
	return nil
}

// Tracks returns the typed tracks in stack order.
func (stack *Stack) Tracks() []*Track {
	if stack == nil {
		return nil
	}
	tracks := make([]*Track, 0, len(stack.Children))
	for _, child := range stack.Children {
		if track, ok := child.(*Track); ok && track != nil {
			tracks = append(tracks, track)
		}
	}
	return tracks
}

// TrackByName finds a track by exact name.
func (timeline *Timeline) TrackByName(name string) *Track {
	if timeline == nil || timeline.Tracks == nil {
		return nil
	}
	for _, track := range timeline.Tracks.Tracks() {
		if track.Name == name {
			return track
		}
	}
	return nil
}

// ClipByName finds the first clip with the exact name.
func (timeline *Timeline) ClipByName(name string) *Clip {
	if timeline == nil || timeline.Tracks == nil {
		return nil
	}
	for _, track := range timeline.Tracks.Tracks() {
		for _, child := range track.Children {
			if clip, ok := child.(*Clip); ok && clip != nil && clip.Name == name {
				return clip
			}
		}
	}
	return nil
}
