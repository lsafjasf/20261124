package demo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lsafjasf/20261124/internal/master"
)

type sceneBuilder func(shot string, frameNo int) (*master.Frame, *master.FrameAnalysis)

type shotDefinition struct {
	ID          string
	Description string
	Build       sceneBuilder
}

// GenerateSource creates a reproducible linear-float test set covering a night
// exterior, a clipped-looking practical window, and a skin-tone close-up.
func GenerateSource(dir string) (*master.SourceManifest, error) {
	if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
		return nil, err
	}
	cfg := master.DefaultConfig()
	shots := []shotDefinition{
		{"S001_NIGHT", "夜景：城市窗灯与 18% 中灰测试区", nightFrame},
		{"S002_WINDOW", "窗外高光：四级 100/200/400/800 nit 窗格", windowFrame},
		{"S003_SKIN", "肤色特写：暖肤色与中性灰参照", skinFrame},
	}
	manifest := &master.SourceManifest{
		Version:      master.ManifestVersion,
		Project:      "Dual-Master Validation",
		WorkingSpace: "rec2020-linear",
		Config:       &cfg,
	}
	for _, shot := range shots {
		for frameNo := 1; frameNo <= 2; frameNo++ {
			frame, analysis := shot.Build(shot.ID, frameNo)
			rel := filepath.ToSlash(filepath.Join("frames", fmt.Sprintf("%s_%04d.pfm", shot.ID, frameNo)))
			abs := filepath.Join(dir, filepath.FromSlash(rel))
			if err := master.WritePFM(abs, frame); err != nil {
				return nil, fmt.Errorf("%s frame %d: %w", shot.ID, frameNo, err)
			}
			manifest.Frames = append(manifest.Frames, master.SourceFrame{
				Shot: shot.ID, Frame: frameNo, File: rel, Subtitle: subtitle,
				Analysis: analysis, Timecode: timecode(frameNo), Description: shot.Description,
			})
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		return nil, err
	}
	return manifest, nil
}
