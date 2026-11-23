package timeline

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// OpType identifies a semantic edit. Operations are absolute-value commands:
// replaying the same command from the same state always produces the same
// state.
type OpType string

const (
	OpCreateClip        OpType = "create_clip"
	OpDeleteClip        OpType = "delete_clip"
	OpMoveClip          OpType = "move_clip"
	OpTrimClip          OpType = "trim_clip"
	OpSetSpeed          OpType = "set_speed"
	OpAddMarker         OpType = "add_marker"
	OpRemoveMarker      OpType = "remove_marker"
	OpMoveMarker        OpType = "move_marker"
	OpAddEffect         OpType = "add_effect"
	OpRemoveEffect      OpType = "remove_effect"
	OpSetEffect         OpType = "set_effect"
	OpSetEffectParam    OpType = "set_effect_param"
	OpDeleteEffectParam OpType = "delete_effect_param"
)

// Operation is the on-disk representation of one edit. Only the fields
// relevant to Type are populated, which keeps records attached to the
// affected entity rather than to a whole-project snapshot.
type Operation struct {
	Type OpType `json:"type"`

	Clip   *Clip  `json:"clip,omitempty"`
	ClipID string `json:"clip_id,omitempty"`
	Track  string `json:"track,omitempty"`

	Start     int64   `json:"start_ms,omitempty"`
	SourceIn  int64   `json:"source_in_ms,omitempty"`
	SourceOut int64   `json:"source_out_ms,omitempty"`
	Speed     float64 `json:"speed,omitempty"`

	Marker   *Marker `json:"marker,omitempty"`
	MarkerID string  `json:"marker_id,omitempty"`
	Time     int64   `json:"time_ms,omitempty"`
	Label    string  `json:"label,omitempty"`

	Effect   *Effect `json:"effect,omitempty"`
	EffectID string  `json:"effect_id,omitempty"`
	Kind     string  `json:"kind,omitempty"`
	Param    string  `json:"param,omitempty"`
	Value    float64 `json:"value,omitempty"`
}

func NewCreateClip(clip Clip) Operation {
	return Operation{Type: OpCreateClip, Clip: cloneClipPointer(clip)}
}

func NewDeleteClip(clipID string) Operation {
	return Operation{Type: OpDeleteClip, ClipID: clipID}
}

func NewMoveClip(clipID, track string, start int64) Operation {
	return Operation{Type: OpMoveClip, ClipID: clipID, Track: track, Start: start}
}

func NewTrimClip(clipID string, start, sourceIn, sourceOut int64) Operation {
	return Operation{Type: OpTrimClip, ClipID: clipID, Start: start, SourceIn: sourceIn, SourceOut: sourceOut}
}

func NewSetSpeed(clipID string, speed float64) Operation {
	return Operation{Type: OpSetSpeed, ClipID: clipID, Speed: speed}
}

func NewAddMarker(marker Marker) Operation {
	copy := marker
	return Operation{Type: OpAddMarker, Marker: &copy}
}

func NewRemoveMarker(markerID string) Operation {
	return Operation{Type: OpRemoveMarker, MarkerID: markerID}
}

func NewMoveMarker(markerID string, time int64, label string) Operation {
	return Operation{Type: OpMoveMarker, MarkerID: markerID, Time: time, Label: label}
}

func NewAddEffect(clipID string, effect Effect) Operation {
	return Operation{Type: OpAddEffect, ClipID: clipID, Effect: cloneEffectPointer(effect)}
}

func NewRemoveEffect(clipID, effectID string) Operation {
	return Operation{Type: OpRemoveEffect, ClipID: clipID, EffectID: effectID}
}

func NewSetEffect(clipID, effectID, kind string) Operation {
	return Operation{Type: OpSetEffect, ClipID: clipID, EffectID: effectID, Kind: kind}
}

func NewSetEffectParam(clipID, effectID, param string, value float64) Operation {
	return Operation{Type: OpSetEffectParam, ClipID: clipID, EffectID: effectID, Param: param, Value: value}
}

func NewDeleteEffectParam(clipID, effectID, param string) Operation {
	return Operation{Type: OpDeleteEffectParam, ClipID: clipID, EffectID: effectID, Param: param}
}

// Apply validates op, mutates p, and returns the operation that reverses that
// exact mutation. If Apply returns an error, p is unchanged.
func (p *Project) Apply(op Operation) (Operation, error) {
	switch op.Type {
	case OpCreateClip:
		return p.applyCreateClip(op)
	case OpDeleteClip:
		return p.applyDeleteClip(op)
	case OpMoveClip:
		return p.applyMoveClip(op)
	case OpTrimClip:
		return p.applyTrimClip(op)
	case OpSetSpeed:
		return p.applySetSpeed(op)
	case OpAddMarker:
		return p.applyAddMarker(op)
	case OpRemoveMarker:
		return p.applyRemoveMarker(op)
	case OpMoveMarker:
		return p.applyMoveMarker(op)
	case OpAddEffect:
		return p.applyAddEffect(op)
	case OpRemoveEffect:
		return p.applyRemoveEffect(op)
	case OpSetEffect:
		return p.applySetEffect(op)
	case OpSetEffectParam:
		return p.applySetEffectParam(op)
	case OpDeleteEffectParam:
		return p.applyDeleteEffectParam(op)
	default:
		return Operation{}, fmt.Errorf("unknown operation type %q", op.Type)
	}
}

