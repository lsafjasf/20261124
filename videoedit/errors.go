package videoedit

import "fmt"

// AbortError marks an application that was rolled back because the resolver
// chose abort or an invalid choice.
type AbortError struct {
	Reason string
}

func (e *AbortError) Error() string {
	return "change order application aborted: " + e.Reason
}

// ConflictError wraps a failure whose project effect was fully rolled back.
type ConflictError struct {
	Err error
}

func (e *ConflictError) Error() string { return e.Err.Error() }
func (e *ConflictError) Unwrap() error { return e.Err }

func errTrackMissing(trackID string) error {
	return fmt.Errorf("rollback failed: track %q missing", trackID)
}

func errJournalInconsistent(entry JournalEntry, reason string) error {
	return fmt.Errorf("rollback failed: journal entry for clip %q inconsistent: %s", entry.ClipID, reason)
}
