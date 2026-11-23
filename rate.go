package edl

import (
	"fmt"
	"strconv"
	"strings"
)

// Rate is an exact frame rate.
type Rate struct {
	r Fraction
}

var (
	Rate2997 = mustRate(30000, 1001)
	Rate30   = mustRate(30, 1)
	Rate25   = mustRate(25, 1)
	Rate24   = mustRate(24, 1)
	Rate2398 = mustRate(24000, 1001)
)

func mustRate(num, den int64) Rate {
	r, err := NewRate(num, den)
	if err != nil {
		panic(err)
	}
	return r
}

// NewRate constructs an exact rate. Both values must be positive.
func NewRate(num, den int64) (Rate, error) {
	if num <= 0 || den <= 0 {
		return Rate{}, fmt.Errorf("rate numerator and denominator must be positive")
	}
	f, err := NewFraction(num, den)
	if err != nil {
		return Rate{}, err
	}
	return Rate{r: f}, nil
}

// ParseRate accepts common EDL rates. The common rounded spellings are
// normalized to their exact NTSC values: 29.97 -> 30000/1001 and
// 23.976/23.98 -> 24000/1001.
func ParseRate(s string) (Rate, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return Rate{}, fmt.Errorf("empty frame rate")
	}
	switch s {
	case "29.97", "29.970", "29.97fps", "ntsc":
		return Rate2997, nil
	case "23.976", "23.98", "23.976fps":
		return Rate2398, nil
	case "59.94", "59.940":
		return mustRate(60000, 1001), nil
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		num, err := strconv.ParseInt(strings.TrimSpace(s[:i]), 10, 64)
		if err != nil {
			return Rate{}, fmt.Errorf("invalid rate %q: %w", s, err)
		}
		den, err := strconv.ParseInt(strings.TrimSpace(s[i+1:]), 10, 64)
		if err != nil {
			return Rate{}, fmt.Errorf("invalid rate %q: %w", s, err)
		}
		return NewRate(num, den)
	}
	if num, err := strconv.ParseInt(s, 10, 64); err == nil {
		return NewRate(num, 1)
	}
	f, err := parseDecimalFraction(s)
	if err != nil {
		return Rate{}, fmt.Errorf("invalid frame rate %q", s)
	}
	if f.Num <= 0 {
		return Rate{}, fmt.Errorf("frame rate must be positive")
	}
	return Rate{r: f}, nil
}

func parseDecimalFraction(s string) (Fraction, error) {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" {
		return Fraction{}, fmt.Errorf("not a decimal")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Fraction{}, err
	}
	var frac int64
	scale := int64(1)
	if len(parts) == 2 {
		digits := parts[1]
		if digits == "" {
			return Fraction{}, fmt.Errorf("not a decimal")
		}
		for _, r := range digits {
			if r < '0' || r > '9' {
				return Fraction{}, fmt.Errorf("not a decimal")
			}
		}
		frac, err = strconv.ParseInt(digits, 10, 64)
		if err != nil {
			return Fraction{}, err
		}
		for range digits {
			scale *= 10
		}
	}
	num := whole*scale + frac
	if neg {
		num = -num
	}
	return NewFraction(num, scale)
}

func (r Rate) Fraction() Fraction { return r.r }
func (r Rate) String() string     { return r.r.String() }

// Decimal returns the conventional editorial spelling while String retains
// the exact rational value.
func (r Rate) Decimal() string {
	switch r.r {
	case mustFraction(30000, 1001):
		return "29.97"
	case mustFraction(24000, 1001):
		return "23.976"
	case mustFraction(60000, 1001):
		return "59.94"
	default:
		return r.r.Decimal(9)
	}
}

func (r Rate) FPS() float64 { return r.r.Float64() }

func (r Rate) Valid() bool {
	return r.r.Num > 0 && r.r.Den > 0
}

// FramesPerSecond returns the exact ratio. It is kept as a Fraction so frame
// conversions can be performed without float64.
func (r Rate) FramesPerSecond() Fraction { return r.r }

// SecondsPerFrame returns 1/fps exactly.
func (r Rate) SecondsPerFrame() Fraction {
	return NewFractionNoErr(r.r.Den, r.r.Num)
}

func NewFractionNoErr(num, den int64) Fraction {
	f, _ := NewFraction(num, den)
	return f
}

// NominalFPS is the integer label base used by timecode. For 30000/1001 this
// is 30; for 24000/1001 it is 24.
func (r Rate) NominalFPS() int64 {
	if r.r.Den == 1 {
		return r.r.Num
	}
	nearest := (r.r.Num + r.r.Den/2) / r.r.Den
	return nearest
}

// DropFrameCapable reports whether SMPTE drop-frame labels are defined for
// this exact rate. This implementation supports the 30000/1001 and 60000/1001
// families used by EDLs.
func (r Rate) DropFrameCapable() bool {
	return r.r == mustFraction(30000, 1001) || r.r == mustFraction(60000, 1001)
}

func (r Rate) dropFrames() int64 {
	switch r.NominalFPS() {
	case 30:
		return 2
	case 60:
		return 4
	default:
		return 0
	}
}

func (r Rate) framesPerMinute() int64 {
	return r.NominalFPS()*60 - r.dropFrames()
}

func (r Rate) framesPer10Minutes() int64 {
	return r.NominalFPS()*600 - r.dropFrames()*9
}

func (r Rate) framesPerDay() int64 {
	if !r.DropFrameCapable() {
		return r.NominalFPS() * 24 * 60 * 60
	}
	minutes := int64(24 * 60)
	return r.NominalFPS()*24*60*60 - r.dropFrames()*(minutes-minutes/10)
}