func (p *Project) applyCreateClip(op Operation) (Operation, error) {
	if op.Clip == nil {
		return Operation{}, errors.New("create_clip: clip is required")
	}
	clip := op.Clip.normalized()
	if err := clip.validate(); err != nil {
		return Operation{}, fmt.Errorf("create_clip: %w", err)
	}
	if _, ok := p.clipIndex(clip.ID); ok {
		return Operation{}, fmt.Errorf("create_clip: clip %q already exists", clip.ID)
	}
	p.Clips = append(p.Clips, clip)
	sort.Slice(p.Clips, func(i, j int) bool { return p.Clips[i].ID < p.Clips[j].ID })
	return NewDeleteClip(clip.ID), nil
}

func (p *Project) applyDeleteClip(op Operation) (Operation, error) {
	index, ok := p.clipIndex(op.ClipID)
	if !ok {
		return Operation{}, fmt.Errorf("delete_clip: clip %q does not exist", op.ClipID)
	}
	removed := p.Clips[index]
	p.Clips = append(p.Clips[:index], p.Clips[index+1:]...)
	return NewCreateClip(removed), nil
}

func (p *Project) applyMoveClip(op Operation) (Operation, error) {
	index, ok := p.clipIndex(op.ClipID)
	if !ok {
		return Operation{}, fmt.Errorf("move_clip: clip %q does not exist", op.ClipID)
	}
	if op.Track == "" {
		return Operation{}, errors.New("move_clip: track is required")
	}
	if op.Start < 0 {
		return Operation{}, errors.New("move_clip: start must be non-negative")
	}
	clip := &p.Clips[index]
	inverse := NewMoveClip(clip.ID, clip.Track, clip.Start)
	clip.Track = op.Track
	clip.Start = op.Start
	return inverse, nil
}

func (p *Project) applyTrimClip(op Operation) (Operation, error) {
	index, ok := p.clipIndex(op.ClipID)
	if !ok {
		return Operation{}, fmt.Errorf("trim_clip: clip %q does not exist", op.ClipID)
	}
	if op.Start < 0 {
		return Operation{}, errors.New("trim_clip: start must be non-negative")
	}
	if op.SourceIn < 0 || op.SourceOut <= op.SourceIn {
		return Operation{}, errors.New("trim_clip: source range is invalid")
	}
	clip := &p.Clips[index]
	inverse := NewTrimClip(clip.ID, clip.Start, clip.SourceIn, clip.SourceOut)
	clip.Start = op.Start
	clip.SourceIn = op.SourceIn
	clip.SourceOut = op.SourceOut
	return inverse, nil
}

func (p *Project) applySetSpeed(op Operation) (Operation, error) {
	index, ok := p.clipIndex(op.ClipID)
	if !ok {
		return Operation{}, fmt.Errorf("set_speed: clip %q does not exist", op.ClipID)
	}
	if !validSpeed(op.Speed) {
		return Operation{}, errors.New("set_speed: speed must be a finite positive number")
	}
	clip := &p.Clips[index]
	inverse := NewSetSpeed(clip.ID, clip.Speed)
	clip.Speed = op.Speed
	return inverse, nil
}

func (p *Project) applyAddMarker(op Operation) (Operation, error) {
	if op.Marker == nil {
		return Operation{}, errors.New("add_marker: marker is required")
	}
	marker := *op.Marker
	if err := marker.validate(); err != nil {
		return Operation{}, fmt.Errorf("add_marker: %w", err)
	}
	if _, ok := p.markerIndex(marker.ID); ok {
		return Operation{}, fmt.Errorf("add_marker: marker %q already exists", marker.ID)
	}
	p.Markers = append(p.Markers, marker)
	p.sortMarkers()
	return NewRemoveMarker(marker.ID), nil
}

func (p *Project) applyRemoveMarker(op Operation) (Operation, error) {
	index, ok := p.markerIndex(op.MarkerID)
	if !ok {
		return Operation{}, fmt.Errorf("remove_marker: marker %q does not exist", op.MarkerID)
	}
	removed := p.Markers[index]
	p.Markers = append(p.Markers[:index], p.Markers[index+1:]...)
	return NewAddMarker(removed), nil
}

func (p *Project) applyMoveMarker(op Operation) (Operation, error) {
	index, ok := p.markerIndex(op.MarkerID)
	if !ok {
		return Operation{}, fmt.Errorf("move_marker: marker %q does not exist", op.MarkerID)
	}
	if op.Time < 0 {
		return Operation{}, errors.New("move_marker: time must be non-negative")
	}
	marker := &p.Markers[index]
	inverse := NewMoveMarker(marker.ID, marker.Time, marker.Label)
	marker.Time = op.Time
	marker.Label = op.Label
	p.sortMarkers()
	return inverse, nil
}

