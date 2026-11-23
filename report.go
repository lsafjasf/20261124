package vobsub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SourceInfo identifies the exact VobSub inputs.
type SourceInfo struct {
	IndexPath string `json:"index_path"`
	SubPath   string `json:"sub_path"`
	IndexSHA  string `json:"index_sha256"`
	SubSHA    string `json:"sub_sha256"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// ASSRef records what was emitted for one cue.
type ASSRef struct {
	StartMS      int64  `json:"start_ms"`
	EndMS        int64  `json:"end_ms"`
	StartDeltaMS int64  `json:"start_delta_ms"`
	EndDeltaMS   int64  `json:"end_delta_ms"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Color        string `json:"color"`
	Alpha        int    `json:"ass_alpha"`
}

// ImageRef is a review image path.
type ImageRef struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}

// CueReport is the machine-readable review record for one timestamp entry.
type CueReport struct {
	Index        int              `json:"index"`
	StartMS      int64            `json:"start_ms"`
	EndMS        int64            `json:"end_ms"`
	Start        string           `json:"start"`
	End          string           `json:"end"`
	FilePos      string           `json:"filepos"`
	Rect         Rect             `json:"rect"`
	Flags        []string         `json:"flags"`
	Notes        []string         `json:"notes,omitempty"`
	Migratable   bool             `json:"migratable"`
	Colors       []VisibleColour  `json:"colors,omitempty"`
	Original     string           `json:"original_image,omitempty"`
	Frame        string           `json:"original_frame,omitempty"`
	RawSPU       string           `json:"raw_spu,omitempty"`
	States       []ImageRef       `json:"state_images,omitempty"`
	ASS          *ASSRef          `json:"ass_event,omitempty"`
	Verification *CueVerification `json:"verification,omitempty"`
}

// Summary is the review count block.
type Summary struct {
	Total       int `json:"total"`
	Migrated    int `json:"migrated"`
	NotMigrated int `json:"not_migrated"`
	Outline     int `json:"outline_or_shadow"`
	Animation   int `json:"animation"`
	DecodeError int `json:"decode_error"`
}

// Review is the complete sidecar report.
type Review struct {
	Version      int                 `json:"version"`
	Method       string              `json:"method"`
	Source       SourceInfo          `json:"source"`
	Summary      Summary             `json:"summary"`
	Cues         []CueReport         `json:"cues"`
	Verification *VerificationReport `json:"verification,omitempty"`
}

