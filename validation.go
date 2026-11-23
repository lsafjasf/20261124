package main

import (
	"fmt"
	"sort"
)

func detectOverlaps(result *Result) {
	order := make([]int, 0, len(result.Cues))
	for index, cue := range result.Cues {
		if cue.EndMS > cue.StartMS {
			order = append(order, index)
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		left := result.Cues[order[i]]
		right := result.Cues[order[j]]
		if left.StartMS != right.StartMS {
			return left.StartMS < right.StartMS
		}
		return left.EndMS < right.EndMS
	})

	active := make([]int, 0)
	for _, index := range order {
		cue := &result.Cues[index]
		kept := active[:0]
		for _, otherIndex := range active {
			if result.Cues[otherIndex].EndMS > cue.StartMS {
				kept = append(kept, otherIndex)
			}
		}
		active = kept

		for _, otherIndex := range active {
			other := &result.Cues[otherIndex]
			issue := Issue{
				Code:       "overlap",
				Message:    fmt.Sprintf("cue starts at %d ms before cue %d ends at %d ms", cue.StartMS, other.Number, other.EndMS),
				Line:       cue.Line,
				Cue:        cue.Number,
				RelatedCue: other.Number,
			}
			cue.Issues = append(cue.Issues, issue)
			result.Issues = append(result.Issues, issue)
		}
		active = append(active, index)
	}

	sort.SliceStable(result.Issues, func(i, j int) bool {
		if result.Issues[i].Line != result.Issues[j].Line {
			return result.Issues[i].Line < result.Issues[j].Line
		}
		if result.Issues[i].Cue != result.Issues[j].Cue {
			return result.Issues[i].Cue < result.Issues[j].Cue
		}
		return result.Issues[i].Code < result.Issues[j].Code
	})
}
