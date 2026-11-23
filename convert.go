package subtime

import (
	"fmt"
	"strings"
)

// SnapPolicy selects an integer frame or millisecond from an exact position.
// SnapNone never rounds and is the zero value, so a missing policy is an error
// rather than a silent floor operation.
type SnapPolicy uint8

const (
	SnapNone SnapPolicy = iota
	SnapFloor
	SnapCeil
	SnapNearest
)

func (p SnapPolicy) String() string {
	switch p {
	case SnapNone:
		return "none"
	case SnapFloor:
		return "floor"
	case SnapCeil:
		return "ceil"
	case SnapNearest:
		return "nearest"
	default:
		return fmt.Sprintf("SnapPolicy(%d)", p)
	}
}

// ParseSnapPolicy parses an explicit snap policy. It intentionally has no
// default; callers must use "floor", "ceil", or "nearest" when snapping.
func ParseSnapPolicy(text string) (SnapPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "floor", "lower":
		return SnapFloor, nil
	case "ceil", "upper":
		return SnapCeil, nil
	case "nearest", "near", "round":
		return SnapNearest, nil
	default:
		return SnapNone, fmt.Errorf("subtime: invalid snap policy %q", text)
	}
}

func snapRational(value Rational, policy SnapPolicy) (int64, error) {
	if policy == SnapNone {
		return 0, fmt.Errorf("subtime: exact value %s straddles frames; choose an explicit snap policy", value)
	}

	var selected int64
	switch policy {
	case SnapFloor:
		number, ok := value.FloorInt64()
		if !ok {
			return 0, fmt.Errorf("subtime: frame position %s is outside int64", value)
		}
		selected = number
	case SnapCeil:
		number, ok := value.CeilInt64()
		if !ok {
			return 0, fmt.Errorf("subtime: frame position %s is outside int64", value)
		}
		selected = number
	case SnapNearest:
		lower, ok := value.FloorInt64()
		if !ok {
			return 0, fmt.Errorf("subtime: frame position %s is outside int64", value)
		}
		upper, ok := value.CeilInt64()
		if !ok {
			return 0, fmt.Errorf("subtime: frame position %s is outside int64", value)
		}
		if lower == upper {
			selected = lower
			break
		}
		below := value.Sub(NewRational(lower, 1))
		above := NewRational(upper, 1).Sub(value)
		if below.Cmp(above) <= 0 {
			selected = lower
		} else {
			selected = upper
		}
	default:
		return 0, fmt.Errorf("subtime: invalid snap policy %d", policy)
	}

	if selected < 0 {
		return 0, fmt.Errorf("subtime: snapped frame is negative")
	}
	return selected, nil
}

// Point is an exact position at a frame rate. Position is measured in frames
// and may be fractional; that fraction is the information needed to make a
// round trip lossless. Seconds is derived from Position/Rate on demand.
type Point struct {
	Rate     Rate     `json:"rate"`
	Position Rational `json:"position"`
}

func PointFromFrame(frame Frame) (Point, error) {
	if err := frame.Validate(); err != nil {
		return Point{}, err
	}
	return Point{Rate: frame.Rate, Position: NewRational(frame.Number, 1)}, nil
}

func PointAtTime(seconds Rational, rate Rate) (Point, error) {
	if err := rate.Validate(); err != nil {
		return Point{}, err
	}
	if seconds.Sign() < 0 {
		return Point{}, fmt.Errorf("subtime: negative presentation time")
	}
	position := seconds.Mul(rate.FPS())
	return Point{Rate: rate, Position: position}, nil
}

func (p Point) Validate() error {
	if err := p.Rate.Validate(); err != nil {
		return err
	}
	if p.Position.Sign() < 0 {
		return fmt.Errorf("subtime: negative frame position")
	}
	return nil
}

// Seconds returns the exact presentation time of the point.
func (p Point) Seconds() (Rational, error) {
	if err := p.Validate(); err != nil {
		return Rational{}, err
	}
	seconds, err := p.Position.Quo(p.Rate.FPS())
	if err != nil {
		return Rational{}, err
	}
	return seconds, nil
}

// At converts the same presentation instant to another frame grid. No
// rounding occurs; inspect Candidates before selecting an integer frame.
func (p Point) At(rate Rate) (Point, error) {
	seconds, err := p.Seconds()
	if err != nil {
		return Point{}, err
	}
	return PointAtTime(seconds, rate)
}

// IsFrame reports whether the point lies exactly on an integer frame.
func (p Point) IsFrame() bool {
	return p.Position.IsInt()
}

