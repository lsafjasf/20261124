package edl

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

func MarshalBatch(batch *BatchTimeline) ([]byte, error) {
	if batch == nil {
		return nil, fmt.Errorf("nil batch")
	}
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func WriteBatchJSON(w io.Writer, batch *BatchTimeline) error {
	data, err := MarshalBatch(batch)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// WriteTextReport writes a compact operator-friendly summary.
func WriteTextReport(w io.Writer, batch *BatchTimeline) error {
	if batch == nil {
		return fmt.Errorf("nil batch")
	}
	for _, file := range batch.Files {
		if _, err := fmt.Fprintf(w, "%s\n", file.Path); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			w,
			"  title=%q mode=%s rate=%s (%s fps) total=%df duration=%ss\n",
			file.Title, file.Mode, file.FrameRate, file.FPS, file.TotalFrames, file.TotalDuration.Seconds,
		); err != nil {
			return err
		}
		for _, ev := range file.Events {
			code := ev.Transition.Code
			if code == "" {
				code = "C"
			}
			flag := ""
			if !ev.Transition.Supported {
				flag = " [DROP]"
			}
			if _, err := fmt.Fprintf(
				w,
				"  %03d %-8s %-3s %-5s src %s-%s (%df) rec %s-%s (%df) timeline %d-%d offset=%+df%s\n",
				ev.Number, ev.Reel, ev.Track, code,
				ev.SourceIn.String(), ev.SourceOut.String(), ev.SourceDurationFrames,
				ev.RecordIn.String(), ev.RecordOut.String(), ev.RecordDurationFrames,
				ev.TimelineIn, ev.TimelineOut, ev.SourceRecordOffset, flag,
			); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(
			w,
			"  cuts=%v first=%d last=%d repeatedReels=%v validation=%s\n",
			file.CutFrames, file.FirstCut.Frame, file.LastCut.Frame,
			file.RepeatedReels, validationText(file.Validation),
		); err != nil {
			return err
		}
		for _, lost := range file.LostTransitions {
			if _, err := fmt.Fprintf(
				w,
				"  unsupported: event %03d %s %s %df -> hard cut (%s)\n",
				lost.Event, lost.Code, lost.Class, lost.DurationFrames, lost.Reason,
			); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(
		w,
		"\nBATCH files=%d total=%df duration=%ss unsupportedTransitions=%d validationIssues=%d\n",
		batch.Summary.FileCount,
		batch.Summary.TotalFrames,
		batch.Summary.TotalDuration.Seconds,
		batch.Summary.UnsupportedTransition,
		batch.Summary.ValidationIssues,
	); err != nil {
		return err
	}
	if len(batch.Summary.RepeatedReels) > 0 {
		_, err := fmt.Fprintf(w, "repeatedReels=%v\n", batch.Summary.RepeatedReels)
		return err
	}
	return nil
}

func validationText(v Validation) string {
	if !v.FirstCutAtZero || !v.LastCutAtTotal || !v.SpanMatchesCutPoints ||
		!v.SourceRecordDurations {
		return "FAIL"
	}
	return "OK"
}

// FrameMappings enumerates [from,to) for one track. This is intentionally a
// slice-producing API only; callers should keep ranges bounded on long EDLs.
func (ft *FileTimeline) FrameMappings(track string, from, to int64) ([]FrameMapping, error) {
	if from < 0 {
		from = 0
	}
	if to < 0 || to > ft.TotalFrames {
		to = ft.TotalFrames
	}
	if to < from {
		return nil, fmt.Errorf("invalid frame range %d..%d", from, to)
	}
	out := make([]FrameMapping, 0, to-from)
	for frame := from; frame < to; frame++ {
		mapping, err := ft.FrameAt(track, frame)
		if err != nil {
			return nil, err
		}
		out = append(out, mapping)
	}
	return out, nil
}

// TrackNames returns the track names in first-seen order.
func (ft *FileTimeline) TrackNames() []string {
	names := make([]string, 0, len(ft.Tracks))
	for _, track := range ft.Tracks {
		names = append(names, track.Name)
	}
	return names
}

func SortedRepeatedReels(reels []string) []string {
	out := append([]string(nil), reels...)
	sort.Strings(out)
	return out
}
