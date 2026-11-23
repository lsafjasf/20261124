package timeline

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strconv"
)

type CutError struct {
	CutID                  string
	EventID                string
	Boundary               string
	Kind                   string
	MediaID                string
	MediaFPS               string
	MediaTimeSeconds       string
	SourceTimeSeconds      string
	SequenceTimeSeconds    string
	SequenceTimeFrames     string
	QuantizedSequenceFrame string
	FrameError             string
	SecondError            string
	RoundTripErrorSeconds  string
	LegacySequenceFrame    string
	LegacyErrorFrames      string
	LegacyErrorSeconds     string
}

type Summary struct {
	SequenceFPS              string   `json:"sequenceFps"`
	MediaRates               []string `json:"mediaRates"`
	EventCount               int      `json:"eventCount"`
	DurationSeconds          string   `json:"durationSeconds"`
	DurationFrames           string   `json:"durationFrames"`
	DurationFrameRounded     string   `json:"durationFrameRounded"`
	MaxCutErrorFrames        string   `json:"maxCutErrorFrames"`
	MaxCutErrorSeconds       string   `json:"maxCutErrorSeconds"`
	MaxRoundTripErrorSeconds string   `json:"maxRoundTripErrorSeconds"`
	RoundTripExact           bool     `json:"roundTripExact"`
	LegacyDurationFrames     string   `json:"legacyDurationFrames"`
	LegacyEndErrorFrames     string   `json:"legacyEndErrorFrames"`
	LegacyEndErrorSeconds    string   `json:"legacyEndErrorSeconds"`
}

