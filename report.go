package otio

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ActionRetained = "retained"
	ActionDropped  = "dropped"
)

// Report names every construct that was not interpreted by the subset. Fields
// in UnknownFields and objects in UnknownObjects are retained in the model and
// re-exported.
type Report struct {
	UnknownFields  []UnknownField  `json:"unknown_fields,omitempty"`
	UnknownObjects []UnknownObject `json:"unknown_objects,omitempty"`
	MissingMedia   []MissingMedia  `json:"missing_media,omitempty"`
}

// UnknownField is an unrecognized JSON field. Path points at the field and
// Field repeats its final JSON key for quick scanning.
type UnknownField struct {
	Path   string `json:"path"`
	Field  string `json:"field"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// UnknownObject is an OTIO object outside the supported subset. Raw stores the
// complete object, including nested fields.
type UnknownObject struct {
	Path   string `json:"path"`
	Schema string `json:"schema"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// MissingMedia identifies a clip without a usable media reference.
type MissingMedia struct {
	Path   string `json:"path"`
	Clip   string `json:"clip,omitempty"`
	Reason string `json:"reason"`
}

// Empty reports whether the report contains no findings.
func (r *Report) Empty() bool {
	return r == nil || (len(r.UnknownFields) == 0 && len(r.UnknownObjects) == 0 && len(r.MissingMedia) == 0)
}

// Summary returns a compact human-readable report.
func (r *Report) Summary() string {
	if r.Empty() {
		return "no unknown fields, unknown objects, or missing media"
	}
	return fmt.Sprintf("%d unknown field(s), %d unknown object(s), %d missing media reference(s)",
		len(r.UnknownFields), len(r.UnknownObjects), len(r.MissingMedia))
}

// Analyze builds a report for the current model. It is used after import and
// after editing so retained unknown data remains visible to callers.
func Analyze(timeline *Timeline) *Report {
	report := &Report{}
	if timeline == nil {
		return report
	}
	analyzeObject(&timeline.Object, "", report)
	analyzeTime(timeline.GlobalStartTime, "/global_start_time", report)
	if timeline.Tracks != nil {
		analyzeStack(timeline.Tracks, "/tracks", report)
	}
	report.sort()
	return report
}

func analyzeStack(stack *Stack, path string, report *Report) {
	analyzeObject(&stack.Object, path, report)
	for i, child := range stack.Children {
		childPath := indexPath(path+"/children", i)
		switch item := child.(type) {
		case *Track:
			analyzeTrack(item, childPath, report)
		case *Opaque:
			analyzeOpaque(item, childPath, report)
		default:
			if child != nil {
				report.UnknownObjects = append(report.UnknownObjects, UnknownObject{
					Path:   childPath,
					Schema: child.ItemSchema(),
					Action: ActionRetained,
					Reason: "item type is not valid in a Stack; raw value is not available",
				})
			}
		}
	}
}

func analyzeTrack(track *Track, path string, report *Report) {
	analyzeObject(&track.Object, path, report)
	for i, child := range track.Children {
		childPath := indexPath(path+"/children", i)
		switch item := child.(type) {
		case *Clip:
			analyzeObject(&item.Object, childPath, report)
			analyzeMissingMedia(item, childPath, report)
		case *Gap:
			analyzeObject(&item.Object, childPath, report)
		case *Opaque:
			analyzeOpaque(item, childPath, report)
		default:
			if child != nil {
				report.UnknownObjects = append(report.UnknownObjects, UnknownObject{
					Path:   childPath,
					Schema: child.ItemSchema(),
					Action: ActionRetained,
					Reason: "item type is not valid in a Track; raw value is not available",
				})
			}
		}
	}
}

func analyzeOpaque(item *Opaque, path string, report *Report) {
	report.UnknownObjects = append(report.UnknownObjects, UnknownObject{
		Path:   path,
		Schema: item.Schema,
		Action: ActionRetained,
		Reason: "object schema is outside the supported subset and was retained verbatim",
	})
}

func analyzeObject(object *Object, path string, report *Report) {
	for key := range object.Extra {
		report.UnknownFields = append(report.UnknownFields, UnknownField{
			Path:   fieldPath(path, key),
			Field:  key,
			Action: ActionRetained,
			Reason: "field is outside the supported subset and was retained",
		})
	}
	analyzeTimeRange(object.SourceRange, path+"/source_range", report)
	for i := range object.Markers {
		analyzeMarker(&object.Markers[i], indexPath(path+"/markers", i), report)
	}
}

func analyzeMarker(marker *Marker, path string, report *Report) {
	for key := range marker.Extra {
		report.UnknownFields = append(report.UnknownFields, UnknownField{
			Path:   fieldPath(path, key),
			Field:  key,
			Action: ActionRetained,
			Reason: "marker field is outside the supported subset and was retained",
		})
	}
	analyzeTimeRange(marker.MarkedRange, path+"/marked_range", report)
}

func analyzeTimeRange(timeRange *TimeRange, path string, report *Report) {
	if timeRange == nil {
		return
	}
	for key := range timeRange.Extra {
		report.UnknownFields = append(report.UnknownFields, UnknownField{
			Path:   fieldPath(path, key),
			Field:  key,
			Action: ActionRetained,
			Reason: "time range field is outside the supported subset and was retained",
		})
	}
	analyzeTime(&timeRange.Start, path+"/start_time", report)
	analyzeTime(&timeRange.Duration, path+"/duration", report)
}

func analyzeTime(value *RationalTime, path string, report *Report) {
	if value == nil {
		return
	}
	for key := range value.Extra {
		report.UnknownFields = append(report.UnknownFields, UnknownField{
			Path:   fieldPath(path, key),
			Field:  key,
			Action: ActionRetained,
			Reason: "rational time field is outside the supported subset and was retained",
		})
	}
}

func analyzeMissingMedia(clip *Clip, path string, report *Report) {
	oldReference, hasOld := clip.Extra["media_reference"]
	references, hasNew := clip.Extra["media_references"]
	if !hasOld && !hasNew {
		report.MissingMedia = append(report.MissingMedia, MissingMedia{
			Path:   path,
			Clip:   clip.Name,
			Reason: "clip has no media_reference or media_references field",
		})
		return
	}
	if hasOld {
		referencePath := path + "/media_reference"
		if missingReferenceReason(oldReference) {
			report.MissingMedia = append(report.MissingMedia, MissingMedia{
				Path:   referencePath,
				Clip:   clip.Name,
				Reason: missingReferenceReasonText(oldReference),
			})
		}
	}
	if hasNew {
		referencePath := path + "/media_references"
		switch value := references.(type) {
		case nil:
			report.MissingMedia = append(report.MissingMedia, MissingMedia{
				Path:   referencePath,
				Clip:   clip.Name,
				Reason: "media_references is null",
			})
		case map[string]any:
			if len(value) == 0 {
				report.MissingMedia = append(report.MissingMedia, MissingMedia{
					Path:   referencePath,
					Clip:   clip.Name,
					Reason: "media_references is empty",
				})
			}
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				reference := value[key]
				if missingReferenceReason(reference) {
					report.MissingMedia = append(report.MissingMedia, MissingMedia{
						Path:   fieldPath(referencePath, key),
						Clip:   clip.Name,
						Reason: missingReferenceReasonText(reference),
					})
				}
			}
		default:
			report.MissingMedia = append(report.MissingMedia, MissingMedia{
				Path:   referencePath,
				Clip:   clip.Name,
				Reason: "media_references is not an object",
			})
		}
	}
}

