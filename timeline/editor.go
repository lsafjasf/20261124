package timeline

import (
	"errors"
	"fmt"
	"sync"
)

type historyEntry struct {
	EventID string
	Op      Operation
	Inverse Operation
}

// Editor applies operations to an in-memory project and journals each
// successful operation before exposing it. Undo/redo history stores only
// per-operation commands; it never stores project snapshots.
type Editor struct {
	mu      sync.Mutex
	log     *Log
	project Project
	undo    []historyEntry
	redo    []historyEntry
}

// CreateEditor creates a new journal and an editor over its base project.
func CreateEditor(path string, base Project) (*Editor, error) {
	log, err := CreateLog(path, base)
	if err != nil {
		return nil, err
	}
	return &Editor{log: log, project: log.Base()}, nil
}

// OpenEditor opens a journal, repairs a partial trailing record, replays all
// verified events, and rebuilds the undo/redo stacks.
func OpenEditor(path string) (*Editor, Recovery, error) {
	log, recovery, err := OpenLog(path)
	if err != nil {
		return nil, recovery, err
	}
	editor, err := rebuildEditor(log)
	if err != nil {
		log.Close()
		return nil, recovery, err
	}
	return editor, recovery, nil
}

// Project returns a deep copy of the current normalized project.
func (e *Editor) Project() Project {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.project.Normalize()
}

// Log returns the journal owned by the editor.
func (e *Editor) Log() *Log {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.log
}

// CanUndo reports whether an undo operation is available.
func (e *Editor) CanUndo() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.undo) > 0
}

// CanRedo reports whether a redo operation is available.
func (e *Editor) CanRedo() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.redo) > 0
}

// UndoDepth and RedoDepth expose history size for diagnostics.
func (e *Editor) UndoDepth() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.undo)
}

func (e *Editor) RedoDepth() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.redo)
}

// Apply journals and applies one operation. eventID is the idempotency key:
// reusing it returns ErrDuplicateEvent and changes neither state nor history.
func (e *Editor) Apply(eventID string, op Operation) (Record, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.applyLocked(eventID, op)
}

// ApplyAuto is Apply with an editor-generated deterministic event ID.
func (e *Editor) ApplyAuto(op Operation) (Record, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.applyLocked(e.log.nextEventIDLocked("edit"), op)
}

func (e *Editor) applyLocked(eventID string, op Operation) (Record, error) {
	if eventID == "" {
		return Record{}, errors.New("event ID is required")
	}
	if e.log.hasEventLocked(eventID) {
		return Record{}, fmt.Errorf("%w: %s", ErrDuplicateEvent, eventID)
	}
	op = cloneOperation(op)
	inverse, err := e.project.Apply(op)
	if err != nil {
		return Record{}, err
	}
	stateHash, err := e.project.StateHash()
	if err == nil {
		var record Record
		record, err = e.log.appendRecord(eventID, RecordEdit, "", op, inverse, stateHash)
		if err == nil {
			e.undo = append(e.undo, historyEntry{EventID: eventID, Op: op, Inverse: inverse})
			e.redo = nil
			return record, nil
		}
	}
	if _, rollbackErr := e.project.Apply(inverse); rollbackErr != nil {
		return Record{}, fmt.Errorf("%v; rollback failed: %w", err, rollbackErr)
	}
	return Record{}, err
}

// Undo reverses the newest appended edit as a new journal record. The redo
// stack entry contains the original command, not a project copy.
func (e *Editor) Undo() (Record, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.undo) == 0 {
		return Record{}, false, nil
	}
	entry := e.undo[len(e.undo)-1]
	forward, err := e.project.Apply(entry.Inverse)
	if err != nil {
		return Record{}, false, fmt.Errorf("undo %s: %w", entry.EventID, err)
	}
	stateHash, hashErr := e.project.StateHash()
	if hashErr != nil {
		_, _ = e.project.Apply(forward)
		return Record{}, false, hashErr
	}
	record, err := e.log.appendRecord(
		e.log.nextEventIDLocked("undo"),
		RecordUndo,
		entry.EventID,
		entry.Inverse,
		forward,
		stateHash,
	)
	if err != nil {
		_, _ = e.project.Apply(forward)
		return Record{}, false, err
	}
	e.undo = e.undo[:len(e.undo)-1]
	e.redo = append(e.redo, historyEntry{EventID: entry.EventID, Op: forward, Inverse: entry.Inverse})
	return record, true, nil
}

// Redo reapplies the most recently undone command and journals it.
func (e *Editor) Redo() (Record, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.redo) == 0 {
		return Record{}, false, nil
	}
	entry := e.redo[len(e.redo)-1]
	inverse, err := e.project.Apply(entry.Op)
	if err != nil {
		return Record{}, false, fmt.Errorf("redo %s: %w", entry.EventID, err)
	}
	stateHash, hashErr := e.project.StateHash()
	if hashErr != nil {
		_, _ = e.project.Apply(inverse)
		return Record{}, false, hashErr
	}
	record, err := e.log.appendRecord(
		e.log.nextEventIDLocked("redo"),
		RecordRedo,
		entry.EventID,
		entry.Op,
		inverse,
		stateHash,
	)
	if err != nil {
		_, _ = e.project.Apply(inverse)
		return Record{}, false, err
	}
	e.redo = e.redo[:len(e.redo)-1]
	e.undo = append(e.undo, historyEntry{EventID: entry.EventID, Op: entry.Op, Inverse: inverse})
	return record, true, nil
}

