package timeline

import (
	"fmt"
	"math/big"
	"strings"
)

var rateAliases = map[string]string{
	"23.976": "24000/1001",
	"23.98":  "24000/1001",
	"29.97":  "30000/1001",
	"59.94":  "60000/1001",
}

// ParseRate parses an exact frame rate. Integer, rational, and decimal
// spellings are accepted; common NTSC spellings are normalized to 1001-denom
// rationals instead of being treated as rounded decimal values.
func ParseRate(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty frame rate")
	}
	if normalized, ok := rateAliases[s]; ok {
		s = normalized
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return nil, fmt.Errorf("invalid frame rate %q", s)
	}
	return r, nil
}

func ParseSeconds(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty time")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid time %q", s)
	}
	return r, nil
}

func ratString(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	return r.Num().String() + "/" + r.Denom().String()
}

func decimalString(r *big.Rat, places int) string {
	return r.FloatString(places)
}

func addRat(a, b *big.Rat) *big.Rat {
	return new(big.Rat).Add(a, b)
}

func subRat(a, b *big.Rat) *big.Rat {
	return new(big.Rat).Sub(a, b)
}

func mulRat(a, b *big.Rat) *big.Rat {
	return new(big.Rat).Mul(a, b)
}

func divRat(a, b *big.Rat) *big.Rat {
	return new(big.Rat).Quo(a, b)
}

func intRat(v int64) *big.Rat {
	return new(big.Rat).SetInt64(v)
}

func absRat(r *big.Rat) *big.Rat {
	return new(big.Rat).Abs(r)
}

// roundRat returns the nearest integer, with exact half values rounded away
// from zero. Sequence frame positions are non-negative in valid timelines.
func roundRat(r *big.Rat) *big.Int {
	n, d := r.Num(), r.Denom()
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(n, d, rem)

	twice := new(big.Int).Abs(rem)
	twice.Lsh(twice, 1)
	if twice.Cmp(d) >= 0 {
		step := int64(1)
		if n.Sign() < 0 {
			step = -1
		}
		q.Add(q, big.NewInt(step))
	}
	return q
}
