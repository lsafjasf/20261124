package master

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type ProcessOptions struct {
	Source        string
	OutputDir     string
	WritePreviews bool
}

type ProcessResult struct {
	Combined CombinedManifest
	SDR      DeliveryManifest
	HDR10    DeliveryManifest
	Report   ValidationReport
}

func Process(opts ProcessOptions) (*ProcessResult, error) {
	src, err := LoadSourceManifest(opts.Source)
	if err != nil {
		return nil, err
	}
	cfg, err := mergeConfig(DefaultConfig(), src.Config)
	if err != nil {
		return nil, err
	}
	outRoot, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		return nil, err
	}
	for _, dir := range []string{"sdr", "hdr10", "previews"} {
		if err := os.MkdirAll(filepath.Join(outRoot, dir), 0o755); err != nil {
			return nil, err
		}
	}
	srcAbs, err := filepath.Abs(opts.Source)
	if err != nil {
		return nil, err
	}
	srcDir := filepath.Dir(srcAbs)
	srcRel, err := filepath.Rel(outRoot, srcAbs)
	if err != nil || strings.HasPrefix(srcRel, "..") {
		srcRel = srcAbs
	}
	srcRel = filepath.ToSlash(srcRel)

	result := &ProcessResult{}
	report := ValidationReport{Version: ManifestVersion, Project: src.Project, Source: srcRel}
	combinedFrames := make([]FrameRecord, 0, len(src.Frames))
	sdrFrames := make([]SetFrameRecord, 0, len(src.Frames))
	hdrFrames := make([]SetFrameRecord, 0, len(src.Frames))
	seen := make(map[string]bool)
	var maxFALL float64

	for _, sf := range src.Frames {
		if err := ValidateShotID(sf.Shot); err != nil {
			return nil, fmt.Errorf("frame %d: %w", sf.Frame, err)
		}
		key := fmt.Sprintf("%s/%d", sf.Shot, sf.Frame)
		if seen[key] {
			return nil, fmt.Errorf("duplicate shot/frame %s", key)
		}
		seen[key] = true
		inputPath := sf.File
		if !filepath.IsAbs(inputPath) {
			inputPath = filepath.Join(srcDir, filepath.FromSlash(sf.File))
		}
		frame, err := ReadPFM(inputPath)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		frame.Shot, frame.FrameNumber, frame.Subtitle = sf.Shot, sf.Frame, sf.Subtitle
		if err := frame.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		srcBytes, srcHash, err := hashFile(inputPath)
		if err != nil {
			return nil, err
		}
		inputRel, err := filepath.Rel(outRoot, inputPath)
		if err != nil || strings.HasPrefix(inputRel, "..") {
			inputRel = inputPath
		}
		inputRel = filepath.ToSlash(inputRel)
		setInputRel := path.Join("..", inputRel)
		if filepath.IsAbs(inputRel) {
			setInputRel = inputRel
		}
		sdr, hdr, stats, err := EncodeFrame(frame, cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		maxFALL = max(maxFALL, stats.FALLNits)

		base := fmt.Sprintf("%s_%04d.rgb10le", sf.Shot, sf.Frame)
		sdrLocal := path.Join(sf.Shot, base)
		hdrLocal := path.Join(sf.Shot, base)
		sdrAbs := filepath.Join(outRoot, "sdr", filepath.FromSlash(sdrLocal))
		hdrAbs := filepath.Join(outRoot, "hdr10", filepath.FromSlash(hdrLocal))
		if err := os.MkdirAll(filepath.Dir(sdrAbs), 0o755); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(hdrAbs), 0o755); err != nil {
			return nil, err
		}
		sdrBytes, sdrHash, err := WriteRGB10LE(sdrAbs, sdr)
		if err != nil {
			return nil, err
		}
		hdrBytes, hdrHash, err := WriteRGB10LE(hdrAbs, hdr)
		if err != nil {
			return nil, err
		}
		if opts.WritePreviews {
			if err := WritePreviewPair(filepath.Join(outRoot, "previews"), sf.Shot, sf.Frame, sdr, hdr, cfg); err != nil {
				return nil, fmt.Errorf("%s preview: %w", key, err)
			}
		}

		validation, err := AnalyzeFrame(sf.Shot, sf.Frame, sf.Analysis, sdr, hdr, stats, cfg)
		if err != nil {
			return nil, fmt.Errorf("%s analysis: %w", key, err)
		}
		report.Frames = append(report.Frames, *validation)

		srcRecord := FileRecord{File: inputRel, Bytes: srcBytes, SHA256: srcHash}
		setSrcRecord := FileRecord{File: setInputRel, Bytes: srcBytes, SHA256: srcHash}
		sdrRecord := FileRecord{File: sdrLocal, Bytes: sdrBytes, SHA256: sdrHash}
		hdrRecord := FileRecord{File: hdrLocal, Bytes: hdrBytes, SHA256: hdrHash}
		combinedFrames = append(combinedFrames, FrameRecord{
			Shot: sf.Shot, Frame: sf.Frame, Timecode: sf.Timecode, Description: sf.Description,
			Width: frame.Width, Height: frame.Height, Subtitle: sf.Subtitle,
			Source: srcRecord, Analysis: sf.Analysis,
			SDR:   &FileRecord{File: "sdr/" + sdrLocal, Bytes: sdrBytes, SHA256: sdrHash},
			HDR10: &FileRecord{File: "hdr10/" + hdrLocal, Bytes: hdrBytes, SHA256: hdrHash},
		})
		baseSet := SetFrameRecord{
			Shot: sf.Shot, Frame: sf.Frame, Timecode: sf.Timecode, Description: sf.Description,
			Width: frame.Width, Height: frame.Height, Subtitle: sf.Subtitle,
			Source: setSrcRecord, Analysis: sf.Analysis,
		}
		s := baseSet
		s.Master = sdrRecord
		sdrFrames = append(sdrFrames, s)
		h := baseSet
		h.Master = hdrRecord
		hdrFrames = append(hdrFrames, h)
	}

	SummarizeReport(&report)
	sdrProfile := SDRProfile(cfg)
	hdrProfile := HDR10Profile(cfg, report.Summary.HDRMaxCLLNits, maxFALL)
	setSource := path.Join("..", srcRel)
	if filepath.IsAbs(srcRel) {
		setSource = srcRel
	}
	result.Combined = CombinedManifest{
		Version: ManifestVersion, Tool: "dual-master/" + ToolVersion, Project: src.Project,
		Source:       srcRel,
		WorkingSpace: src.WorkingSpace, Config: cfg,
		SDRProfile: sdrProfile, HDR10Profile: hdrProfile, Frames: combinedFrames,
	}
	result.SDR = DeliveryManifest{
		Version: ManifestVersion, Tool: "dual-master/" + ToolVersion, Project: src.Project,
		Source: setSource, WorkingSpace: src.WorkingSpace, Profile: sdrProfile, Frames: sdrFrames,
	}
	result.HDR10 = DeliveryManifest{
		Version: ManifestVersion, Tool: "dual-master/" + ToolVersion, Project: src.Project,
		Source: setSource, WorkingSpace: src.WorkingSpace, Profile: hdrProfile, Frames: hdrFrames,
	}
	result.Report = report

	if err := writeJSON(filepath.Join(outRoot, "delivery.json"), result.Combined); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(outRoot, "sdr", "manifest.json"), result.SDR); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(outRoot, "hdr10", "manifest.json"), result.HDR10); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(outRoot, "validation.json"), result.Report); err != nil {
		return nil, err
	}
	return result, nil
}

func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
