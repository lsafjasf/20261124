package tagfix

import "errors"

var (
	// ErrInvalidTags reports a reserved code point or a contradictory color
	// tag combination.
	ErrInvalidTags = errors.New("invalid color tag combination")

	// ErrUnsupportedMedia means the file has no video track that can be
	// synchronized safely, or uses a structure this package does not edit.
	ErrUnsupportedMedia = errors.New("unsupported media structure")

	// ErrCannotAutoFix means the requested fields could not be represented in
	// both the container and the H.264 bitstream without damaging the payload.
	ErrCannotAutoFix = errors.New("conflict cannot be fixed automatically")

	// ErrReadbackMismatch means bytes were produced but parsing them back did
	// not yield the requested tags.
	ErrReadbackMismatch = errors.New("readback verification failed")

	// ErrWriteFailed wraps failures while creating, syncing, or replacing the
	// output file.
	ErrWriteFailed = errors.New("write failed")

	// ErrRollbackFailed means the original file could not be restored after a
	// post-write verification failure.
	ErrRollbackFailed = errors.New("rollback failed")
)
