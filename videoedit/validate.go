package videoedit

import (
	"errors"
	"fmt"
	"time"
)

// Validate checks structural invariants that the applier relies on.
func (p *Project) Validate() error {
	if p == nil {
		return errors.New("project is nil")
	}
	tracks := make(map[string]int, len(p.Tracks))
	clips := make(map[string]struct{})
	for ti, track := range p.Tracks {
		if track.ID == "" {
			return fmt.Errorf("track %d has empty id", ti)
		}
		if prev, ok := tracks[track.ID]; ok {
			return fmt.Errorf("duplicate track id %q (tracks %d and %d)", track.ID, prev, ti)
		}
		tracks[track.ID] = ti
		if track.ParentID != "" {
			if _, ok := tracks[track.ParentID]; !ok {
				return fmt.Errorf("track %q references unknown parent track %q", track.ID, track.ParentID)
			}
		}
		var lastEnd time.Duration
		for ci, clip := range track.Clips {
			if clip.ID == "" {
				return fmt.Errorf("clip %d on track %q has empty id", ci, track.ID)
			}
			if _, ok := clips[clip.ID]; ok {
				return fmt.Errorf("duplicate clip id %q", clip.ID)
			}
			clips[clip.ID] = struct{}{}
			if clip.Length.Duration <= 0 {
				return fmt.Errorf("clip %q has non-positive length", clip.ID)
			}
			if clip.Start.Duration < 0 || clip.SourceIn.Duration < 0 {
				return fmt.Errorf("clip %q has negative start or sourceIn", clip.ID)
			}
			if ci > 0 && clip.Start.Duration < lastEnd {
				return fmt.Errorf("clips overlap on track %q at clip %q", track.ID, clip.ID)
			}
			lastEnd = clip.End()
		}
	}
	return nil
}
