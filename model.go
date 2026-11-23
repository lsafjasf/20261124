package fcpxml

import (
	"encoding/json"
	"math/big"
	"sort"
	"strings"
)

type Severity string

const (
	SeverityLoss    Severity = "loss"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Category string

const (
	CategoryTrack   Category = "track"
	CategoryEffect  Category = "effect"
	CategoryMedia   Category = "media"
	CategoryFrame   Category = "frame"
	CategoryXML     Category = "xml"
	CategoryRepair  Category = "repair"
	CategoryUnknown Category = "unknown"
)

// FramePosition is a sequence time expressed in seconds and, when a frame
// duration is known, exact and rounded frames.
type FramePosition struct {
	Seconds      string `json:"seconds"`
	FrameExact   string `json:"frameExact,omitempty"`
	FrameRounded int64  `json:"frameRounded"`

	seconds *big.Rat
	frame   *big.Rat
}

func framePosition(seconds, frameDuration *big.Rat) *FramePosition {
	if seconds == nil {
		return nil
	}
	position := &FramePosition{
		Seconds: DecimalString(seconds, 9),
		seconds: new(big.Rat).Set(seconds),
	}
	if frameDuration != nil && frameDuration.Sign() > 0 {
		position.frame = new(big.Rat).Quo(seconds, frameDuration)
		position.FrameExact = RatString(position.frame)
		if rounded := roundFrame(position.frame); rounded != nil {
			position.FrameRounded = rounded.Int64()
		}
	}
	return position
}

func (p *FramePosition) MarshalJSON() ([]byte, error) {
	type alias FramePosition
	return json.Marshal((*alias)(p))
}

type TrackInfo struct {
	ID               string `json:"id"`
	Key              string `json:"key"`
	Path             string `json:"path"`
	Sequence         string `json:"sequence"`
	Lane             int    `json:"lane"`
	Kind             string `json:"kind"`
	FrameDuration    string `json:"frameDuration,omitempty"`
	SequenceDuration string `json:"sequenceDuration,omitempty"`
	ItemCount        int    `json:"itemCount"`
}

type ItemInfo struct {
	ID       string         `json:"id"`
	Path     string         `json:"path"`
	TrackID  string         `json:"trackId"`
	Kind     string         `json:"kind"`
	Name     string         `json:"name,omitempty"`
	Ref      string         `json:"ref,omitempty"`
	MediaKey string         `json:"mediaKey,omitempty"`
	OwnerKey string         `json:"ownerKey,omitempty"`
	Offset   *FramePosition `json:"offset"`
	Duration *FramePosition `json:"duration,omitempty"`
	SourceIn *FramePosition `json:"sourceIn,omitempty"`
	Node     *Element       `json:"-"`
}

type TimePointInfo struct {
	Time   string `json:"time"`
	Value  string `json:"value"`
	Interp string `json:"interp,omitempty"`
}

type EffectInfo struct {
	ID           string          `json:"id"`
	Path         string          `json:"path"`
	TrackID      string          `json:"trackId"`
	ItemID       string          `json:"itemId"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name,omitempty"`
	Ref          string          `json:"ref,omitempty"`
	ResourceKey  string          `json:"resourceKey,omitempty"`
	ResourceType string          `json:"resourceType,omitempty"`
	ResourceName string          `json:"resourceName,omitempty"`
	OwnerKey     string          `json:"ownerKey,omitempty"`
	Resolved     bool            `json:"resolved"`
	Offset       *FramePosition  `json:"offset,omitempty"`
	Duration     *FramePosition  `json:"duration,omitempty"`
	Speed        string          `json:"speed,omitempty"`
	TimePoints   []TimePointInfo `json:"timePoints,omitempty"`
	Fingerprint  string          `json:"fingerprint"`
	Identity     string          `json:"-"`
	Node         *Element        `json:"-"`
}

type MediaReferenceInfo struct {
	ID           string         `json:"id"`
	Path         string         `json:"path"`
	Kind         string         `json:"kind"`
	Value        string         `json:"value"`
	Resolved     bool           `json:"resolved"`
	ResourceType string         `json:"resourceType,omitempty"`
	ResourceName string         `json:"resourceName,omitempty"`
	ResourceKey  string         `json:"resourceKey,omitempty"`
	TrackID      string         `json:"trackId,omitempty"`
	ItemID       string         `json:"itemId,omitempty"`
	OwnerKey     string         `json:"ownerKey,omitempty"`
	URI          string         `json:"uri,omitempty"`
	FileState    string         `json:"fileState,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Position     *FramePosition `json:"position,omitempty"`
}

type Finding struct {
	Code       string         `json:"code"`
	Severity   Severity       `json:"severity"`
	Category   Category       `json:"category"`
	Message    string         `json:"message"`
	TrackID    string         `json:"trackId,omitempty"`
	ItemID     string         `json:"itemId,omitempty"`
	EffectID   string         `json:"effectId,omitempty"`
	MediaRef   string         `json:"mediaRef,omitempty"`
	Path       string         `json:"path,omitempty"`
	Position   *FramePosition `json:"position,omitempty"`
	SourcePath string         `json:"sourcePath,omitempty"`
	TargetPath string         `json:"targetPath,omitempty"`
}

func (f Finding) IsLoss() bool { return f.Severity == SeverityLoss }

type Inventory struct {
	Version   string               `json:"version"`
	Sequences []string             `json:"sequences"`
	Tracks    []TrackInfo          `json:"tracks"`
	Items     []*ItemInfo          `json:"items"`
	Effects   []*EffectInfo        `json:"effects"`
	MediaRefs []MediaReferenceInfo `json:"mediaReferences"`
	Findings  []Finding            `json:"findings"`
}

func (inv *Inventory) Counts() map[string]int {
	return map[string]int{
		"tracks":          len(inv.Tracks),
		"items":           len(inv.Items),
		"effects":         len(inv.Effects),
		"mediaReferences": len(inv.MediaRefs),
		"findings":        len(inv.Findings),
	}
}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		ac, bc := findingSortKey(a), findingSortKey(b)
		if ac != bc {
			return ac < bc
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Path < b.Path
	})
}

func findingSortKey(f Finding) string {
	seconds := "~"
	frame := "~"
	if f.Position != nil {
		seconds = f.Position.Seconds
		frame = f.Position.FrameExact
	}
	return seconds + "|" + frame + "|" + string(f.Category) + "|" + f.TrackID
}

func positionLess(a, b *FramePosition) bool {
	if a == nil {
		return b != nil
	}
	if b == nil {
		return false
	}
	if a.seconds != nil && b.seconds != nil {
		return a.seconds.Cmp(b.seconds) < 0
	}
	return a.Seconds < b.Seconds
}

func cleanName(value string) string {
	return strings.TrimSpace(value)
}
