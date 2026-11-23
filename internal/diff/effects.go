package diff

import (
	"fmt"
	"sort"

	"ediff/internal/project"
)

func diffTransition(b, t project.Clip) []FieldChange {
	if b.Transition == nil && t.Transition == nil {
		return nil
	}
	prefix := "transition"
	if b.Transition == nil {
		return []FieldChange{{Path: prefix, From: nil, To: transitionSummary(t.Transition)}}
	}
	if t.Transition == nil {
		return []FieldChange{{Path: prefix, From: transitionSummary(b.Transition), To: nil}}
	}
	var out []FieldChange
	bt, tt := b.Transition, t.Transition
	if bt.Type != tt.Type {
		out = append(out, FieldChange{Path: prefix + ".type", From: bt.Type, To: tt.Type})
	}
	if bt.At != tt.At {
		out = append(out, FieldChange{Path: prefix + ".at", From: bt.At, To: tt.At})
	}
	if bt.Duration != tt.Duration {
		out = append(out, FieldChange{Path: prefix + ".duration", From: bt.Duration, To: tt.Duration})
	}
	out = append(out, diffParams(prefix+".params", bt.Params, tt.Params)...)
	return out
}

func transitionSummary(tr *project.Transition) string {
	if tr == nil {
		return ""
	}
	return fmt.Sprintf("%s@%s/%v", tr.Type, tr.At, tr.Duration)
}

// diffEffects 以 effect.id（缺失时退化为 type）为对齐键，
// 分别产出新增、删除与参数修改。
func diffEffects(b, t project.Clip) []FieldChange {
	bMap := mapEffects(b.Effects)
	tMap := mapEffects(t.Effects)
	var out []FieldChange

	keys := make(map[string]bool)
	for k := range bMap {
		keys[k] = true
	}
	for k := range tMap {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)

	for _, k := range ordered {
		be, bok := bMap[k]
		te, tok := tMap[k]
		prefix := "effects[" + k + "]"
		switch {
		case !bok:
			out = append(out, FieldChange{Path: prefix, From: nil, To: effectSummary(te)})
		case !tok:
			out = append(out, FieldChange{Path: prefix, From: effectSummary(be), To: nil})
		default:
			if be.Type != te.Type {
				out = append(out, FieldChange{Path: prefix + ".type", From: be.Type, To: te.Type})
			}
			if boolVal(be.Enabled) != boolVal(te.Enabled) {
				out = append(out, FieldChange{Path: prefix + ".enabled", From: boolVal(be.Enabled), To: boolVal(te.Enabled)})
			}
			out = append(out, diffParams(prefix+".params", be.Params, te.Params)...)
		}
	}
	return out
}

func mapEffects(effs []project.Effect) map[string]project.Effect {
	out := make(map[string]project.Effect)
	for _, ef := range effs {
		key := ef.ID
		if key == "" {
			key = "type:" + ef.Type
		}
		out[key] = ef
	}
	return out
}

func effectSummary(e project.Effect) string {
	return fmt.Sprintf("%s(enabled=%v)", e.Type, boolVal(e.Enabled))
}

// diffParams 对数值参数做按键对齐的最小差异比对。
func diffParams(prefix string, b, t map[string]float64) []FieldChange {
	keys := make(map[string]bool)
	for k := range b {
		keys[k] = true
	}
	for k := range t {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)

	var out []FieldChange
	for _, k := range ordered {
		bv, bok := b[k]
		tv, tok := t[k]
		switch {
		case !bok:
			out = append(out, FieldChange{Path: prefix + "." + k, From: nil, To: tv})
		case !tok:
			out = append(out, FieldChange{Path: prefix + "." + k, From: bv, To: nil})
		case bv != tv:
			out = append(out, FieldChange{Path: prefix + "." + k, From: bv, To: tv})
		}
	}
	return out
}
