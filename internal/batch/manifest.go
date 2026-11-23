package batch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Manifest is the batch description. Each job targets one AC-3 elementary
// stream and one permitted field (currently dialnorm only).
type Manifest struct {
	Jobs []Job `json:"jobs"`
}

// Job edits a single file. Boundaries lists frame indices where an
// advertisement/program splice starts; the frame pair straddling each splice
// must not be a duplicated frame or the whole batch aborts.
type Job struct {
	Path       string `json:"path"`
	Dialnorm   *int   `json:"dialnorm"`
	Boundaries []int  `json:"boundaries,omitempty"`
}

// LoadManifest reads and validates a JSON manifest without touching media.
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("batch: read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("batch: parse manifest: %w", err)
	}
	if len(manifest.Jobs) == 0 {
		return nil, errors.New("batch: manifest contains no jobs")
	}
	seen := make(map[string]bool)
	for i := range manifest.Jobs {
		job := &manifest.Jobs[i]
		if job.Path == "" {
			return nil, fmt.Errorf("batch: job %d has empty path", i)
		}
		if seen[job.Path] {
			return nil, fmt.Errorf("batch: repeated job path %q", job.Path)
		}
		seen[job.Path] = true
		if job.Dialnorm == nil {
			return nil, fmt.Errorf("batch: job %q requests no permitted-field edit", job.Path)
		}
		if *job.Dialnorm < 0 || *job.Dialnorm > 31 {
			return nil, fmt.Errorf("batch: job %q dialnorm %d out of range [0,31]", job.Path, *job.Dialnorm)
		}
		previous := -1
		for _, frameIndex := range job.Boundaries {
			if frameIndex <= previous {
				return nil, fmt.Errorf("batch: job %q boundaries must be strictly increasing", job.Path)
			}
			previous = frameIndex
			if frameIndex <= 0 {
				return nil, fmt.Errorf("batch: job %q boundary frame %d must be > 0", job.Path, frameIndex)
			}
		}
	}
	return &manifest, nil
}
