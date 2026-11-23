package subtime

import (
	"fmt"
	"strconv"
	"strings"
)

// Rate is an exact frame rate. Num/Den is the real frame rate; DropFrame
// selects SMPTE drop-frame numbering and is valid for 30000/1001 only.
type Rate struct {
	Num       int64
	Den       int64
	DropFrame bool
}

var (
	Rate23976  = Rate{Num: 24000, Den: 1001}
	Rate24     = Rate{Num: 24, Den: 1}
	Rate25     = Rate{Num: 25, Den: 1}
	Rate2997   = Rate{Num: 30000, Den: 1001}
	Rate2997DF = Rate{Num: 30000, Den: 1001, DropFrame: true}
	Rate30     = Rate{Num: 30, Den: 1}
)

// NewRate validates and constructs a frame rate.
func NewRate(num, den int64, dropFrame bool) (Rate, error) {
	rate := Rate{Num: num, Den: den, DropFrame: dropFrame}
	if err := rate.Validate(); err != nil {
		return Rate{}, err
	}
	return rate, nil
}

func (r Rate) Validate() error {
	if r.Num <= 0 || r.Den <= 0 {
		return fmt.Errorf("subtime: frame rate must be positive, got %d/%d", r.Num, r.Den)
	}
	if r.DropFrame && (r.Num != 30000 || r.Den != 1001) {
		return fmt.Errorf("subtime: drop-frame is only supported for 30000/1001")
	}
	return nil
}

func (r Rate) FPS() Rational {
	return NewRational(r.Num, r.Den)
}

// NominalFPS is the integer rate used by SMPTE frame labels. For NTSC rates
// this is 24 or 30 rather than the exact 24000/1001 or 30000/1001.
func (r Rate) NominalFPS() int64 {
	quotient := r.Num / r.Den
	remainder := r.Num % r.Den
	if remainder*2 >= r.Den {
		quotient++
	}
	return quotient
}

func (r Rate) String() string {
	switch {
	case r.DropFrame:
		return "29.97DF"
	case r.Num == 24000 && r.Den == 1001:
		return "23.976"
	case r.Num == 30000 && r.Den == 1001:
		return "29.97"
	case r.Den == 1:
		return strconv.FormatInt(r.Num, 10)
	default:
		return fmt.Sprintf("%d/%d", r.Num, r.Den)
	}
}

// ParseRate accepts common rate names and exact rational forms. "29.97" and
// "29.97df" are treated as the exact NTSC rate 30000/1001; only the latter
// enables SMPTE drop-frame numbering.
func ParseRate(text string) (Rate, error) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.TrimSuffix(normalized, "fps")
	normalized = strings.ReplaceAll(normalized, "_", "")

	dropFrame := false
	for _, suffix := range []string{"drop-frame", "dropframe", "ndf", "df"} {
		if strings.HasSuffix(normalized, suffix) {
			dropFrame = suffix != "ndf"
			normalized = strings.TrimSpace(strings.TrimSuffix(normalized, suffix))
			break
		}
	}

	switch normalized {
	case "23.976", "23.98":
		if dropFrame {
			return Rate{}, fmt.Errorf("subtime: drop-frame is not valid for 23.976")
		}
		return Rate23976, nil
	case "29.97":
		if dropFrame {
			return Rate2997DF, nil
		}
		return Rate2997, nil
	}

	if numerator, denominator, ok := strings.Cut(normalized, "/"); ok {
		num, err := strconv.ParseInt(strings.TrimSpace(numerator), 10, 64)
		if err != nil {
			return Rate{}, fmt.Errorf("subtime: invalid rate %q", text)
		}
		den, err := strconv.ParseInt(strings.TrimSpace(denominator), 10, 64)
		if err != nil {
			return Rate{}, fmt.Errorf("subtime: invalid rate %q", text)
		}
		return NewRate(num, den, dropFrame)
	}

	num, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil {
		return Rate{}, fmt.Errorf("subtime: invalid rate %q", text)
	}
	return NewRate(num, 1, dropFrame)
}
