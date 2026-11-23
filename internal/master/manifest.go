package master

import (
	"encoding/json"
	"fmt"
	"os"
)

const ManifestVersion = 1

type SourceManifest struct {
	Version      int           `json:"version"`
	Project      string        `json:"project"`
	WorkingSpace string        `json:"working_space"`
	Config       *Config       `json:"config,omitempty"`
	Frames       []SourceFrame `json:"frames"`
}

type SourceFrame struct {
	Shot        string         `json:"shot"`
	Frame       int            `json:"frame"`
	File        string         `json:"file"`
	Subtitle    Rect           `json:"subtitle"`
	Analysis    *FrameAnalysis `json:"analysis,omitempty"`
	Timecode    string         `json:"timecode,omitempty"`
	Description string         `json:"description,omitempty"`
}

// FrameAnalysis contains optional ROIs used by the validation report. They do
// not alter the image or delivery metadata.
type FrameAnalysis struct {
	MiddleGray *Rect             `json:"middle_gray,omitempty"`
	Skin       *Rect             `json:"skin,omitempty"`
	Highlights []HighlightRegion `json:"highlights,omitempty"`
}

type HighlightRegion struct {
	Name         string  `json:"name"`
	Rect         Rect    `json:"rect"`
	ExpectedNits float64 `json:"expected_nits,omitempty"`
}

type Profile struct {
	Name               string  `json:"name"`
	Primaries          string  `json:"primaries"`
	WhitePoint         string  `json:"white_point"`
	Transfer           string  `json:"transfer"`
	ReferenceWhiteNits float64 `json:"reference_white_nits"`
	PeakNits           float64 `json:"peak_nits"`
	Range              string  `json:"range"`
	Bits               int     `json:"bits"`
	BlackCode          uint16  `json:"black_code"`
	WhiteCode          uint16  `json:"white_code"`
	MasterFormat       string  `json:"master_format"`
	MaxCLLNits         float64 `json:"max_cll_nits,omitempty"`
	MaxFALLNits        float64 `json:"max_fall_nits,omitempty"`
	MasteringMinNits   float64 `json:"mastering_display_min_nits,omitempty"`
	MasteringMaxNits   float64 `json:"mastering_display_max_nits,omitempty"`
}

type FileRecord struct {
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256,omitempty"`
}

type FrameRecord struct {
	Shot        string         `json:"shot"`
	Frame       int            `json:"frame"`
	Timecode    string         `json:"timecode,omitempty"`
	Description string         `json:"description,omitempty"`
	Width       int            `json:"width"`
	Height      int            `json:"height"`
	Subtitle    Rect           `json:"subtitle"`
	Source      FileRecord     `json:"source"`
	Analysis    *FrameAnalysis `json:"analysis,omitempty"`
	SDR         *FileRecord    `json:"sdr,omitempty"`
	HDR10       *FileRecord    `json:"hdr10,omitempty"`
}

type DeliveryManifest struct {
	Version      int              `json:"version"`
	Tool         string           `json:"tool"`
	Project      string           `json:"project"`
	Source       string           `json:"source_manifest"`
	WorkingSpace string           `json:"working_space"`
	Profile      Profile          `json:"profile"`
	Frames       []SetFrameRecord `json:"frames"`
}

type SetFrameRecord struct {
	Shot        string         `json:"shot"`
	Frame       int            `json:"frame"`
	Timecode    string         `json:"timecode,omitempty"`
	Description string         `json:"description,omitempty"`
	Width       int            `json:"width"`
	Height      int            `json:"height"`
	Subtitle    Rect           `json:"subtitle"`
	Source      FileRecord     `json:"source"`
	Analysis    *FrameAnalysis `json:"analysis,omitempty"`
	Master      FileRecord     `json:"master"`
}

type CombinedManifest struct {
	Version      int           `json:"version"`
	Tool         string        `json:"tool"`
	Project      string        `json:"project"`
	Source       string        `json:"source_manifest"`
	WorkingSpace string        `json:"working_space"`
	Config       Config        `json:"config"`
	SDRProfile   Profile       `json:"sdr_profile"`
	HDR10Profile Profile       `json:"hdr10_profile"`
	Frames       []FrameRecord `json:"frames"`
}

func LoadSourceManifest(path string) (*SourceManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m SourceManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Version != ManifestVersion {
		return nil, fmt.Errorf("unsupported manifest version %d", m.Version)
	}
	if m.WorkingSpace == "" {
		m.WorkingSpace = "rec2020-linear"
	}
	if m.WorkingSpace != "rec2020-linear" {
		return nil, fmt.Errorf("unsupported working space %q", m.WorkingSpace)
	}
	if len(m.Frames) == 0 {
		return nil, fmt.Errorf("manifest contains no frames")
	}
	return &m, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func mergeConfig(base Config, override *Config) (Config, error) {
	if override == nil {
		return base, base.Validate()
	}
	if override.ReferenceWhiteNits != 0 {
		base.ReferenceWhiteNits = override.ReferenceWhiteNits
	}
	if override.SDRPeakNits != 0 {
		base.SDRPeakNits = override.SDRPeakNits
	}
	if override.HDRPeakNits != 0 {
		base.HDRPeakNits = override.HDRPeakNits
	}
	if override.HDRKneeNits != 0 {
		base.HDRKneeNits = override.HDRKneeNits
	}
	if override.SDRGamma != 0 {
		base.SDRGamma = override.SDRGamma
	}
	if override.LegalBlack != 0 {
		base.LegalBlack = override.LegalBlack
	}
	if override.LegalWhite != 0 {
		base.LegalWhite = override.LegalWhite
	}
	if override.Bits != 0 {
		base.Bits = override.Bits
	}
	if override.ditherSet {
		base.Dither = override.Dither
	}
	return base, base.Validate()
}

func SDRProfile(cfg Config) Profile {
	return Profile{
		Name:               "SDR",
		Primaries:          "BT.709",
		WhitePoint:         "D65",
		Transfer:           "BT.1886",
		ReferenceWhiteNits: cfg.ReferenceWhiteNits,
		PeakNits:           cfg.SDRPeakNits,
		Range:              "video-limited",
		Bits:               cfg.Bits,
		BlackCode:          cfg.LegalBlack,
		WhiteCode:          cfg.LegalWhite,
		MasterFormat:       "rgb10le interleaved uint16",
	}
}

func HDR10Profile(cfg Config, maxCLL, maxFALL float64) Profile {
	return Profile{
		Name:               "HDR10",
		Primaries:          "BT.2020",
		WhitePoint:         "D65",
		Transfer:           "SMPTE ST 2084 (PQ)",
		ReferenceWhiteNits: cfg.ReferenceWhiteNits,
		PeakNits:           cfg.HDRPeakNits,
		Range:              "video-limited",
		Bits:               cfg.Bits,
		BlackCode:          cfg.LegalBlack,
		WhiteCode:          cfg.LegalWhite,
		MasterFormat:       "rgb10le interleaved uint16",
		MaxCLLNits:         maxCLL,
		MaxFALLNits:        maxFALL,
		MasteringMinNits:   0.0001,
		MasteringMaxNits:   cfg.HDRPeakNits,
	}
}
