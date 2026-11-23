package spatial

import (
	"fmt"
	"math"
)

const (
	defaultReferenceDistance = 1.0
	defaultPanningSharpness  = 32.0
	defaultWeightFloor       = 1e-9
)

// Config controls amplitude panning and distance attenuation. Zero values
// select the defaults.
type Config struct {
	// ReferenceDistance is the distance where distance gain starts to fall.
	// The model is unity below this distance and reference/distance beyond it.
	ReferenceDistance float64
	// PanningSharpness controls how concentrated gains are around the source
	// direction. Larger values produce narrower speaker spreads.
	PanningSharpness float64
	// WeightFloor discards speaker weights below this fraction of the largest
	// weight before energy normalization.
	WeightFloor float64
}

// Renderer maps bed channels and objects into a fixed speaker layout.
type Renderer struct {
	layout            Layout
	referenceDistance float64
	sharpness         float64
	weightFloor       float64
}

// NewRenderer validates layout and config and returns a renderer.
func NewRenderer(layout Layout, config Config) (*Renderer, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	if config.ReferenceDistance < 0 || math.IsNaN(config.ReferenceDistance) || math.IsInf(config.ReferenceDistance, 0) {
		return nil, fmt.Errorf("reference distance must be finite and non-negative")
	}
	if config.PanningSharpness < 0 || math.IsNaN(config.PanningSharpness) || math.IsInf(config.PanningSharpness, 0) {
		return nil, fmt.Errorf("panning sharpness must be finite and non-negative")
	}
	if config.WeightFloor < 0 || config.WeightFloor >= 1 || math.IsNaN(config.WeightFloor) || math.IsInf(config.WeightFloor, 0) {
		return nil, fmt.Errorf("weight floor must be finite and in [0, 1)")
	}
	if config.ReferenceDistance == 0 {
		config.ReferenceDistance = defaultReferenceDistance
	}
	if config.PanningSharpness == 0 {
		config.PanningSharpness = defaultPanningSharpness
	}
	if config.WeightFloor == 0 {
		config.WeightFloor = defaultWeightFloor
	}
	return &Renderer{
		layout:            layout,
		referenceDistance: config.ReferenceDistance,
		sharpness:         config.PanningSharpness,
		weightFloor:       config.WeightFloor,
	}, nil
}

// Render maps scene into layout using the default configuration.
func Render(layout Layout, scene Scene) (Mix, error) {
	renderer, err := NewRenderer(layout, Config{})
	if err != nil {
		return Mix{}, err
	}
	return renderer.Render(scene)
}

// Render returns every source plan and the summed speaker mix.
func (r *Renderer) Render(scene Scene) (Mix, error) {
	if r == nil {
		return Mix{}, fmt.Errorf("renderer is nil")
	}

	mix := Mix{Layout: r.layout.Name}
	totals := make([]float64, len(r.layout.Speakers))
	speakerIndex := make(map[string]int, len(r.layout.Speakers))
	for i, speaker := range r.layout.Speakers {
		speakerIndex[speaker.Name] = i
	}

	for _, bed := range scene.Bed {
		result, err := r.renderBed(bed)
		if err != nil {
			return Mix{}, err
		}
		if err := accumulateSource(&mix, totals, speakerIndex, result); err != nil {
			return Mix{}, err
		}
	}
	for _, object := range scene.Objects {
		result, err := r.renderObject(object)
		if err != nil {
			return Mix{}, err
		}
		if err := accumulateSource(&mix, totals, speakerIndex, result); err != nil {
			return Mix{}, err
		}
	}

	mix.Speakers = make([]Contribution, len(r.layout.Speakers))
	for i, speaker := range r.layout.Speakers {
		mix.Speakers[i] = Contribution{Speaker: speaker.Name, Gain: totals[i]}
		if totals[i] > mix.Peak {
			mix.Peak = totals[i]
		}
	}
	return mix, nil
}

