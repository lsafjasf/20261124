package chromasiting

import "fmt"

// FieldSource identifies which container/bitstream field a claim came from.
type FieldSource string

const (
	SourceH264VUI FieldSource = "h264_vui.chroma_sample_location_type"
	SourceMKV     FieldSource = "mkv.ChromaSitingHorz/Vert"
	SourceAV1     FieldSource = "av1.chroma_sample_position"
	SourceCodec   FieldSource = "codec_default"
)

// Claim is one field's statement about the chroma siting. Supported is false
// when the field holds a value outside the three sitings this tool knows
// (e.g. H.264 top-left); such claims always force manual review.
type Claim struct {
	Source    FieldSource
	Detail    string
	Siting    Siting
	Supported bool
}

// ContainerFields carries the siting-related fields found in one file.
// Nil pointers mean the field is absent/unspecified.
type ContainerFields struct {
	// H.264 VUI chroma_sample_location_type (0..5).
	H264ChromaSampleLocationType *int
	// Matroska ChromaSitingHorz / ChromaSitingVert:
	// 0 = unspecified, 1 = left/top collocated, 2 = half (centered).
	MKVChromaSitingHorz *uint64
	MKVChromaSitingVert *uint64
	// AV1 chroma_sample_position: 0 = unknown, 1 = vertical, 2 = colocated.
	AV1ChromaSamplePosition *int
	// CodecID supplies the codec-level default (e.g. "mjpeg", "mpeg2video").
	CodecID string
}

// Status is the outcome of resolving the declared fields.
type Status int

const (
	// StatusUnspecified: no field said anything.
	StatusUnspecified Status = iota
	// StatusResolved: every present field agrees on one physical position.
	StatusResolved
	// StatusManualReview: fields disagree (or use unsupported values).
	// The tool never auto-selects the most common value; a human must decide.
	StatusManualReview
)

func (s Status) String() string {
	switch s {
	case StatusResolved:
		return "resolved"
	case StatusManualReview:
		return "manual_review"
	default:
		return "unspecified"
	}
}

// Resolution is the result of Identify.
type Resolution struct {
	Status Status
	// Siting is meaningful only when Status == StatusResolved.
	Siting Siting
	Claims []Claim
}

// Identify collects every declared field and resolves the siting.
//
// Rules:
//   - absent/unspecified fields are ignored;
//   - if all present fields agree on one physical position, resolve to it;
//   - any disagreement (even 2-vs-1) or any unsupported value goes to
//     StatusManualReview with all claims listed. There is deliberately no
//     majority vote: conflicting fields may only be listed for a human.
func Identify(f ContainerFields) Resolution {
	claims := collectClaims(f)
	res := Resolution{Status: StatusUnspecified, Claims: claims}
	if len(claims) == 0 {
		return res
	}
	agreed := SitingUnknown
	for _, c := range claims {
		if !c.Supported || c.Siting == SitingUnknown {
			res.Status = StatusManualReview
			return res
		}
		if agreed == SitingUnknown {
			agreed = c.Siting
			continue
		}
		if !agreed.SamePosition(c.Siting) {
			res.Status = StatusManualReview
			return res
		}
	}
	res.Status = StatusResolved
	res.Siting = agreed
	return res
}

func collectClaims(f ContainerFields) []Claim {
	var claims []Claim
	if v := f.H264ChromaSampleLocationType; v != nil {
		claims = append(claims, claimH264(*v))
	}
	if f.MKVChromaSitingHorz != nil && f.MKVChromaSitingVert != nil &&
		*f.MKVChromaSitingHorz != 0 && *f.MKVChromaSitingVert != 0 {
		claims = append(claims, claimMKV(*f.MKVChromaSitingHorz, *f.MKVChromaSitingVert))
	}
	if v := f.AV1ChromaSamplePosition; v != nil && *v != 0 {
		claims = append(claims, claimAV1(*v))
	}
	if c, ok := claimCodec(f.CodecID); ok {
		claims = append(claims, c)
	}
	return claims
}

func claimH264(v int) Claim {
	c := Claim{Source: SourceH264VUI, Detail: fmt.Sprintf("value=%d", v)}
	switch v {
	case 0:
		c.Siting, c.Supported = SitingMPEG2, true
	case 1:
		c.Siting, c.Supported = SitingCenter, true
	default: // 2..5: top-left/top/bottom-left/bottom, outside this tool's scope
		c.Detail += " (top-left/top/bottom-left/bottom: unsupported)"
	}
	return c
}

func claimMKV(horz, vert uint64) Claim {
	c := Claim{Source: SourceMKV, Detail: fmt.Sprintf("horz=%d vert=%d", horz, vert)}
	switch {
	case horz == 1 && vert == 2:
		c.Siting, c.Supported = SitingMPEG2, true
	case horz == 2 && vert == 2:
		c.Siting, c.Supported = SitingCenter, true
	default:
		c.Detail += " (combination unsupported)"
	}
	return c
}

func claimAV1(v int) Claim {
	c := Claim{Source: SourceAV1, Detail: fmt.Sprintf("value=%d", v)}
	switch v {
	case 1: // CSP_VERTICAL: left collocated horizontally, centered vertically
		c.Siting, c.Supported = SitingMPEG2, true
	default: // 2 = CSP_COLOCATED (top-left), 3 = reserved
		c.Detail += " (colocated/reserved: unsupported)"
	}
	return c
}

func claimCodec(codecID string) (Claim, bool) {
	c := Claim{Source: SourceCodec, Detail: "codec=" + codecID}
	switch codecID {
	case "mjpeg", "jpeg", "jpg", "mjpa", "mjpb":
		c.Siting, c.Supported = SitingJPEG, true
	case "mpeg2video", "mpeg2":
		c.Siting, c.Supported = SitingMPEG2, true
	case "mpeg1video", "mpeg1":
		c.Siting, c.Supported = SitingCenter, true
	default:
		return Claim{}, false
	}
	return c, true
}
