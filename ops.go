package main

import (
	"errors"
	"fmt"
	"sort"
)

// OpKind 描述一次剪辑时间线修改的类型。
type OpKind string

const (
	OpMoveClip       OpKind = "move_clip"
	OpSetKeyframe    OpKind = "set_keyframe"
	OpRemoveKeyframe OpKind = "remove_keyframe"
)

// Op 是写入项目日志的一条操作。Prev* 字段由服务端在应用时填充，
// 用于撤销；客户端提交时无需提供。
type Op struct {
	Seq       int64  `json:"seq"`
	RequestID string `json:"requestId"`
	Author    string `json:"author,omitempty"`
	Kind      OpKind `json:"kind"`
	ClipID    string `json:"clipId"`

	// move_clip 参数
	Start float64 `json:"start,omitempty"`

	// keyframe 参数
	Property string  `json:"property,omitempty"`
	Time     float64 `json:"time,omitempty"`
	Value    float64 `json:"value,omitempty"`

	// 服务端填充的撤销信息
	PrevStart float64 `json:"prevStart,omitempty"`
	PrevValue float64 `json:"prevValue,omitempty"`
	HadPrev   bool    `json:"hadPrev,omitempty"`

	UndoOf int64 `json:"undoOf,omitempty"`
	Undone bool  `json:"undone,omitempty"`
}

// Keyframe 是某个属性在某一时刻的取值。
type Keyframe struct {
	Time  float64 `json:"time"`
	Value float64 `json:"value"`
}

// Clip 是时间线上的一个片段。
type Clip struct {
	ID        string                `json:"id"`
	Start     float64               `json:"start"`
	Keyframes map[string][]Keyframe `json:"keyframes,omitempty"`
}

// ProjectState 是操作日志重放后的物化视图。
type ProjectState struct {
	Clips map[string]*Clip `json:"clips"`
}

func newProjectState() *ProjectState {
	return &ProjectState{Clips: map[string]*Clip{}}
}

var ErrUnknownOpKind = errors.New("unknown op kind")

func (s *ProjectState) clip(id string) *Clip {
	c, ok := s.Clips[id]
	if !ok {
		c = &Clip{ID: id, Keyframes: map[string][]Keyframe{}}
		s.Clips[id] = c
	}
	return c
}

func findKeyframe(c *Clip, prop string, t float64) (float64, bool) {
	for _, kf := range c.Keyframes[prop] {
		if kf.Time == t {
			return kf.Value, true
		}
	}
	return 0, false
}

func setKeyframe(c *Clip, prop string, t, v float64) {
	kfs := c.Keyframes[prop]
	for i := range kfs {
		if kfs[i].Time == t {
			kfs[i].Value = v
			return
		}
	}
	kfs = append(kfs, Keyframe{Time: t, Value: v})
	sort.Slice(kfs, func(i, j int) bool { return kfs[i].Time < kfs[j].Time })
	c.Keyframes[prop] = kfs
}

func removeKeyframe(c *Clip, prop string, t float64) {
	kfs := c.Keyframes[prop]
	for i := range kfs {
		if kfs[i].Time == t {
			c.Keyframes[prop] = append(kfs[:i], kfs[i+1:]...)
			return
		}
	}
}

// apply 将操作应用到状态上，并填充 Prev* 撤销信息。
func (s *ProjectState) apply(op *Op) error {
	c := s.clip(op.ClipID)
	switch op.Kind {
	case OpMoveClip:
		op.PrevStart = c.Start
		c.Start = op.Start
	case OpSetKeyframe:
		prev, had := findKeyframe(c, op.Property, op.Time)
		op.PrevValue, op.HadPrev = prev, had
		setKeyframe(c, op.Property, op.Time, op.Value)
	case OpRemoveKeyframe:
		prev, had := findKeyframe(c, op.Property, op.Time)
		if !had {
			return fmt.Errorf("no keyframe on clip %q property %q at %v", op.ClipID, op.Property, op.Time)
		}
		op.PrevValue, op.HadPrev = prev, true
		removeKeyframe(c, op.Property, op.Time)
	default:
		return ErrUnknownOpKind
	}
	return nil
}

// invert 根据日志中记录的 Prev* 信息生成逆操作。
func invert(op *Op) (*Op, error) {
	inv := &Op{
		ClipID:   op.ClipID,
		Property: op.Property,
		Time:     op.Time,
		UndoOf:   op.Seq,
	}
	switch op.Kind {
	case OpMoveClip:
		inv.Kind = OpMoveClip
		inv.Start = op.PrevStart
	case OpSetKeyframe:
		if op.HadPrev {
			inv.Kind = OpSetKeyframe
			inv.Value = op.PrevValue
		} else {
			inv.Kind = OpRemoveKeyframe
		}
	case OpRemoveKeyframe:
		inv.Kind = OpSetKeyframe
		inv.Value = op.PrevValue
	default:
		return nil, ErrUnknownOpKind
	}
	return inv, nil
}
