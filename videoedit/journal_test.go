package videoedit

import "testing"

func TestParseJournalErrors(t *testing.T) {
	if _, err := ParseJournal([]byte("{")); err == nil {
		t.Fatal("expected journal parse error")
	}
}

func TestRollbackErrorPaths(t *testing.T) {
	t.Run("missing track", func(t *testing.T) {
		journal := &Journal{Entries: []JournalEntry{
			{Kind: EntryUpdateClip, TrackID: "ghost", ClipID: "c1", Before: &Clip{ID: "c1"}},
		}}
		if err := journal.Rollback(sampleProject()); err == nil {
			t.Fatal("expected missing track error")
		}
	})

	t.Run("update without before", func(t *testing.T) {
		journal := &Journal{Entries: []JournalEntry{
			{Kind: EntryUpdateClip, TrackID: "video", ClipID: "c1"},
		}}
		if err := journal.Rollback(sampleProject()); err == nil {
			t.Fatal("expected inconsistent journal error")
		}
	})

	t.Run("unknown entry kind", func(t *testing.T) {
		journal := &Journal{Entries: []JournalEntry{
			{Kind: "mystery", TrackID: "video", ClipID: "c1"},
		}}
		if err := journal.Rollback(sampleProject()); err == nil {
			t.Fatal("expected unknown kind error")
		}
	})

	t.Run("delete reinsert restores order", func(t *testing.T) {
		project := sampleProject()
		journal := &Journal{Entries: []JournalEntry{
			{
				Kind:      EntryDeleteClip,
				TrackID:   "video",
				ClipID:    "c1",
				ClipIndex: 0,
				Before:    &project.Tracks[0].Clips[0],
			},
		}}
		// Simulate the post-delete state manually.
		project.Tracks[0].Clips = project.Tracks[0].Clips[1:]
		if err := journal.Rollback(project); err != nil {
			t.Fatalf("rollback failed: %v", err)
		}
		if _, ok := project.FindClip("c1"); !ok {
			t.Fatal("c1 should have been reinserted")
		}
	})
}

func TestConflictErrorWrapping(t *testing.T) {
	inner := errTrackMissing("t1")
	wrapped := &ConflictError{Err: inner}
	if wrapped.Error() == "" || wrapped.Unwrap() != inner {
		t.Fatal("conflict error wrapping is broken")
	}
	abort := &AbortError{Reason: "x"}
	if abort.Error() == "" {
		t.Fatal("abort error message empty")
	}
}
