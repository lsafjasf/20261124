package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func defaultConfig() Config {
	return Config{
		FPS:                  25,
		MinOverlapMs:         120,
		CandidateThreshold:   0.38,
		MinTextSimilarity:    0.34,
		ManualReviewBelow:    0.72,
		ManualRegionBelow:    0.25,
		ManualTextBelow:      0.72,
		ManualShortTextRunes: 3,
		MinRegionOverlap:     0,
	}
}

// LoadConfig reads a review configuration and all referenced subtitle files.
// Relative track source paths are resolved from the configuration directory.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := defaultConfig()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := ensureEOF(dec); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	for i := range cfg.Tracks {
		track := &cfg.Tracks[i]
		track.Kind = canonicalKind(track.Kind)
		source := track.Source
		if !filepath.IsAbs(source) {
			source = filepath.Join(filepath.Dir(path), source)
		}
		cues, err := loadCues(source)
		if err != nil {
			return nil, fmt.Errorf("track %q: %w", track.ID, err)
		}
		for j := range cues {
			if cues[j].Region == nil {
				if track.Region == nil {
					return nil, fmt.Errorf("track %q cue %q has no region", track.ID, cues[j].ID)
				}
				region := *track.Region
				cues[j].Region = &region
			}
		}
		track.Cues = cues
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func canonicalKind(kind string) string {
	key := strings.ToLower(strings.TrimSpace(kind))
	key = strings.NewReplacer("_", "-", " ", "-").Replace(key)
	switch key {
	case "forced", "forced-narrative", "narrative-forced", "forced-subtitle":
		return "forced_narrative"
	case "full", "full-subtitle", "full-subtitles", "complete", "complete-subtitle":
		return "full"
	default:
		return key
	}
}

func (c *Config) validate() error {
	if c.FPS <= 0 || c.FPS > 240 || math.IsNaN(c.FPS) || math.IsInf(c.FPS, 0) {
		return errors.New("fps must be greater than 0 and at most 240")
	}
	if c.MinOverlapMs < 0 {
		return errors.New("minOverlapMs must not be negative")
	}
	for name, value := range map[string]float64{
		"candidateThreshold": c.CandidateThreshold,
		"minTextSimilarity":  c.MinTextSimilarity,
		"manualReviewBelow":  c.ManualReviewBelow,
		"manualRegionBelow":  c.ManualRegionBelow,
		"manualTextBelow":    c.ManualTextBelow,
		"minRegionOverlap":   c.MinRegionOverlap,
	} {
		if value < 0 || value > 1 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%s must be in the range [0, 1]", name)
		}
	}
	if c.CandidateThreshold > c.ManualReviewBelow {
		return errors.New("candidateThreshold must not exceed manualReviewBelow")
	}
	if c.ManualShortTextRunes < 0 {
		return errors.New("manualShortTextRunes must not be negative")
	}

	ids := make(map[string]bool, len(c.Tracks))
	var forced, full int
	for i := range c.Tracks {
		track := &c.Tracks[i]
		if strings.TrimSpace(track.ID) == "" {
			return fmt.Errorf("track %d has an empty id", i)
		}
		if ids[track.ID] {
			return fmt.Errorf("duplicate track id %q", track.ID)
		}
		ids[track.ID] = true
		if strings.TrimSpace(track.Source) == "" {
			return fmt.Errorf("track %q has an empty source", track.ID)
		}
		switch track.Kind {
		case "forced_narrative":
			forced++
		case "full":
			full++
		default:
			return fmt.Errorf("track %q has unsupported kind %q", track.ID, track.Kind)
		}
		if track.Region != nil {
			if err := validateRegion(*track.Region); err != nil {
				return fmt.Errorf("track %q region: %w", track.ID, err)
			}
		}
		if len(track.Cues) == 0 {
			return fmt.Errorf("track %q has no cues", track.ID)
		}
		for _, cue := range track.Cues {
			if cue.StartMs < 0 || cue.EndMs <= cue.StartMs {
				return fmt.Errorf("track %q cue %q has an invalid time range", track.ID, cue.ID)
			}
			if cue.Region == nil && track.Region == nil {
				return fmt.Errorf("track %q cue %q has no region", track.ID, cue.ID)
			}
			if cue.Region != nil {
				if err := validateRegion(*cue.Region); err != nil {
					return fmt.Errorf("track %q cue %q region: %w", track.ID, cue.ID, err)
				}
			}
		}
	}
	if forced == 0 {
		return errors.New("config must contain at least one forced_narrative track")
	}
	if full == 0 {
		return errors.New("config must contain at least one full track")
	}
	return nil
}

func validateRegion(region Region) error {
	if region.X < 0 || region.Y < 0 || region.Width <= 0 || region.Height <= 0 {
		return errors.New("coordinates must be non-negative and width/height must be positive")
	}
	if region.X+region.Width > 1.000001 || region.Y+region.Height > 1.000001 {
		return errors.New("region must fit inside the normalized frame")
	}
	return nil
}
