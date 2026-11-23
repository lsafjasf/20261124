package hdr

// Preset controls how much highlight range is folded into the displayable
// interval. A lower knee starts the shoulder earlier and therefore compresses
// more aggressively. Negative components are treated as gamut information, not
// as overflow, so presets do not clamp them per channel.
type Preset struct {
	Name string
	// Knee is the largest channel value that is passed through unchanged.
	// It must be in (0,1).
	Knee float64
	// DetailLimit is the input magnitude above which highlight detail can no
	// longer be represented faithfully in an 8-bit thumbnail. Values above it
	// are still tone mapped, but are recorded as unrecoverable detail.
	DetailLimit float64
}

var (
	// Mild keeps most of the highlight range and is appropriate for previews
	// that will be tone mapped again later.
	Mild = Preset{Name: "mild", Knee: 0.92, DetailLimit: 8}
	// Balanced is the default thumbnail preset.
	Balanced = Preset{Name: "balanced", Knee: 0.75, DetailLimit: 32}
	// Strong folds the widest range into the display interval and is useful
	// for small, low-contrast thumbnails.
	Strong = Preset{Name: "strong", Knee: 0.55, DetailLimit: 128}
)

// Presets returns the three supported compression strengths in increasing
// order of compression.
func Presets() []Preset {
	return []Preset{Mild, Balanced, Strong}
}

func (p Preset) normalized() Preset {
	p.Knee = clamp(p.Knee, 0.01, 0.999)
	if p.DetailLimit <= 1 {
		p.DetailLimit = 1
	}
	if p.Name == "" {
		p.Name = "custom"
	}
	return p
}
