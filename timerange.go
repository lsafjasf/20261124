package otio

import (
	"fmt"
	"math"
	"time"
)

// Seconds returns the time in seconds.
func (t RationalTime) Seconds() (float64, error) {
	if err := t.validate("time"); err != nil {
		return 0, err
	}
	return t.Value / t.Rate, nil
}

// Rescaled converts the time to another frame rate without changing its
// duration in seconds.
func (t RationalTime) Rescaled(rate float64) (RationalTime, error) {
	if err := t.validate("time"); err != nil {
		return RationalTime{}, err
	}
	if !finite(rate) || rate <= 0 {
		return RationalTime{}, fmt.Errorf("target rate must be finite and greater than zero")
	}
	seconds := t.Value / t.Rate
	converted := t
	converted.Rate = rate
	converted.Value = seconds * rate
	return converted, nil
}

// Compare compares two times in seconds. It returns -1, 0, or 1.
func (t RationalTime) Compare(other RationalTime) (int, error) {
	a, err := t.Seconds()
	if err != nil {
		return 0, err
	}
	b, err := other.Seconds()
	if err != nil {
		return 0, err
	}
	switch {
	case a < b:
		return -1, nil
	case a > b:
		return 1, nil
	default:
		return 0, nil
	}
}

// Add adds another time after rescaling it to the receiver's rate.
func (t RationalTime) Add(other RationalTime) (RationalTime, error) {
	converted, err := other.Rescaled(t.Rate)
	if err != nil {
		return RationalTime{}, err
	}
	t.Value += converted.Value
	return t, nil
}

// Sub subtracts another time after rescaling it to the receiver's rate.
func (t RationalTime) Sub(other RationalTime) (RationalTime, error) {
	converted, err := other.Rescaled(t.Rate)
	if err != nil {
		return RationalTime{}, err
	}
	t.Value -= converted.Value
	return t, nil
}

// Validate checks rates, values, and duration.
func (r TimeRange) Validate(path string) error {
	if err := r.Start.validate(path + ".start_time"); err != nil {
		return err
	}
	if err := r.Duration.validate(path + ".duration"); err != nil {
		return err
	}
	if r.Duration.Value < 0 {
		return fmt.Errorf("%s.duration.value must not be negative", path)
	}
	return nil
}

// End returns the exclusive end time in the start time's rate.
func (r TimeRange) End() (RationalTime, error) {
	if err := r.Validate("range"); err != nil {
		return RationalTime{}, err
	}
	return r.Start.Add(r.Duration)
}

// LengthSeconds returns the duration in seconds.
func (r TimeRange) LengthSeconds() (float64, error) {
	return r.Duration.Seconds()
}

// TrimStart changes the source in-point by delta. Positive values trim inward
// from the start; negative values extend the range before its current start.
// The start and duration keep their original rates.
func (r *TimeRange) TrimStart(delta time.Duration) error {
	if r == nil {
		return fmt.Errorf("source range is nil")
	}
	if err := r.Validate("source_range"); err != nil {
		return err
	}
	startSeconds, _ := r.Start.Seconds()
	durationSeconds, _ := r.Duration.Seconds()
	offset := delta.Seconds()
	nextStart := startSeconds + offset
	nextDuration := durationSeconds - offset
	if nextStart < -1e-12 {
		return fmt.Errorf("trim would move source start before zero")
	}
	if nextDuration < -1e-12 {
		return fmt.Errorf("trim would make source duration negative")
	}
	r.Start.Value = math.Max(0, nextStart) * r.Start.Rate
	r.Duration.Value = math.Max(0, nextDuration) * r.Duration.Rate
	return nil
}

// TrimEnd changes the source out-point by delta. Positive values shorten the
// range; negative values extend it.
func (r *TimeRange) TrimEnd(delta time.Duration) error {
	if r == nil {
		return fmt.Errorf("source range is nil")
	}
	if err := r.Validate("source_range"); err != nil {
		return err
	}
	durationSeconds, _ := r.Duration.Seconds()
	nextDuration := durationSeconds - delta.Seconds()
	if nextDuration < -1e-12 {
		return fmt.Errorf("trim would make source duration negative")
	}
	r.Duration.Value = math.Max(0, nextDuration) * r.Duration.Rate
	return nil
}
