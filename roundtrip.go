package timeline

import (
	"encoding/json"
	"fmt"
	"io"
)

type roundTripDocument struct {
	Version  int              `json:"version"`
	Sequence Sequence         `json:"sequence"`
	Media    []Media          `json:"media"`
	Events   []roundTripEvent `json:"events"`
	Computed roundTripSummary `json:"computed"`
}

type roundTripEvent struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Media          string `json:"media,omitempty"`
	MediaInFrame   int64  `json:"mediaInFrame,omitempty"`
	MediaOutFrame  int64  `json:"mediaOutFrame,omitempty"`
	Speed          string `json:"speed,omitempty"`
	SequenceFrames int64  `json:"sequenceFrames,omitempty"`

	ComputedSequenceInSeconds  string `json:"computedSequenceInSeconds"`
	ComputedSequenceOutSeconds string `json:"computedSequenceOutSeconds"`
	ComputedSequenceInFrames   string `json:"computedSequenceInFrames"`
	ComputedSequenceOutFrames  string `json:"computedSequenceOutFrames"`
	ComputedMediaInSeconds     string `json:"computedMediaInSeconds,omitempty"`
	ComputedMediaOutSeconds    string `json:"computedMediaOutSeconds,omitempty"`
	ComputedSourceInSeconds    string `json:"computedSourceInSeconds,omitempty"`
	ComputedSourceOutSeconds   string `json:"computedSourceOutSeconds,omitempty"`
}

type roundTripSummary struct {
	DurationSeconds      string `json:"durationSeconds"`
	DurationFrames       string `json:"durationFrames"`
	DurationFrameRounded string `json:"durationFrameRounded"`
	EventCount           int    `json:"eventCount"`
}

func RoundTripBytes(t *Timeline) ([]byte, error) {
	doc := roundTripDocument{
		Version:  DocumentVersion,
		Sequence: Sequence{ID: t.SequenceID, FPS: ratString(t.SequenceFPS)},
		Computed: roundTripSummary{
			DurationSeconds:      decimalString(t.Duration, 9),
			DurationFrames:       ratString(mulRat(t.Duration, t.SequenceFPS)),
			DurationFrameRounded: roundRat(mulRat(t.Duration, t.SequenceFPS)).String(),
			EventCount:           len(t.Events),
		},
	}
	for id, src := range t.Media {
		doc.Media = append(doc.Media, Media{ID: id, FPS: ratString(src.FPS), Frames: src.Frames})
	}
	sortMedia(doc.Media)

	for _, ev := range t.Events {
		out := roundTripEvent{
			ID:                         ev.Definition.ID,
			Kind:                       ev.Kind,
			Media:                      ev.Definition.Media,
			MediaInFrame:               ev.Definition.MediaInFrame,
			MediaOutFrame:              ev.Definition.MediaOutFrame,
			Speed:                      ev.Definition.Speed,
			SequenceFrames:             ev.Definition.SequenceFrames,
			ComputedSequenceInSeconds:  decimalString(ev.SequenceIn, 9),
			ComputedSequenceOutSeconds: decimalString(ev.SequenceOut, 9),
			ComputedSequenceInFrames:   ratString(ev.SequenceInFrames),
			ComputedSequenceOutFrames:  ratString(ev.SequenceOutFrames),
		}
		if out.Speed == "" && ev.IsClip() {
			out.Speed = ratString(ev.Speed)
		}
		if ev.IsClip() {
			out.ComputedMediaInSeconds = decimalString(ev.MediaIn, 9)
			out.ComputedMediaOutSeconds = decimalString(ev.MediaOut, 9)
			out.ComputedSourceInSeconds = decimalString(ev.SourceIn, 9)
			out.ComputedSourceOutSeconds = decimalString(ev.SourceOut, 9)
		}
		doc.Events = append(doc.Events, out)
	}
	return json.MarshalIndent(doc, "", "  ")
}

func WriteRoundTrip(w io.Writer, t *Timeline) error {
	data, err := RoundTripBytes(t)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func VerifyRoundTrip(original, imported *Timeline) error {
	if original.SequenceFPS.Cmp(imported.SequenceFPS) != 0 {
		return fmt.Errorf("sequence fps changed: %s -> %s", ratString(original.SequenceFPS), ratString(imported.SequenceFPS))
	}
	if original.Duration.Cmp(imported.Duration) != 0 {
		return fmt.Errorf("duration changed: %s -> %s", ratString(original.Duration), ratString(imported.Duration))
	}
	if len(original.Events) != len(imported.Events) {
		return fmt.Errorf("event count changed: %d -> %d", len(original.Events), len(imported.Events))
	}
	for i := range original.Events {
		a, b := original.Events[i], imported.Events[i]
		if a.Definition.ID != b.Definition.ID || a.Kind != b.Kind {
			return fmt.Errorf("event %d identity changed", i)
		}
		if a.Definition.Media != b.Definition.Media ||
			a.Definition.MediaInFrame != b.Definition.MediaInFrame ||
			a.Definition.MediaOutFrame != b.Definition.MediaOutFrame ||
			a.Definition.SequenceFrames != b.Definition.SequenceFrames {
			return fmt.Errorf("event %q definition changed", a.Definition.ID)
		}
		if a.SequenceIn.Cmp(b.SequenceIn) != 0 || a.SequenceOut.Cmp(b.SequenceOut) != 0 {
			return fmt.Errorf("event %q sequence time changed", a.Definition.ID)
		}
		if a.IsClip() &&
			(a.MediaIn.Cmp(b.MediaIn) != 0 ||
				a.MediaOut.Cmp(b.MediaOut) != 0 ||
				a.SourceIn.Cmp(b.SourceIn) != 0 ||
				a.SourceOut.Cmp(b.SourceOut) != 0) {
			return fmt.Errorf("event %q media/source time changed", a.Definition.ID)
		}
	}
	return nil
}

func sortMedia(media []Media) {
	for i := 1; i < len(media); i++ {
		for j := i; j > 0 && media[j].ID < media[j-1].ID; j-- {
			media[j], media[j-1] = media[j-1], media[j]
		}
	}
}
