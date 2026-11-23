package edl

import (
	"fmt"
	"io"
	"strings"
)

// ExportEDL writes a cut-only CMX 3600 EDL. Non-cut transitions are replaced
// by C and documented with a comment. Every source and record boundary is kept
// byte-for-byte at the frame level.
func ExportEDL(w io.Writer, ft *FileTimeline) error {
	if ft == nil {
		return fmt.Errorf("nil timeline")
	}
	title := strings.TrimSpace(ft.Title)
	if title == "" {
		title = "EDL TIMELINE"
	}
	if _, err := fmt.Fprintf(w, "TITLE: %s\n", title); err != nil {
		return err
	}
	if ft.Mode == DF {
		_, err := fmt.Fprintln(w, "FCM: DROP FRAME")
		if err != nil {
			return err
		}
	} else {
		_, err := fmt.Fprintln(w, "FCM: NON-DROP FRAME")
		if err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	currentMode := ft.Mode
	for _, ev := range ft.Events {
		eventMode := eventTimecodeMode(ev)
		if eventMode == NDF && currentMode == DF {
			// CMX has a file-level FCM header. A scoped FCM comment keeps
			// colon-separated NDF labels unambiguous in mixed-mode files.
			if _, err := fmt.Fprintln(w, "* FCM: NON-DROP FRAME"); err != nil {
				return err
			}
			currentMode = NDF
		}
		code := "C"
		if ev.Transition.Supported && ev.Transition.Code != "" {
			code = ev.Transition.Code
		}
		if _, err := fmt.Fprintf(
			w,
			"%03d  %-8s %-5s %-8s %s %s %s %s\n",
			ev.Number,
			ev.Reel,
			ev.Track,
			code,
			ev.SourceIn.String(),
			ev.SourceOut.String(),
			ev.RecordIn.String(),
			ev.RecordOut.String(),
		); err != nil {
			return err
		}
		if ev.ClipName != "" {
			if _, err := fmt.Fprintf(w, "* FROM CLIP NAME: %s\n", ev.ClipName); err != nil {
				return err
			}
		}
		if !ev.Transition.Supported {
			if _, err := fmt.Fprintf(
				w,
				"* DROPPED TRANSITION: %s %s duration=%d frames; exported as hard cut\n",
				ev.Transition.Code,
				ev.Transition.Class,
				ev.Transition.DurationFrames,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func eventTimecodeMode(ev Event) TimecodeMode {
	for _, tc := range []Timecode{ev.SourceIn, ev.SourceOut, ev.RecordIn, ev.RecordOut} {
		if tc.Mode == NDF {
			return NDF
		}
	}
	return DF
}

// RoundtripResult documents a cut-only EDL export followed by a fresh parse.
type RoundtripResult struct {
	OK                     bool     `json:"ok"`
	OriginalTotalFrames    int64    `json:"originalTotalFrames"`
	RoundtripTotalFrames   int64    `json:"roundtripTotalFrames"`
	OriginalFirstCutFrame  int64    `json:"originalFirstCutFrame"`
	RoundtripFirstCutFrame int64    `json:"roundtripFirstCutFrame"`
	OriginalLastCutFrame   int64    `json:"originalLastCutFrame"`
	RoundtripLastCutFrame  int64    `json:"roundtripLastCutFrame"`
	OriginalEventCount     int      `json:"originalEventCount"`
	RoundtripEventCount    int      `json:"roundtripEventCount"`
	LostTransitionCount    int      `json:"lostTransitionCount"`
	Mismatches             []string `json:"mismatches,omitempty"`
}

// CompareCutRoundtrip checks that the lossy transition export did not move a
// record boundary or a source frame.
func CompareCutRoundtrip(original, roundtrip *FileTimeline) RoundtripResult {
	result := RoundtripResult{OK: true}
	if original == nil || roundtrip == nil {
		result.OK = false
		result.Mismatches = append(result.Mismatches, "nil timeline")
		return result
	}
	result.OriginalTotalFrames = original.TotalFrames
	result.RoundtripTotalFrames = roundtrip.TotalFrames
	result.OriginalFirstCutFrame = original.FirstCut.Frame
	result.RoundtripFirstCutFrame = roundtrip.FirstCut.Frame
	result.OriginalLastCutFrame = original.LastCut.Frame
	result.RoundtripLastCutFrame = roundtrip.LastCut.Frame
	result.OriginalEventCount = len(original.Events)
	result.RoundtripEventCount = len(roundtrip.Events)
	result.LostTransitionCount = len(original.LostTransitions)

	check := func(ok bool, format string, args ...any) {
		if !ok {
			result.OK = false
			result.Mismatches = append(result.Mismatches, fmt.Sprintf(format, args...))
		}
	}
	check(original.TotalFrames == roundtrip.TotalFrames,
		"total frames: got %d, want %d", roundtrip.TotalFrames, original.TotalFrames)
	check(original.FirstCut.Frame == roundtrip.FirstCut.Frame,
		"first cut: got %d, want %d", roundtrip.FirstCut.Frame, original.FirstCut.Frame)
	check(original.LastCut.Frame == roundtrip.LastCut.Frame,
		"last cut: got %d, want %d", roundtrip.LastCut.Frame, original.LastCut.Frame)
	check(len(original.Events) == len(roundtrip.Events),
		"event count: got %d, want %d", len(roundtrip.Events), len(original.Events))
	if len(original.Events) != len(roundtrip.Events) {
		return result
	}
	for i := range original.Events {
		a, b := original.Events[i], roundtrip.Events[i]
		prefix := fmt.Sprintf("event %d", a.Number)
		check(a.Reel == b.Reel, "%s reel: got %q, want %q", prefix, b.Reel, a.Reel)
		check(a.Track == b.Track, "%s track: got %q, want %q", prefix, b.Track, a.Track)
		check(a.SourceInAbs == b.SourceInAbs, "%s source in: got %d, want %d", prefix, b.SourceInAbs, a.SourceInAbs)
		check(a.SourceOutAbs == b.SourceOutAbs, "%s source out: got %d, want %d", prefix, b.SourceOutAbs, a.SourceOutAbs)
		check(a.RecordInAbs == b.RecordInAbs, "%s record in: got %d, want %d", prefix, b.RecordInAbs, a.RecordInAbs)
		check(a.RecordOutAbs == b.RecordOutAbs, "%s record out: got %d, want %d", prefix, b.RecordOutAbs, a.RecordOutAbs)
	}
	return result
}
