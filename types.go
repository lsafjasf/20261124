package otio

import (
	"fmt"
	"math"
	"strings"
)

// ItemKind identifies the supported timeline child types.
type ItemKind string

const (
	KindTrack  ItemKind = "track"
	KindClip   ItemKind = "clip"
	KindGap    ItemKind = "gap"
	KindOpaque ItemKind = "opaque"
)

// RationalTime is an OTIO RationalTime. Rate is frames per second.
type RationalTime struct {
	Schema string
	Rate   float64
	Value  float64
	Extra  map[string]any
}

// TimeRange is an OTIO TimeRange. Start and Duration may have different rates.
type TimeRange struct {
	Schema   string
	Start    RationalTime
	Duration RationalTime
	Extra    map[string]any
}

// Object contains fields shared by OTIO timeline objects.
type Object struct {
	Schema      string
	Name        string
	Metadata    map[string]any
	SourceRange *TimeRange
	Markers     []Marker
	Extra       map[string]any

	nameSet        bool
	metadataSet    bool
	sourceRangeSet bool
	markersSet     bool
	markersNull    bool
}

// Marker is an OTIO marker. Markers may overlap freely.
type Marker struct {
	Schema      string
	Name        string
	Color       string
	Metadata    map[string]any
	MarkedRange *TimeRange
	Extra       map[string]any

	nameSet     bool
	colorSet    bool
	metadataSet bool
	rangeSet    bool
	rangeField  string
}

// Timeline is the root OTIO object.
type Timeline struct {
	Object
	GlobalStartTime *RationalTime
	Tracks          *Stack

	globalStartSet bool
}

// Stack holds the tracks of a timeline.
type Stack struct {
	Object
	Children []Item
}

// Track holds clips, gaps, and opaque unsupported children.
type Track struct {
	Object
	Children []Item
}

// Clip references a source range, but media references are deliberately not
// interpreted by this subset.
type Clip struct {
	Object
}

// Gap is an empty interval on a track.
type Gap struct {
	Object
}

// Opaque preserves an object schema outside the supported subset.
type Opaque struct {
	Schema string
	Raw    map[string]any
}

// Item is a child of a Stack or Track.
type Item interface {
	Kind() ItemKind
	ItemName() string
	ItemSchema() string
	itemNode()
}

func (t *Track) Kind() ItemKind      { return KindTrack }
func (t *Track) ItemName() string    { return t.Name }
func (t *Track) ItemSchema() string  { return defaultSchema(t.Schema, "Track.1") }
func (t *Track) itemNode()           {}
func (c *Clip) Kind() ItemKind       { return KindClip }
func (c *Clip) ItemName() string     { return c.Name }
func (c *Clip) ItemSchema() string   { return defaultSchema(c.Schema, "Clip.2") }
func (c *Clip) itemNode()            {}
func (g *Gap) Kind() ItemKind        { return KindGap }
func (g *Gap) ItemName() string      { return g.Name }
func (g *Gap) ItemSchema() string    { return defaultSchema(g.Schema, "Gap.1") }
func (g *Gap) itemNode()             {}
func (o *Opaque) Kind() ItemKind     { return KindOpaque }
func (o *Opaque) ItemSchema() string { return o.Schema }
func (o *Opaque) itemNode()          {}
func (o *Opaque) ItemName() string {
	if s, ok := o.Raw["name"].(string); ok {
		return s
	}
	return ""
}

func defaultSchema(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func schemaFamily(schema string) string {
	if i := strings.LastIndexByte(schema, '.'); i >= 0 {
		return schema[:i]
	}
	return schema
}

func (t RationalTime) validate(path string) error {
	if !finite(t.Rate) || t.Rate <= 0 {
		return fmt.Errorf("%s.rate must be finite and greater than zero", path)
	}
	if !finite(t.Value) {
		return fmt.Errorf("%s.value must be finite", path)
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
