package inspect

import "vxcheck/internal/vx"

// FrameAtTick returns the frame whose PTS equals tick, or -1 when that
// timeline position is absent (for example a dropped frame).
func FrameAtTick(f *vx.File, tick int64) int {
	for i, fr := range f.Frames {
		if fr.PTS == tick {
			return i
		}
		if fr.PTS > tick {
			break
		}
	}
	return -1
}

// FrameAtOrBefore returns the last frame with PTS <= tick.
func FrameAtOrBefore(f *vx.File, tick int64) int {
	best := -1
	for i, fr := range f.Frames {
		if fr.PTS <= tick {
			best = i
		} else {
			break
		}
	}
	return best
}
