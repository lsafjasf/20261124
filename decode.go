package vobsub

import (
	"fmt"
	"image"
	"strings"
)

// DecodeCues extracts and classifies every timestamp entry.
func DecodeCues(idx *Index, sub *SubFile) []*Cue {
	cues := make([]*Cue, 0, len(idx.Timestamps))
	for i, ts := range idx.Timestamps {
		cue := &Cue{
			Index:   i + 1,
			StartMS: ts.StartMS,
			EndMS:   ts.StartMS,
			FilePos: ts.FilePos,
			Palette: idx.Palette,
		}
		cue.AddFlag(FlagTextNotRecognized)

		raw, err := sub.SPUAt(ts.FilePos)
		if err != nil {
			cue.AddFlag(FlagDecodeError)
			cue.AddFlag(FlagNoOriginalImage)
			cue.Notes = append(cue.Notes, err.Error())
			cues = append(cues, cue)
			continue
		}
		spu, err := DecodeSPU(raw)
		cue.SPU = spu
		if err != nil {
			classifyDecodeError(cue, err)
			cue.Notes = append(cue.Notes, err.Error())
		}
		if spu == nil {
			cue.AddFlag(FlagNoOriginalImage)
			cues = append(cues, cue)
			continue
		}
		if spu.Forced {
			cue.AddFlag(FlagForced)
		}
		if spu.Animation {
			cue.AddFlag(FlagAnimation)
		}
		switch {
		case !spu.HasStart:
			cue.AddFlag(FlagMissingStart)
			cue.Notes = append(cue.Notes, "SPU has no start-display command")
		case !spu.HasStop:
			cue.AddFlag(FlagMissingStop)
			cue.Notes = append(cue.Notes, "SPU has no stop-display command; end time is not guessed")
		default:
			cue.StartMS = ts.StartMS + ticksToMS(spu.StartTick)
			cue.EndMS = ts.StartMS + ticksToMS(spu.EndTick)
			if cue.EndMS <= cue.StartMS {
				cue.AddFlag(FlagBadTiming)
				cue.Notes = append(cue.Notes, fmt.Sprintf("non-positive interval %d..%d ms", cue.StartMS, cue.EndMS))
			} else if cue.EndMS-cue.StartMS < 10 {
				cue.AddFlag(FlagTimingPrecision)
				cue.Notes = append(cue.Notes, fmt.Sprintf("interval %d ms is below ASS centisecond precision", cue.EndMS-cue.StartMS))
			}
		}

		state := spu.MainState
		if state.Rect.Width == 0 {
			for _, snap := range spu.Snapshots {
				if snap.Active && snap.State.Rect.Width > 0 {
					state = snap.State
					break
				}
			}
		}
		if state.Rect.Width <= 0 || state.Rect.Height <= 0 {
			cue.AddFlag(FlagDecodeError)
			cue.AddFlag(FlagNoOriginalImage)
			cue.Notes = append(cue.Notes, "SPU has no display area")
			cues = append(cues, cue)
			continue
		}
		if !rectWithinFrame(state.Rect, idx.Frame.X, idx.Frame.Y) {
			cue.AddFlag(FlagGeometry)
			cue.Notes = append(cue.Notes, fmt.Sprintf("rect %+v outside %dx%d frame", state.Rect, idx.Frame.X, idx.Frame.Y))
		}
		if hasZeroAlpha(state.Alpha) {
			cue.AddFlag(FlagPalette)
			cue.Notes = append(cue.Notes, "SPU alpha map makes every pixel transparent")
		}

		raster, rasterErr := DecodeRaster(spu.Raw, state)
		cue.State = state
		cue.Palette = idx.Palette
		cue.Raster = raster
		if rasterErr != nil {
			cue.AddFlag(FlagDecodeError)
			cue.AddFlag(FlagNoOriginalImage)
			cue.Notes = append(cue.Notes, rasterErr.Error())
		} else {
			cue.Colors = visibleColours(raster, state, idx.Palette)
			if len(cue.Colors) == 0 {
				cue.AddFlag(FlagPalette)
				cue.AddFlag(FlagNoOriginalImage)
				cue.Notes = append(cue.Notes, "decoded bitmap has no visible pixels")
			} else if len(cue.Colors) > 1 {
				cue.AddFlag(FlagOutline)
			}
		}
		if cue.HasFlag(FlagAnimation) {
			cue.StateImages = decodeStateImages(spu, idx.Palette, idx.Frame)
			for _, state := range cue.StateImages {
				if state.Raster != nil && len(visibleColours(state.Raster, state.State, idx.Palette)) > 1 {
					cue.AddFlag(FlagOutline)
					break
				}
			}
		}
		cues = append(cues, cue)
	}

	for _, cue := range cues {
		cue.Migratable = cue.Raster != nil && len(cue.Colors) > 0 &&
			!cue.HasFlag(FlagAnimation) &&
			!cue.HasFlag(FlagMissingStart) &&
			!cue.HasFlag(FlagMissingStop) &&
			!cue.HasFlag(FlagBadTiming) &&
			!cue.HasFlag(FlagTimingPrecision) &&
			!cue.HasFlag(FlagDecodeError) &&
			!cue.HasFlag(FlagUnsupported) &&
			!cue.HasFlag(FlagGeometry) &&
			!cue.HasFlag(FlagPalette) &&
			!cue.HasFlag(FlagTruncated)
	}
	return cues
}

