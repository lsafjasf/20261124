package spatial

import "math"

// SourceKind distinguishes channel-based bed content from dynamic objects.
type SourceKind uint8

const (
	// BedSource is channel-based content that should not be distance-attenuated.
	BedSource SourceKind = iota + 1
	// ObjectSource is an object placed at a listener-local position.
	ObjectSource
)

// String returns a stable human-readable name for the source kind.
func (k SourceKind) String() string {
	switch k {
	case BedSource:
		return "bed"
	case ObjectSource:
		return "object"
	default:
		return "unknown"
	}
}

// BedChannel is one channel of channel-based content. Gain is linear amplitude.
type BedChannel struct {
	ID      string
	Channel string
	Gain    float64
}

// Object is a dynamic mono source. Gain is linear amplitude.
type Object struct {
	ID       string
	Position Vec3
	Gain     float64
}

// Scene is a bed plus a set of dynamic objects.
type Scene struct {
	Bed     []BedChannel
	Objects []Object
}

// Contribution is a linear amplitude applied to one speaker.
type Contribution struct {
	Speaker string
	Gain    float64
}

// WarningCode identifies a non-fatal spatial degradation.
type WarningCode string

const (
	// WarnNoDirection means a source is at the listener and therefore has no bearing.
	WarnNoDirection WarningCode = "no_direction"
	// WarnHeightFolded means elevation was discarded by a layout without height.
	WarnHeightFolded WarningCode = "height_folded"
	// WarnHeightOnly means a layout without height received a purely vertical source.
	WarnHeightOnly WarningCode = "height_only_folded_front"
	// WarnLFEOmni means LFE was spread over non-LFE speakers during bed folding.
	WarnLFEOmni WarningCode = "lfe_omni"
	// WarnOutsideCoverage means no pannable speaker is in the source hemisphere.
	WarnOutsideCoverage WarningCode = "outside_speaker_coverage"
)

// Warning records a fallback that changed the spatial interpretation.
type Warning struct {
	Code    WarningCode
	Source  string
	Message string
}

// SourceResult is the complete set of speaker gains produced by one input.
type SourceResult struct {
	ID                string
	Kind              SourceKind
	Channel           string
	Position          Vec3
	Distance          float64
	DistanceGain      float64
	Direction         Vec3
	HeightLossDegrees float64
	Contributions     []Contribution
	Warnings          []Warning
}

// Mix is the summed output and the per-source plan that produced it.
type Mix struct {
	Layout   string
	Sources  []SourceResult
	Speakers []Contribution
	Peak     float64
	Warnings []Warning
}

func equalContributions(speakers []Speaker) []Contribution {
	if len(speakers) == 0 {
		return nil
	}
	gain := 1 / math.Sqrt(float64(len(speakers)))
	result := make([]Contribution, 0, len(speakers))
	for _, speaker := range speakers {
		result = append(result, Contribution{Speaker: speaker.Name, Gain: gain})
	}
	return result
}

func scaleContributions(in []Contribution, scale float64) []Contribution {
	result := make([]Contribution, 0, len(in))
	for _, contribution := range in {
		if contribution.Gain == 0 || scale == 0 {
			continue
		}
		result = append(result, Contribution{
			Speaker: contribution.Speaker,
			Gain:    contribution.Gain * scale,
		})
	}
	return result
}
