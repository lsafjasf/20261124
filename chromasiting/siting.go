// Package chromasiting identifies 4:2:0 chroma sample siting declared by
// different container/bitstream fields, flags conflicting declarations for
// manual review, and measures how siting shifts color edges across
// downsampling/upsampling.
package chromasiting

// Siting names a 4:2:0 chroma sample placement.
//
// The three placements recognized by this tool:
//
//	SitingMPEG2  MPEG-2 style ("left"): chroma co-sited horizontally with the
//	             left luma column, centered vertically between luma rows.
//	SitingJPEG   JPEG style ("center"): chroma centered horizontally and
//	             vertically inside the 2x2 luma block (ITU T.81).
//	SitingCenter H.264 VUI "center" (chroma_sample_location_type = 1).
//
// JPEG and Center are kept as distinct labels because containers declare them
// through different fields, but for 4:2:0 they describe the same physical
// position (0.5, 0.5); SamePosition normalizes them before comparison.
type Siting int

const (
	SitingUnknown Siting = iota
	SitingMPEG2
	SitingJPEG
	SitingCenter
)

func (s Siting) String() string {
	switch s {
	case SitingMPEG2:
		return "MPEG-2(left)"
	case SitingJPEG:
		return "JPEG(center)"
	case SitingCenter:
		return "Center"
	default:
		return "Unknown"
	}
}

// Offset returns the chroma sample position inside the 2x2 luma block, in
// luma sample units, relative to the top-left luma sample of the block.
// Chroma sample (cx, cy) therefore sits at luma position (2*cx+ox, 2*cy+oy).
func (s Siting) Offset() (ox, oy float64) {
	switch s {
	case SitingMPEG2:
		return 0.0, 0.5
	case SitingJPEG, SitingCenter:
		return 0.5, 0.5
	default:
		return 0, 0
	}
}

// SamePosition reports whether two sitings describe the same physical chroma
// position. JPEG and Center coincide for 4:2:0, so a JPEG label next to a
// Center label is not a conflict.
func (s Siting) SamePosition(o Siting) bool {
	if s == SitingUnknown || o == SitingUnknown {
		return false
	}
	ox1, oy1 := s.Offset()
	ox2, oy2 := o.Offset()
	return ox1 == ox2 && oy1 == oy2
}
