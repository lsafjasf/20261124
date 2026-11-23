package main

// Params carries the per-frame processing parameters that are also written
// into the dump, so the postmortem can correlate behavior with parameters.
type Params struct {
	Frame            int
	Shot             string
	ExposureGain     float64 // auto exposure, linear domain multiplier
	HighlightProtect bool    // auto-enabled on bright shots
}

// StageDump records the full pixel buffer after each pipeline stage.
type StageDump struct {
	Params Params
	Stages map[string]*Frame // "scene","exposure","tonemap","look","encode"
	Order  []string
}

// Pipeline runs the HDR->SDR chain. If Buggy is true, the look module
// re-applies the scene-referred tone curve when HighlightProtect is set
// (the defect under investigation). If false, the look module never tone
// maps: highlight rolloff lives only in the tonemap stage.
type Pipeline struct {
	Buggy bool
}

// AutoExposure picks the gain that maps the shot's key-light peak
// (KeyLight * WhitePoint) onto WhitePoint, i.e. gain = 1/KeyLight.
func AutoExposure(shot Shot) float64 { return 1.0 / shot.KeyLight }

// AutoParams mirrors the production heuristic: bright shots get highlight
// protection switched on.
func AutoParams(frameIdx int, shot Shot) Params {
	return Params{
		Frame:            frameIdx,
		Shot:             shot.Name,
		ExposureGain:     AutoExposure(shot),
		HighlightProtect: shot.KeyLight > 1.0,
	}
}

func (p *Pipeline) Run(src *Frame, pr Params) *StageDump {
	d := &StageDump{Params: pr, Stages: map[string]*Frame{}}
	record := func(name string, f *Frame) {
		d.Stages[name] = f
		d.Order = append(d.Order, name)
	}
	record("scene", src.Clone())
	record("exposure", exposureStage(src, pr.ExposureGain))
	record("tonemap", tonemapStage(d.Stages["exposure"]))
	record("look", p.lookStage(d.Stages["tonemap"], pr))
	record("encode", encodeStage(d.Stages["look"]))
	return d
}

func exposureStage(in *Frame, gain float64) *Frame {
	out := in.Clone()
	for i := range out.Pix {
		out.Pix[i] *= gain
	}
	return out
}

func tonemapStage(in *Frame) *Frame {
	out := in.Clone()
	for i := range out.Pix {
		out.Pix[i] = ToneMap(out.Pix[i])
	}
	return out
}

// lookStage is the "creative look / output" module. The defect: with
// HighlightProtect on, it applies the scene-referred tone curve a second
// time on already display-referred data.
func (p *Pipeline) lookStage(in *Frame, pr Params) *Frame {
	out := in.Clone()
	if pr.HighlightProtect && p.Buggy {
		for i := range out.Pix {
			out.Pix[i] = ToneMap(out.Pix[i])
		}
	}
	return out
}

func encodeStage(in *Frame) *Frame {
	out := in.Clone()
	for i := range out.Pix {
		out.Pix[i] = SRGBEncode(out.Pix[i])
	}
	return out
}

// ReferenceSDR is the known-good single-tone-map result: exposure, one
// tone map, encode. It is the golden baseline for the fix comparison.
func ReferenceSDR(src *Frame, pr Params) *Frame {
	return encodeStage(tonemapStage(exposureStage(src, pr.ExposureGain)))
}

// Shots is the test timeline: a normal shot, a bright shot (frames 2-4),
// then normal again.
func Shots() []Shot {
	return []Shot{
		{Name: "normal-a", KeyLight: 1.0}, // frame 0
		{Name: "normal-a", KeyLight: 1.0}, // frame 1
		{Name: "bright", KeyLight: 4.0},   // frame 2
		{Name: "bright", KeyLight: 4.0},   // frame 3
		{Name: "bright", KeyLight: 4.0},   // frame 4
		{Name: "normal-b", KeyLight: 1.0}, // frame 5
	}
}

// RunTimeline executes the pipeline over all shots and returns per-frame
// dumps plus the golden single-map reference outputs.
func (p *Pipeline) RunTimeline() (dumps []*StageDump, golden []*Frame) {
	for i, shot := range Shots() {
		src := MakeScene(shot)
		pr := AutoParams(i, shot)
		dumps = append(dumps, p.Run(src, pr))
		golden = append(golden, ReferenceSDR(src, pr))
	}
	return dumps, golden
}
