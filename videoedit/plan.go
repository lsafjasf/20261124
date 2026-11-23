package videoedit

import (
	"fmt"
	"sort"
	"time"
)

// mergedOp accumulates every order entry that targets one clip.
type mergedOp struct {
	clipID string
	delete bool
	// head > 0 shortens from the head; tail < 0 shortens from the tail.
	head time.Duration
	tail time.Duration
	move time.Duration
}

// PlannedAction is one order-independent action derived from the order.
type PlannedAction struct {
	ClipID string
	Delete bool
	Head   time.Duration
	Tail   time.Duration
	Move   time.Duration
}

// Plan consolidates a change order into deterministic, order-independent
// actions and validates them against the project. It never mutates the
// project.
func Plan(project *Project, order *ChangeOrder) ([]PlannedAction, error) {
	if err := project.Validate(); err != nil {
		return nil, fmt.Errorf("invalid project: %w", err)
	}
	if err := order.Validate(); err != nil {
		return nil, err
	}

	merged := make(map[string]*mergedOp)
	for _, op := range order.Operations {
		entry, ok := merged[op.ClipID]
		if !ok {
			if _, exists := project.FindClip(op.ClipID); !exists {
				return nil, fmt.Errorf("operation targets unknown clip %q", op.ClipID)
			}
			entry = &mergedOp{clipID: op.ClipID}
			merged[op.ClipID] = entry
		}
		switch op.Action {
		case ActionDelete:
			entry.delete = true
		case ActionTrimStart:
			entry.head += op.Delta.Duration
		case ActionTrimEnd:
			entry.tail += op.Delta.Duration
		case ActionMove:
			entry.move += op.Delta.Duration
		}
	}

	actions := make([]PlannedAction, 0, len(merged))
	for _, entry := range merged {
		action := PlannedAction{
			ClipID: entry.clipID,
			Delete: entry.delete,
			Head:   entry.head,
			Tail:   entry.tail,
			Move:   entry.move,
		}
		if err := validateAction(project, action); err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	if err := checkOverlaps(project, actions); err != nil {
		return nil, err
	}
	sortActions(project, actions)
	return actions, nil
}

func validateAction(project *Project, action PlannedAction) error {
	return validateBaseline(project, action)
}

func validateBaseline(project *Project, action PlannedAction) error {
	ref, ok := project.FindClip(action.ClipID)
	if !ok {
		return fmt.Errorf("unknown clip %q", action.ClipID)
	}
	clip := ref.Clip
	if action.Delete {
		return nil
	}
	newStart := clip.Start.Duration + action.Move + action.Head
	newLength := clip.Length.Duration - action.Head + action.Tail
	newSourceIn := clip.SourceIn.Duration + action.Head
	if newLength <= 0 {
		return fmt.Errorf("clip %q would shrink to non-positive length %s", clip.ID, newLength)
	}
	if newStart < 0 {
		return fmt.Errorf("clip %q would move before timeline origin", clip.ID)
	}
	if newSourceIn < 0 {
		return fmt.Errorf("clip %q trim would reach before source start", clip.ID)
	}
	return nil
}

// sortActions gives a deterministic application order: parent tracks before
// their child tracks, then timeline position, then clip ID.
func sortActions(project *Project, actions []PlannedAction) {
	refOf := make(map[string]ClipRef, len(actions))
	for _, action := range actions {
		ref, _ := project.FindClip(action.ClipID)
		refOf[action.ClipID] = ref
	}
	depth := make(map[string]int, len(project.Tracks))
	var trackDepth func(trackID string) int
	trackDepth = func(trackID string) int {
		if d, ok := depth[trackID]; ok {
			return d
		}
		track := project.Tracks[refOfTrack(project, trackID)]
		if track.ParentID == "" {
			depth[trackID] = 0
			return 0
		}
		d := trackDepth(track.ParentID) + 1
		depth[trackID] = d
		return d
	}
	sort.SliceStable(actions, func(i, j int) bool {
		ri, rj := refOf[actions[i].ClipID], refOf[actions[j].ClipID]
		ti, tj := project.Tracks[ri.TrackIndex], project.Tracks[rj.TrackIndex]
		di, dj := trackDepth(ti.ID), trackDepth(tj.ID)
		if di != dj {
			return di < dj
		}
		if ri.TrackIndex != rj.TrackIndex {
			return ri.TrackIndex < rj.TrackIndex
		}
		if ri.Clip.Start.Duration != rj.Clip.Start.Duration {
			return ri.Clip.Start.Duration < rj.Clip.Start.Duration
		}
		return actions[i].ClipID < actions[j].ClipID
	})
}

func refOfTrack(project *Project, trackID string) int {
	for i := range project.Tracks {
		if project.Tracks[i].ID == trackID {
			return i
		}
	}
	return -1
}