func BuildCutErrors(original, roundTrip *Timeline) ([]CutError, error) {
	if len(original.Events) != len(roundTrip.Events) {
		return nil, fmt.Errorf("round-trip event count changed: %d -> %d", len(original.Events), len(roundTrip.Events))
	}
	legacy := legacyBoundaries(original)
	rows := make([]CutError, 0, len(original.Events)*2)
	cutNo := 0
	for i, ev := range original.Events {
		for _, out := range []bool{false, true} {
			cutNo++
			boundary := "in"
			rtEvent := roundTrip.Events[i]
			exactSec := ev.SequenceIn
			exactFrames := ev.SequenceInFrames
			roundFrame := ev.SequenceInRound
			rtSec := rtEvent.SequenceIn
			legacyFrame := legacy[i].start
			if out {
				boundary = "out"
				exactSec = ev.SequenceOut
				exactFrames = ev.SequenceOutFrames
				roundFrame = ev.SequenceOutRound
				rtSec = rtEvent.SequenceOut
				legacyFrame = legacy[i].end
			}
			frameErr := subRat(new(big.Rat).SetInt(roundFrame), exactFrames)
			secondErr := divRat(frameErr, original.SequenceFPS)
			rtErr := subRat(rtSec, exactSec)
			legacyErrFrames := subRat(intRat(legacyFrame), exactFrames)
			legacyErrSeconds := divRat(legacyErrFrames, original.SequenceFPS)

			row := CutError{
				CutID:                  fmt.Sprintf("cut-%02d", cutNo),
				EventID:                ev.Definition.ID,
				Boundary:               boundary,
				Kind:                   ev.Kind,
				SequenceTimeSeconds:    decimalString(exactSec, 9),
				SequenceTimeFrames:     decimalString(exactFrames, 9),
				QuantizedSequenceFrame: roundFrame.String(),
				FrameError:             decimalString(frameErr, 9),
				SecondError:            decimalString(secondErr, 12),
				RoundTripErrorSeconds:  decimalString(rtErr, 12),
				LegacySequenceFrame:    strconv.FormatInt(legacyFrame, 10),
				LegacyErrorFrames:      decimalString(legacyErrFrames, 9),
				LegacyErrorSeconds:     decimalString(legacyErrSeconds, 12),
			}
			if ev.IsClip() {
				row.MediaID = ev.Source.ID
				row.MediaFPS = ratString(ev.Source.FPS)
				row.MediaTimeSeconds = decimalString(ev.MediaIn, 9)
				row.SourceTimeSeconds = decimalString(ev.SourceIn, 9)
				if out {
					row.MediaTimeSeconds = decimalString(ev.MediaOut, 9)
					row.SourceTimeSeconds = decimalString(ev.SourceOut, 9)
				}
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func WriteCutErrorsCSV(w io.Writer, rows []CutError) error {
	cw := csv.NewWriter(w)
	header := []string{
		"cut_id", "event_id", "boundary", "kind", "media_id", "media_fps",
		"media_time_seconds", "source_time_seconds", "sequence_time_seconds",
		"sequence_time_frames", "quantized_sequence_frame", "frame_error",
		"second_error", "roundtrip_error_seconds", "legacy_sequence_frame",
		"legacy_error_frames", "legacy_error_seconds",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		record := []string{
			r.CutID, r.EventID, r.Boundary, r.Kind, r.MediaID, r.MediaFPS,
			r.MediaTimeSeconds, r.SourceTimeSeconds, r.SequenceTimeSeconds,
			r.SequenceTimeFrames, r.QuantizedSequenceFrame, r.FrameError,
			r.SecondError, r.RoundTripErrorSeconds, r.LegacySequenceFrame,
			r.LegacyErrorFrames, r.LegacyErrorSeconds,
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func Summarize(t *Timeline, rows []CutError) Summary {
	legacy := legacyBoundaries(t)
	legacyEnd := int64(0)
	if len(legacy) > 0 {
		legacyEnd = legacy[len(legacy)-1].end
	}
	exactEndFrames := mulRat(t.Duration, t.SequenceFPS)
	legacyEndErr := subRat(intRat(legacyEnd), exactEndFrames)
	maxFixedFrames := new(big.Rat)
	maxRoundTripSeconds := new(big.Rat)
	for _, row := range rows {
		fixed, _ := new(big.Rat).SetString(row.FrameError)
		if absRat(fixed).Cmp(maxFixedFrames) > 0 {
			maxFixedFrames = absRat(fixed)
		}
		rt, _ := new(big.Rat).SetString(row.RoundTripErrorSeconds)
		if absRat(rt).Cmp(maxRoundTripSeconds) > 0 {
			maxRoundTripSeconds = absRat(rt)
		}
	}

	mediaRates := make([]string, 0, len(t.Media))
	for _, src := range t.Media {
		mediaRates = append(mediaRates, src.ID+"="+ratString(src.FPS))
	}
	sortStrings(mediaRates)

	return Summary{
		SequenceFPS:              ratString(t.SequenceFPS),
		MediaRates:               mediaRates,
		EventCount:               len(t.Events),
		DurationSeconds:          decimalString(t.Duration, 9),
		DurationFrames:           decimalString(exactEndFrames, 9),
		DurationFrameRounded:     roundRat(exactEndFrames).String(),
		MaxCutErrorFrames:        decimalString(maxFixedFrames, 9),
		MaxCutErrorSeconds:       decimalString(divRat(maxFixedFrames, t.SequenceFPS), 12),
		MaxRoundTripErrorSeconds: decimalString(maxRoundTripSeconds, 12),
		RoundTripExact:           maxRoundTripSeconds.Sign() == 0,
		LegacyDurationFrames:     strconv.FormatInt(legacyEnd, 10),
		LegacyEndErrorFrames:     decimalString(legacyEndErr, 9),
		LegacyEndErrorSeconds:    decimalString(divRat(legacyEndErr, t.SequenceFPS), 12),
	}
}

func WriteSummaryJSON(w io.Writer, s Summary) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

type legacyInterval struct {
	start int64
	end   int64
}

// legacyBoundaries reproduces the two importer bugs under test: gaps have no
// media range and therefore become zero length, and every segment is rounded
// before being added to the running cursor. The latter makes one bad boundary
// contaminate every later cut.
func legacyBoundaries(t *Timeline) []legacyInterval {
	out := make([]legacyInterval, len(t.Events))
	cursor := int64(0)
	for i, ev := range t.Events {
		out[i].start = cursor
		if ev.IsClip() {
			durationFrames := roundRat(mulRat(ev.SequenceDuration, t.SequenceFPS))
			cursor += durationFrames.Int64()
		}
		out[i].end = cursor
	}
	return out
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
