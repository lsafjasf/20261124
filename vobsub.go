// Package vobsub decodes VobSub (.idx/.sub) bitmap subtitles without OCR.
//
// It migrates only facts that are present in the container: display timing,
// placement, palette colours and alpha. Text is never inferred.
package vobsub

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RGB is an 8-bit, non-premultiplied colour.
type RGB struct {
	R uint8
	G uint8
	B uint8
}

// Hex returns #RRGGBB.
func (c RGB) Hex() string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// Rect is an inclusive pixel rectangle in the video frame.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// TimestampEntry is one VobSub timestamp/filepos pair.
type TimestampEntry struct {
	StartMS int64
	FilePos int64
	Raw     string
}

// Index is the parsed .idx sidecar.
type Index struct {
	Path       string
	Frame      image.Point
	Palette    [16]RGB
	Timestamps []TimestampEntry
	LanguageID string
	Language   int
	RawSHA256  string
}

// Flag identifies a condition that must be reviewed by a human.
type Flag string

const (
	FlagTextNotRecognized Flag = "text_not_recognized_no_ocr"
	FlagOutline           Flag = "outline_or_shadow_or_antialias"
	FlagAnimation         Flag = "animation"
	FlagForced            Flag = "forced_display"
	FlagMissingStart      Flag = "timing_missing_start"
	FlagMissingStop       Flag = "timing_missing_stop"
	FlagBadTiming         Flag = "timing_invalid"
	FlagTimingPrecision   Flag = "timing_below_ass_precision"
	FlagDecodeError       Flag = "decode_error"
	FlagUnsupported       Flag = "unsupported_control"
	FlagGeometry          Flag = "geometry_out_of_bounds"
	FlagPalette           Flag = "palette_invalid"
	FlagTruncated         Flag = "spu_truncated"
	FlagNoOriginalImage   Flag = "original_image_unavailable"
)

// State is one decoded visual state of an SPU.
type State struct {
	PaletteMap [4]uint8
	Alpha      [4]uint8
	Rect       Rect
	TopOffset  int
	BotOffset  int
}

// Snapshot is a visual state observed at a control-sequence tick.
type Snapshot struct {
	TickMS int64
	State  State
	Active bool
}

// SPU is the useful portion of a decoded sub-picture unit.
type SPU struct {
	Size          int
	ControlOffset int
	StartTick     int64
	EndTick       int64
	HasStart      bool
	HasStop       bool
	Forced        bool
	Animation     bool
	Snapshots     []Snapshot
	MainState     State
	Raw           []byte
}

// Raster is a 2-bit indexed sub-picture bitmap. Pixel values are 0..3.
type Raster struct {
	Width  int
	Height int
	Pixels []uint8
	Used   [4]bool
}

// At returns the palette index at x,y.
func (r *Raster) At(x, y int) uint8 {
	if r == nil || x < 0 || y < 0 || x >= r.Width || y >= r.Height {
		return 0
	}
	return r.Pixels[y*r.Width+x]
}

// VisibleColour is a palette-mapped colour used by a raster.
type VisibleColour struct {
	Index    int    `json:"index"`
	RGB      string `json:"rgb"`
	Alpha    int    `json:"alpha"`
	Pixels   int    `json:"pixels"`
	Area     int    `json:"area"`
	Dominant bool   `json:"dominant"`
}

// StateImage is a decoded visual state retained for animated SPUs.
type StateImage struct {
	Label  string
	TickMS int64
	Active bool
	State  State
	Raster *Raster
	Error  string
}

// Cue is one timestamp entry and its decoded SPU.
type Cue struct {
	Index       int
	StartMS     int64
	EndMS       int64
	FilePos     int64
	SPU         *SPU
	Raster      *Raster
	State       State
	Palette     [16]RGB
	Colors      []VisibleColour
	StateImages []StateImage
	Flags       []Flag
	Notes       []string
	Migratable  bool
}

// AddFlag adds a flag once.
func (c *Cue) AddFlag(f Flag) {
	for _, existing := range c.Flags {
		if existing == f {
			return
		}
	}
	c.Flags = append(c.Flags, f)
}

// HasFlag reports whether f is present.
func (c *Cue) HasFlag(f Flag) bool {
	for _, existing := range c.Flags {
		if existing == f {
			return true
		}
	}
	return false
}

// FormatMS formats milliseconds as HH:MM:SS.mmm.
func FormatMS(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3_600_000
	ms -= h * 3_600_000
	m := ms / 60_000
	ms -= m * 60_000
	s := ms / 1000
	ms -= s * 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// ParseMS parses HH:MM:SS.mmm or HH:MM:SS:mmm.
func ParseMS(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty timestamp")
	}
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		s = s[:i] + ":" + s[i+1:]
	}
	parts := strings.Split(s, ":")
	if len(parts) != 4 {
		return 0, fmt.Errorf("timestamp %q must be HH:MM:SS.mmm", s)
	}
	var h, m, sec, ms int64
	for i, dst := range []*int64{&h, &m, &sec, &ms} {
		n, err := strconv.ParseInt(parts[i], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("timestamp %q: %w", s, err)
		}
		*dst = n
	}
	if h < 0 || m < 0 || m > 59 || sec < 0 || sec > 59 || ms < 0 || ms > 999 {
		return 0, fmt.Errorf("timestamp %q out of range", s)
	}
	return ((h*60+m)*60+sec)*1000 + ms, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func subPathFor(idxPath, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	ext := strings.ToLower(filepath.Ext(idxPath))
	if ext != ".idx" {
		return "", fmt.Errorf("cannot infer .sub path from %q; use -sub", idxPath)
	}
	return strings.TrimSuffix(idxPath, filepath.Ext(idxPath)) + ".sub", nil
}
