package videoedit

import (
	"encoding/json"
	"fmt"
)

// ActionType enumerates the operations a change order can carry.
type ActionType string

const (
	// ActionTrimStart moves the clip head by Delta (>0 shortens, <0 extends).
	ActionTrimStart ActionType = "trimStart"
	// ActionTrimEnd moves the clip tail by Delta (<0 shortens, >0 extends).
	ActionTrimEnd ActionType = "trimEnd"
	// ActionMove shifts the whole clip on the timeline by Delta.
	ActionMove ActionType = "move"
	// ActionDelete removes the clip.
	ActionDelete ActionType = "delete"
)

// Operation is one change-order entry targeting a single clip.
// Deltas are timeline durations.
type Operation struct {
	ClipID string     `json:"clipId"`
	Action ActionType `json:"action"`
	Delta  Duration   `json:"delta,omitempty"`
}

// ChangeOrder is an ordered list of operations against a project.
type ChangeOrder struct {
	ID         string      `json:"id"`
	Operations []Operation `json:"operations"`
}

// ParseOrder decodes a change order from JSON.
func ParseOrder(data []byte) (*ChangeOrder, error) {
	var order ChangeOrder
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("parse change order: %w", err)
	}
	if err := order.Validate(); err != nil {
		return nil, err
	}
	return &order, nil
}

// Validate checks operation-level invariants.
func (o *ChangeOrder) Validate() error {
	if o == nil {
		return fmt.Errorf("change order is nil")
	}
	for i, op := range o.Operations {
		if op.ClipID == "" {
			return fmt.Errorf("operation %d has empty clipId", i)
		}
		switch op.Action {
		case ActionTrimStart, ActionTrimEnd, ActionMove, ActionDelete:
		default:
			return fmt.Errorf("operation %d on clip %q has unknown action %q", i, op.ClipID, op.Action)
		}
	}
	return nil
}
