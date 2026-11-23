package diff

import (
	"fmt"

	"ediff/internal/project"
)

// clipIdentity 是判定“同一片段”的身份字段：
// 只要其中任何一项变化，就不是移动，而是内容修改。
type clipIdentity struct {
	mediaID   string
	sourceIn  float64
	sourceOut float64
	speed     float64
}

func identity(c project.Clip) clipIdentity {
	return clipIdentity{
		mediaID:   c.MediaID,
		sourceIn:  c.SourceIn,
		sourceOut: c.SourceOut,
		speed:     c.Speed,
	}
}

func (e *Engine) diffClips() {
	baseClips := indexClips(e.base)
	targetClips := indexClips(e.target)

	for id, bf := range baseClips {
		tf, ok := targetClips[id]
		if !ok {
			e.report.Changes = append(e.report.Changes, Change{
				Op: "clip.remove", ID: id,
				Track:  pathString(bf.TrackPath),
				Detail: "clip present only in base",
			})
			continue
		}
		e.diffOneClip(id, bf, tf)
	}

	for id, tf := range targetClips {
		if _, ok := baseClips[id]; !ok {
			e.report.Changes = append(e.report.Changes, Change{
				Op: "clip.add", ID: id,
				Track:  pathString(tf.TrackPath),
				Detail: "clip present only in target",
			})
		}
	}
}

// indexClips 以 ID 建索引；重复 ID 的片段已在校验阶段产出冲突，
// 这里保留首个出现项，保证比对过程确定且不崩溃。
func indexClips(l *project.Loaded) map[string]project.FlatClip {
	out := make(map[string]project.FlatClip)
	for _, fc := range l.Clips {
		if _, exists := out[fc.Clip.ID]; !exists && fc.Clip.ID != "" {
			out[fc.Clip.ID] = fc
		}
	}
	return out
}

func (e *Engine) diffOneClip(id string, base, target project.FlatClip) {
	bc, tc := base.Clip, target.Clip
	fromTrack := pathString(base.TrackPath)
	toTrack := pathString(target.TrackPath)
	trackChanged := fromTrack != toTrack
	timelineChanged := bc.TimelineIn != tc.TimelineIn

	fields := diffClipScalarFields(bc, tc)
	fields = append(fields, diffTransition(bc, tc)...)
	fields = append(fields, diffEffects(bc, tc)...)

	pureRelocation := identity(bc) == identity(tc) && len(fields) == 0

	if pureRelocation && (trackChanged || timelineChanged) {
		c := Change{
			Op:        "clip.move",
			ID:        id,
			FromTrack: fromTrack,
			ToTrack:   toTrack,
		}
		if timelineChanged {
			c.Fields = []FieldChange{{
				Path: "timelineIn", From: bc.TimelineIn, To: tc.TimelineIn,
			}}
			c.Detail = fmt.Sprintf("relocated timelineIn %v -> %v", bc.TimelineIn, tc.TimelineIn)
		} else {
			c.Detail = "moved between tracks"
		}
		e.report.Changes = append(e.report.Changes, c)
		return
	}

	// 非纯移动：任何身份字段或参数变化都走字段级修改，
	// 绝不把“整段替换”伪装成一次移动。
	if trackChanged {
		fields = append(fields, FieldChange{Path: "track", From: fromTrack, To: toTrack})
	}
	if timelineChanged {
		fields = append(fields, FieldChange{Path: "timelineIn", From: bc.TimelineIn, To: tc.TimelineIn})
	}
	if len(fields) > 0 {
		e.report.Changes = append(e.report.Changes, Change{
			Op:     "clip.modify",
			ID:     id,
			Track:  toTrack,
			Fields: fields,
			Detail: modificationKind(bc, tc, pureRelocation),
		})
	}
}

func modificationKind(bc, tc project.Clip, pureRelocation bool) string {
	if identity(bc) != identity(tc) {
		return "identity fields changed; this is a content modification, not a move"
	}
	return "parameter modification"
}

func diffClipScalarFields(b, t project.Clip) []FieldChange {
	var out []FieldChange
	if b.Name != t.Name {
		out = append(out, FieldChange{Path: "name", From: b.Name, To: t.Name})
	}
	if b.MediaID != t.MediaID {
		out = append(out, FieldChange{Path: "mediaId", From: b.MediaID, To: t.MediaID})
	}
	if b.SourceIn != t.SourceIn {
		out = append(out, FieldChange{Path: "sourceIn", From: b.SourceIn, To: t.SourceIn})
	}
	if b.SourceOut != t.SourceOut {
		out = append(out, FieldChange{Path: "sourceOut", From: b.SourceOut, To: t.SourceOut})
	}
	if b.Speed != t.Speed {
		out = append(out, FieldChange{Path: "speed", From: b.Speed, To: t.Speed})
	}
	if boolVal(b.Enabled) != boolVal(t.Enabled) {
		out = append(out, FieldChange{Path: "enabled", From: boolVal(b.Enabled), To: boolVal(t.Enabled)})
	}
	return out
}
