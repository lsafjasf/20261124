// Package audit reproduces and diagnoses subtitle timestamp drift.
//
// The reference path keeps the editorial frame instant exact and quantizes once
// at the SRT millisecond boundary. The legacy path reinterprets frames at a
// nominal integer rate, passes them through a 90 kHz timebase, floors to
// milliseconds, round-trips through ASS centiseconds, and floors again.
package audit

import (
	"fmt"
	"math/big"
	"sort"

	"subdrift/subtitle"
	"subdrift/timebase"
)

var (
	msRate = timebase.Rate{Num: 1000, Den: 1}
	csRate = timebase.Rate{Num: 100, Den: 1}
)

// Mode describes one conversion path. InputRate is deliberately separate from
// the project's VideoRate: the legacy bug is exactly reinterpreting a frame
// count at NominalRate.
type Mode struct {
	Name        string
	InputRate   timebase.Rate
	PTSRounding timebase.Rounding
	MSRounding  timebase.Rounding
	UseCS       bool
	CSRounding  timebase.Rounding
}

// ReferenceMode is the fixed path: exact 24000/1001 frames, nearest 90 kHz tick,
// nearest millisecond, and no lossy intermediate representation.
func ReferenceMode(p *subtitle.Project) Mode {
	return Mode{
		Name:        "fixed",
		InputRate:   p.VideoRate,
		PTSRounding: timebase.Nearest,
		MSRounding:  timebase.Nearest,
	}
}

// LegacyMode is the observed download path.
func LegacyMode(p *subtitle.Project) Mode {
	return Mode{
		Name:        "legacy",
		InputRate:   p.NominalRate,
		PTSRounding: timebase.Floor,
		MSRounding:  timebase.Floor,
		UseCS:       p.UseCentiseconds(),
		CSRounding:  timebase.Floor,
	}
}

// Stage is one leg of the legacy pipeline. Expected is the count a correct
// pipeline would have at this point; Exact is the unrounded result of feeding
// this stage's (possibly already wrong) input into the target rate.
type Stage struct {
	Name              string
	From              timebase.Quantity
	To                timebase.Rate
	Rounding          timebase.Rounding
	Output            timebase.Quantity
	Exact             *big.Rat
	Expected          *big.Rat
	CumulativeErrorMS *big.Rat
}

// RoundingDelta is the local quantization error in target counts.
func (s Stage) RoundingDelta() *big.Rat {
	out := new(big.Rat).SetInt64(s.Output.Count)
	return new(big.Rat).Sub(out, s.Exact)
}

// Boundary is the evaluation of one cue start or end frame.
type Boundary struct {
	CueID         string
	Endpoint      string
	Frame         int64
	GroundSeconds *big.Rat
	GroundMS      *big.Rat
	ReferenceMS   int64
	OutputMS      int64
	ErrorMS       int64
	ErrorSeconds  *big.Rat
	ErrorFrames   *big.Rat
	Stages        []Stage
	FirstStage    int
}

// CurvePoint is one sample of the legacy error curve.
type CurvePoint struct {
	Frame       int64
	GroundMS    *big.Rat
	ReferenceMS int64
	OutputMS    int64
	ErrorMS     int64
}

// VariantResult isolates one layer of the pipeline at the final boundary.
type VariantResult struct {
	Name       string `json:"name"`
	OutputMS   int64  `json:"output_ms"`
	ErrorMS    int64  `json:"error_ms"`
	ErrorFirst bool   `json:"error_first"`
}

// Report is the complete audit result for a project.
type Report struct {
	Project        *subtitle.Project
	Mode           Mode
	Boundaries     []Boundary
	Curve          []CurvePoint
	Variants       []VariantResult
	FirstError     *Boundary
	MaxAbsErrorMS  int64
	MaxAbsErrorIdx int
}