// BuildReview writes per-cue original images and returns the review model.
func BuildReview(outDir string, idx *Index, sub *SubFile, cues []*Cue, events []ASSEvent) (*Review, error) {
	eventByIndex := make(map[int]ASSEvent, len(events))
	for _, event := range events {
		eventByIndex[event.Index] = event
	}
	review := &Review{
		Version: 1,
		Method:  "container metadata only; no OCR, no inferred dialogue",
		Source: SourceInfo{
			IndexPath: idx.Path, SubPath: sub.Path,
			IndexSHA: idx.RawSHA256, SubSHA: sub.SHA,
			Width: idx.Frame.X, Height: idx.Frame.Y,
		},
	}
	for _, cue := range cues {
		item := CueReport{
			Index: cue.Index, StartMS: cue.StartMS, EndMS: cue.EndMS,
			Start: FormatMS(cue.StartMS), End: FormatMS(cue.EndMS),
			FilePos: fmt.Sprintf("%#x", cue.FilePos),
			Rect:    cue.State.Rect, Migratable: cue.Migratable,
			Colors: append([]VisibleColour(nil), cue.Colors...),
		}
		for _, flag := range cue.Flags {
			item.Flags = append(item.Flags, string(flag))
		}
		item.Notes = append(item.Notes, cue.Notes...)
		review.Summary.Total++
		if cue.Migratable {
			review.Summary.Migrated++
		} else {
			review.Summary.NotMigrated++
		}
		if cue.HasFlag(FlagOutline) {
			review.Summary.Outline++
		}
		if cue.HasFlag(FlagAnimation) {
			review.Summary.Animation++
		}
		if cue.HasFlag(FlagDecodeError) || cue.HasFlag(FlagTruncated) || cue.HasFlag(FlagUnsupported) {
			review.Summary.DecodeError++
		}
		if cue.SPU != nil && len(cue.SPU.Raw) > 0 {
			rel := fmt.Sprintf("raw/%04d.spu", cue.Index)
			if err := writeBytes(filepath.Join(outDir, rel), cue.SPU.Raw); err != nil {
				return nil, fmt.Errorf("cue %04d raw SPU: %w", cue.Index, err)
			}
			item.RawSPU = filepath.ToSlash(rel)
		}
		if cue.Raster != nil {
			rel := fmt.Sprintf("original/%04d.png", cue.Index)
			if err := writePNG(filepath.Join(outDir, rel), CueCrop(cue)); err != nil {
				return nil, fmt.Errorf("cue %04d original: %w", cue.Index, err)
			}
			item.Original = filepath.ToSlash(rel)
			frameRel := fmt.Sprintf("original/%04d_frame.png", cue.Index)
			if err := writePNG(filepath.Join(outDir, frameRel), SourceFrameAt(cues, idx.Frame, cue.StartMS)); err != nil {
				return nil, fmt.Errorf("cue %04d frame: %w", cue.Index, err)
			}
			item.Frame = filepath.ToSlash(frameRel)
		}
		for _, state := range cue.StateImages {
			if state.Raster == nil {
				continue
			}
			rel := fmt.Sprintf("original/%04d_%s.png", cue.Index, state.Label)
			img := RasterImage(state.Raster, state.State, cue.Palette, &reviewBackground)
			if err := writePNG(filepath.Join(outDir, rel), img); err != nil {
				return nil, fmt.Errorf("cue %04d %s image: %w", cue.Index, state.Label, err)
			}
			item.States = append(item.States, ImageRef{
				Label: fmt.Sprintf("%s @ %d ms", state.Label, state.TickMS),
				Path:  filepath.ToSlash(rel),
			})
			frameRel := fmt.Sprintf("original/%04d_%s_frame.png", cue.Index, state.Label)
			frameImg := RasterAtFrame(state.Raster, state.State, cue.Palette, idx.Frame)
			if err := writePNG(filepath.Join(outDir, frameRel), frameImg); err != nil {
				return nil, fmt.Errorf("cue %04d %s frame image: %w", cue.Index, state.Label, err)
			}
			item.States = append(item.States, ImageRef{
				Label: fmt.Sprintf("%s frame @ %d ms", state.Label, state.TickMS),
				Path:  filepath.ToSlash(frameRel),
			})
		}
		if event, ok := eventByIndex[cue.Index]; ok {
			item.ASS = &ASSRef{
				StartMS: event.StartMS, EndMS: event.EndMS,
				StartDeltaMS: event.StartMS - cue.StartMS,
				EndDeltaMS:   event.EndMS - cue.EndMS,
				X:            event.X, Y: event.Y, Color: event.Color, Alpha: event.Alpha,
			}
		}
		review.Cues = append(review.Cues, item)
	}
	sort.SliceStable(review.Cues, func(i, j int) bool { return review.Cues[i].Index < review.Cues[j].Index })
	return review, nil
}

// WriteReview writes review.json and review.md.
func WriteReview(outDir string, review *Review) error {
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeBytes(filepath.Join(outDir, "review.json"), data); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "review.md"), []byte(reviewMarkdown(review)), 0o644)
}