func (p *Project) applyAddEffect(op Operation) (Operation, error) {
	clip, err := p.clip(op.ClipID, "add_effect")
	if err != nil {
		return Operation{}, err
	}
	if op.Effect == nil {
		return Operation{}, errors.New("add_effect: effect is required")
	}
	effect := op.Effect.normalized()
	if err := effect.validate(); err != nil {
		return Operation{}, fmt.Errorf("add_effect: %w", err)
	}
	if _, ok := clip.effectIndex(effect.ID); ok {
		return Operation{}, fmt.Errorf("add_effect: effect %q already exists on clip %q", effect.ID, op.ClipID)
	}
	clip.Effects = append(clip.Effects, effect)
	sort.Slice(clip.Effects, func(i, j int) bool { return clip.Effects[i].ID < clip.Effects[j].ID })
	return NewRemoveEffect(op.ClipID, effect.ID), nil
}

func (p *Project) applyRemoveEffect(op Operation) (Operation, error) {
	clip, err := p.clip(op.ClipID, "remove_effect")
	if err != nil {
		return Operation{}, err
	}
	index, ok := clip.effectIndex(op.EffectID)
	if !ok {
		return Operation{}, fmt.Errorf("remove_effect: effect %q does not exist", op.EffectID)
	}
	removed := clip.Effects[index]
	clip.Effects = append(clip.Effects[:index], clip.Effects[index+1:]...)
	return NewAddEffect(op.ClipID, removed), nil
}

func (p *Project) applySetEffect(op Operation) (Operation, error) {
	clip, err := p.clip(op.ClipID, "set_effect")
	if err != nil {
		return Operation{}, err
	}
	index, ok := clip.effectIndex(op.EffectID)
	if !ok {
		return Operation{}, fmt.Errorf("set_effect: effect %q does not exist", op.EffectID)
	}
	if op.Kind == "" {
		return Operation{}, errors.New("set_effect: kind is required")
	}
	effect := &clip.Effects[index]
	inverse := NewSetEffect(op.ClipID, op.EffectID, effect.Kind)
	effect.Kind = op.Kind
	return inverse, nil
}

func (p *Project) applySetEffectParam(op Operation) (Operation, error) {
	effect, err := p.effect(op.ClipID, op.EffectID, "set_effect_param")
	if err != nil {
		return Operation{}, err
	}
	if op.Param == "" {
		return Operation{}, errors.New("set_effect_param: parameter name is required")
	}
	if math.IsNaN(op.Value) || math.IsInf(op.Value, 0) {
		return Operation{}, errors.New("set_effect_param: value must be finite")
	}
	oldValue, existed := effect.Params[op.Param]
	var inverse Operation
	if existed {
		inverse = NewSetEffectParam(op.ClipID, op.EffectID, op.Param, oldValue)
	} else {
		inverse = NewDeleteEffectParam(op.ClipID, op.EffectID, op.Param)
	}
	if effect.Params == nil {
		effect.Params = map[string]float64{}
	}
	effect.Params[op.Param] = op.Value
	return inverse, nil
}

func (p *Project) applyDeleteEffectParam(op Operation) (Operation, error) {
	effect, err := p.effect(op.ClipID, op.EffectID, "delete_effect_param")
	if err != nil {
		return Operation{}, err
	}
	oldValue, ok := effect.Params[op.Param]
	if !ok {
		return Operation{}, fmt.Errorf("delete_effect_param: parameter %q does not exist", op.Param)
	}
	delete(effect.Params, op.Param)
	return NewSetEffectParam(op.ClipID, op.EffectID, op.Param, oldValue), nil
}

func (p *Project) clipIndex(id string) (int, bool) {
	for i := range p.Clips {
		if p.Clips[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

func (p *Project) markerIndex(id string) (int, bool) {
	for i := range p.Markers {
		if p.Markers[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

func (c *Clip) effectIndex(id string) (int, bool) {
	for i := range c.Effects {
		if c.Effects[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

func (p *Project) clip(id, action string) (*Clip, error) {
	index, ok := p.clipIndex(id)
	if !ok {
		return nil, fmt.Errorf("%s: clip %q does not exist", action, id)
	}
	return &p.Clips[index], nil
}

func (p *Project) effect(clipID, effectID, action string) (*Effect, error) {
	clip, err := p.clip(clipID, action)
	if err != nil {
		return nil, err
	}
	index, ok := clip.effectIndex(effectID)
	if !ok {
		return nil, fmt.Errorf("%s: effect %q does not exist on clip %q", action, effectID, clipID)
	}
	return &clip.Effects[index], nil
}

func (p *Project) sortMarkers() {
	sort.Slice(p.Markers, func(i, j int) bool {
		if p.Markers[i].Time != p.Markers[j].Time {
			return p.Markers[i].Time < p.Markers[j].Time
		}
		return p.Markers[i].ID < p.Markers[j].ID
	})
}

func cloneClipPointer(clip Clip) *Clip {
	copy := clip.normalized()
	return &copy
}

func cloneEffectPointer(effect Effect) *Effect {
	copy := effect.normalized()
	return &copy
}
