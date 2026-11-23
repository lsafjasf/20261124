package edl

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Fraction is a signed rational number. It is used for frame rates and
// durations so that 30000/1001 never turns into a drifting float.
type Fraction struct {
	Num int64
	Den int64
}

// NewFraction returns a normalized fraction.
func NewFraction(num, den int64) (Fraction, error) {
	if den == 0 {
		return Fraction{}, fmt.Errorf("zero denominator")
	}
	if den < 0 {
		num, den = -num, -den
	}
	g := gcd(abs64(num), den)
	return Fraction{Num: num / g, Den: den / g}, nil
}

func mustFraction(num, den int64) Fraction {
	f, err := NewFraction(num, den)
	if err != nil {
		panic(err)
	}
	return f
}

func (f Fraction) String() string {
	if f.Den == 0 {
		return "0/0"
	}
	if f.Den == 1 {
		return strconv.FormatInt(f.Num, 10)
	}
	return strconv.FormatInt(f.Num, 10) + "/" + strconv.FormatInt(f.Den, 10)
}

func (f Fraction) Decimal(places int) string {
	if f.Den == 0 {
		return "NaN"
	}
	v := float64(f.Num) / float64(f.Den)
	s := strconv.FormatFloat(v, 'f', places, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

func (f Fraction) Float64() float64 {
	if f.Den == 0 {
		return math.NaN()
	}
	return float64(f.Num) / float64(f.Den)
}

func (f Fraction) Sign() int {
	switch {
	case f.Num < 0:
		return -1
	case f.Num > 0:
		return 1
	default:
		return 0
	}
}

func (f Fraction) Add(g Fraction) Fraction {
	return mustFraction(f.Num*g.Den+g.Num*f.Den, f.Den*g.Den)
}

func (f Fraction) Sub(g Fraction) Fraction {
	return mustFraction(f.Num*g.Den-g.Num*f.Den, f.Den*g.Den)
}

func (f Fraction) Mul(g Fraction) Fraction {
	return mustFraction(f.Num*g.Num, f.Den*g.Den)
}

func (f Fraction) Div(g Fraction) Fraction {
	return mustFraction(f.Num*g.Den, f.Den*g.Num)
}

func (f Fraction) Cmp(g Fraction) int {
	left := f.Num * g.Den
	right := g.Num * f.Den
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}
