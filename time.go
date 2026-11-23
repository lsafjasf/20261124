package fcpxml

import (
	"fmt"
	"math/big"
	"strings"
)

// Rational is an exact FCPXML time value in seconds.
type Rational struct {
	r *big.Rat
}

func NewRational(r *big.Rat) Rational {
	if r == nil {
		return Rational{}
	}
	return Rational{r: new(big.Rat).Set(r)}
}

func (v Rational) Valid() bool { return v.r != nil }

func (v Rational) Rat() *big.Rat {
	if v.r == nil {
		return nil
	}
	return new(big.Rat).Set(v.r)
}

func (v Rational) String() string {
	if v.r == nil {
		return ""
	}
	return v.r.RatString()
}

func (v Rational) Decimal(precision int) string {
	if v.r == nil {
		return ""
	}
	return v.r.FloatString(precision)
}

func (v Rational) Cmp(other Rational) int {
	switch {
	case !v.Valid() && !other.Valid():
		return 0
	case !v.Valid():
		return -1
	case !other.Valid():
		return 1
	default:
		return v.r.Cmp(other.r)
	}
}

func (v Rational) Add(other Rational) Rational {
	if !v.Valid() || !other.Valid() {
		return Rational{}
	}
	return NewRational(new(big.Rat).Add(v.r, other.r))
}

func (v Rational) Sub(other Rational) Rational {
	if !v.Valid() || !other.Valid() {
		return Rational{}
	}
	return NewRational(new(big.Rat).Sub(v.r, other.r))
}

func (v Rational) Quo(other Rational) Rational {
	if !v.Valid() || !other.Valid() || other.r.Sign() == 0 {
		return Rational{}
	}
	return NewRational(new(big.Rat).Quo(v.r, other.r))
}

// ParseTime parses an FCPXML time. Bare numbers and values ending in "s" are
// seconds; fractions such as "1001/24000s" remain exact.
func ParseTime(value string) (Rational, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return Rational{}, fmt.Errorf("empty time")
	}
	if strings.Contains(text, ":") {
		return parseClockTime(text)
	}
	text = strings.TrimSuffix(text, "s")
	if text == "" {
		return Rational{}, fmt.Errorf("empty time")
	}
	rat, ok := new(big.Rat).SetString(text)
	if !ok {
		return Rational{}, fmt.Errorf("invalid time %q", value)
	}
	return NewRational(rat), nil
}

func parseClockTime(value string) (Rational, error) {
	negative := strings.HasPrefix(value, "-")
	text := strings.TrimPrefix(value, "-")
	parts := strings.Split(text, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return Rational{}, fmt.Errorf("invalid clock time %q", value)
	}
	total := new(big.Rat)
	for _, part := range parts {
		part = strings.TrimSuffix(part, "s")
		value, ok := new(big.Rat).SetString(part)
		if !ok || value.Sign() < 0 {
			return Rational{}, fmt.Errorf("invalid clock time %q", value)
		}
		total.Mul(total, big.NewRat(60, 1))
		total.Add(total, value)
	}
	if negative {
		total.Neg(total)
	}
	return NewRational(total), nil
}

func RatString(rat *big.Rat) string {
	if rat == nil {
		return ""
	}
	return rat.RatString()
}

func DecimalString(rat *big.Rat, precision int) string {
	if rat == nil {
		return ""
	}
	return rat.FloatString(precision)
}

func roundFrame(rat *big.Rat) *big.Int {
	if rat == nil {
		return nil
	}
	half := big.NewRat(1, 2)
	if rat.Sign() < 0 {
		half.Neg(half)
	}
	rounded := new(big.Rat).Add(rat, half)
	quotient := new(big.Int).Quo(rounded.Num(), rounded.Denom())
	return quotient
}
