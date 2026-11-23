package videoedit

import (
	"sort"
	"time"
)

// ConflictKind explains why parent and child track timings disagree.
type ConflictKind string

const (
	// ConflictOverflow means a child clip would land outside its parent clip.
	ConflictOverflow ConflictKind = "overflow"
	// ConflictParentDelete means a parent clip is deleted while child clips stay.
	ConflictParentDelete ConflictKind = "parentDelete"
)

// Choice is a user-selectable resolution for a conflict.
type Choice string

const (
	ChoiceAbort          Choice = "abort"
	ChoiceCascade        Choice = "cascade"
	ChoiceDetach         Choice = "detach"
	ChoiceDeleteChildren Choice = "deleteChildren"
	ChoiceSkipParent     Choice = "skipParent"
)

// Conflict describes one parent/child timing disagreement and the choices
// available to resolve it.
type Conflict struct {
	Kind          ConflictKind
	ParentTrackID string
	ParentClipID  string
	ChildTrackID  string
	ChildClipIDs  []string
	Message       string
	Choices       []Choice
}

// geometry is the timeline shape a clip has after the order is applied.
type geometry struct {
	clipID  string
	start   time.Duration
	end     time.Duration
	deleted bool
}

// finalGeometry computes the post-order geometry of every affected clip
// straight from the baseline project and the merged actions, without
// mutating anything. Order independence therefore follows from Plan.
func finalGeometry(project *Project, actions []PlannedAction) map[string]geometry {
	out := make(map[string]geometry, len(actions))
	for _, action := range actions {
		ref, ok := project.FindClip(action.ClipID)
		if !ok {
			continue
		}
		clip := ref.Clip
		if action.Delete {
			out[clip.ID] = geometry{clipID: clip.ID, deleted: true}
			continue
		}
		start := clip.Start.Duration + action.Move + action.Head
		length := clip.Length.Duration - action.Head + action.Tail
		out[clip.ID] = geometry{clipID: clip.ID, start: start, end: start + length}
	}
	return out
}

// detectConflicts precomputes every parent/child disagreement that would
// appear once the planned actions are applied. The result is deterministic.
func detectConflicts(project *Project, actions []PlannedAction) []Conflict {
	geom := finalGeometry(project, actions)
	actionByClip := make(map[string]PlannedAction, len(actions))
	for _, action := range actions {
		actionByClip[action.ClipID] = action
	}

	var conflicts []Conflict
	for _, track := range project.Tracks {
		if track.ParentID == "" {
			continue
		}
		parentTrack := project.Tracks[refOfTrack(project, track.ParentID)]
		for _, child := range track.Clips {
			parent := parentClipAt(project, parentTrack, child)
			if parent.ID == "" {
				continue
			}
			pg, parentAffected := geom[parent.ID]
			if parentAffected && pg.deleted {
				conflicts = append(conflicts, Conflict{
					Kind:          ConflictParentDelete,
					ParentTrackID: parentTrack.ID,
					ParentClipID:  parent.ID,
					ChildTrackID:  track.ID,
					ChildClipIDs:  []string{child.ID},
					Message:       "parent clip " + parent.ID + " is deleted but child clip " + child.ID + " remains",
					Choices:       []Choice{ChoiceAbort, ChoiceDeleteChildren, ChoiceDetach, ChoiceSkipParent},
				})
				continue
			}
			cg, childAffected := geom[child.ID]
			parentStart, parentEnd := parent.Start.Duration, parent.End()
			if parentAffected {
				parentStart, parentEnd = pg.start, pg.end
			}
			childStart, childEnd := child.Start.Duration, child.End()
			if childAffected {
				childStart, childEnd = cg.start, cg.end
			}
			if !parentAffected && !childAffected {
				continue
			}
			if childStart < parentStart || childEnd > parentEnd {
				conflicts = append(conflicts, Conflict{
					Kind:          ConflictOverflow,
					ParentTrackID: parentTrack.ID,
					ParentClipID:  parent.ID,
					ChildTrackID:  track.ID,
					ChildClipIDs:  []string{child.ID},
					Message:       "child clip " + child.ID + " would leave parent clip " + parent.ID,
					Choices:       []Choice{ChoiceAbort, ChoiceCascade, ChoiceDetach, ChoiceDeleteChildren},
				})
			}
		}
	}

	sort.SliceStable(conflicts, func(i, j int) bool {
		if conflicts[i].ParentClipID != conflicts[j].ParentClipID {
			return conflicts[i].ParentClipID < conflicts[j].ParentClipID
		}
		return conflicts[i].ChildClipIDs[0] < conflicts[j].ChildClipIDs[0]
	})
	return conflicts
}

// parentClipAt returns the clip on the parent track whose interval contains
// the child clip on the baseline project.
func parentClipAt(project *Project, parentTrack Track, child Clip) Clip {
	for _, parent := range parentTrack.Clips {
		if child.Start.Duration >= parent.Start.Duration && child.End() <= parent.End() {
			return parent
		}
	}
	return Clip{}
}
