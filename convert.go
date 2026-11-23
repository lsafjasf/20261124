package vobsub

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConvertOptions configures one VobSub conversion.
type ConvertOptions struct {
	IndexPath string
	SubPath   string
	OutDir    string
	Rounds    int
	ASS       ASSOptions
}

// Result summarizes a conversion.
type Result struct {
	Index        *Index
	Cues         []*Cue
	Events       []ASSEvent
	Review       *Review
	Verification *VerificationReport
	ASSPath      string
}

// Convert decodes a VobSub pair, writes marker-only ASS, review images and a
// repeated boundary/composite verification report.
func Convert(opts ConvertOptions) (*Result, error) {
	if opts.IndexPath == "" {
		return nil, fmt.Errorf("missing .idx path")
	}
	if opts.OutDir == "" {
		return nil, fmt.Errorf("missing output directory")
	}
	if opts.Rounds <= 0 {
		opts.Rounds = 3
	}
	subPath, err := subPathFor(opts.IndexPath, opts.SubPath)
	if err != nil {
		return nil, err
	}
	idx, err := ParseIndex(opts.IndexPath)
	if err != nil {
		return nil, err
	}
	sub, err := OpenSub(subPath)
	if err != nil {
		return nil, err
	}
	defer sub.Close()

	cues := DecodeCues(idx, sub)
	if len(cues) == 0 {
		return nil, fmt.Errorf("%s: no cues", opts.IndexPath)
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, err
	}
	assPath := filepath.Join(opts.OutDir, "subtitles.ass")
	events, err := WriteASS(assPath, idx.Frame, cues, opts.ASS)
	if err != nil {
		return nil, err
	}
	review, err := BuildReview(opts.OutDir, idx, sub, cues, events)
	if err != nil {
		return nil, err
	}
	verification, err := VerifyASS(opts.OutDir, idx.Frame, cues, opts.Rounds)
	if err != nil {
		return nil, err
	}
	AttachVerification(review, verification)
	if err := WriteReview(opts.OutDir, review); err != nil {
		return nil, err
	}
	return &Result{
		Index: idx, Cues: cues, Events: events,
		Review: review, Verification: verification, ASSPath: assPath,
	}, nil
}
