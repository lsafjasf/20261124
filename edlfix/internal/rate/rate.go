package rate

import (
	"fmt"
	"strconv"
	"strings"
)

// Rate describes a media frame rate in the form timecode math needs.
type Rate struct {
	// NominalFPS is the integer frame count used inside HH:MM:SS:FF labels.
	NominalFPS int
	// DropCapable reports whether SMPTE drop-frame numbering applies
	// (30000/1001 and 60000/1001 only).
	DropCapable bool
	Input       string
}

func (r Rate) String() string { return r.Input }

// Resolve accepts common EDL/media spellings: integers ("30"), decimals
// ("29.97", "23.976") and rational pairs ("30000/1001", "24000/1001").
func Resolve(input string) (Rate, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return Rate{}, fmt.Errorf("empty frame rate")
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		num, err1 := strconv.ParseFloat(s[:i], 64)
		den, err2 := strconv.ParseFloat(s[i+1:], 64)
		if err1 != nil || err2 != nil || den == 0 {
			return Rate{}, fmt.Errorf("unparseable frame rate %q", input)
		}
		return fromFloat(s, num/den)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return Rate{}, fmt.Errorf("unparseable frame rate %q", input)
	}
	return fromFloat(s, f)
}

func fromFloat(input string, fps float64) (Rate, error) {
	if fps <= 0 {
		return Rate{}, fmt.Errorf("frame rate must be positive, got %q", input)
	}
	nominal := int(fps + 0.5)
	if nominal < 1 || nominal > 240 {
		return Rate{}, fmt.Errorf("frame rate %q outside supported range", input)
	}
	// Fractional pull-down rates (29.97, 59.94, 23.976...) map to the nominal
	// label rate; only the 30k/60k family uses drop-frame numbering.
	r := Rate{NominalFPS: nominal, Input: input}
	if nominal == 30 && fps < float64(nominal) {
		r.DropCapable = true
	}
	if nominal == 60 && fps < float64(nominal) {
		r.DropCapable = true
	}
	return r, nil
}
