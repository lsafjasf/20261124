package batch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"ac3norm/internal/ac3"
)

// ErrAborted marks a batch-wide stop: CRC conflict or duplicated boundary.
var ErrAborted = errors.New("batch aborted")

// FramePreview shows the bit-level before/after state of one frame.
type FramePreview struct {
	Index          int    `json:"index"`
	SampleRate     int    `json:"sampleRateHz"`
	BitRateKbps    int    `json:"bitrateKbps"`
	Bsid           int    `json:"bsid"`
	Bsmod          int    `json:"bsmod"`
	Acmod          int    `json:"acmod"`
	Lfeon          int    `json:"lfeon"`
	DialnormBefore int    `json:"dialnormBefore"`
	DialnormAfter  int    `json:"dialnormAfter"`
	Changed        bool   `json:"changed"`
	HashBefore     string `json:"hashBefore"`
	HashAfter      string `json:"hashAfter"`
}

// JobPreview is the full change preview for one file.
type JobPreview struct {
	Path          string         `json:"path"`
	FrameCount    int            `json:"frameCount"`
	ChangedFrames int            `json:"changedFrames"`
	FileHashOld   string         `json:"fileHashOld"`
	FileHashNew   string         `json:"fileHashNew"`
	BoundaryHits  []int          `json:"boundaryHits"`
	Frames        []FramePreview `json:"frames"`

	outputBytes []byte
}

// Preview validates every job in memory and computes the resulting changes.
// No file is modified; any CRC conflict or duplicated boundary aborts the
// entire batch before the write phase can begin.
func Preview(manifest *Manifest) ([]JobPreview, error) {
	previews := make([]JobPreview, 0, len(manifest.Jobs))
	for _, job := range manifest.Jobs {
		preview, err := previewJob(job)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrAborted, job.Path, err)
		}
		previews = append(previews, *preview)
	}
	return previews, nil
}

func previewJob(job Job) (*JobPreview, error) {
	raw, err := os.ReadFile(job.Path)
	if err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}
	stream, err := ac3.ParseStream(raw)
	if err != nil {
		return nil, err
	}
	for _, boundary := range job.Boundaries {
		if boundary >= len(stream.Frames) {
			return nil, fmt.Errorf("boundary frame %d beyond stream length %d", boundary, len(stream.Frames))
		}
		if ac3.FramesEqual(stream.Frames[boundary-1], stream.Frames[boundary]) {
			return nil, fmt.Errorf("duplicated frame across ad/program boundary at frame %d", boundary)
		}
	}

	preview := &JobPreview{
		Path:         job.Path,
		FrameCount:   len(stream.Frames),
		FileHashOld:  stream.FileHash(),
		BoundaryHits: append([]int(nil), job.Boundaries...),
	}
	for _, frame := range stream.Frames {
		hashBefore := stream.FrameHash(frame.Index)
		dialnormBefore := frame.Dialnorm
		changed, err := ac3.SetDialnorm(frame, *job.Dialnorm)
		if err != nil {
			return nil, err
		}
		if changed {
			preview.ChangedFrames++
		}
		preview.Frames = append(preview.Frames, FramePreview{
			Index:          frame.Index,
			SampleRate:     frame.SampleRate,
			BitRateKbps:    frame.BitRateKbps,
			Bsid:           frame.Bsid,
			Bsmod:          frame.Bsmod,
			Acmod:          frame.Acmod,
			Lfeon:          frame.Lfeon,
			DialnormBefore: dialnormBefore,
			DialnormAfter:  *job.Dialnorm,
			Changed:        changed,
			HashBefore:     hashBefore,
			HashAfter:      stream.FrameHash(frame.Index),
		})
	}

	output := stream.Bytes()
	// Re-parse the exact output bytes: a CRC conflict here aborts before any
	// write phase, proving the rewritten CRC words are valid.
	if _, err := ac3.ParseStream(output); err != nil {
		return nil, fmt.Errorf("post-edit verification: %w", err)
	}
	preview.outputBytes = output
	preview.FileHashNew = stream.FileHash()
	return preview, nil
}

// Commit reruns the full validation on fresh bytes and replaces each input
// atomically: temp file in the same directory, fsync, then rename. A failure
// stops the whole batch; untouched files stay untouched and temp files vanish.
func Commit(manifest *Manifest) ([]JobPreview, error) {
	previews := make([]JobPreview, 0, len(manifest.Jobs))
	for _, job := range manifest.Jobs {
		preview, err := previewJob(job)
		if err != nil {
			return previews, fmt.Errorf("%w: %s: %w", ErrAborted, job.Path, err)
		}
		if err := atomicWrite(job.Path, preview.outputBytes); err != nil {
			return previews, fmt.Errorf("%w: %s: %w", ErrAborted, job.Path, err)
		}
		previews = append(previews, *preview)
	}
	return previews, nil
}

func atomicWrite(path string, output []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".ac3norm-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() { _ = os.Remove(tempName) }

	if _, err := temp.Write(output); err != nil {
		temp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		cleanup()
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		cleanup()
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}
