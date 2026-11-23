package spatial

import (
	"fmt"
	"math"
)

// Channel names used by the standard 5.1.4 and binaural layouts.
const (
	ChannelL   = "L"
	ChannelR   = "R"
	ChannelC   = "C"
	ChannelLFE = "LFE"
	ChannelLs  = "Ls"
	ChannelRs  = "Rs"
	ChannelTFL = "TFL"
	ChannelTFR = "TFR"
	ChannelTBL = "TBL"
	ChannelTBR = "TBR"
)

// BedMode selects how channel-based bed inputs are mapped.
type BedMode uint8

const (
	// BedDirect sends a bed channel to the speaker with the same name.
	BedDirect BedMode = iota
	// BedFold derives the output from the standard 5.1.4 bed direction.
	// It is used for layouts such as binaural that do not contain all bed channels.
	BedFold
)

// Speaker is one output channel and its nominal direction from the listener.
// Position magnitude is ignored by the panner; only its direction matters.
type Speaker struct {
	Name     string
	Position Vec3
}

// Layout describes an output speaker layout.
type Layout struct {
	Name      string
	Speakers  []Speaker
	LFE       string
	HasHeight bool
	BedMode   BedMode
}

// Standard514 returns the common 5.1.4 layout. Front and surround speakers
// are at 30 and 110 degrees azimuth; height speakers are at 45 degrees
// elevation and 45/135 degrees azimuth.
func Standard514() Layout {
	return Layout{
		Name: "5.1.4",
		Speakers: []Speaker{
			{Name: ChannelL, Position: spherical(-30, 0)},
			{Name: ChannelR, Position: spherical(30, 0)},
			{Name: ChannelC, Position: spherical(0, 0)},
			{Name: ChannelLFE, Position: Vec3{}},
			{Name: ChannelLs, Position: spherical(-110, 0)},
			{Name: ChannelRs, Position: spherical(110, 0)},
			{Name: ChannelTFL, Position: spherical(-45, 45)},
			{Name: ChannelTFR, Position: spherical(45, 45)},
			{Name: ChannelTBL, Position: spherical(-135, 45)},
			{Name: ChannelTBR, Position: spherical(135, 45)},
		},
		LFE:       ChannelLFE,
		HasHeight: true,
		BedMode:   BedDirect,
	}
}

// BinauralSimple returns a two-channel headphone layout with virtual
// loudspeakers at 30 degrees left and right. It deliberately has no height
// dimension so callers exercise the height-loss fallback.
func BinauralSimple() Layout {
	return Layout{
		Name: "binaural-simple",
		Speakers: []Speaker{
			{Name: ChannelL, Position: spherical(-30, 0)},
			{Name: ChannelR, Position: spherical(30, 0)},
		},
		HasHeight: false,
		BedMode:   BedFold,
	}
}

// Validate checks the structural invariants needed by the renderer.
func (l Layout) Validate() error {
	if len(l.Speakers) == 0 {
		return fmt.Errorf("layout has no speakers")
	}
	if l.BedMode != BedDirect && l.BedMode != BedFold {
		return fmt.Errorf("invalid bed mode %d", l.BedMode)
	}

	seen := make(map[string]bool, len(l.Speakers))
	pannable := 0
	for _, speaker := range l.Speakers {
		if speaker.Name == "" {
			return fmt.Errorf("layout contains a speaker with an empty name")
		}
		if seen[speaker.Name] {
			return fmt.Errorf("duplicate speaker %q", speaker.Name)
		}
		seen[speaker.Name] = true
		if !speaker.Position.IsFinite() {
			return fmt.Errorf("speaker %q has a non-finite position", speaker.Name)
		}
		if speaker.Name != l.LFE {
			if _, ok := speaker.Position.Unit(); !ok {
				return fmt.Errorf("speaker %q has no direction", speaker.Name)
			}
			pannable++
		}
	}
	if pannable == 0 {
		return fmt.Errorf("layout has no pannable speakers")
	}
	if l.LFE != "" && !seen[l.LFE] {
		return fmt.Errorf("LFE speaker %q is not present", l.LFE)
	}
	return nil
}

func (l Layout) speakerByName(name string) (Speaker, bool) {
	for _, speaker := range l.Speakers {
		if speaker.Name == name {
			return speaker, true
		}
	}
	return Speaker{}, false
}

func spherical(azimuthDegrees, elevationDegrees float64) Vec3 {
	azimuth := azimuthDegrees * math.Pi / 180
	elevation := elevationDegrees * math.Pi / 180
	horizontal := math.Cos(elevation)
	return Vec3{
		X: math.Sin(azimuth) * horizontal,
		Y: math.Sin(elevation),
		Z: math.Cos(azimuth) * horizontal,
	}
}

func standardBedDirection(channel string) (Vec3, bool) {
	for _, speaker := range Standard514().Speakers {
		if speaker.Name == channel && channel != ChannelLFE {
			return speaker.Position, true
		}
	}
	return Vec3{}, false
}