// Evaluate runs one frame through mode and compares it with the exact reference.
func Evaluate(frame int64, p *subtitle.Project, mode Mode) Boundary {
	ground := new(big.Rat).Quo(new(big.Rat).SetInt64(frame), p.VideoRate.Rat())
	groundMS := new(big.Rat).Mul(ground, big.NewRat(1000, 1))
	refMS, _ := timebase.Quantize(groundMS, timebase.Nearest)

	q := timebase.Quantity{Count: frame, Rate: mode.InputRate}
	stages := make([]Stage, 0, 4)
	firstStage := -1
	add := func(name string, to timebase.Rate, rounding timebase.Rounding) {
		step, err := timebase.ConvertStep(name, q, to, rounding)
		if err != nil {
			panic(err)
		}
		cum := new(big.Rat).Sub(
			new(big.Rat).Mul(step.Output.Seconds(), big.NewRat(1000, 1)),
			groundMS,
		)
		if firstStage < 0 && cum.Sign() != 0 {
			firstStage = len(stages)
		}
		stages = append(stages, Stage{
			Name:              name,
			From:              q,
			To:                to,
			Rounding:          rounding,
			Output:            step.Output,
			Exact:             step.Exact,
			Expected:          new(big.Rat).Mul(ground, to.Rat()),
			CumulativeErrorMS: cum,
		})
		q = step.Output
	}

	add("frame->pts", p.PTSTimebase, mode.PTSRounding)
	add("pts->ms", msRate, mode.MSRounding)
	if mode.UseCS {
		add("ms->ass-cs", csRate, mode.CSRounding)
		add("ass-cs->ms", msRate, timebase.Nearest)
	}

	outputMS := q.Count
	errMS := outputMS - refMS
	return Boundary{
		Frame:         frame,
		GroundSeconds: ground,
		GroundMS:      groundMS,
		ReferenceMS:   refMS,
		OutputMS:      outputMS,
		ErrorMS:       errMS,
		ErrorSeconds:  new(big.Rat).SetFrac64(errMS, 1000),
		ErrorFrames:   msToFrames(errMS, p.VideoRate),
		Stages:        stages,
		FirstStage:    firstStage,
	}
}

func msToFrames(ms int64, rate timebase.Rate) *big.Rat {
	v := new(big.Rat).SetFrac64(ms, 1000)
	return v.Mul(v, rate.Rat())
}

// Run audits every cue boundary and samples the curve across the whole reel.
func Run(p *subtitle.Project) (*Report, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	mode := LegacyMode(p)
	r := &Report{Project: p, Mode: mode}

	events := boundaryEvents(p)
	for _, ev := range events {
		b := Evaluate(ev.frame, p, mode)
		b.CueID, b.Endpoint = ev.cueID, ev.endpoint
		r.Boundaries = append(r.Boundaries, b)
		if r.FirstError == nil && b.ErrorMS != 0 {
			copy := b
			r.FirstError = &copy
		}
	}

	r.Curve = buildCurve(p, mode, events)
	r.MaxAbsErrorMS = 0
	r.MaxAbsErrorIdx = 0
	for i, pt := range r.Curve {
		if abs(pt.ErrorMS) > r.MaxAbsErrorMS {
			r.MaxAbsErrorMS = abs(pt.ErrorMS)
			r.MaxAbsErrorIdx = i
		}
	}
	lastFrame := p.DurationFrames()
	for _, v := range variantModes(p) {
		b := Evaluate(lastFrame, p, v.mode)
		r.Variants = append(r.Variants, VariantResult{
			Name:       v.name,
			OutputMS:   b.OutputMS,
			ErrorMS:    b.ErrorMS,
			ErrorFirst: b.ErrorMS != 0,
		})
	}
	return r, nil
}

type event struct {
	frame    int64
	cueID    string
	endpoint string
	order    int
}

func boundaryEvents(p *subtitle.Project) []event {
	events := make([]event, 0, len(p.Cues)*2)
	for i, cue := range p.Cues {
		events = append(events,
			event{frame: cue.StartFrame, cueID: cue.ID, endpoint: "start", order: i * 2},
			event{frame: cue.EndFrame, cueID: cue.ID, endpoint: "end", order: i*2 + 1},
		)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].frame != events[j].frame {
			return events[i].frame < events[j].frame
		}
		return events[i].order < events[j].order
	})
	return events
}

