// Package scene parses and resolves MPEG-H audio scene metadata segments.
//
// A configuration segment is a complete scene snapshot. An update segment is
// applied transactionally and all references are resolved again after every
// update. Missing references are reported and never replaced by a mapping from
// an earlier layout.
package scene

import (
	"encoding/json"
	"fmt"
	"sort"
)

// SegmentKind identifies a configuration or update segment.
type SegmentKind string

const (
	SegmentConfig SegmentKind = "config"
	SegmentUpdate SegmentKind = "update"
)

// Extension is an opaque metadata extension. Data is retained so a later
// update can report it as unresolved instead of silently discarding it.
type Extension struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data,omitempty"`
}

// UnresolvedField describes a field that could not be resolved or interpreted.
// Path is a stable JSON-ish path, not an array index into an internal map.
type UnresolvedField struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Value  string `json:"value,omitempty"`
}

// Channel is one output channel in a speaker layout. Index in the rendered
// output is assigned by the order of Channels and is reported with mappings.
type Channel struct {
	ID string `json:"id"`

	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// SpeakerLayout is a named set of output channels.
type SpeakerLayout struct {
	ID       string    `json:"id"`
	Name     string    `json:"name,omitempty"`
	Channels []Channel `json:"channels"`

	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// Program is a selectable audio presentation. LayoutID names the active
// speaker layout and ObjectIDs lists the objects belonging to the program.
type Program struct {
	ID        string   `json:"id"`
	LayoutID  string   `json:"layoutId"`
	ObjectIDs []string `json:"objectIds"`

	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// Object is an audio object and its explicit channel bindings.
//
// LayoutChannels takes precedence. Channels is a layout-independent fallback:
// every referenced channel must still exist in the active layout. Neither form
// is ever inherited from a previous layout.
type Object struct {
	ID             string              `json:"id"`
	Channels       []string            `json:"channels,omitempty"`
	LayoutChannels map[string][]string `json:"layoutChannels,omitempty"`

	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// Config is a complete scene snapshot.
type Config struct {
	SceneID  string          `json:"sceneId,omitempty"`
	Layouts  []SpeakerLayout `json:"layouts"`
	Programs []Program       `json:"programs"`
	Objects  []Object        `json:"objects"`

	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
	Unresolved []UnresolvedField          `json:"-"`
}

// ProgramPatch modifies an existing program or adds a new one. A nil field is
// unchanged; an empty ObjectIDs slice clears the object list.
type ProgramPatch struct {
	ID        string    `json:"id"`
	LayoutID  *string   `json:"layoutId,omitempty"`
	ObjectIDs *[]string `json:"objectIds,omitempty"`

	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// ObjectPatch modifies an existing object or adds a new one. A nil
// LayoutChannels field is unchanged; an empty map removes all bindings.
type ObjectPatch struct {
	ID             string               `json:"id"`
	Channels       *[]string            `json:"channels,omitempty"`
	LayoutChannels *map[string][]string `json:"layoutChannels,omitempty"`

	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// Update is an explicit incremental segment. Layouts are complete upserts.
// Deletions are applied before upserts, so a delete and upsert of the same ID
// is reported as conflicting and the deletion wins.
type Update struct {
	Layouts  []SpeakerLayout `json:"layouts,omitempty"`
	Programs []ProgramPatch  `json:"programs,omitempty"`
	Objects  []ObjectPatch   `json:"objects,omitempty"`

	DeleteLayouts  []string `json:"deleteLayouts,omitempty"`
	DeletePrograms []string `json:"deletePrograms,omitempty"`
	DeleteObjects  []string `json:"deleteObjects,omitempty"`

	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
	Unresolved []UnresolvedField          `json:"-"`
}

// Segment is one configuration or update segment. Sequence is the effective
// sequence number reported by every Apply operation.
type Segment struct {
	Kind     SegmentKind `json:"type"`
	Sequence uint64      `json:"sequence"`
	Config   *Config     `json:"config,omitempty"`
	Update   *Update     `json:"update,omitempty"`

	Extensions  map[string]json.RawMessage `json:"extensions,omitempty"`
	Unresolved  []UnresolvedField          `json:"unresolved,omitempty"`
	HasSequence bool                       `json:"-"`
}

// OutputChannel identifies a channel and its zero-based output slot.
type OutputChannel struct {
	ID    string `json:"id"`
	Index int    `json:"index"`
}

// ObjectChannelMapping is a resolved object-to-output-channel relationship.
// One object can produce multiple mappings when it belongs to multiple
// programs.
type ObjectChannelMapping struct {
	ObjectID  string          `json:"objectId"`
	ProgramID string          `json:"programId"`
	LayoutID  string          `json:"layoutId"`
	Channels  []OutputChannel `json:"channels"`
}

// Snapshot is the effective scene after a segment has been applied.
type Snapshot struct {
	SceneID  string                     `json:"sceneId,omitempty"`
	Layouts  []SpeakerLayout            `json:"layouts"`
	Programs []Program                  `json:"programs"`
	Objects  []Object                   `json:"objects"`
	Ext      map[string]json.RawMessage `json:"extensions,omitempty"`
}

// Result is returned for every attempted update. EffectiveSequence is always
// populated when the segment itself could be parsed.
type Result struct {
	EffectiveSequence uint64                 `json:"effectiveSequence"`
	Revision          uint64                 `json:"revision"`
	Applied           bool                   `json:"applied"`
	Snapshot          *Snapshot              `json:"snapshot,omitempty"`
	ObjectChannels    []ObjectChannelMapping `json:"objectChannels"`
	Unresolved        []UnresolvedField      `json:"unresolved"`
	Errors            []string               `json:"errors,omitempty"`
}

func cloneExtensions(in map[string]json.RawMessage) map[string]json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

func sortedExtensionFields(prefix string, ext map[string]json.RawMessage) []UnresolvedField {
	keys := make([]string, 0, len(ext))
	for key := range ext {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]UnresolvedField, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, UnresolvedField{
			Path:   prefix + key,
			Reason: "unknown_extension",
			Value:  compactJSON(ext[key]),
		})
	}
	return fields
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(data)
}

func unresolved(path, reason string, value any) UnresolvedField {
	field := UnresolvedField{Path: path, Reason: reason}
	if value != nil {
		field.Value = fmt.Sprint(value)
	}
	return field
}
