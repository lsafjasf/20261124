package timecode

import (
	"errors"
	"fmt"
)

// Mode describes how a timecode string labels its frame counting convention.
type Mode int

const (
	// NDF labels use ':' between seconds and frames (HH:MM:SS:FF).
	NDF Mode = iota
	// DF labels use ';' (or a non-colon separator) between seconds and frames.
	DF
)

func (m Mode) String() string {
	switch m {
	case DF:
		return "DF"
	default:
		return "NDF"
	}
}

// Timecode is a parsed HH:MM:SS:FF label together with its declared mode.
type Timecode struct {
	Hour      int
	Minute    int
	Second    int
	Frame     int
	Mode      Mode
	Raw       string // exact 11-character token as found in the source EDL
	ByteStart int    // byte offset of Raw inside its source line
}

// ErrInvalidToken means a token is not a syntactically valid HH:MM:SS:FF label.
var ErrInvalidToken = errors.New("invalid timecode token")

// Parse extracts an 11-character timecode token at byte offset start.
// Any separator other than ':' between seconds and frames marks it as DF.
func Parse(token string, start int) (Timecode, error) {
	if len(token) != 11 ||
		token[2] != ':' || token[5] != ':' ||
		(token[8] != ':' && token[8] != ';' && token[8] != '.' && token[8] != ',') {
		return Timecode{}, fmt.Errorf("%w: %q", ErrInvalidToken, token)
	}
	tc := Timecode{Raw: token, ByteStart: start, Mode: NDF}
	if token[8] != ':' {
		tc.Mode = DF
	}
	vals := [4]int{}
	pos := [4]int{0, 3, 6, 9}
	for i, p := range pos {
		hi, lo := token[p], token[p+1]
		if hi < '0' || hi > '9' || lo < '0' || lo > '9' {
			return Timecode{}, fmt.Errorf("%w: %q", ErrInvalidToken, token)
		}
		vals[i] = int(hi-'0')*10 + int(lo-'0')
	}
	tc.Hour, tc.Minute, tc.Second, tc.Frame = vals[0], vals[1], vals[2], vals[3]
	return tc, nil
}

// IsValidLabel reports whether the label is legal for the given nominal frame
// rate. A DF label that actually falls on a dropped frame number is malformed
// and can never appear in a valid EDL (e.g. 00:01:00;00 at 29.97).
func (tc Timecode) IsValidLabel(nominalFPS int) bool {
	if tc.Minute > 59 || tc.Second > 59 || tc.Frame >= nominalFPS {
		return false
	}
	if tc.Mode != DF || !IsDropRate(nominalFPS) {
		return true
	}
	return !isDroppedNumber(tc, nominalFPS)
}

// IsDropRate reports whether a nominal frame rate uses drop-frame numbering.
// Only the SMPTE 30000/1001 (29.97) and 60000/1001 (59.94) rates do.
func IsDropRate(nominalFPS int) bool {
	return nominalFPS == 30 || nominalFPS == 60
}

func isDroppedNumber(tc Timecode, nominalFPS int) bool {
	if tc.Second != 0 || tc.Minute%10 == 0 {
		return false
	}
	return tc.Frame < dropPerMinute(nominalFPS)
}

// ToFrames converts a label to an absolute frame count using the convention
// that the label itself declares. Frame counts are the only invariant used
// when repairing: they describe the same physical media frame regardless of
// whether the label was written DF or NDF.
func ToFrames(tc Timecode, nominalFPS int) (int64, error) {
	if tc.Minute > 59 || tc.Second > 59 || tc.Frame >= nominalFPS {
		return 0, fmt.Errorf("timecode out of range for %dfps: %q", nominalFPS, tc.Raw)
	}
	totalMinutes := int64(tc.Hour)*60 + int64(tc.Minute)
	base := totalMinutes*60*int64(nominalFPS) +
		int64(tc.Second)*int64(nominalFPS) + int64(tc.Frame)
	if tc.Mode != DF || !IsDropRate(nominalFPS) {
		return base, nil
	}
	if isDroppedNumber(tc, nominalFPS) {
		return 0, fmt.Errorf("illegal drop-frame label %q: %02d:%02d:%02d;%02d is a dropped frame number",
			tc.Raw, tc.Hour, tc.Minute, tc.Second, tc.Frame)
	}
	drop := int64(dropPerMinute(nominalFPS))
	dropped := drop * (totalMinutes - totalMinutes/10)
	return base - dropped, nil
}

// FromFrames renders an absolute frame count in the requested mode at the
// given nominal frame rate.
func FromFrames(frames int64, nominalFPS int, mode Mode) (Timecode, error) {
	if frames < 0 {
		return Timecode{}, fmt.Errorf("negative frame count %d", frames)
	}
	if mode == DF && !IsDropRate(nominalFPS) {
		mode = NDF // DF numbering only exists for 29.97/59.94; render NDF instead.
	}
	if mode != DF {
		fps := int64(nominalFPS)
		rest := frames
		tc := Timecode{Mode: NDF}
		tc.Frame = int(rest % fps)
		rest /= fps
		tc.Second = int(rest % 60)
		rest /= 60
		tc.Minute = int(rest % 60)
		tc.Hour = int(rest / 60)
		return tc, nil
	}
	return fromFramesDF(frames, nominalFPS), nil
}

func fromFramesDF(frames int64, nominalFPS int) Timecode {
	drop := int64(dropPerMinute(nominalFPS))
	fps := int64(nominalFPS)
	// Invert the drop-frame layout by construction. A 10-minute cycle holds
	// 10*fpm - 9*drop real frames: its first minute is full and each of the
	// other nine minutes loses "drop" labels. Recover the minute index inside
	// the cycle, add the skipped labels back, then decompose the counter.
	fpm := int64(60 * fps)
	framesPer10Min := 10*fpm - 9*drop
	cycles := frames / framesPer10Min
	within := frames % framesPer10Min
	minutesAfterFirst := int64(0)
	if within >= fpm {
		minutesAfterFirst = 1 + (within-fpm)/(fpm-drop)
	}
	counter := within + drop*minutesAfterFirst
	totalMinutes := int64(10)*cycles + counter/fpm
	tc := Timecode{Mode: DF}
	tc.Hour = int(totalMinutes / 60)
	tc.Minute = int(totalMinutes % 60)
	tc.Second = int((counter / fps) % 60)
	tc.Frame = int(counter % fps)
	return tc
}

func dropPerMinute(nominalFPS int) int {
	if nominalFPS == 60 {
		return 4 // 60000/1001: four labels skipped each minute except every tenth
	}
	return 2
}

// Render produces the canonical 11-character label for the parsed components.
func (tc Timecode) Render() string {
	sep := byte(':')
	if tc.Mode == DF {
		sep = ';'
	}
	return fmt.Sprintf("%02d:%02d:%02d%c%02d", tc.Hour, tc.Minute, tc.Second, sep, tc.Frame)
}