func accumulateSource(mix *Mix, totals []float64, speakerIndex map[string]int, source SourceResult) error {
	mix.Sources = append(mix.Sources, source)
	mix.Warnings = append(mix.Warnings, source.Warnings...)
	for _, contribution := range source.Contributions {
		if index, ok := speakerIndex[contribution.Speaker]; ok {
			totals[index] += contribution.Gain
			if math.IsInf(totals[index], 0) || math.IsNaN(totals[index]) {
				return fmt.Errorf("mix overflowed speaker %q", contribution.Speaker)
			}
		}
	}
	return nil
}

func (r *Renderer) renderBed(bed BedChannel) (SourceResult, error) {
	if bed.Channel == "" {
		return SourceResult{}, fmt.Errorf("bed channel has an empty channel name")
	}
	if err := validateGain(bed.Gain); err != nil {
		return SourceResult{}, fmt.Errorf("bed %q: %w", sourceID(bed.ID, "bed/"+bed.Channel), err)
	}
	id := sourceID(bed.ID, "bed/"+bed.Channel)
	result := SourceResult{
		ID:           id,
		Kind:         BedSource,
		Channel:      bed.Channel,
		DistanceGain: 1,
	}

	if r.layout.BedMode == BedDirect {
		speaker, ok := r.layout.speakerByName(bed.Channel)
		if !ok {
			return SourceResult{}, fmt.Errorf("bed %q: channel %q is not in layout %q", id, bed.Channel, r.layout.Name)
		}
		result.Position = speaker.Position
		result.Direction = speaker.Position
		if bed.Gain > 0 {
			result.Contributions = []Contribution{{Speaker: speaker.Name, Gain: bed.Gain}}
		}
		return result, nil
	}

	if bed.Channel == ChannelLFE {
		result.Contributions = scaleContributions(equalContributions(r.pannableSpeakers()), bed.Gain)
		result.Warnings = append(result.Warnings, Warning{
			Code:    WarnLFEOmni,
			Source:  id,
			Message: "layout has no LFE channel; LFE folded equally over non-LFE speakers",
		})
		return result, nil
	}

	direction, ok := standardBedDirection(bed.Channel)
	if !ok {
		return SourceResult{}, fmt.Errorf("bed %q: unknown standard 5.1.4 channel %q", id, bed.Channel)
	}
	effective, loss, warnings := r.degradeDirection(id, direction)
	result.Position = direction
	result.Direction = effective
	result.HeightLossDegrees = loss
	result.Warnings = warnings
	if !r.hasCoverage(effective) {
		result.Warnings = append(result.Warnings, r.coverageWarning(id))
	}
	result.Contributions = scaleContributions(r.pan(effective), bed.Gain)
	return result, nil
}

func (r *Renderer) renderObject(object Object) (SourceResult, error) {
	if !object.Position.IsFinite() {
		return SourceResult{}, fmt.Errorf("object %q: position must be finite", object.ID)
	}
	if err := validateGain(object.Gain); err != nil {
		return SourceResult{}, fmt.Errorf("object %q: %w", object.ID, err)
	}

	distance := object.Position.Length()
	if math.IsInf(distance, 0) {
		return SourceResult{}, fmt.Errorf("object %q: position distance overflows", object.ID)
	}
	result := SourceResult{
		ID:           object.ID,
		Kind:         ObjectSource,
		Position:     object.Position,
		Distance:     distance,
		DistanceGain: r.distanceGain(distance),
	}

	direction, ok := object.Position.Unit()
	if !ok {
		result.Contributions = scaleContributions(equalContributions(r.pannableSpeakers()), object.Gain*result.DistanceGain)
		result.Warnings = append(result.Warnings, Warning{
			Code:    WarnNoDirection,
			Source:  object.ID,
			Message: "object is at the listener and has no direction; using equal-power omni fallback",
		})
		return result, nil
	}

	effective, loss, warnings := r.degradeDirection(object.ID, direction)
	result.Direction = effective
	result.HeightLossDegrees = loss
	result.Warnings = warnings
	if !r.hasCoverage(effective) {
		result.Warnings = append(result.Warnings, r.coverageWarning(object.ID))
	}
	result.Contributions = scaleContributions(r.pan(effective), object.Gain*result.DistanceGain)
	return result, nil
}