// AttachVerification copies per-cue boundary results into the review.
func AttachVerification(review *Review, verification *VerificationReport) {
	if review == nil || verification == nil {
		return
	}
	review.Verification = verification
	byIndex := make(map[int]CueVerification, len(verification.Cues))
	for _, item := range verification.Cues {
		byIndex[item.Index] = item
	}
	for i := range review.Cues {
		if item, ok := byIndex[review.Cues[i].Index]; ok {
			copy := item
			review.Cues[i].Verification = &copy
		}
	}
}

func reviewMarkdown(r *Review) string {
	var b strings.Builder
	b.WriteString("# VobSub review\n\n")
	fmt.Fprintf(&b, "Source: `%s` + `%s`  \n", r.Source.IndexPath, r.Source.SubPath)
	fmt.Fprintf(&b, "Frame: `%dx%d`  \n", r.Source.Width, r.Source.Height)
	b.WriteString("Method: container metadata only. Text was **not** OCRed or guessed. ")
	b.WriteString("Every cue below is attached to its decoded source image.\n\n")
	fmt.Fprintf(&b, "Summary: %d cues, %d migrated, %d held for review, %d with outline/shadow/AA, %d animated, %d decode failures.\n\n",
		r.Summary.Total, r.Summary.Migrated, r.Summary.NotMigrated,
		r.Summary.Outline, r.Summary.Animation, r.Summary.DecodeError)
	if r.Verification != nil {
		fmt.Fprintf(&b, "Boundary verification: **%s** (%d rounds, %d checks, %d failures).\n\n",
			passText(r.Verification.OK), r.Verification.Rounds,
			r.Verification.Checks, len(r.Verification.Failures))
	}
	for _, cue := range r.Cues {
		fmt.Fprintf(&b, "## Cue %04d\n\n", cue.Index)
		fmt.Fprintf(&b, "- In/out: `%s` .. `%s` (`%d..%d ms`)\n", cue.Start, cue.End, cue.StartMS, cue.EndMS)
		fmt.Fprintf(&b, "- Source rect: `x=%d y=%d w=%d h=%d`\n", cue.Rect.X, cue.Rect.Y, cue.Rect.Width, cue.Rect.Height)
		fmt.Fprintf(&b, "- Flags: `%s`\n", strings.Join(cue.Flags, ", "))
		if cue.ASS != nil {
			fmt.Fprintf(&b, "- ASS: `%d..%d ms`, delta `%+d/%+d ms`, colour `%s`, alpha `%d`\n",
				cue.ASS.StartMS, cue.ASS.EndMS, cue.ASS.StartDeltaMS, cue.ASS.EndDeltaMS,
				cue.ASS.Color, cue.ASS.Alpha)
		} else {
			b.WriteString("- ASS: not emitted (unreliable or not representable)\n")
		}
		if len(cue.Notes) > 0 {
			fmt.Fprintf(&b, "- Notes: %s\n", strings.Join(cue.Notes, "; "))
		}
		if v := cue.Verification; v != nil {
			fmt.Fprintf(&b, "- Composite check: pixel match `%v`, source deltas `%+d/%+d ms`\n",
				v.PixelMatch, v.StartDeltaMS, v.EndDeltaMS)
		}
		if cue.Original != "" {
			fmt.Fprintf(&b, "\n![cue %04d original](%s)\n\n", cue.Index, cue.Original)
		}
		if cue.Frame != "" {
			fmt.Fprintf(&b, "![cue %04d frame](%s)\n\n", cue.Index, cue.Frame)
		}
		for _, state := range cue.States {
			fmt.Fprintf(&b, "![cue %04d %s](%s)\n\n", cue.Index, state.Label, state.Path)
		}
		if v := cue.Verification; v != nil {
			fmt.Fprintf(&b, "![cue %04d in](%s)\n\n", cue.Index, v.InImage)
			fmt.Fprintf(&b, "![cue %04d last](%s)\n\n", cue.Index, v.LastImage)
			fmt.Fprintf(&b, "![cue %04d out](%s)\n\n", cue.Index, v.OutImage)
		}
	}
	return b.String()
}

func passText(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func writeBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
