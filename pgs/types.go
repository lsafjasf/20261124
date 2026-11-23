package pgs

import (
	"fmt"
	"image"
	"time"
)

// Rect is an axis-aligned rectangle. Width and Height are always non-negative.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Empty reports whether the rectangle contains no pixels.
func (r Rect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// Intersect returns the overlap of a and b.
func Intersect(a, b Rect) Rect {
	x0 := max(a.X, b.X)
	y0 := max(a.Y, b.Y)
	x1 := min(a.X+a.Width, b.X+b.Width)
	y1 := min(a.Y+a.Height, b.Y+b.Height)
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

// WindowInfo describes one PGS window in source and output coordinates.
type WindowInfo struct {
	ID            uint8    `json:"id"`
	Source        Rect     `json:"source"`
	Clipped       Rect     `json:"clipped"`
	Output        Rect     `json:"output"`
	OutputClipped Rect     `json:"output_clipped"`
	ObjectIDs     []uint16 `json:"object_ids,omitempty"`
}

// ObjectInfo describes how a composition object was clipped and scaled.
type ObjectInfo struct {
	ID          uint16 `json:"id"`
	WindowID    uint8  `json:"window_id"`
	Cropped     bool   `json:"cropped"`
	Composition Rect   `json:"composition"`
	Source      Rect   `json:"source"`
	Clipped     Rect   `json:"clipped"`
	Output      Rect   `json:"output,omitempty"`
}

// Frame is a rendered subtitle layer. Image uses premultiplied RGBA, matching
// image.RGBA. Transparent pixels have zero RGB and zero A.
type Frame struct {
	PTS          uint32        `json:"pts"`
	Time         time.Duration `json:"-"`
	SourceWidth  int           `json:"source_width"`
	SourceHeight int           `json:"source_height"`
	Width        int           `json:"width"`
	Height       int           `json:"height"`
	PaletteID    uint8         `json:"palette_id"`
	Image        *image.RGBA   `json:"-"`
	Windows      []WindowInfo  `json:"windows"`
	Objects      []ObjectInfo  `json:"objects"`
}

// RenderOptions selects a timestamp and optional output dimensions. If both
// dimensions are zero, the composition's native resolution is used. If only
// one dimension is set, the other is derived from the source aspect ratio.
type RenderOptions struct {
	At     time.Duration
	PTS    uint32
	UsePTS bool
	Width  int
	Height int
}

// ErrNoComposition means the requested timestamp precedes the first PCS.
var ErrNoComposition = fmt.Errorf("pgs: no composition at requested timestamp")

type paletteEntry struct {
	r, g, b, a uint8
}

type parsedPalette struct {
	id      uint8
	version uint8
	entries [256]paletteEntry
	present [256]bool
}

type parsedObject struct {
	id       uint16
	version  uint8
	width    int
	height   int
	data     []byte
	pixels   []uint8
	complete bool
}

type parsedWindow struct {
	id   uint8
	rect Rect
}

type compositionObject struct {
	id       uint16
	windowID uint8
	cropped  bool
	forced   bool
	x        int
	y        int
	crop     Rect
}

type parsedComposition struct {
	pts           uint32
	width         int
	height        int
	number        uint16
	state         uint8
	paletteUpdate bool
	paletteID     uint8
	objects       []compositionObject
}
