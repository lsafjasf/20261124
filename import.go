package timeline

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
)

func ImportFile(path string) (*Timeline, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Import(f)
}

func Import(r io.Reader) (*Timeline, error) {
	var doc Document
	dec := json.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode timeline: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode timeline: multiple JSON values")
		}
		return nil, fmt.Errorf("decode timeline: %w", err)
	}
	return BuildTimeline(doc)
}

func BuildTimeline(doc Document) (*Timeline, error) {
	if doc.Version != DocumentVersion {
		return nil, fmt.Errorf("unsupported document version %d", doc.Version)
	}
	seqFPS, err := ParseRate(doc.Sequence.FPS)
	if err != nil {
		return nil, fmt.Errorf("sequence fps: %w", err)
	}

	media := make(map[string]Source, len(doc.Media))
	for i, m := range doc.Media {
		if strings.TrimSpace(m.ID) == "" {
			return nil, fmt.Errorf("media[%d]: empty id", i)
		}
		if _, exists := media[m.ID]; exists {
			return nil, fmt.Errorf("media[%d]: duplicate id %q", i, m.ID)
		}
		fps, err := ParseRate(m.FPS)
		if err != nil {
			return nil, fmt.Errorf("media %q fps: %w", m.ID, err)
		}
		if m.Frames <= 0 {
			return nil, fmt.Errorf("media %q: frames must be positive", m.ID)
		}
		media[m.ID] = Source{ID: m.ID, FPS: fps, Frames: m.Frames}
	}

	tl := &Timeline{
		SequenceID:  doc.Sequence.ID,
		SequenceFPS: seqFPS,
		Media:       media,
		Duration:    new(big.Rat),
	}
	eventIDs := make(map[string]bool, len(doc.Events))
	cursor := new(big.Rat)

	for i, def := range doc.Events {
		if strings.TrimSpace(def.ID) == "" {
			return nil, fmt.Errorf("events[%d]: empty id", i)
		}
		if eventIDs[def.ID] {
			return nil, fmt.Errorf("events[%d]: duplicate id %q", i, def.ID)
		}
		eventIDs[def.ID] = true

		ev := TimedEvent{
			Definition: def,
			Kind:       def.Kind,
			SequenceIn: new(big.Rat).Set(cursor),
			SourceIn:   new(big.Rat),
		}
		switch def.Kind {
		case "clip":
			if err := fillClip(&ev, media); err != nil {
				return nil, fmt.Errorf("event %q: %w", def.ID, err)
			}
		case "gap":
			if def.SequenceFrames <= 0 {
				return nil, fmt.Errorf("event %q: gap sequenceFrames must be positive; omit the gap for zero length", def.ID)
			}
			ev.SequenceDuration = divRat(intRat(def.SequenceFrames), seqFPS)
		default:
			return nil, fmt.Errorf("event %q: unknown kind %q", def.ID, def.Kind)
		}

		ev.SequenceOut = addRat(ev.SequenceIn, ev.SequenceDuration)
		ev.SequenceInFrames = mulRat(ev.SequenceIn, seqFPS)
		ev.SequenceOutFrames = mulRat(ev.SequenceOut, seqFPS)
		ev.SequenceInRound = roundRat(ev.SequenceInFrames)
		ev.SequenceOutRound = roundRat(ev.SequenceOutFrames)
		tl.Events = append(tl.Events, ev)
		cursor = new(big.Rat).Set(ev.SequenceOut)
	}
	tl.Duration = cursor
	return tl, nil
}

func fillClip(ev *TimedEvent, media map[string]Source) error {
	def := ev.Definition
	if def.Media == "" {
		return fmt.Errorf("clip has no media")
	}
	src, ok := media[def.Media]
	if !ok {
		return fmt.Errorf("unknown media %q", def.Media)
	}
	if def.MediaInFrame < 0 || def.MediaOutFrame <= def.MediaInFrame {
		return fmt.Errorf("invalid media range [%d,%d)", def.MediaInFrame, def.MediaOutFrame)
	}
	if def.MediaOutFrame > src.Frames {
		return fmt.Errorf("media range end %d exceeds %q length %d", def.MediaOutFrame, src.ID, src.Frames)
	}

	speed := new(big.Rat).SetInt64(1)
	if strings.TrimSpace(def.Speed) != "" {
		var err error
		speed, err = ParseSeconds(def.Speed)
		if err != nil {
			return fmt.Errorf("speed: %w", err)
		}
		if speed.Sign() <= 0 {
			return fmt.Errorf("speed must be positive")
		}
	}

	frames := intRat(def.MediaOutFrame - def.MediaInFrame)
	ev.Source = &src
	ev.Speed = speed
	ev.MediaIn = divRat(intRat(def.MediaInFrame), src.FPS)
	ev.MediaOut = divRat(intRat(def.MediaOutFrame), src.FPS)
	ev.MediaDuration = subRat(ev.MediaOut, ev.MediaIn)
	ev.SourceOut = new(big.Rat).Set(ev.MediaDuration)
	ev.SequenceDuration = divRat(divRat(frames, src.FPS), speed)
	return nil
}