// ExactFrame returns the frame only when the point is exactly on a frame.
func (p Point) ExactFrame() (Frame, bool) {
	number, ok := p.Position.Int64()
	if !ok {
		return Frame{}, false
	}
	frame := Frame{Number: number, Rate: p.Rate}
	if err := frame.Validate(); err != nil {
		return Frame{}, false
	}
	return frame, true
}

// Snap selects an integer frame using a policy supplied by the caller.
func (p Point) Snap(policy SnapPolicy) (Frame, error) {
	number, err := snapRational(p.Position, policy)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Number: number, Rate: p.Rate}, nil
}

// SnapCandidate is one valid integer frame on either side of an exact point.
type SnapCandidate struct {
	Frame        int64    `json:"frame"`
	Timecode     string   `json:"timecode"`
	Seconds      Rational `json:"seconds"`
	OffsetFrames Rational `json:"offset_frames"`
	OffsetTime   Rational `json:"offset_time"`
	IsNearest    bool     `json:"is_nearest"`
}

// Candidates returns the explicit floor/ceil snap choices. An on-frame point
// has exactly one candidate with zero offset; a straddling point has two.
func (p Point) Candidates() ([]SnapCandidate, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	lower, ok := p.Position.FloorInt64()
	if !ok {
		return nil, fmt.Errorf("subtime: frame position %s is outside int64", p.Position)
	}
	upper, ok := p.Position.CeilInt64()
	if !ok {
		return nil, fmt.Errorf("subtime: frame position %s is outside int64", p.Position)
	}

	numbers := []int64{lower}
	if upper != lower {
		numbers = append(numbers, upper)
	}

	result := make([]SnapCandidate, 0, len(numbers))
	nearestIndex := 0
	nearestOffset := NewRational(0, 1)
	for i, number := range numbers {
		frame := Frame{Number: number, Rate: p.Rate}
		timecode, err := frame.Timecode()
		if err != nil {
			return nil, err
		}
		seconds, err := frame.Seconds()
		if err != nil {
			return nil, err
		}
		offsetFrames := NewRational(number, 1).Sub(p.Position)
		offsetTime := seconds.Sub(mustSeconds(p))
		candidate := SnapCandidate{
			Frame:        number,
			Timecode:     timecode,
			Seconds:      seconds,
			OffsetFrames: offsetFrames,
			OffsetTime:   offsetTime,
		}
		if i == 0 || absRational(offsetFrames).Cmp(nearestOffset) < 0 {
			nearestOffset = absRational(offsetFrames)
			nearestIndex = i
		}
		result = append(result, candidate)
	}
	result[nearestIndex].IsNearest = true
	return result, nil
}

func mustSeconds(point Point) Rational {
	seconds, err := point.Seconds()
	if err != nil {
		return Rational{}
	}
	return seconds
}

func absRational(value Rational) Rational {
	if value.Sign() < 0 {
		return value.Neg()
	}
	return value
}

// ProjectFrame converts a frame through a chain of rates using exact points.
// Intermediate grids are never rounded.
func ProjectFrame(frame Frame, via ...Rate) (Point, error) {
	point, err := PointFromFrame(frame)
	if err != nil {
		return Point{}, err
	}
	for _, rate := range via {
		point, err = point.At(rate)
		if err != nil {
			return Point{}, err
		}
	}
	return point, nil
}

// RoundTrip verifies one complete exact conversion through via and back. The
// returned frame must equal the input frame number.
func RoundTrip(frame Frame, via ...Rate) (Frame, error) {
	return RoundTripN(frame, 1, via...)
}

// RoundTripN repeats RoundTrip cycles. Exact rational positions are carried
// between all hops, including across whole-hour boundaries.
func RoundTripN(frame Frame, cycles int, via ...Rate) (Frame, error) {
	if cycles < 1 {
		return Frame{}, fmt.Errorf("subtime: cycles must be at least 1")
	}
	current := frame
	for cycle := 0; cycle < cycles; cycle++ {
		point, err := ProjectFrame(current, via...)
		if err != nil {
			return Frame{}, err
		}
		for i := len(via) - 2; i >= 0; i-- {
			point, err = point.At(via[i])
			if err != nil {
				return Frame{}, err
			}
		}
		point, err = point.At(frame.Rate)
		if err != nil {
			return Frame{}, err
		}
		recovered, ok := point.ExactFrame()
		if !ok || recovered.Number != frame.Number {
			return Frame{}, fmt.Errorf(
				"subtime: round trip changed frame %d to %s at %s",
				frame.Number, point.Position, frame.Rate,
			)
		}
		current = recovered
	}
	return current, nil
}
