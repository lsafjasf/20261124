package hdr

import (
	"fmt"
)

const (
	ChromaticityScale = 0.00002
	LuminanceScale    = 0.0001
)

type Point struct {
	X uint16 `json:"x_raw"`
	Y uint16 `json:"y_raw"`
}

func (p Point) XValue() float64 { return float64(p.X) * ChromaticityScale }
func (p Point) YValue() float64 { return float64(p.Y) * ChromaticityScale }

// Primaries are stored in the SMPTE ST 2086 order: green, blue, red.
type Primaries [3]Point

type Mastering struct {
	Primaries    Primaries `json:"primaries"`
	WhitePoint   Point     `json:"white_point"`
	MaxLuminance uint32    `json:"max_luminance_raw"`
	MinLuminance uint32    `json:"min_luminance_raw"`
}

func (m Mastering) MaxNits() float64 { return float64(m.MaxLuminance) * LuminanceScale }
func (m Mastering) MinNits() float64 { return float64(m.MinLuminance) * LuminanceScale }

type ContentLight struct {
	MaxCLL  uint16 `json:"max_cll"`
	MaxFALL uint16 `json:"max_fall"`
}

type Metadata struct {
	Mastering *Mastering    `json:"mastering_display,omitempty"`
	Light     *ContentLight `json:"content_light,omitempty"`
}

func (m Metadata) Empty() bool {
	return m.Mastering == nil && m.Light == nil
}

type Source string

const (
	SourceContainer Source = "container"
	SourceBitstream Source = "bitstream"
)

type Occurrence struct {
	Source   Source
	Location string
	Metadata Metadata
	Count    int
}

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Issue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Source   Source   `json:"source,omitempty"`
	Field    string   `json:"field,omitempty"`
	Location string   `json:"location,omitempty"`
	Message  string   `json:"message"`
}

func (p Primaries) Point(name string) (Point, bool) {
	switch name {
	case "G":
		return p[0], true
	case "B":
		return p[1], true
	case "R":
		return p[2], true
	default:
		return Point{}, false
	}
}

func (p Primaries) String() string {
	return fmt.Sprintf("G(%.5f,%.5f) B(%.5f,%.5f) R(%.5f,%.5f)",
		p[0].XValue(), p[0].YValue(),
		p[1].XValue(), p[1].YValue(),
		p[2].XValue(), p[2].YValue())
}