func buildCurve(p *subtitle.Project, mode Mode, events []event) []CurvePoint {
	step := int64(24)
	if p.NominalRate.Den == 1 && p.NominalRate.Num >= 1 && p.NominalRate.Num <= 120 {
		step = p.NominalRate.Num
	}
	last := p.DurationFrames()
	frames := make(map[int64]bool)
	for f := int64(0); f <= last; f += step {
		frames[f] = true
	}
	frames[last] = true
	for _, ev := range events {
		frames[ev.frame] = true
	}
	sorted := make([]int64, 0, len(frames))
	for f := range frames {
		sorted = append(sorted, f)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	points := make([]CurvePoint, 0, len(sorted))
	for _, f := range sorted {
		b := Evaluate(f, p, mode)
		points = append(points, CurvePoint{
			Frame:       f,
			GroundMS:    b.GroundMS,
			ReferenceMS: b.ReferenceMS,
			OutputMS:    b.OutputMS,
			ErrorMS:     b.ErrorMS,
		})
	}
	return points
}

func variantModes(p *subtitle.Project) []struct {
	name string
	mode Mode
} {
	return []struct {
		name string
		mode Mode
	}{
		{
			name: "nominal-rate-only",
			mode: Mode{InputRate: p.NominalRate, PTSRounding: timebase.Nearest, MSRounding: timebase.Nearest},
		},
		{
			name: "timebase-floor-only",
			mode: Mode{InputRate: p.VideoRate, PTSRounding: timebase.Floor, MSRounding: timebase.Floor},
		},
		{
			name: "rounding-cs-only",
			mode: Mode{InputRate: p.VideoRate, PTSRounding: timebase.Floor, MSRounding: timebase.Floor, UseCS: true, CSRounding: timebase.Floor},
		},
		{
			name: "full-legacy",
			mode: LegacyMode(p),
		},
	}
}

// RenderSRT converts all cues through mode and returns SRT cues.
func RenderSRT(p *subtitle.Project, mode Mode) ([]subtitle.SRTCue, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	out := make([]subtitle.SRTCue, 0, len(p.Cues))
	for i, cue := range p.Cues {
		start := Evaluate(cue.StartFrame, p, mode)
		end := Evaluate(cue.EndFrame, p, mode)
		out = append(out, subtitle.SRTCue{
			Index:   i + 1,
			StartMS: start.OutputMS,
			EndMS:   end.OutputMS,
			Text:    cue.Text,
		})
	}
	return out, nil
}

// RenderSRTString is the fixed-by-default SRT rendering.
func RenderSRTString(p *subtitle.Project, mode Mode) (string, error) {
	cues, err := RenderSRT(p, mode)
	if err != nil {
		return "", err
	}
	return subtitle.FormatSRT(cues), nil
}

// FirstErrorStage returns the first boundary with a non-zero internal error,
// even when final millisecond formatting happened to hide it.
func (r *Report) FirstErrorStage() *Boundary {
	for i := range r.Boundaries {
		if r.Boundaries[i].FirstStage >= 0 {
			return &r.Boundaries[i]
		}
	}
	return nil
}

// String renders a compact terminal audit.
func (r *Report) String() string {
	return fmt.Sprintf("%s: %d cues, first error %s, max |error| %d ms",
		r.Project.Name, len(r.Project.Cues), r.FirstErrorText(), r.MaxAbsErrorMS)
}

func (r *Report) FirstErrorText() string {
	if r.FirstError == nil {
		return "none"
	}
	return fmt.Sprintf("%s %s @ frame %d (%s)",
		r.FirstError.CueID, r.FirstError.Endpoint, r.FirstError.Frame,
		subtitle.FormatTimestamp(r.FirstError.ReferenceMS))
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
