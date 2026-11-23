package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// Loaded 是加载并校验后的工程视图。
type Loaded struct {
	Project   Project
	Source    string
	SHA256    string
	Tracks    []FlatTrack
	Clips     []FlatClip
	Issues    []Issue
	mediaByID map[string]Media
}

// LoadFile 读取、解析并展开一份工程文件。
func LoadFile(path string) (*Loaded, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bytes.TrimSpace(raw))

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p Project
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	normalize(&p)

	l := &Loaded{
		Project:   p,
		Source:    path,
		SHA256:    hex.EncodeToString(sum[:]),
		mediaByID: make(map[string]Media),
	}
	l.flatten()
	l.validate()
	return l, nil
}

// normalize 对缺省速度做归一化：缺省或 0 视为 1.0（常速），
// 避免“未填写”与“明确 1.0”在比对时产生伪差异。
func normalize(p *Project) {
	var walk func(tracks []Track)
	walk = func(tracks []Track) {
		for i := range tracks {
			for j := range tracks[i].Clips {
				if tracks[i].Clips[j].Speed == 0 {
					tracks[i].Clips[j].Speed = 1
				}
			}
			walk(tracks[i].Children)
		}
	}
	walk(p.Tracks)
}

// MediaByID 返回媒体库索引；不存在时第二个返回值为 false。
func (l *Loaded) MediaByID(id string) (Media, bool) {
	m, ok := l.mediaByID[id]
	return m, ok
}

func (l *Loaded) flatten() {
	var walk func(tracks []Track, path []string, depth int)
	walk = func(tracks []Track, path []string, depth int) {
		for _, t := range tracks {
			here := append(append([]string(nil), path...), t.ID)
			l.Tracks = append(l.Tracks, FlatTrack{Track: t, Path: here, Depth: depth})
			walk(t.Children, here, depth+1)
		}
	}
	walk(l.Project.Tracks, nil, 0)

	for _, ft := range l.Tracks {
		for _, c := range ft.Track.Clips {
			l.Clips = append(l.Clips, FlatClip{Clip: c, TrackPath: append([]string(nil), ft.Path...)})
		}
	}
}

func (l *Loaded) validate() {
	seenMedia := make(map[string]bool)
	for _, m := range l.Project.Media {
		if m.ID == "" {
			l.Issues = append(l.Issues, Issue{Kind: "MEDIA_EMPTY_ID", Detail: "media with empty id"})
			continue
		}
		if seenMedia[m.ID] {
			l.Issues = append(l.Issues, Issue{Kind: "MEDIA_DUPLICATE_ID", ID: m.ID, Detail: "media id declared more than once"})
		}
		seenMedia[m.ID] = true
		l.mediaByID[m.ID] = m
	}

	seenTrack := make(map[string]bool)
	for _, ft := range l.Tracks {
		id := ft.Track.ID
		if id == "" {
			l.Issues = append(l.Issues, Issue{Kind: "TRACK_EMPTY_ID", Detail: "track with empty id"})
			continue
		}
		if seenTrack[id] {
			l.Issues = append(l.Issues, Issue{Kind: "TRACK_DUPLICATE_ID", ID: id, Detail: "track id used in multiple locations"})
		}
		seenTrack[id] = true
	}

	seenClip := make(map[string]string)
	for _, fc := range l.Clips {
		c := fc.Clip
		trackID := c.TrackID
		if trackID == "" && len(fc.TrackPath) > 0 {
			trackID = fc.TrackPath[len(fc.TrackPath)-1]
		}
		if c.ID == "" {
			l.Issues = append(l.Issues, Issue{Kind: "CLIP_EMPTY_ID", Track: trackID, Detail: "clip with empty id"})
			continue
		}
		if first, ok := seenClip[c.ID]; ok {
			l.Issues = append(l.Issues, Issue{
				Kind:   "CLIP_DUPLICATE_ID",
				ID:     c.ID,
				Track:  trackID,
				Detail: fmt.Sprintf("clip id also appears in track %q", first),
			})
			continue
		}
		seenClip[c.ID] = trackID

		if c.MediaID != "" {
			if _, ok := l.mediaByID[c.MediaID]; !ok {
				l.Issues = append(l.Issues, Issue{
					Kind:   "CLIP_UNKNOWN_MEDIA",
					ID:     c.ID,
					Track:  trackID,
					Detail: fmt.Sprintf("mediaId %q not found in media library", c.MediaID),
				})
			}
		}
		if c.SourceOut < c.SourceIn {
			l.Issues = append(l.Issues, Issue{
				Kind:   "CLIP_BAD_SOURCE_RANGE",
				ID:     c.ID,
				Track:  trackID,
				Detail: fmt.Sprintf("sourceOut %v < sourceIn %v", c.SourceOut, c.SourceIn),
			})
		}
	}
}
