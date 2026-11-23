package scene

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrMissingSequence   = errors.New("update sequence is missing")
	ErrDuplicateSequence = errors.New("duplicate update sequence")
	ErrStaleSequence     = errors.New("stale update sequence")
	ErrNoConfiguration   = errors.New("no configuration has been applied")
)

// SequenceError reports a sequence that cannot take effect.
type SequenceError struct {
	Sequence uint64
	Last     uint64
	Err      error
}

func (e *SequenceError) Error() string {
	if e.Err == ErrDuplicateSequence {
		return fmt.Sprintf("sequence %d was already applied", e.Sequence)
	}
	return fmt.Sprintf("sequence %d is older than last applied sequence %d", e.Sequence, e.Last)
}

func (e *SequenceError) Unwrap() error { return e.Err }

// Engine applies configuration and update segments in sequence order.
type Engine struct {
	mu           sync.Mutex
	state        *state
	lastSequence uint64
	hasSequence  bool
	revision     uint64
}

// NewEngine returns an engine with no configuration.
func NewEngine() *Engine { return &Engine{} }

// Apply applies one parsed segment. The receiver is unchanged when an error is
// returned. Resolvable reference errors are reported in the Result without
// rejecting the update; the affected mappings are simply absent.
func (e *Engine) Apply(segment Segment) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	result := Result{
		EffectiveSequence: segment.Sequence,
		Revision:          e.revision,
		Applied:           false,
		ObjectChannels:    []ObjectChannelMapping{},
	}
	result.Unresolved = append(result.Unresolved, segment.Unresolved...)
	result.Unresolved = append(result.Unresolved, segmentExtensionIssues(segment)...)
	sequencePresent := segment.HasSequence || segment.Sequence != 0
	if !sequencePresent {
		return failResult(result, "sequence", "missing_sequence", ErrMissingSequence)
	}
	if e.hasSequence {
		if segment.Sequence == e.lastSequence {
			return failResult(result, "sequence", "duplicate_sequence",
				&SequenceError{Sequence: segment.Sequence, Last: e.lastSequence, Err: ErrDuplicateSequence})
		}
		if segment.Sequence < e.lastSequence {
			return failResult(result, "sequence", "stale_sequence",
				&SequenceError{Sequence: segment.Sequence, Last: e.lastSequence, Err: ErrStaleSequence})
		}
	}

	switch segment.Kind {
	case SegmentConfig:
		if segment.Config == nil {
			return failResult(result, "config", "missing_config", ErrInvalidSegment)
		}
		if err := validateConfig(segment.Config); err != nil {
			result.Unresolved = append(result.Unresolved,
				unresolved("config", "invalid_configuration", err.Error()))
			return failResult(result, "config", "invalid_configuration", err)
		}
		candidate := stateFromConfig(segment.Config, segment.Extensions)
		mappings, issues := resolveScene(candidate)
		result.Snapshot = candidate.snapshot()
		result.ObjectChannels = mappings
		result.Unresolved = normalizeIssues(append(result.Unresolved, issues...))
		e.commit(candidate, segment.Sequence)
		result.Revision = e.revision
		result.Applied = true
		return result, nil

	case SegmentUpdate:
		if segment.Update == nil {
			return failResult(result, "update", "missing_update", ErrInvalidSegment)
		}
		if e.state == nil {
			return failResult(result, "update", "no_configuration", ErrNoConfiguration)
		}
		candidate := e.state.clone()
		updateIssues, err := applyUpdate(candidate, segment.Update)
		if err != nil {
			result.Unresolved = append(result.Unresolved, updateIssues...)
			result.Unresolved = append(result.Unresolved, segment.Update.Unresolved...)
			result.Unresolved = append(result.Unresolved, unresolved("update", "invalid_update", err.Error()))
			return failResult(result, "update", "invalid_update", err)
		}
		mergeStateExtensions(candidate, segment.Extensions)
		mappings, issues := resolveScene(candidate)
		result.Snapshot = candidate.snapshot()
		result.ObjectChannels = mappings
		result.Unresolved = append(result.Unresolved, segment.Update.Unresolved...)
		result.Unresolved = append(result.Unresolved, updateIssues...)
		result.Unresolved = append(result.Unresolved, issues...)
		result.Unresolved = normalizeIssues(result.Unresolved)
		e.commit(candidate, segment.Sequence)
		result.Revision = e.revision
		result.Applied = true
		return result, nil

	default:
		return failResult(result, "type", "unsupported_segment_type", ErrInvalidSegment)
	}
}

// ApplyJSON parses and applies one segment.
func (e *Engine) ApplyJSON(raw []byte) (Result, error) {
	segment, err := ParseSegment(raw)
	if err == nil {
		return e.Apply(segment)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	result := Result{
		EffectiveSequence: extractSequence(raw),
		Revision:          e.revision,
		Applied:           false,
		ObjectChannels:    []ObjectChannelMapping{},
		Unresolved:        []UnresolvedField{},
	}
	return failResult(result, "segment", "parse_failure", err)
}

func (e *Engine) commit(next *state, sequence uint64) {
	e.state = next
	e.lastSequence = sequence
	e.hasSequence = true
	e.revision++
}

func failResult(result Result, path, reason string, err error) (Result, error) {
	result.Unresolved = append(result.Unresolved, unresolved(path, reason, nil))
	result.Unresolved = normalizeIssues(result.Unresolved)
	result.Errors = append(result.Errors, err.Error())
	return result, err
}

// Current returns a copy of the current snapshot, mappings, and unresolved
// fields. It is safe to call between updates.
func (e *Engine) Current() (Snapshot, []ObjectChannelMapping, []UnresolvedField) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state == nil {
		return Snapshot{}, []ObjectChannelMapping{}, []UnresolvedField{}
	}
	mappings, issues := resolveScene(e.state)
	return *e.state.snapshot(), mappings, issues
}

func extractSequence(raw []byte) uint64 {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &fields); err != nil {
		return 0
	}
	for _, key := range []string{"sequence", "effectiveSequence", "effective_sequence"} {
		value, err := parseSequence(fields[key], key)
		if err == nil && len(fields[key]) > 0 {
			return value
		}
	}
	return 0
}
