package videoedit

import (
	"fmt"
	"sort"
)

// checkOverlaps validates that merged actions keep every track's clips
// disjoint after deletions. It uses final geometry only, so the answer does
// not depend on the order of the source operations.
func checkOverlaps(project *Project, actions []PlannedAction) error {
	byClip := make(map[string]PlannedAction, len(actions))
	for _, action := range actions {
		byClip[action.ClipID] = action
	}
	for _, track := range project.Tracks {
		type interval struct {
			id    string
			start int64
			end   int64
		}
		intervals := make([]interval, 0, len(track.Clips))
		for _, clip := range track.Clips {
			action, touched := byClip[clip.ID]
			if touched && action.Delete {
				continue
			}
			start := clip.Start.Duration
			end := clip.End()
			if touched {
				start = clip.Start.Duration + action.Move + action.Head
				end = start + clip.Length.Duration - action.Head + action.Tail
			}
			intervals = append(intervals, interval{clip.ID, int64(start), int64(end)})
		}
		sort.Slice(intervals, func(i, j int) bool {
			if intervals[i].start != intervals[j].start {
				return intervals[i].start < intervals[j].start
			}
			return intervals[i].id < intervals[j].id
		})
		for i := 1; i < len(intervals); i++ {
			if intervals[i].start < intervals[i-1].end {
				return fmt.Errorf(
					"clips %q and %q would overlap on track %q after applying the order",
					intervals[i-1].id, intervals[i].id, track.ID)
			}
		}
	}
	return nil
}
