// Package repair diagnoses DF/NDF timecode inconsistencies in CMX 3600 EDLs
// and repairs them by recomputing labels from absolute frame counts at the
// referenced media frame rate. Frame counts are the sole invariant, so the
// edit intent never changes; a fixed offset in seconds would be wrong because
// drop-frame drift is non-linear and grows per segment.
package repair

import (
	"fmt"
	"sort"

	"edlfix/internal/edl"
	"edlfix/internal/rate"
	"edlfix/internal/timecode"
)

// Severity classifies a finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Finding is one diagnostic entry, located precisely inside the source EDL.
type Finding struct {
	Line       int      `json:"line"`
	Column     int      `json:"column"`
	EventNum   string   `json:"eventNum,omitempty"`
	Reel       string   `json:"reel,omitempty"`
	Field      string   `json:"field,omitempty"`
	Severity   Severity `json:"severity"`
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Original   string   `json:"original,omitempty"`
	Repaired   string   `json:"repaired,omitempty"`
	Rate       string   `json:"rate,omitempty"`
	FrameCount int64    `json:"frameCount,omitempty"`
}

// SkipReason explains why an event could not be repaired and was kept verbatim.
type SkipReason struct {
	Line     int    `json:"line"`
	EventNum string `json:"eventNum,omitempty"`
	Reel     string `json:"reel,omitempty"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// Result holds the repaired document and everything the report should say.
type Result struct {
	RepairedText      string
	Findings          []Finding
	Skipped           []SkipReason
	FirstInconsistent *Finding
	ReelRates         map[string]rate.Rate
	RepairedEvents    int
	SkippedEvents     int
	TotalEvents       int
}

// Options supplies frame-rate knowledge for the referenced media.
type Options struct {
	// DefaultRate applies when a reel has no explicit ReelRates entry.
	DefaultRate *rate.Rate
	// ReelRates maps reel identifiers (as written in the EDL) to rates.
	ReelRates map[string]rate.Rate
}

type eventWork struct {
	event  *edl.Event
	rate   rate.Rate
	rateOK bool
	reason string
}

// Run performs diagnosis and repair and returns the corrected EDL text.
func Run(doc *edl.Document, opts Options) *Result {
	res := &Result{ReelRates: map[string]rate.Rate{}}
	res.TotalEvents = len(doc.Events)
	work := make([]eventWork, 0, len(doc.Events))
	for _, ev := range doc.Events {
		w := eventWork{event: ev}
		r, ok := resolveRate(ev, opts)
		w.rate, w.rateOK = r, ok
		if ok {
			res.ReelRates[ev.Reel] = r
		} else {
			w.reason = "no frame rate known for reel " + ev.Reel
		}
		work = append(work, w)
	}

	for _, w := range work {
		ev := w.event
		if !w.rateOK {
			res.Skipped = append(res.Skipped, SkipReason{
				Line: ev.LineIndex + 1, EventNum: ev.EventNum, Reel: ev.Reel,
				Code:    "RATE_UNKNOWN",
				Message: "无法确定引用媒体帧率：" + w.reason + "；事件原样保留",
			})
			res.SkippedEvents++
			continue
		}
		repairEvent(doc, res, ev, w.rate)
	}

	for i := range doc.Lines {
		if doc.Lines[i].IsEvent && doc.Lines[i].Event != nil && doc.Lines[i].Event.EventRepaired {
			res.RepairedEvents++
		}
	}
	res.RepairedText = render(doc)
	return res
}

func resolveRate(ev *edl.Event, opts Options) (rate.Rate, bool) {
	if r, ok := opts.ReelRates[ev.Reel]; ok {
		return r, true
	}
	if opts.DefaultRate != nil {
		return *opts.DefaultRate, true
	}
	return rate.Rate{}, false
}

func repairEvent(doc *edl.Document, res *Result, ev *edl.Event, r rate.Rate) {
	line := &doc.Lines[ev.LineIndex]
	canonicalMode := timecode.NDF
	if r.DropCapable {
		canonicalMode = timecode.DF
	}

	type planned struct {
		ref      edl.TokenRef
		rendered string
		frames   int64
		finding  *Finding
	}
	plans := make([]planned, 0, len(ev.Tokens))
	hardError := false

	for _, ref := range ev.Tokens {
		tc := ref.TC
		// 1. Label must be legal for the media rate at all.
		if !tc.IsValidLabel(r.NominalFPS) {
			res.Skipped = append(res.Skipped, SkipReason{
				Line: ev.LineIndex + 1, EventNum: ev.EventNum, Reel: ev.Reel,
				Code: "ILLEGAL_LABEL",
				Message: fmt.Sprintf("时间码 %s 对帧率 %s 非法（越界或落在被跳过的丢帧号上），无法确定其指向的帧；原样保留",
					tc.Raw, r.Input),
			})
			hardError = true
			break
		}
		// 2. DF label on a non-drop media rate is contradictory.
		if tc.Mode == timecode.DF && !r.DropCapable {
			res.Skipped = append(res.Skipped, SkipReason{
				Line: ev.LineIndex + 1, EventNum: ev.EventNum, Reel: ev.Reel,
				Code: "DF_ON_NONDROP_RATE",
				Message: fmt.Sprintf("媒体帧率 %s 不支持丢帧编号，却出现 DF 标签 %s；语义有歧义，原样保留",
					r.Input, tc.Raw),
			})
			hardError = true
			break
		}
		frames, err := timecode.ToFrames(tc, r.NominalFPS)
		if err != nil {
			res.Skipped = append(res.Skipped, SkipReason{
				Line: ev.LineIndex + 1, EventNum: ev.EventNum, Reel: ev.Reel,
				Code:    "FRAME_CONVERT_FAILED",
				Message: fmt.Sprintf("时间码 %s 无法转换为帧号：%v；原样保留", tc.Raw, err),
			})
			hardError = true
			break
		}
		target, err := timecode.FromFrames(frames, r.NominalFPS, canonicalMode)
		if err != nil {
			res.Skipped = append(res.Skipped, SkipReason{
				Line: ev.LineIndex + 1, EventNum: ev.EventNum, Reel: ev.Reel,
				Code:    "RELABEL_FAILED",
				Message: fmt.Sprintf("帧号 %d 无法按帧率 %s 重新标注：%v；原样保留", frames, r.Input, err),
			})
			hardError = true
			break
		}
		rendered := target.Render()
		p := planned{ref: ref, rendered: rendered, frames: frames}
		if rendered != tc.Raw {
			p.finding = &Finding{
				Line:       ev.LineIndex + 1,
				Column:     tc.ByteStart + 1,
				EventNum:   ev.EventNum,
				Reel:       ev.Reel,
				Field:      ref.Field,
				Severity:   SeverityError,
				Code:       inconsistencyCode(tc.Mode, canonicalMode),
				Message:    inconsistencyMessage(tc.Mode, canonicalMode, r),
				Original:   tc.Raw,
				Repaired:   rendered,
				Rate:       r.Input,
				FrameCount: frames,
			}
		} else if ev.FCM != edl.FCMUnspecified {
			declared := timecode.NDF
			if ev.FCM == edl.FCMDrop {
				declared = timecode.DF
			}
			if declared != canonicalMode {
				p.finding = &Finding{
					Line:     ev.LineIndex + 1,
					Column:   tc.ByteStart + 1,
					EventNum: ev.EventNum,
					Reel:     ev.Reel,
					Field:    ref.Field,
					Severity: SeverityWarning,
					Code:     "FCM_SECTION_CONFLICT",
					Message: fmt.Sprintf("FCM 段声明 %s，但媒体帧率 %s 的规范约定为 %s；标签本身仍可正确解析",
						declared, r.Input, canonicalMode),
					Original:   tc.Raw,
					Repaired:   rendered,
					Rate:       r.Input,
					FrameCount: frames,
				}
			}
		}
		plans = append(plans, p)
	}

	if hardError {
		res.SkippedEvents++
		return
	}

	changed := false
	for _, p := range plans {
		if p.finding != nil && p.finding.Severity == SeverityError {
			f := *p.finding
			res.Findings = append(res.Findings, f)
			if res.FirstInconsistent == nil {
				first := f
				res.FirstInconsistent = &first
			}
		} else if p.finding != nil {
			res.Findings = append(res.Findings, *p.finding)
		}
		if p.rendered != p.ref.TC.Raw {
			line.Text = edl.ReplaceToken(line.Text, p.ref, p.rendered)
			changed = true
		}
	}
	if changed {
		ev.MarkRepaired()
	}
}

func inconsistencyCode(got, want timecode.Mode) string {
	if got == timecode.NDF && want == timecode.DF {
		return "NDF_LABEL_ON_DROP_RATE"
	}
	if got == timecode.DF && want == timecode.NDF {
		return "DF_LABEL_ON_NONDROP_RATE"
	}
	return "MODE_MISMATCH"
}

func inconsistencyMessage(got, want timecode.Mode, r rate.Rate) string {
	return fmt.Sprintf("该标签按 %s 书写，但引用媒体帧率 %s 要求 %s 标注；已以帧号为唯一不变量重新计算，剪辑意图不变",
		got, r.Input, want)
}

func render(doc *edl.Document) string {
	out := make([]string, len(doc.Lines))
	for i, l := range doc.Lines {
		out[i] = l.Text
	}
	return joinLines(out)
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	joined := lines[0]
	for _, l := range lines[1:] {
		joined += "\n" + l
	}
	return joined
}

// ReelNames returns sorted reel identifiers, useful for stable reports.
func ReelRatesSorted(res *Result) []string {
	names := make([]string, 0, len(res.ReelRates))
	for k := range res.ReelRates {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
