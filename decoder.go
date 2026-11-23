package eac3sm

// Decoder is the logical audio decoder managed by the state machine. It is
// rebuilt from scratch whenever a new program's core config is committed, so
// stale channel-mode state can never leak across a program boundary.
type Decoder struct {
	Config     ProgramConfig
	Rebuilds   int  // number of full rebuilds
	DRCUpdates int  // number of DRC-only reconfigurations
	Muted      bool // true while no committed config exists
}

// Rebuild tears down all internal state and reinitializes the decoder for
// cfg. Channel mapping, downmix matrices and sample-rate converters are all
// derived exclusively from cfg.
func (d *Decoder) Rebuild(cfg ProgramConfig) {
	d.Config = cfg
	d.Rebuilds++
	d.Muted = false
}

// ConfigureDRC applies a new DRC profile without touching the channel or
// sample-rate pipeline.
func (d *Decoder) ConfigureDRC(profile string) {
	d.Config.DRCProfile = profile
	d.DRCUpdates++
}

// Mute silences output, e.g. after sync is lost for good.
func (d *Decoder) Mute() {
	d.Muted = true
}