func (r *Renderer) distanceGain(distance float64) float64 {
	if distance <= r.referenceDistance {
		return 1
	}
	return r.referenceDistance / distance
}

func (r *Renderer) pannableSpeakers() []Speaker {
	result := make([]Speaker, 0, len(r.layout.Speakers))
	for _, speaker := range r.layout.Speakers {
		if speaker.Name != r.layout.LFE {
			result = append(result, speaker)
		}
	}
	return result
}

func (r *Renderer) hasCoverage(direction Vec3) bool {
	unit, ok := direction.Unit()
	if !ok {
		return false
	}
	for _, speaker := range r.pannableSpeakers() {
		speakerUnit, _ := speaker.Position.Unit()
		if unit.Dot(speakerUnit) > 0 {
			return true
		}
	}
	return false
}

func (r *Renderer) coverageWarning(id string) Warning {
	return Warning{
		Code:    WarnOutsideCoverage,
		Source:  id,
		Message: fmt.Sprintf("layout %q has no speaker in the source hemisphere; nearest-energy fallback used", r.layout.Name),
	}
}

func (r *Renderer) pan(direction Vec3) []Contribution {
	speakers := r.pannableSpeakers()
	unit, ok := direction.Unit()
	if !ok {
		return equalContributions(speakers)
	}

	weights := make([]float64, len(speakers))
	maxWeight := 0.0
	for i, speaker := range speakers {
		speakerUnit, _ := speaker.Position.Unit()
		cosine := clamp(unit.Dot(speakerUnit), -1, 1)
		weight := math.Exp(r.sharpness * (cosine - 1))
		weights[i] = weight
		if weight > maxWeight {
			maxWeight = weight
		}
	}
	if maxWeight == 0 {
		return equalContributions(speakers)
	}

	floor := maxWeight * r.weightFloor
	energy := 0.0
	for i := range weights {
		if weights[i] < floor {
			weights[i] = 0
			continue
		}
		energy += weights[i] * weights[i]
	}
	if energy == 0 {
		return equalContributions(speakers)
	}

	scale := 1 / math.Sqrt(energy)
	result := make([]Contribution, 0, len(speakers))
	for i, weight := range weights {
		if weight == 0 {
			continue
		}
		result = append(result, Contribution{Speaker: speakers[i].Name, Gain: weight * scale})
	}
	return result
}

func (r *Renderer) degradeDirection(id string, direction Vec3) (Vec3, float64, []Warning) {
	unit, ok := direction.Unit()
	if !ok {
		return Vec3{}, 0, nil
	}
	elevation := math.Asin(clamp(unit.Y, -1, 1))
	loss := math.Abs(elevation) * 180 / math.Pi
	if r.layout.HasHeight {
		return unit, 0, nil
	}

	horizontal, ok := unit.Horizontal().Unit()
	if ok {
		if loss <= 1e-9 {
			return horizontal, 0, nil
		}
		return horizontal, loss, []Warning{{
			Code:    WarnHeightFolded,
			Source:  id,
			Message: fmt.Sprintf("layout %q has no height; %.3f degrees of elevation folded to %.3f degrees azimuth", r.layout.Name, loss, azimuthDegrees(horizontal)),
		}}
	}

	return Vec3{Z: 1}, loss, []Warning{{
		Code:    WarnHeightOnly,
		Source:  id,
		Message: "source has no horizontal bearing; vertical direction folded to front",
	}}
}

func validateGain(gain float64) error {
	if math.IsNaN(gain) || math.IsInf(gain, 0) {
		return fmt.Errorf("gain must be finite")
	}
	if gain < 0 {
		return fmt.Errorf("gain must be non-negative")
	}
	return nil
}

func sourceID(id, fallback string) string {
	if id != "" {
		return id
	}
	return fallback
}

func azimuthDegrees(direction Vec3) float64 {
	return math.Atan2(direction.X, direction.Z) * 180 / math.Pi
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
