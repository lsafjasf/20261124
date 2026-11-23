package subtime

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// Rational is an immutable exact rational number. Its zero value is 0.
//
// All timing conversions in this package use Rational; float64 is only
// available through Float64 for presentation and is never used internally.
type Rational struct {
	value *big.Rat
}

// NewRational returns num/den exactly.
func NewRational(num, den int64) Rational {
	if den == 0 {
		panic("subtime: zero denominator")
	}
	return Rational{value: big.NewRat(num, den)}
}

// NewRationalBig returns num/den exactly.
func NewRationalBig(num, den *big.Int) (Rational, error) {
	if num == nil || den == nil {
		return Rational{}, fmt.Errorf("subtime: nil rational component")
	}
	if den.Sign() == 0 {
		return Rational{}, fmt.Errorf("subtime: zero denominator")
	}
	return Rational{value: new(big.Rat).SetFrac(num, den)}, nil
}

// ParseRational parses an integer, decimal, or "num/den" string exactly.
// A decimal such as "23.976" is parsed as 2997/125, not as a float.
func ParseRational(text string) (Rational, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Rational{}, fmt.Errorf("subtime: empty rational")
	}
	rat, ok := new(big.Rat).SetString(text)
	if !ok {
		return Rational{}, fmt.Errorf("subtime: invalid rational %q", text)
	}
	return Rational{value: rat}, nil
}

func (r Rational) get() *big.Rat {
	if r.value == nil {
		return new(big.Rat)
	}
	return r.value
}

// BigRat returns a copy of the underlying value.
func (r Rational) BigRat() *big.Rat {
	return new(big.Rat).Set(r.get())
}

func (r Rational) Num() *big.Int {
	return new(big.Int).Set(r.get().Num())
}

func (r Rational) Den() *big.Int {
	return new(big.Int).Set(r.get().Denom())
}

func (r Rational) String() string {
	return r.get().RatString()
}

func (r Rational) GoString() string {
	return "subtime.Rational(" + r.String() + ")"
}

func (r Rational) Float64() float64 {
	value, _ := r.get().Float64()
	return value
}

func (r Rational) Sign() int {
	return r.get().Sign()
}

func (r Rational) IsZero() bool {
	return r.get().Sign() == 0
}

func (r Rational) IsInt() bool {
	return r.get().IsInt()
}

func (r Rational) Cmp(other Rational) int {
	return r.get().Cmp(other.get())
}

func (r Rational) Equal(other Rational) bool {
	return r.Cmp(other) == 0
}

func (r Rational) Add(other Rational) Rational {
	return Rational{value: new(big.Rat).Add(r.get(), other.get())}
}

func (r Rational) Sub(other Rational) Rational {
	return Rational{value: new(big.Rat).Sub(r.get(), other.get())}
}

func (r Rational) Mul(other Rational) Rational {
	return Rational{value: new(big.Rat).Mul(r.get(), other.get())}
}

func (r Rational) Quo(other Rational) (Rational, error) {
	if other.IsZero() {
		return Rational{}, fmt.Errorf("subtime: division by zero")
	}
	return Rational{value: new(big.Rat).Quo(r.get(), other.get())}, nil
}

func (r Rational) Neg() Rational {
	return Rational{value: new(big.Rat).Neg(r.get())}
}

// Floor returns the greatest integer not greater than r.
func (r Rational) Floor() *big.Int {
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(r.get().Num(), r.get().Denom(), remainder)
	if r.Sign() < 0 && remainder.Sign() != 0 {
		quotient.Sub(quotient, big.NewInt(1))
	}
	return quotient
}

// Ceil returns the least integer not less than r.
func (r Rational) Ceil() *big.Int {
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(r.get().Num(), r.get().Denom(), remainder)
	if r.Sign() > 0 && remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

// Int64 returns the exact integer value of r, if it fits in an int64.
func (r Rational) Int64() (int64, bool) {
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, false
	}
	return r.Num().Int64(), true
}

// FloorInt64 returns floor(r), if the result fits in an int64.
func (r Rational) FloorInt64() (int64, bool) {
	value := r.Floor()
	if !value.IsInt64() {
		return 0, false
	}
	return value.Int64(), true
}

// CeilInt64 returns ceil(r), if the result fits in an int64.
func (r Rational) CeilInt64() (int64, bool) {
	value := r.Ceil()
	if !value.IsInt64() {
		return 0, false
	}
	return value.Int64(), true
}

func (r Rational) MarshalText() ([]byte, error) {
	return []byte(r.String()), nil
}

func (r *Rational) UnmarshalText(text []byte) error {
	value, err := ParseRational(string(text))
	if err != nil {
		return err
	}
	*r = value
	return nil
}

func (r Rational) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *Rational) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("subtime: rational must be a string: %w", err)
	}
	return r.UnmarshalText([]byte(text))
}
