package edl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// TimecodeMode describes the label arithmetic, not the playback rate. Both
// DF and NDF labels can exist at 30000/1001.
type TimecodeMode string

const (
	DF  TimecodeMode = "DF"
	NDF TimecodeMode = "NDF"
)

func ParseTimecodeMode(s string) (TimecodeMode, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DF", "DROP", "DROP FRAME", "DROP-FRAME":
		return DF, nil
	case "NDF", "NON-DROP", "NONDROP", "NON-DROP FRAME":
		return NDF, nil
	default:
		return "", fmt.Errorf("unknown timecode mode %q", s)
	}
}

// Timecode is a signed, unwrapped frame index plus the labels used to express
// it. A negative value is accepted for negative-offset edit lists.
type Timecode struct {
	Frames int64
	Rate   Rate
	Mode   TimecodeMode
}

// MarshalJSON emits a readable label next to the exact frame index.
func (t Timecode) MarshalJSON() ([]byte, error) {
	rate := t.Rate
	if !rate.Valid() {
		rate = Rate2997
	}
	return json.Marshal(struct {
		Text      string       `json:"text"`
		Frames    int64        `json:"frames"`
		Mode      TimecodeMode `json:"mode"`
		Rate      string       `json:"rate"`
		FPS       string       `json:"fps"`
		DropFrame bool         `json:"dropFrame"`
	}{
		Text:      t.String(),
		Frames:    t.Frames,
		Mode:      t.Mode,
		Rate:      rate.String(),
		FPS:       rate.Decimal(),
		DropFrame: t.Mode == DF,
	})
}

// ParseTimecode parses HH:MM:SS:FF or HH:MM:SS;FF. A semicolon (or comma)
// explicitly means drop-frame. Otherwise defaultMode is used. The caller can
// therefore honor a CMX FCM header while still accepting semicolon-marked DF
// lines in an otherwise NDF file.
func ParseTimecode(s string, rate Rate, defaultMode TimecodeMode) (Timecode, error) {
	if !rate.Valid() {
		return Timecode{}, fmt.Errorf("invalid frame rate")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return Timecode{}, fmt.Errorf("empty timecode")
	}
	sign := int64(1)
	switch s[0] {
	case '-':
		sign = -1
		s = s[1:]
	case '+':
		s = s[1:]
	}
	if s == "" {
		return Timecode{}, fmt.Errorf("empty timecode")
	}

	separators := make([]byte, 0, 3)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ':', ';', ',':
			separators = append(separators, s[i])
		}
	}
	if len(separators) != 3 {
		return Timecode{}, fmt.Errorf("timecode %q must contain four fields", s)
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ':' || r == ';' || r == ','
	})
	if len(fields) != 4 {
		return Timecode{}, fmt.Errorf("timecode %q must contain four fields", s)
	}

	var values [4]int64
	for i, field := range fields {
		if field == "" {
			return Timecode{}, fmt.Errorf("timecode %q has an empty field", s)
		}
		for _, r := range field {
			if r < '0' || r > '9' {
				return Timecode{}, fmt.Errorf("timecode %q has non-digits", s)
			}
		}
		v, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return Timecode{}, fmt.Errorf("timecode %q: %w", s, err)
		}
		values[i] = v
	}

	mode := defaultMode
	if mode == "" {
		mode = NDF
	}
	if last := separators[2]; last == ';' || last == ',' {
		mode = DF
	}
	if mode == DF && !rate.DropFrameCapable() {
		return Timecode{}, fmt.Errorf("drop-frame timecode is not valid at %s fps", rate)
	}

	nominal := rate.NominalFPS()
	if values[1] >= 60 || values[2] >= 60 {
		return Timecode{}, fmt.Errorf("timecode %q has minutes/seconds outside 0..59", s)
	}
	if values[3] >= nominal {
		return Timecode{}, fmt.Errorf("timecode %q has frame %d outside 0..%d", s, values[3], nominal-1)
	}
	totalMinutes := values[0]*60 + values[1]
	if mode == DF && values[2] == 0 && totalMinutes%10 != 0 {
		if values[3] < rate.dropFrames() {
			return Timecode{}, fmt.Errorf("timecode %q names a frame skipped by drop-frame numbering", s)
		}
	}

	labelFrames := ((values[0]*3600+values[1]*60+values[2])*nominal + values[3])
	dropCount := int64(0)
	if mode == DF {
		dropCount = rate.dropFrames() * (totalMinutes - totalMinutes/10)
	}
	return Timecode{
		Frames: sign * (labelFrames - dropCount),
		Rate:   rate,
		Mode:   mode,
	}, nil
}

// FromFrames resolves an absolute frame index back to SMPTE labels.
func FromFrames(frames int64, rate Rate, mode TimecodeMode) (Timecode, error) {
	if !rate.Valid() {
		return Timecode{}, fmt.Errorf("invalid frame rate")
	}
	if mode == "" {
		mode = NDF
	}
	if mode == DF && !rate.DropFrameCapable() {
		return Timecode{}, fmt.Errorf("drop-frame timecode is not valid at %s fps", rate)
	}
	sign := int64(1)
	if frames < 0 {
		sign = -1
		frames = -frames
	}
	labelFrames := frames
	if mode == DF {
		drop := rate.dropFrames()
		per10 := rate.framesPer10Minutes()
		perMinute := rate.framesPerMinute()
		tens, rem := frames/per10, frames%per10
		labelFrames += drop * 9 * tens
		if rem > drop {
			labelFrames += drop * ((rem - drop) / perMinute)
		}
	}
	return Timecode{
		Frames: sign * frames,
		Rate:   rate,
		Mode:   mode,
	}, nil
}

func (t Timecode) String() string {
	rate := t.Rate
	if !rate.Valid() {
		rate = Rate2997
	}
	mode := t.Mode
	if mode == "" {
		mode = NDF
	}
	frames := t.Frames
	sign := ""
	if frames < 0 {
		sign = "-"
		frames = -frames
	}
	labelFrames := frames
	if mode == DF {
		drop := rate.dropFrames()
		per10 := rate.framesPer10Minutes()
		perMinute := rate.framesPerMinute()
		tens, rem := frames/per10, frames%per10
		labelFrames += drop * 9 * tens
		if rem > drop {
			labelFrames += drop * ((rem - drop) / perMinute)
		}
	}
	nominal := rate.NominalFPS()
	ff := labelFrames % nominal
	totalSeconds := labelFrames / nominal
	ss := totalSeconds % 60
	totalMinutes := totalSeconds / 60
	mm := totalMinutes % 60
	hh := totalMinutes / 60
	sep := ":"
	if mode == DF {
		sep = ";"
	}
	return fmt.Sprintf("%s%02d:%02d:%02d%s%02d", sign, hh, mm, ss, sep, ff)
}

func (t Timecode) Seconds() Fraction {
	return t.Rate.SecondsPerFrame().Mul(NewFractionNoErr(t.Frames, 1))
}

func looksLikeTimecode(s string) bool {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' || s[i] == ';' || s[i] == ',' {
			count++
		}
	}
	return count == 3
}

func framesPerDay(mode TimecodeMode, rate Rate) int64 {
	if mode == DF {
		return rate.framesPerDay()
	}
	return rate.NominalFPS() * 24 * 60 * 60
}

// unwrapForward expands a wrapped timecode until it is strictly after before.
// It is used for source and record out-points that cross midnight.
func unwrapForward(raw, before int64, mode TimecodeMode, rate Rate) int64 {
	day := framesPerDay(mode, rate)
	for raw <= before {
		raw += day
	}
	return raw
}
