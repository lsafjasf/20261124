package timemap

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by Build and the lookup helpers.
var (
	// ErrNonMonotonic is returned for keyframes whose output order is
	// invalid or whose source trajectory contains an illegal loop.
	ErrNonMonotonic = errors.New("timemap: non-monotonic trajectory")
	// ErrOutOfBounds is returned when a mapped source time falls outside
	// the source clip, including a row-level violation.
	ErrOutOfBounds = errors.New("timemap: source time out of bounds")
	// ErrSubFrameLoop is returned when a direction change turns back for
	// less than one source frame, which would re-read the same source
	// frames without a meaningful position change.
	ErrSubFrameLoop = errors.New("timemap: sub-frame loop rejected")
	// ErrTickOverflow is returned when exact integer tick arithmetic
	// cannot represent the timeline inside int64.
	ErrTickOverflow = errors.New("timemap: tick domain overflow")
	// ErrFrameNotFound is returned by Table.SourceFrame when the requested
	// output frame is not part of the table.
	ErrFrameNotFound = errors.New("timemap: output frame not found")
)

// FrameError carries the output frame index together with the rule that
// failed, so callers can locate a rejected row precisely.
type FrameError struct {
	Frame int64
	Err   error
}

func (e *FrameError) Error() string {
	return fmt.Sprintf("%s at output frame %d", e.Err, e.Frame)
}

func (e *FrameError) Unwrap() error { return e.Err }

func frameError(frame int64, err error) error {
	return &FrameError{Frame: frame, Err: err}
}
