// Package timebase provides exact rational frame-rate and timebase arithmetic.
//
// A Quantity is an integer count at a rational rate. No conversion in this
// package uses float64: a conversion is expressed as an exact fraction and is
// rounded only when the caller asks for an integer destination count.
package timebase

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Rate is a rational number of units per second. For example, frame rate
// 24000/1001 is stored as {Num: 24000, Den: 1001}; a 90 kHz clock is {90000, 1}.
type Rate struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

var rateAliases = map[string]string{
	"23.976": "24000/1001",
	"23.98":  "24000/1001",
	"29.97":  "30000/1001",
	"29.970": "30000/1001",
	"47.952": "48000/1001",
	"59.94":  "60000/1001",
	"59.940": "60000/1001",
	"119.88": "120000/1001",
}

// NewRate validates and normalizes a rational rate.
func NewRate(num, den int64) (Rate, error) {
	if num <= 0 || den <= 0 {
		return Rate{}, fmt.Errorf("rate must be positive: %d/%d", num, den)
	}
	g := gcd(num, den)
	return Rate{Num: num / g, Den: den / g}, nil
}

// ParseRate accepts "24", "24000/1001", or a decimal spelling. Common NTSC
// spellings such as 23.976 are normalized to 24000/1001 instead of being
// treated as a rounded decimal, because the 1001 denominator is the whole
// point of this diagnostic.
func ParseRate(s string) (Rate, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Rate{}, errors.New("empty rate")
	}
	if normalized, ok := rateAliases[s]; ok {
		s = normalized
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return Rate{}, fmt.Errorf("invalid rate %q", s)
	}
	// Keep the parse exact. big.Rat.SetString rejects the spelling "1/0".
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())
	if !num.IsInt64() || !den.IsInt64() {
		return Rate{}, fmt.Errorf("rate out of int64 range: %q", s)
	}
	return NewRate(num.Int64(), den.Int64())
}

// Rat returns the rate as an exact rational.
func (r Rate) Rat() *big.Rat {
	return new(big.Rat).SetFrac(big.NewInt(r.Num), big.NewInt(r.Den))
}

// Valid reports whether r is a positive rational.
func (r Rate) Valid() bool { return r.Num > 0 && r.Den > 0 }

// String renders an exact rate.
func (r Rate) String() string {
	if r.Den == 1 {
		return strconv.FormatInt(r.Num, 10)
	}
	return strconv.FormatInt(r.Num, 10) + "/" + strconv.FormatInt(r.Den, 10)
}

// MarshalJSON writes the exact spelling, which avoids a decimal round-trip.
func (r Rate) MarshalJSON() ([]byte, error) {
	if !r.Valid() {
		return nil, errors.New("cannot encode invalid rate")
	}
	return json.Marshal(r.String())
}

// UnmarshalJSON accepts either an exact string or a {"num":...,"den":...}
// object. Decimal strings are still parsed exactly, then normalized.
func (r *Rate) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		parsed, err := ParseRate(s)
		if err != nil {
			return err
		}
		*r = parsed
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		parsed, err := ParseRate(number.String())
		if err != nil {
			return err
		}
		*r = parsed
		return nil
	}
	var raw struct {
		Num int64 `json:"num"`
		Den int64 `json:"den"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("rate must be a string or {num,den}: %w", err)
	}
	parsed, err := NewRate(raw.Num, raw.Den)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// Quantity is an integer count at Rate. A frame count and a PTS tick count are
// both Quantities; the rate is what gives them meaning.
type Quantity struct {
	Count int64
	Rate  Rate
}

// Seconds returns the exact time represented by q.
func (q Quantity) Seconds() *big.Rat {
	// count / (num/den) = count*den/num
	num := new(big.Int).Mul(big.NewInt(q.Count), big.NewInt(q.Rate.Den))
	return new(big.Rat).SetFrac(num, big.NewInt(q.Rate.Num))
}

// CountIn returns the exact (possibly fractional) count in the target rate.
func (q Quantity) CountIn(to Rate) *big.Rat {
	// count * to.num/to.den / (from.num/from.den)
	num := new(big.Int).Mul(big.NewInt(q.Count), big.NewInt(to.Num))
	num.Mul(num, big.NewInt(q.Rate.Den))
	den := new(big.Int).Mul(big.NewInt(to.Den), big.NewInt(q.Rate.Num))
	return new(big.Rat).SetFrac(num, den)
}

// Rounding selects the quantization direction.
type Rounding uint8

const (
	Floor Rounding = iota
	Ceil
	Nearest
)

func (r Rounding) String() string {
	switch r {
	case Floor:
		return "floor"
	case Ceil:
		return "ceil"
	case Nearest:
		return "nearest"
	default:
		return "unknown"
	}
}

// Quantize converts an exact rational count to an integer using mode. Values
// exactly halfway between two integers round away from zero.
func Quantize(v *big.Rat, mode Rounding) (int64, error) {
	if v == nil {
		return 0, errors.New("nil rational")
	}
	n, d := v.Num(), v.Denom()
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(n, d, rem)
	sign := n.Sign()

	switch mode {
	case Floor:
		if sign < 0 && rem.Sign() != 0 {
			q.Sub(q, big.NewInt(1))
		}
	case Ceil:
		if sign > 0 && rem.Sign() != 0 {
			q.Add(q, big.NewInt(1))
		}
	case Nearest:
		twice := new(big.Int).Abs(rem)
		twice.Lsh(twice, 1)
		if twice.Cmp(d) >= 0 {
			if sign < 0 {
				q.Sub(q, big.NewInt(1))
			} else {
				q.Add(q, big.NewInt(1))
			}
		}
	default:
		return 0, fmt.Errorf("unknown rounding mode %d", mode)
	}
	if !q.IsInt64() {
		return 0, fmt.Errorf("quantized value out of int64 range: %s", q.String())
	}
	return q.Int64(), nil
}

// Step records one conversion, including the exact destination count before
// rounding. Delta is quantized-minus-exact in destination units.
type Step struct {
	Name     string
	Input    Quantity
	To       Rate
	Rounding Rounding
	Output   Quantity
	Exact    *big.Rat
	Delta    *big.Rat
}

// ConvertStep performs q -> to and records the exact pre-rounding value.
func ConvertStep(name string, q Quantity, to Rate, mode Rounding) (Step, error) {
	if !q.Rate.Valid() || !to.Valid() {
		return Step{}, errors.New("invalid rate in conversion step")
	}
	exact := q.CountIn(to)
	count, err := Quantize(exact, mode)
	if err != nil {
		return Step{}, fmt.Errorf("%s: %w", name, err)
	}
	delta := new(big.Rat).Sub(new(big.Rat).SetInt64(count), exact)
	return Step{
		Name:     name,
		Input:    q,
		To:       to,
		Rounding: mode,
		Output:   Quantity{Count: count, Rate: to},
		Exact:    exact,
		Delta:    delta,
	}, nil
}

// ErrorSeconds returns the exact difference between the rounded output and the
// unrounded destination value.
func (s Step) ErrorSeconds() *big.Rat {
	sec := s.Output.Seconds()
	exactSec := new(big.Rat).Quo(s.Exact, s.To.Rat())
	return new(big.Rat).Sub(sec, exactSec)
}

func gcd(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
