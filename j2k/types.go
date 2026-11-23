// Package j2k parses JPEG 2000 codestream structure without decoding pixels.
package j2k

import "fmt"

// ByteRange is a half-open byte interval [Start, End).
type ByteRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Len returns the number of bytes covered by the range.
func (r ByteRange) Len() int {
	if r.End <= r.Start {
		return 0
	}
	return r.End - r.Start
}

func (r ByteRange) String() string {
	return fmt.Sprintf("[%d,%d)", r.Start, r.End)
}

// Severity describes whether an issue prevents further structural analysis.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Issue is a structural problem. Range always identifies the bytes that caused
// or contain the problem; Related points at the other side of a conflict.
type Issue struct {
	Code     string      `json:"code"`
	Severity Severity    `json:"severity"`
	Message  string      `json:"message"`
	Range    ByteRange   `json:"range"`
	Related  []ByteRange `json:"related,omitempty"`
}

// Marker is a marker or marker segment found by the parser.
type Marker struct {
	Name         string    `json:"name"`
	Code         uint16    `json:"code"`
	Scope        string    `json:"scope"`
	TilePart     int       `json:"tilePart,omitempty"`
	Range        ByteRange `json:"range"`
	MarkerRange  ByteRange `json:"markerRange"`
	PayloadRange ByteRange `json:"payloadRange"`
	Length       int       `json:"length"`
	Standalone   bool      `json:"standalone"`
}

// SIZ is the image and tile geometry from the SIZ marker segment.
type SIZ struct {
	Range      ByteRange   `json:"range"`
	Rsiz       uint16      `json:"rsiz"`
	Xsiz       uint32      `json:"xsiz"`
	Ysiz       uint32      `json:"ysiz"`
	XOsiz      uint32      `json:"xOsiz"`
	YOsiz      uint32      `json:"yOsiz"`
	XTsiz      uint32      `json:"xTsiz"`
	YTsiz      uint32      `json:"yTsiz"`
	XTOsiz     uint32      `json:"xTOsiz"`
	YTOsiz     uint32      `json:"yTOsiz"`
	Csiz       uint16      `json:"csiz"`
	Components []Component `json:"components"`
	TilesX     int         `json:"tilesX"`
	TilesY     int         `json:"tilesY"`
}

// Component describes one image component in SIZ.
type Component struct {
	Index int       `json:"index"`
	Ssiz  uint8     `json:"ssiz"`
	XRsiz uint8     `json:"xRsiz"`
	YRsiz uint8     `json:"yRsiz"`
	Range ByteRange `json:"range"`
}

// PrecinctSize is the width and height exponent at one resolution level.
type PrecinctSize struct {
	WidthExp  int `json:"widthExp"`
	HeightExp int `json:"heightExp"`
}

// COD is the default coding style from the COD marker segment.
type COD struct {
	Range              ByteRange      `json:"range"`
	Scod               uint8          `json:"scod"`
	Progression        int            `json:"progression"`
	ProgressionName    string         `json:"progressionName"`
	Layers             int            `json:"layers"`
	MCT                uint8          `json:"mct"`
	Decompositions     int            `json:"decompositions"`
	Resolutions        int            `json:"resolutions"`
	CodeBlockWidthExp  int            `json:"codeBlockWidthExp"`
	CodeBlockHeightExp int            `json:"codeBlockHeightExp"`
	CodeBlockStyle     uint8          `json:"codeBlockStyle"`
	Transform          uint8          `json:"transform"`
	Precincts          []PrecinctSize `json:"precincts"`
}

// ComponentCoding is effective coding style for one component.
type ComponentCoding struct {
	Index              int            `json:"index"`
	Decompositions     int            `json:"decompositions"`
	Resolutions        int            `json:"resolutions"`
	CodeBlockWidthExp  int            `json:"codeBlockWidthExp"`
	CodeBlockHeightExp int            `json:"codeBlockHeightExp"`
	CodeBlockStyle     uint8          `json:"codeBlockStyle"`
	Transform          uint8          `json:"transform"`
	Precincts          []PrecinctSize `json:"precincts"`
}

