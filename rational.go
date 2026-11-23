package fcpxml

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Time is an exact, non-negative-denominator rational number of seconds.
// Keeping time rational avoids frame drift when FCPXML values such as
// 1001/30000s are mapped through several nested timelines.
type Time struct {
	Num int64
	Den int64
}

// ZeroTime is the zero point of an FCPXML timeline.
var ZeroTime = Time{Num: 0, Den: 1}

// ParseTime parses an FCPXML time value. FCPXML writes times as a rational
// number followed by "s" ("1001/30000s"); plain integers and decimals are
// accepted as a convenience.
func ParseTime(value string) (Time, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return Time{}, errors.New("empty time")
	}
	if strings.HasSuffix(text, "s") || strings.HasSuffix(text, "S") {
		text = text[:len(text)-1]
	}
	if text == "" {
		return Time{}, fmt.Errorf("invalid time %q", value)
	}

	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return Time{}, fmt.Errorf("invalid FCPXML time %q", value)
	}
	if r.Denom().Sign() <= 0 {
		return Time{}, fmt.Errorf("invalid denominator in time %q", value)
	}
	if !r.Num().IsInt64() || !r.Denom().IsInt64() {
		return Time{}, fmt.Errorf("time %q exceeds int64 range", value)
	}
	return Time{Num: r.Num().Int64(), Den: r.Denom().Int64()}, nil
}

// String returns the canonical FCPXML representation of t.
func (t Time) String() string {
	t = t.canonical()
	if t.Den == 1 {
		return fmt.Sprintf("%ds", t.Num)
	}
	return fmt.Sprintf("%d/%ds", t.Num, t.Den)
}

func (t Time) canonical() Time {
	if t.Den == 0 {
		return Time{Num: 0, Den: 1}
	}
	r := t.rat()
	if !r.Num().IsInt64() || !r.Denom().IsInt64() {
		return t
	}
	return Time{Num: r.Num().Int64(), Den: r.Denom().Int64()}
}

func (t Time) rat() *big.Rat {
	if t.Den == 0 {
		return new(big.Rat)
	}
	return new(big.Rat).SetFrac64(t.Num, t.Den)
}

func timeFromRat(r *big.Rat) (Time, error) {
	if r == nil {
		return Time{}, errors.New("nil rational time")
	}
	if !r.Num().IsInt64() || !r.Denom().IsInt64() {
		return Time{}, errors.New("rational time exceeds int64 range")
	}
	return Time{Num: r.Num().Int64(), Den: r.Denom().Int64()}, nil
}

func (t Time) Add(other Time) (Time, error) {
	return timeFromRat(new(big.Rat).Add(t.rat(), other.rat()))
}

func (t Time) Sub(other Time) (Time, error) {
	return timeFromRat(new(big.Rat).Sub(t.rat(), other.rat()))
}

func (t Time) Mul(other Time) (Time, error) {
	return timeFromRat(new(big.Rat).Mul(t.rat(), other.rat()))
}

func (t Time) Div(other Time) (Time, error) {
	if other.Num == 0 {
		return Time{}, errors.New("division by zero time")
	}
	return timeFromRat(new(big.Rat).Quo(t.rat(), other.rat()))
}

// Cmp returns -1, 0, or 1 as t is less than, equal to, or greater than other.
func (t Time) Cmp(other Time) int {
	return t.rat().Cmp(other.rat())
}

func (t Time) IsZero() bool {
	return t.Num == 0
}

func (t Time) Abs() Time {
	if t.Num >= 0 {
		return t.canonical()
	}
	if t.Num == -1<<63 {
		return t
	}
	return Time{Num: -t.Num, Den: t.Den}.canonical()
}

func timeMin(a, b Time) Time {
	if a.Cmp(b) <= 0 {
		return a
	}
	return b
}

func timeMax(a, b Time) Time {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}

// Rounding controls conversion of an exact time to a frame index.
type Rounding int

const (
	RoundNearest Rounding = iota
	RoundFloor
	RoundCeil
)

// FrameAt converts t to a frame index at frameDuration. FCPXML cuts should
// normally land exactly on frame boundaries; RoundNearest is the default for
// values that do not because a nested sequence may use a different frame rate.
func FrameAt(t, frameDuration Time, rounding Rounding) (int64, error) {
	if frameDuration.Num <= 0 || frameDuration.Den <= 0 {
		return 0, errors.New("frame duration must be positive")
	}
	q := new(big.Rat).Quo(t.rat(), frameDuration.rat())
	num, den := q.Num(), q.Denom()
	whole, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() == 0 {
		if !whole.IsInt64() {
			return 0, errors.New("frame index exceeds int64 range")
		}
		return whole.Int64(), nil
	}

	switch rounding {
	case RoundFloor:
		if num.Sign() < 0 {
			whole.Sub(whole, big.NewInt(1))
		}
	case RoundCeil:
		if num.Sign() > 0 {
			whole.Add(whole, big.NewInt(1))
		}
	case RoundNearest:
		twiceRem := new(big.Int).Abs(rem)
		twiceRem.Lsh(twiceRem, 1)
		if twiceRem.Cmp(den) >= 0 {
			if num.Sign() < 0 {
				whole.Sub(whole, big.NewInt(1))
			} else {
				whole.Add(whole, big.NewInt(1))
			}
		}
	default:
		return 0, fmt.Errorf("unknown rounding mode %d", rounding)
	}
	if !whole.IsInt64() {
		return 0, errors.New("frame index exceeds int64 range")
	}
	return whole.Int64(), nil
}