func missingReferenceReason(value any) bool {
	if value == nil {
		return true
	}
	reference, ok := value.(map[string]any)
	if !ok {
		return true
	}
	if len(reference) == 0 {
		return true
	}
	if schema, _ := reference["OTIO_SCHEMA"].(string); schemaFamily(schema) == "MissingReference" {
		return true
	}
	if target, ok := reference["target_url"]; ok {
		text, ok := target.(string)
		return !ok || strings.TrimSpace(text) == ""
	}
	if schema, _ := reference["OTIO_SCHEMA"].(string); schemaFamily(schema) == "ExternalReference" {
		return true
	}
	return false
}

func missingReferenceReasonText(value any) string {
	if value == nil {
		return "media reference is null"
	}
	reference, ok := value.(map[string]any)
	if !ok {
		return "media reference is not an object"
	}
	if len(reference) == 0 {
		return "media reference object is empty"
	}
	if schema, _ := reference["OTIO_SCHEMA"].(string); schemaFamily(schema) == "MissingReference" {
		return "media reference is explicitly MissingReference"
	}
	if target, ok := reference["target_url"]; ok {
		text, _ := target.(string)
		if strings.TrimSpace(text) == "" {
			return "target_url is empty"
		}
	}
	return "media reference has no usable target_url"
}

func (r *Report) sort() {
	sort.SliceStable(r.UnknownFields, func(i, j int) bool {
		if r.UnknownFields[i].Path == r.UnknownFields[j].Path {
			return r.UnknownFields[i].Field < r.UnknownFields[j].Field
		}
		return r.UnknownFields[i].Path < r.UnknownFields[j].Path
	})
	sort.SliceStable(r.UnknownObjects, func(i, j int) bool {
		return r.UnknownObjects[i].Path < r.UnknownObjects[j].Path
	})
	sort.SliceStable(r.MissingMedia, func(i, j int) bool {
		return r.MissingMedia[i].Path < r.MissingMedia[j].Path
	})
}

func indexPath(parent string, index int) string {
	return fmt.Sprintf("%s/%d", parent, index)
}

func fieldPath(parent, key string) string {
	escaped := strings.ReplaceAll(key, "~", "~0")
	escaped = strings.ReplaceAll(escaped, "/", "~1")
	if parent == "" {
		return "/" + escaped
	}
	return parent + "/" + escaped
}
