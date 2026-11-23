package eac3sm

import "fmt"

var channelModeNames = [8]string{"1+1", "1/0", "2/0", "3/0", "2/1", "3/1", "2/2", "3/2"}

// ChannelModeName maps an acmod value to its loudspeaker layout, e.g. "3/2".
func ChannelModeName(acmod int) string {
	if acmod < 0 || acmod > 7 {
		return "unknown"
	}
	return channelModeNames[acmod]
}

// ProgramConfig is the decoder-relevant description of one program. A new
// program is only allowed to take over the decoder once a complete initial
// config has been collected; until then the old config must not be reused.
type ProgramConfig struct {
	SampleRate  int
	Acmod       int
	ChannelMode string
	LFE         bool
	DRCProfile  string
}

func (c ProgramConfig) String() string {
	mode := c.ChannelMode
	if c.LFE {
		mode += "+LFE"
	}
	return fmt.Sprintf("%s @ %d Hz, drc=%s", mode, c.SampleRate, c.DRCProfile)
}

// sameCore reports whether the decoder-hardware-relevant parts match, i.e.
// everything that requires a full decoder rebuild when it changes. DRC
// profile differences are handled by a lighter reconfiguration path.
func (c ProgramConfig) sameCore(o ProgramConfig) bool {
	return c.SampleRate == o.SampleRate && c.Acmod == o.Acmod && c.LFE == o.LFE
}

// configFromHeader derives the program config from a parsed syncframe header.
// The DRC profile is derived from the dynamic-range compression metadata
// carried in the header (compr / compr2 gain words).
func configFromHeader(h *Header) ProgramConfig {
	drc := "none"
	if h.Compr != nil {
		drc = fmt.Sprintf("compr/%d", *h.Compr)
	}
	if h.Compr2 != nil {
		drc += fmt.Sprintf("+compr2/%d", *h.Compr2)
	}
	return ProgramConfig{
		SampleRate:  h.SampleRate,
		Acmod:       h.Acmod,
		ChannelMode: ChannelModeName(h.Acmod),
		LFE:         h.LFE,
		DRCProfile:  drc,
	}
}