// Resolution describes one resolution level of a tile component.
type Resolution struct {
	Level      int   `json:"level"`
	X0         int64 `json:"x0"`
	Y0         int64 `json:"y0"`
	X1         int64 `json:"x1"`
	Y1         int64 `json:"y1"`
	PrecinctsX int   `json:"precinctsX"`
	PrecinctsY int   `json:"precinctsY"`
}

// TileComponent holds tile-component geometry and resolution indices.
type TileComponent struct {
	Index       int          `json:"index"`
	XRsiz       int          `json:"xRsiz"`
	YRsiz       int          `json:"yRsiz"`
	X0          int64        `json:"x0"`
	Y0          int64        `json:"y0"`
	X1          int64        `json:"x1"`
	Y1          int64        `json:"y1"`
	Resolutions []Resolution `json:"resolutions"`
}

// SOT is the tile-part header fields from a SOT marker segment.
type SOT struct {
	Range            ByteRange `json:"range"`
	LengthFieldRange ByteRange `json:"lengthFieldRange"`
	Isot             int       `json:"isot"`
	Psot             uint32    `json:"psot"`
	TPsot            int       `json:"tPsot"`
	TNsot            int       `json:"tNsot"`
}

// Packet identifies one packet and its packet header/body byte ranges.
type Packet struct {
	Index          int        `json:"index"`
	Layer          int        `json:"layer"`
	Resolution     int        `json:"resolution"`
	Component      int        `json:"component"`
	Precinct       int        `json:"precinct"`
	Header         ByteRange  `json:"header"`
	Data           ByteRange  `json:"data"`
	Empty          bool       `json:"empty"`
	SOP            *ByteRange `json:"sop,omitempty"`
	EPH            *ByteRange `json:"eph,omitempty"`
	CodeBlockBytes int64      `json:"codeBlockBytes"`
}

// TilePart is one SOT/SOD tile-part.
type TilePart struct {
	Index          int        `json:"index"`
	Isot           int        `json:"isot"`
	TPsot          int        `json:"tPsot"`
	TNsot          int        `json:"tNsot"`
	DeclaredLength uint32     `json:"declaredLength"`
	Range          ByteRange  `json:"range"`
	SOT            SOT        `json:"sot"`
	Header         ByteRange  `json:"header"`
	SOD            *ByteRange `json:"sod,omitempty"`
	Data           ByteRange  `json:"data"`
	Packets        []Packet   `json:"packets"`
	Complete       bool       `json:"complete"`
	DuplicateOf    *ByteRange `json:"duplicateOf,omitempty"`
}

// Tile aggregates the pieces carrying one tile index.
type Tile struct {
	Index       int               `json:"index"`
	GridX       int               `json:"gridX"`
	GridY       int               `json:"gridY"`
	X0          int64             `json:"x0"`
	Y0          int64             `json:"y0"`
	X1          int64             `json:"x1"`
	Y1          int64             `json:"y1"`
	Layers      int               `json:"layers"`
	Resolutions int               `json:"resolutions"`
	Coding      []ComponentCoding `json:"coding"`
	Components  []TileComponent   `json:"components"`
	TileParts   []TilePart        `json:"tileParts"`
	Packets     []Packet          `json:"packets"`
}

// Codestream is the complete structural parse result.
type Codestream struct {
	ByteLength int        `json:"byteLength"`
	SOC        ByteRange  `json:"soc"`
	EOC        *ByteRange `json:"eoc,omitempty"`
	SIZ        *SIZ       `json:"siz,omitempty"`
	COD        *COD       `json:"cod,omitempty"`
	Markers    []Marker   `json:"markers"`
	Tiles      []Tile     `json:"tiles"`
	Issues     []Issue    `json:"issues"`
}

func (c *Codestream) addIssue(code string, severity Severity, r ByteRange, format string, args ...any) {
	c.Issues = append(c.Issues, Issue{
		Code:     code,
		Severity: severity,
		Message:  fmt.Sprintf(format, args...),
		Range:    r,
	})
}