func decodeStateImages(spu *SPU, pal [16]RGB, frame image.Point) []StateImage {
	var out []StateImage
	seen := make(map[State]bool)
	for _, snap := range spu.Snapshots {
		if !snap.Active {
			continue
		}
		if seen[snap.State] {
			continue
		}
		seen[snap.State] = true
		item := StateImage{
			Label:  fmt.Sprintf("state_%02d", len(out)+1),
			TickMS: snap.TickMS,
			Active: snap.Active,
			State:  snap.State,
		}
		if !rectWithinFrame(snap.State.Rect, frame.X, frame.Y) {
			item.Error = "state display area is outside the frame"
			out = append(out, item)
			continue
		}
		raster, err := DecodeRaster(spu.Raw, snap.State)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Raster = raster
			if len(visibleColours(raster, snap.State, pal)) == 0 {
				item.Error = "state has no visible pixels"
			}
		}
		out = append(out, item)
	}
	return out
}

func classifyDecodeError(cue *Cue, err error) {
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "chg_colcon"),
		strings.Contains(text, "unsupported control"),
		strings.Contains(text, "not migrated"):
		cue.AddFlag(FlagUnsupported)
	case strings.Contains(text, "truncated") || strings.Contains(text, "declares") || strings.Contains(text, "exceeds"):
		cue.AddFlag(FlagTruncated)
	default:
		cue.AddFlag(FlagDecodeError)
	}
}

func rectWithinFrame(r Rect, w, h int) bool {
	return r.X >= 0 && r.Y >= 0 && r.Width > 0 && r.Height > 0 &&
		r.X+r.Width <= w && r.Y+r.Height <= h
}

func hasZeroAlpha(a [4]uint8) bool {
	return a[0] == 0 && a[1] == 0 && a[2] == 0 && a[3] == 0
}

func visibleColours(r *Raster, st State, pal [16]RGB) []VisibleColour {
	var counts [4]int
	for _, p := range r.Pixels {
		if p < 4 {
			counts[p]++
		}
	}
	out := make([]VisibleColour, 0, 4)
	for p := 0; p < 4; p++ {
		if !r.Used[p] || st.Alpha[p] == 0 {
			continue
		}
		c := pal[int(st.PaletteMap[p])&15]
		out = append(out, VisibleColour{
			Index:  p,
			RGB:    c.Hex(),
			Alpha:  int(st.Alpha[p]) * 17,
			Pixels: counts[p],
			Area:   counts[p],
		})
	}
	if len(out) == 0 {
		return nil
	}
	best := 0
	for i := 1; i < len(out); i++ {
		if out[i].Area > out[best].Area {
			best = i
		}
	}
	out[best].Dominant = true
	return out
}

// DominantColour returns the largest opaque/translucent colour area.
func (c *Cue) DominantColour() (VisibleColour, bool) {
	for _, color := range c.Colors {
		if color.Dominant {
			return color, true
		}
	}
	return VisibleColour{}, false
}
