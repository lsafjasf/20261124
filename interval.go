package subtime

import "fmt"

const (
	IssueZeroLength        = "zero_length"
	IssueTailUnderOneFrame = "tail_under_one_frame"
	IssueSameFloorFrame    = "same_floor_frame"
	IssueEndBeforeStart    = "end_before_start"
)

// IntervalIssue describes a condition that needs a caller decision. Issues do
// not silently change the exact in/out points.
type IntervalIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Interval is an exact in/out pair. ID is carried through batch mapping and
// is not used as a deduplication key.
type Interval struct {
	ID    string `json:"id,omitempty"`
	Start Point  `json:"start"`
	End   Point  `json:"end"`
}

// IntervalMapping exposes exact target points and explicit snap candidates.
// Start and End are never rounded by MapInterval.
type IntervalMapping struct {
	Source          Interval        `json:"source"`
	TargetRate      Rate            `json:"target_rate"`
	Start           Point           `json:"start"`
	End             Point           `json:"end"`
	StartCandidates []SnapCandidate `json:"start_candidates"`
	EndCandidates   []SnapCandidate `json:"end_candidates"`
	Issues          []IntervalIssue `json:"issues,omitempty"`
}

// ProjectInterval carries exact in/out points through intermediate rates
// without snapping them. Snap only once at the final target.
func ProjectInterval(interval Interval, rates ...Rate) (Interval, error) {
	if err := interval.Start.Validate(); err != nil {
		return Interval{}, fmt.Errorf("subtime: invalid in point: %w", err)
	}
	if err := interval.End.Validate(); err != nil {
		return Interval{}, fmt.Errorf("subtime: invalid out point: %w", err)
	}
	projected := Interval{ID: interval.ID, Start: interval.Start, End: interval.End}
	for _, rate := range rates {
		start, err := projected.Start.At(rate)
		if err != nil {
			return Interval{}, err
		}
		end, err := projected.End.At(rate)
		if err != nil {
			return Interval{}, err
		}
		projected.Start, projected.End = start, end
	}
	return projected, nil
}

// MapInterval maps an in/out pair onto a target frame grid with exact rational
// arithmetic. A tail shorter than one target frame is reported, not padded or
// collapsed.
func MapInterval(interval Interval, target Rate) (IntervalMapping, error) {
	if err := target.Validate(); err != nil {
		return IntervalMapping{}, err
	}
	if err := interval.Start.Validate(); err != nil {
		return IntervalMapping{}, fmt.Errorf("subtime: invalid in point: %w", err)
	}
	if err := interval.End.Validate(); err != nil {
		return IntervalMapping{}, fmt.Errorf("subtime: invalid out point: %w", err)
	}

	start, err := interval.Start.At(target)
	if err != nil {
		return IntervalMapping{}, err
	}
	end, err := interval.End.At(target)
	if err != nil {
		return IntervalMapping{}, err
	}
	startCandidates, err := start.Candidates()
	if err != nil {
		return IntervalMapping{}, err
	}
	endCandidates, err := end.Candidates()
	if err != nil {
		return IntervalMapping{}, err
	}

	mapping := IntervalMapping{
		Source:          interval,
		TargetRate:      target,
		Start:           start,
		End:             end,
		StartCandidates: startCandidates,
		EndCandidates:   endCandidates,
	}

	span := end.Position.Sub(start.Position)
	switch {
	case span.Sign() < 0:
		mapping.Issues = append(mapping.Issues, IntervalIssue{
			Code:    IssueEndBeforeStart,
			Message: "out point is earlier than in point",
		})
	case span.Sign() == 0:
		mapping.Issues = append(mapping.Issues, IntervalIssue{
			Code:    IssueZeroLength,
			Message: "in and out points are identical",
		})
	case span.Cmp(NewRational(1, 1)) < 0:
		mapping.Issues = append(mapping.Issues, IntervalIssue{
			Code:    IssueTailUnderOneFrame,
			Message: "exact target duration is shorter than one frame",
		})
	}

	startFloor, _ := start.Position.FloorInt64()
	endFloor, _ := end.Position.FloorInt64()
	if span.Sign() > 0 && endFloor <= startFloor {
		mapping.Issues = append(mapping.Issues, IntervalIssue{
			Code:    IssueSameFloorFrame,
			Message: "both points fall inside the same floor frame",
		})
	}
	return mapping, nil
}

// MapIntervals maps every input independently. Repeated IDs and repeated
// timestamps are preserved in input order.
func MapIntervals(intervals []Interval, target Rate) ([]IntervalMapping, error) {
	mappings := make([]IntervalMapping, len(intervals))
	for i, interval := range intervals {
		mapping, err := MapInterval(interval, target)
		if err != nil {
			return nil, fmt.Errorf("subtime: interval %d (%q): %w", i, interval.ID, err)
		}
		mappings[i] = mapping
	}
	return mappings, nil
}

// IntervalPolicy makes in/out snapping explicit. EnsureOneFrame only acts
// when the selected integer end is not after the selected integer start; the
// adjustment is reported in ResolvedInterval.
type IntervalPolicy struct {
	Start          SnapPolicy `json:"start"`
	End            SnapPolicy `json:"end"`
	EnsureOneFrame bool       `json:"ensure_one_frame"`
}

type ResolvedInterval struct {
	ID          string          `json:"id,omitempty"`
	Start       Frame           `json:"start"`
	End         Frame           `json:"end"`
	ExactStart  Point           `json:"exact_start"`
	ExactEnd    Point           `json:"exact_end"`
	AdjustedEnd bool            `json:"adjusted_end"`
	Issues      []IntervalIssue `json:"issues,omitempty"`
}

// Resolve applies explicit snap policies. If both selected frames collapse,
// Resolve returns an error unless EnsureOneFrame was explicitly requested.
func (m IntervalMapping) Resolve(policy IntervalPolicy) (ResolvedInterval, error) {
	if policy.Start == SnapNone || policy.End == SnapNone {
		return ResolvedInterval{}, fmt.Errorf(
			"subtime: explicit start and end snap policies are required",
		)
	}

	start, err := m.Start.Snap(policy.Start)
	if err != nil {
		return ResolvedInterval{}, fmt.Errorf("subtime: snapping in point: %w", err)
	}
	end, err := m.End.Snap(policy.End)
	if err != nil {
		return ResolvedInterval{}, fmt.Errorf("subtime: snapping out point: %w", err)
	}

	result := ResolvedInterval{
		ID:         m.Source.ID,
		Start:      start,
		End:        end,
		ExactStart: m.Start,
		ExactEnd:   m.End,
		Issues:     append([]IntervalIssue(nil), m.Issues...),
	}
	if end.Number <= start.Number {
		if !policy.EnsureOneFrame {
			return ResolvedInterval{}, fmt.Errorf(
				"subtime: in/out collapse to frame %d at %s; choose other candidates or enable EnsureOneFrame",
				start.Number, m.TargetRate,
			)
		}
		result.End = Frame{Number: start.Number + 1, Rate: m.TargetRate}
		result.AdjustedEnd = true
		result.Issues = append(result.Issues, IntervalIssue{
			Code:    "minimum_one_frame_applied",
			Message: "out frame was advanced to keep at least one target frame",
		})
	}
	return result, nil
}
