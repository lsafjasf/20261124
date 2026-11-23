package videoedit

import (
	"fmt"
	"sort"
)

func (a *applier) orderedActions(planned []PlannedAction) []PlannedAction {
	plannedID := map[string]bool{}
	for _, action := range planned {
		plannedID[action.ClipID] = true
	}
	var extra []PlannedAction
	for clipID, action := range a.actions {
		if !plannedID[clipID] {
			extra = append(extra, action)
		}
	}
	sortActions(a.baseline, extra)
	merged := make([]PlannedAction, 0, len(a.actions))
	for _, action := range planned {
		merged = append(merged, a.actions[action.ClipID])
	}
	return append(merged, extra...)
}

func (a *applier) writeOne(action PlannedAction) error {
	ref, ok := a.project.FindClip(action.ClipID)
	if !ok {
		return fmt.Errorf("clip %q disappeared during application", action.ClipID)
	}
	track := &a.project.Tracks[ref.TrackIndex]
	sourceOp := a.sourceOpOf[action.ClipID]

	if action.Delete {
		before := track.Clips[ref.ClipIndex]
		track.Clips = removeClip(track.Clips, ref.ClipIndex)
		a.journal.append(JournalEntry{
			Kind:      EntryDeleteClip,
			TrackID:   track.ID,
			ClipID:    action.ClipID,
			ClipIndex: ref.ClipIndex,
			Before:    &before,
			SourceOp:  sourceOp,
		})
		return nil
	}

	clip := track.Clips[ref.ClipIndex]
	before := clip
	newStart := clip.Start.Duration + action.Move + action.Head
	newLength := clip.Length.Duration - action.Head + action.Tail
	newSourceIn := clip.SourceIn.Duration + action.Head
	if newLength <= 0 || newStart < 0 || newSourceIn < 0 {
		return fmt.Errorf("clip %q violates timeline invariants after apply", action.ClipID)
	}
	clip.Start = Duration{Duration: newStart}
	clip.Length = Duration{Duration: newLength}
	clip.SourceIn = Duration{Duration: newSourceIn}
	if a.detached[action.ClipID] {
		clip.Detached = true
	}
	track.Clips[ref.ClipIndex] = clip
	a.journal.append(JournalEntry{
		Kind:      EntryUpdateClip,
		TrackID:   track.ID,
		ClipID:    action.ClipID,
		ClipIndex: ref.ClipIndex,
		Before:    &before,
		After:     &clip,
		SourceOp:  sourceOp,
	})
	return nil
}

func (a *applier) rollbackOrPanic(cause error) {
	if err := a.journal.Rollback(a.project); err != nil {
		panic(fmt.Sprintf("undo log replay failed after %v: %v", cause, err))
	}
	if !a.project.Equal(a.baseline) {
		panic("project did not return to baseline after rollback")
	}
}

func (a *applier) buildResult() *Result {
	applied := make([]PlannedAction, 0, len(a.actions))
	for _, action := range a.actions {
		if _, skip := a.skipped[action.ClipID]; skip {
			continue
		}
		applied = append(applied, action)
	}
	skipIDs := make([]string, 0, len(a.skipped))
	for clipID := range a.skipped {
		skipIDs = append(skipIDs, clipID)
	}
	sort.Strings(skipIDs)
	skips := make([]Skip, 0, len(skipIDs))
	for _, clipID := range skipIDs {
		skips = append(skips, Skip{ClipID: clipID, Reason: a.skipped[clipID]})
	}
	return &Result{
		Applied:   applied,
		Skipped:   skips,
		Conflicts: a.conflicts,
		Journal:   a.journal,
	}
}
