package subtime

import (
	"fmt"
	"strconv"
	"strings"
)

// Frame identifies an actual frame count at a rate. For drop-frame timecode,
// Number counts real frames; it is not the displayed frame-label number.
type Frame struct {
	Number int64
	Rate   Rate
}

func NewFrame(number int64, rate Rate) (Frame, error) {
	frame := Frame{Number: number, Rate: rate}
	if err := frame.Validate(); err != nil {
		return Frame{}, err
	}
	return frame, nil
}

func (f Frame) Validate() error {
	if f.Number < 0 {
		return fmt.Errorf("subtime: negative frame number %d", f.Number)
	}
	return f.Rate.Validate()
}

// Seconds is the exact presentation time of the frame.
func (f Frame) Seconds() (Rational, error) {
	if err := f.Validate(); err != nil {
		return Rational{}, err
	}
	seconds, err := NewRational(f.Number, 1).Quo(f.Rate.FPS())
	if err != nil {
		return Rational{}, err
	}
	return seconds, nil
}

func (f Frame) Timecode() (string, error) {
	return FormatTimecode(f)
}

func (f Frame) String() string {
	text, err := FormatTimecode(f)
	if err != nil {
		return fmt.Sprintf("invalid-frame(%d@%s)", f.Number, f.Rate)
	}
	return text
}

// ParseTimecode parses SMPTE-style HH:MM:SS:FF or HH:MM:SS;FF. Drop-frame
// numbering is selected by the rate. A semicolon is accepted for either kind;
// the rate, not punctuation, determines drop-frame semantics.
func ParseTimecode(text string, rate Rate) (Frame, error) {
	if err := rate.Validate(); err != nil {
		return Frame{}, err
	}
	text = strings.TrimSpace(text)
	if separator := strings.LastIndex(text, ";"); separator >= 0 {
		text = text[:separator] + ":" + text[separator+1:]
	}
	parts := strings.Split(text, ":")
	if len(parts) != 4 {
		return Frame{}, fmt.Errorf("subtime: invalid timecode %q, want HH:MM:SS:FF", text)
	}

	values := make([]int64, 4)
	for i, part := range parts {
		value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || value < 0 {
			return Frame{}, fmt.Errorf("subtime: invalid timecode %q", text)
		}
		values[i] = value
	}

	hours, minutes, seconds, frameLabel := values[0], values[1], values[2], values[3]
	nominal := rate.NominalFPS()
	if minutes >= 60 || seconds >= 60 {
		return Frame{}, fmt.Errorf("subtime: invalid timecode %q: minutes and seconds must be below 60", text)
	}
	if frameLabel >= nominal {
		return Frame{}, fmt.Errorf("subtime: invalid timecode %q: frame label %d exceeds %d fps", text, frameLabel, nominal)
	}

	totalMinutes := hours*60 + minutes
	totalNominalFrames := ((hours*3600+minutes*60+seconds)*nominal + frameLabel)
	if !rate.DropFrame {
		return Frame{Number: totalNominalFrames, Rate: rate}, nil
	}

	if seconds == 0 && minutes%10 != 0 && frameLabel < 2 {
		return Frame{}, fmt.Errorf("subtime: %q uses a frame label dropped by 29.97DF", text)
	}
	dropped := 2 * (totalMinutes - totalMinutes/10)
	if totalNominalFrames < dropped {
		return Frame{}, fmt.Errorf("subtime: invalid drop-frame timecode %q", text)
	}
	return Frame{Number: totalNominalFrames - dropped, Rate: rate}, nil
}

// FormatTimecode renders a frame as SMPTE-style timecode. At 29.97DF the
// skipped labels 00 and 01 are omitted at every minute except tenth minutes.
func FormatTimecode(frame Frame) (string, error) {
	if err := frame.Validate(); err != nil {
		return "", err
	}
	nominal := frame.Rate.NominalFPS()
	label := frame.Number

	if frame.Rate.DropFrame {
		// Convert actual frame count to the nominal 30 fps label count.
		// 17982 actual frames cover ten minutes; 1798 cover one minute.
		tens, remainder := frame.Number/17982, frame.Number%17982
		label += 18 * tens
		if remainder > 1 {
			label += 2 * ((remainder - 2) / 1798)
		}
	}

	hours := label / (nominal * 3600)
	label %= nominal * 3600
	minutes := label / (nominal * 60)
	label %= nominal * 60
	seconds := label / nominal
	frames := label % nominal

	separator := ":"
	if frame.Rate.DropFrame {
		separator = ";"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%02d", hours, minutes, seconds, separator, frames), nil
}

// ParseClock parses a subtitle clock value such as 01:02:03,456 or
// 01:02:03.456 as exact seconds. Floating point is not used.
func ParseClock(text string) (Rational, error) {
	text = strings.TrimSpace(text)
	parts := strings.Split(text, ":")
	if len(parts) != 3 {
		return Rational{}, fmt.Errorf("subtime: invalid clock %q, want HH:MM:SS,mmm", text)
	}

	secondsText := parts[2]
	if before, after, ok := strings.Cut(secondsText, ","); ok {
		secondsText = before + "." + after
	}
	secondsValue, err := ParseRational(secondsText)
	if err != nil {
		return Rational{}, fmt.Errorf("subtime: invalid clock %q: %w", text, err)
	}

	hours, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || hours < 0 {
		return Rational{}, fmt.Errorf("subtime: invalid clock %q", text)
	}
	minutes, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || minutes < 0 || minutes >= 60 {
		return Rational{}, fmt.Errorf("subtime: invalid clock %q", text)
	}
	if secondsValue.Sign() < 0 || secondsValue.Cmp(NewRational(60, 1)) >= 0 {
		return Rational{}, fmt.Errorf("subtime: invalid clock %q", text)
	}

	total := NewRational(hours*3600+minutes*60, 1).Add(secondsValue)
	return total, nil
}

// SnapClockMillis converts exact seconds to integer milliseconds using an
// explicit policy.
func SnapClockMillis(seconds Rational, policy SnapPolicy) (int64, error) {
	if seconds.Sign() < 0 {
		return 0, fmt.Errorf("subtime: negative clock value")
	}
	millis, err := snapRational(seconds.Mul(NewRational(1000, 1)), policy)
	if err != nil {
		return 0, err
	}
	if millis < 0 {
		return 0, fmt.Errorf("subtime: negative clock value")
	}
	return millis, nil
}

// FormatClockMillis renders nonnegative integer milliseconds as HH:MM:SS,mmm.
func FormatClockMillis(millis int64) (string, error) {
	if millis < 0 {
		return "", fmt.Errorf("subtime: negative clock value")
	}
	hours := millis / 3_600_000
	millis %= 3_600_000
	minutes := millis / 60_000
	millis %= 60_000
	secs := millis / 1_000
	millis %= 1_000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, millis), nil
}

// FormatClock renders exact seconds as HH:MM:SS,mmm using an explicit
// millisecond snap policy.
func FormatClock(seconds Rational, policy SnapPolicy) (string, error) {
	millis, err := SnapClockMillis(seconds, policy)
	if err != nil {
		return "", err
	}
	return FormatClockMillis(millis)
}