// Close closes the underlying journal.
func (e *Editor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.log.Close()
}

func (e *Editor) CreateClip(eventID string, clip Clip) error {
	_, err := e.Apply(eventID, NewCreateClip(clip))
	return err
}

func (e *Editor) DeleteClip(eventID, clipID string) error {
	_, err := e.Apply(eventID, NewDeleteClip(clipID))
	return err
}

func (e *Editor) MoveClip(eventID, clipID, track string, start int64) error {
	_, err := e.Apply(eventID, NewMoveClip(clipID, track, start))
	return err
}

func (e *Editor) TrimClip(eventID, clipID string, start, sourceIn, sourceOut int64) error {
	_, err := e.Apply(eventID, NewTrimClip(clipID, start, sourceIn, sourceOut))
	return err
}

func (e *Editor) SetSpeed(eventID, clipID string, speed float64) error {
	_, err := e.Apply(eventID, NewSetSpeed(clipID, speed))
	return err
}

func (e *Editor) AddMarker(eventID string, marker Marker) error {
	_, err := e.Apply(eventID, NewAddMarker(marker))
	return err
}

func (e *Editor) RemoveMarker(eventID, markerID string) error {
	_, err := e.Apply(eventID, NewRemoveMarker(markerID))
	return err
}

func (e *Editor) MoveMarker(eventID, markerID string, at int64, label string) error {
	_, err := e.Apply(eventID, NewMoveMarker(markerID, at, label))
	return err
}

func (e *Editor) AddEffect(eventID, clipID string, effect Effect) error {
	_, err := e.Apply(eventID, NewAddEffect(clipID, effect))
	return err
}

func (e *Editor) RemoveEffect(eventID, clipID, effectID string) error {
	_, err := e.Apply(eventID, NewRemoveEffect(clipID, effectID))
	return err
}

func (e *Editor) SetEffect(eventID, clipID, effectID, kind string) error {
	_, err := e.Apply(eventID, NewSetEffect(clipID, effectID, kind))
	return err
}

func (e *Editor) SetEffectParam(eventID, clipID, effectID, param string, value float64) error {
	_, err := e.Apply(eventID, NewSetEffectParam(clipID, effectID, param, value))
	return err
}

func (e *Editor) DeleteEffectParam(eventID, clipID, effectID, param string) error {
	_, err := e.Apply(eventID, NewDeleteEffectParam(clipID, effectID, param))
	return err
}

func rebuildEditor(log *Log) (*Editor, error) {
	project, _, err := log.Replay()
	if err != nil {
		return nil, err
	}
	records := log.Records()
	seen := make(map[string]struct{}, len(records))
	var undo []historyEntry
	var redo []historyEntry
	for _, record := range records {
		if _, duplicate := seen[record.ID]; duplicate {
			continue
		}
		seen[record.ID] = struct{}{}
		switch record.Kind {
		case RecordEdit:
			redo = nil
			undo = append(undo, historyEntry{
				EventID: record.ID,
				Op:      record.Op,
				Inverse: record.Inverse,
			})
		case RecordUndo:
			if len(undo) == 0 {
				return nil, fmt.Errorf("undo record %q has no undoable edit", record.ID)
			}
			entry := undo[len(undo)-1]
			if entry.EventID != record.Target {
				return nil, fmt.Errorf(
					"undo record %q targets %q, want %q",
					record.ID, record.Target, entry.EventID,
				)
			}
			if diff := FirstDifference(entry.Inverse, record.Op); diff != nil {
				return nil, fmt.Errorf("undo record %q operation mismatch at %s", record.ID, diff.Path)
			}
			if diff := FirstDifference(entry.Op, record.Inverse); diff != nil {
				return nil, fmt.Errorf("undo record %q inverse mismatch at %s", record.ID, diff.Path)
			}
			undo = undo[:len(undo)-1]
			redo = append(redo, entry)
		case RecordRedo:
			if len(redo) == 0 {
				return nil, fmt.Errorf("redo record %q has no undone edit", record.ID)
			}
			entry := redo[len(redo)-1]
			if entry.EventID != record.Target {
				return nil, fmt.Errorf(
					"redo record %q targets %q, want %q",
					record.ID, record.Target, entry.EventID,
				)
			}
			if diff := FirstDifference(entry.Op, record.Op); diff != nil {
				return nil, fmt.Errorf("redo record %q operation mismatch at %s", record.ID, diff.Path)
			}
			if diff := FirstDifference(entry.Inverse, record.Inverse); diff != nil {
				return nil, fmt.Errorf("redo record %q inverse mismatch at %s", record.ID, diff.Path)
			}
			redo = redo[:len(redo)-1]
			undo = append(undo, entry)
		default:
			return nil, fmt.Errorf("unknown record kind %q", record.Kind)
		}
	}
	return &Editor{log: log, project: *project, undo: undo, redo: redo}, nil
}
