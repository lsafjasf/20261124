package vobsub

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"path/filepath"
)

// CueVerification is the boundary result for one emitted cue.
type CueVerification struct {
	Index        int    `json:"index"`
	AssStartMS   int64  `json:"ass_start_ms"`
	AssEndMS     int64  `json:"ass_end_ms"`
	StartDeltaMS int64  `json:"source_start_delta_ms"`
	EndDeltaMS   int64  `json:"source_end_delta_ms"`
	InImage      string `json:"in_image,omitempty"`
	LastImage    string `json:"last_image,omitempty"`
	OutImage     string `json:"out_image,omitempty"`
	InSHA256     string `json:"in_sha256,omitempty"`
	LastSHA256   string `json:"last_sha256,omitempty"`
	OutSHA256    string `json:"out_sha256,omitempty"`
	PixelMatch   bool   `json:"pixel_match"`
}

// VerificationReport is written to verification.json.
type VerificationReport struct {
	OK             bool              `json:"ok"`
	Rounds         int               `json:"rounds"`
	Checks         int               `json:"checks"`
	ChecksPerRound int               `json:"checks_per_round"`
	Failures       []string          `json:"failures"`
	Cues           []CueVerification `json:"cues"`
}

// VerifyASS reparses subtitles.ass on every round and checks emitted timing,
// placement, colour and boundary composites.
func VerifyASS(outDir string, frame image.Point, cues []*Cue, rounds int) (*VerificationReport, error) {
	if rounds < 1 {
		rounds = 1
	}
	assPath := filepath.Join(outDir, "subtitles.ass")
	cueByIndex := make(map[int]*Cue, len(cues))
	for _, cue := range cues {
		cueByIndex[cue.Index] = cue
	}
	report := &VerificationReport{OK: true, Rounds: rounds, Failures: []string{}}
	var firstHashes map[int]CueVerification

	for round := 1; round <= rounds; round++ {
		ass, err := ParseASS(assPath)
		if err != nil {
			return nil, err
		}
		if ass.Frame != frame {
			report.failf("round %d: ASS frame %dx%d, want %dx%d", round, ass.Frame.X, ass.Frame.Y, frame.X, frame.Y)
		}
		wantEvents := 0
		for _, cue := range cues {
			if cue.Migratable {
				wantEvents++
			}
		}
		if len(ass.Events) != wantEvents {
			report.failf("round %d: %d ASS events, want %d", round, len(ass.Events), wantEvents)
		}

		seen := make(map[int]bool)
		roundChecks := 0
		roundHashes := make(map[int]CueVerification)
		for _, event := range ass.Events {
			roundChecks += 8
			cue, ok := cueByIndex[event.Index]
			if !ok {
				report.failf("round %d cue %04d: event has no source cue", round, event.Index)
				continue
			}
			if seen[event.Index] {
				report.failf("round %d cue %04d: duplicate event", round, event.Index)
				continue
			}
			seen[event.Index] = true
			if !cue.Migratable {
				report.failf("round %d cue %04d: non-migratable cue was emitted", round, event.Index)
				continue
			}
			expectedStart := round10(cue.StartMS)
			expectedEnd := round10(cue.EndMS)
			if expectedEnd <= expectedStart {
				expectedEnd = expectedStart + 10
			}
			if event.StartMS != expectedStart {
				report.failf("round %d cue %04d: ASS in %d ms, want %d ms", round, cue.Index, event.StartMS, expectedStart)
			}
			if event.EndMS != expectedEnd {
				report.failf("round %d cue %04d: ASS out %d ms, want %d ms", round, cue.Index, event.EndMS, expectedEnd)
			}
			if event.EndMS <= event.StartMS {
				report.failf("round %d cue %04d: empty ASS interval", round, cue.Index)
			}
			rect := cue.State.Rect
			if event.X != rect.X || event.Y != rect.Y {
				report.failf("round %d cue %04d: ASS pos (%d,%d), want (%d,%d)", round, cue.Index, event.X, event.Y, rect.X, rect.Y)
			}
			color, hasColor := cue.DominantColour()
			if !hasColor {
				report.failf("round %d cue %04d: no dominant colour", round, cue.Index)
			} else {
				if event.Color != color.RGB {
					report.failf("round %d cue %04d: ASS colour %s, want %s", round, cue.Index, event.Color, color.RGB)
				}
				if event.Alpha != 255-color.Alpha {
					report.failf("round %d cue %04d: ASS alpha %d, want %d", round, cue.Index, event.Alpha, 255-color.Alpha)
				}
			}
			if !eventActive(ass.Events, event.Index, event.StartMS) {
				report.failf("round %d cue %04d: inactive at in point", round, cue.Index)
			}
			if event.EndMS > event.StartMS && !eventActive(ass.Events, event.Index, event.EndMS-1) {
				report.failf("round %d cue %04d: inactive one millisecond before out point", round, cue.Index)
			}
			if eventActive(ass.Events, event.Index, event.EndMS) {
				report.failf("round %d cue %04d: still active at out point", round, cue.Index)
			}

			item := CueVerification{
				Index: event.Index, AssStartMS: event.StartMS, AssEndMS: event.EndMS,
				StartDeltaMS: event.StartMS - cue.StartMS,
				EndDeltaMS:   event.EndMS - cue.EndMS,
				PixelMatch:   true,
			}
			if round == 1 {
				paths, hashes, match, err := writeBoundaryComposites(outDir, ass.Events, cueByIndex, frame, cue.Index, item)
				if err != nil {
					return nil, err
				}
				item.InImage, item.LastImage, item.OutImage = paths[0], paths[1], paths[2]
				item.InSHA256, item.LastSHA256, item.OutSHA256 = hashes[0], hashes[1], hashes[2]
				item.PixelMatch = match
				if !match {
					report.failf("round %d cue %04d: boundary composite differs from cue rectangle", round, cue.Index)
				}
			} else {
				prev := firstHashes[event.Index]
				hashes, err := boundaryHashes(ass.Events, cueByIndex, frame, event.Index, item)
				if err != nil {
					return nil, err
				}
				if hashes[0] != prev.InSHA256 || hashes[1] != prev.LastSHA256 || hashes[2] != prev.OutSHA256 {
					report.failf("round %d cue %04d: composite hash changed", round, cue.Index)
				}
				item.InSHA256, item.LastSHA256, item.OutSHA256 = hashes[0], hashes[1], hashes[2]
			}
			roundHashes[event.Index] = item
			if round == 1 {
				report.Cues = append(report.Cues, item)
			}
		}
		if round == 1 {
			firstHashes = roundHashes
			report.ChecksPerRound = roundChecks
		}
		report.Checks += roundChecks
	}
	if len(report.Failures) > 0 {
		report.OK = false
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return report, writeBytes(filepath.Join(outDir, "verification.json"), append(data, '\n'))
}

func (r *VerificationReport) failf(format string, args ...any) {
	r.Failures = append(r.Failures, fmt.Sprintf(format, args...))
}

func eventActive(events []ASSEvent, index int, t int64) bool {
	count := 0
	for _, event := range events {
		if event.Index == index && event.StartMS <= t && t < event.EndMS {
			count++
		}
	}
	return count == 1
}

func writeBoundaryComposites(outDir string, events []ASSEvent, cues map[int]*Cue, frame image.Point, index int, item CueVerification) ([3]string, [3]string, bool, error) {
	hashes, err := boundaryHashes(events, cues, frame, index, item)
	if err != nil {
		return [3]string{}, [3]string{}, false, err
	}
	times := [3]int64{item.AssStartMS, item.AssEndMS - 1, item.AssEndMS}
	names := [3]string{"in", "last", "out"}
	paths := [3]string{}
	match := true
	for i := 0; i < 3; i++ {
		actual := compositeFromEvents(events, cues, frame, times[i], true)
		expected := compositeFromEvents(events, cues, frame, times[i], false)
		if !equalImages(actual, expected) {
			match = false
		}
		rel := fmt.Sprintf("composite/%04d_%s.png", index, names[i])
		if err := writePNG(filepath.Join(outDir, rel), actual); err != nil {
			return [3]string{}, [3]string{}, false, err
		}
		paths[i] = filepath.ToSlash(rel)
	}
	return paths, hashes, match, nil
}

func boundaryHashes(events []ASSEvent, cues map[int]*Cue, frame image.Point, index int, item CueVerification) ([3]string, error) {
	_ = index
	times := [3]int64{item.AssStartMS, item.AssEndMS - 1, item.AssEndMS}
	var hashes [3]string
	for i := 0; i < 3; i++ {
		img := compositeFromEvents(events, cues, frame, times[i], true)
		hashes[i] = imageSHA(img)
	}
	return hashes, nil
}

func compositeFromEvents(events []ASSEvent, cues map[int]*Cue, frame image.Point, t int64, useEventPos bool) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, frame.X, frame.Y))
	for y := 0; y < frame.Y; y++ {
		for x := 0; x < frame.X; x++ {
			dst.SetNRGBA(x, y, reviewBackground)
		}
	}
	for _, event := range events {
		if !(event.StartMS <= t && t < event.EndMS) {
			continue
		}
		cue := cues[event.Index]
		if cue == nil || cue.Raster == nil {
			continue
		}
		x0, y0 := cue.State.Rect.X, cue.State.Rect.Y
		if useEventPos {
			x0, y0 = event.X, event.Y
		}
		drawCueAt(dst, cue, x0, y0, frame)
	}
	return dst
}

func drawCueAt(dst *image.NRGBA, cue *Cue, x0, y0 int, frame image.Point) {
	for y := 0; y < cue.Raster.Height; y++ {
		fy := y0 + y
		if fy < 0 || fy >= frame.Y {
			continue
		}
		for x := 0; x < cue.Raster.Width; x++ {
			fx := x0 + x
			if fx < 0 || fx >= frame.X {
				continue
			}
			p := cue.Raster.Pixels[y*cue.Raster.Width+x] & 3
			a := int(cue.State.Alpha[p]) * 17
			if a == 0 {
				continue
			}
			c := cue.Palette[int(cue.State.PaletteMap[p])&15]
			dst.SetNRGBA(fx, fy, blend(c, a, dst.NRGBAAt(fx, fy)))
		}
	}
}

func equalImages(a, b *image.NRGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	return bytes.Equal(a.Pix, b.Pix)
}

func imageSHA(img *image.NRGBA) string {
	h := sha256.New()
	b := img.Bounds()
	fmt.Fprintf(h, "%d,%d,%d,%d\n", b.Min.X, b.Min.Y, b.Max.X, b.Max.Y)
	_, _ = h.Write(img.Pix)
	return fmt.Sprintf("%x", h.Sum(nil))
}
