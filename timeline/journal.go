package timeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	logFormat  = "clipreplay-log"
	logVersion = 1
)

// RecordKind separates normal edits from undo/redo records. Undo and redo are
// themselves journaled so crash recovery restores the same history state.
type RecordKind string

const (
	RecordEdit RecordKind = "edit"
	RecordUndo RecordKind = "undo"
	RecordRedo RecordKind = "redo"
)

func (kind RecordKind) valid() bool {
	return kind == RecordEdit || kind == RecordUndo || kind == RecordRedo
}

// Record is one line in the journal. Op is the operation being applied now;
// Inverse reverses only that operation. Target links undo/redo records to the
// original edit record.
type Record struct {
	Seq       uint64     `json:"seq"`
	ID        string     `json:"id"`
	Kind      RecordKind `json:"kind"`
	Target    string     `json:"target,omitempty"`
	Op        Operation  `json:"op"`
	Inverse   Operation  `json:"inverse"`
	StateHash string     `json:"state_hash"`
}

type logHeader struct {
	Format  string  `json:"format"`
	Version int     `json:"version"`
	Base    Project `json:"base"`
}

// Recovery describes repairs made while opening a journal. A truncated tail
// is the expected result of a process stopping during an append.
type Recovery struct {
	TruncatedBytes  int64 `json:"truncated_bytes"`
	RepairedNewline bool  `json:"repaired_newline"`
}

func (r Recovery) Recovered() bool {
	return r.TruncatedBytes != 0 || r.RepairedNewline
}

// ErrDuplicateEvent reports that an event ID has already been recorded. The
// first occurrence wins and a duplicate append leaves both the file and the
// in-memory record list unchanged.
var ErrDuplicateEvent = errors.New("duplicate event ID")

// ErrLogExists is returned by CreateLog instead of overwriting an existing log.
var ErrLogExists = errors.New("journal already exists")

// Log is an append-only JSON Lines journal. It owns an open file so each
// successful append is synced before the in-memory project is advanced.
type Log struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	base    Project
	records []Record
	seen    map[string]struct{}
	lastSeq uint64
}

// ReplayStep records the verified result after one journal line. Duplicate
// records are listed but do not alter the project.
type ReplayStep struct {
	Seq       uint64     `json:"seq"`
	EventID   string     `json:"event_id"`
	Kind      RecordKind `json:"kind"`
	StateHash string     `json:"state_hash"`
	Duplicate bool       `json:"duplicate,omitempty"`
}

// ReplayResult contains replay metadata only; it does not keep full project
// snapshots for each edit.
type ReplayResult struct {
	Steps           []ReplayStep `json:"steps"`
	AppliedEvents   int          `json:"applied_events"`
	DuplicateEvents int          `json:"duplicate_events"`
}

// CreateLog atomically creates a journal containing base as its sole initial
// state and no edit records. The parent directory is created when necessary.
func CreateLog(path string, base Project) (*Log, error) {
	normalized := base.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, fmt.Errorf("invalid base project: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrLogExists, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	headerData, err := json.Marshal(logHeader{Format: logFormat, Version: logVersion, Base: normalized})
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, ".clipreplay-*.tmp")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return nil, err
	}
	if _, err := tmp.Write(append(headerData, '\n')); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return nil, err
	}
	log, _, err := OpenLog(path)
	return log, err
}

// OpenLog opens a journal and repairs a partial final record if present. It
// returns the repair details so callers can log or surface crash recovery.
func OpenLog(path string) (*Log, Recovery, error) {
	var recovery Recovery
	file, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return nil, recovery, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		file.Close()
		return nil, recovery, err
	}
	header, records, validBytes, recovery, err := parseJournal(data)
	if err != nil {
		file.Close()
		return nil, recovery, err
	}
	if validBytes < int64(len(data)) {
		if err := file.Truncate(validBytes); err != nil {
			file.Close()
			return nil, recovery, err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return nil, recovery, err
		}
	}
	if recovery.RepairedNewline {
		if _, err := file.Seek(0, io.SeekEnd); err != nil {
			file.Close()
			return nil, recovery, err
		}
		if _, err := file.Write([]byte{'\n'}); err != nil {
			file.Close()
			return nil, recovery, err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return nil, recovery, err
		}
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		file.Close()
		return nil, recovery, err
	}
	log := &Log{
		path:    path,
		file:    file,
		base:    header.Base.Normalize(),
		records: records,
		seen:    make(map[string]struct{}, len(records)),
	}
	for _, record := range records {
		if _, exists := log.seen[record.ID]; !exists {
			log.seen[record.ID] = struct{}{}
		}
		if record.Seq > log.lastSeq {
			log.lastSeq = record.Seq
		}
	}
	return log, recovery, nil
}

// Base returns a copy of the journal's initial project.
func (l *Log) Base() Project {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.base.Normalize()
}

// Path returns the journal filename.
func (l *Log) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// Records returns a copy of all records in file order, including duplicate
// event IDs.
func (l *Log) Records() []Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Record, len(l.records))
	for i, record := range l.records {
		out[i] = cloneRecord(record)
	}
	return out
}

// Close closes the underlying journal file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *Log) appendRecord(id string, kind RecordKind, target string, op, inverse Operation, stateHash string) (Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return Record{}, errors.New("journal is closed")
	}
	if _, exists := l.seen[id]; exists {
		return Record{}, fmt.Errorf("%w: %s", ErrDuplicateEvent, id)
	}
	record := Record{
		Seq:       l.lastSeq + 1,
		ID:        id,
		Kind:      kind,
		Target:    target,
		Op:        cloneOperation(op),
		Inverse:   cloneOperation(inverse),
		StateHash: stateHash,
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	line, err := json.Marshal(record)
	if err != nil {
		return Record{}, fmt.Errorf("encode record: %w", err)
	}
	line = append(line, '\n')
	end, err := l.file.Seek(0, io.SeekEnd)
	if err != nil {
		return Record{}, err
	}
	written, err := l.file.Write(line)
	if err != nil || written != len(line) {
		_ = l.file.Truncate(end)
		_, _ = l.file.Seek(0, io.SeekEnd)
		if err == nil {
			err = io.ErrShortWrite
		}
		return Record{}, fmt.Errorf("append record: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		_ = l.file.Truncate(end)
		_, _ = l.file.Seek(0, io.SeekEnd)
		return Record{}, fmt.Errorf("sync record: %w", err)
	}
	l.records = append(l.records, record)
	l.seen[id] = struct{}{}
	l.lastSeq = record.Seq
	return cloneRecord(record), nil
}

func (l *Log) hasEventLocked(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, exists := l.seen[id]
	return exists
}

func (l *Log) nextEventIDLocked(prefix string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	number := l.lastSeq + 1
	for {
		id := fmt.Sprintf("%s-%d", prefix, number)
		if _, exists := l.seen[id]; !exists {
			return id
		}
		number++
	}
}

// Replay applies all records in order from the journal base. The first event
// whose operation fails, inverse mismatches, or post-state hash mismatches is
// returned as a *Difference; replay stops at that step.
func (l *Log) Replay() (*Project, ReplayResult, error) {
	l.mu.Lock()
	base := l.base.Normalize()
	records := make([]Record, len(l.records))
	for i, record := range l.records {
		records[i] = cloneRecord(record)
	}
	l.mu.Unlock()

	project := base
	result := ReplayResult{Steps: make([]ReplayStep, 0, len(records))}
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		step := ReplayStep{
			Seq:       record.Seq,
			EventID:   record.ID,
			Kind:      record.Kind,
			StateHash: record.StateHash,
		}
		if _, duplicate := seen[record.ID]; duplicate {
			step.Duplicate = true
			result.Steps = append(result.Steps, step)
			result.DuplicateEvents++
			continue
		}
		seen[record.ID] = struct{}{}
		inverse, err := project.Apply(record.Op)
		if err != nil {
			return nil, result, &Difference{
				Step:     record.Seq,
				EventID:  record.ID,
				Reason:   "operation failed: " + err.Error(),
				Expected: record.Op,
			}
		}
		if diff := FirstDifference(record.Inverse, inverse); diff != nil {
			return nil, result, &Difference{
				Step:     record.Seq,
				EventID:  record.ID,
				Path:     "inverse." + diff.Path,
				Reason:   "recorded inverse does not match replay",
				Expected: record.Inverse,
				Actual:   inverse,
			}
		}
		actualHash, err := project.StateHash()
		if err != nil {
			return nil, result, &Difference{
				Step:    record.Seq,
				EventID: record.ID,
				Reason:  "state hash failed: " + err.Error(),
			}
		}
		step.StateHash = actualHash
		if actualHash != record.StateHash {
			step.StateHash = record.StateHash
			return nil, result, &Difference{
				Step:         record.Seq,
				EventID:      record.ID,
				Reason:       "post-state hash mismatch",
				ExpectedHash: record.StateHash,
				ActualHash:   actualHash,
			}
		}
		result.Steps = append(result.Steps, step)
		result.AppliedEvents++
	}
	return &project, result, nil
}

// ReplayAndCompare replays the journal and compares the final project field by
// field. A replay divergence is returned as an error containing the first
// differing step; a final-state mismatch is returned as the second value with
// its first differing field path.
func (l *Log) ReplayAndCompare(expected Project) (Project, *Difference, error) {
	project, _, err := l.Replay()
	if err != nil {
		var difference *Difference
		if errors.As(err, &difference) {
			return Project{}, difference, err
		}
		return Project{}, nil, err
	}
	if diff := DiffProjects(expected, *project); diff != nil {
		return *project, diff, nil
	}
	return *project, nil, nil
}

func cloneRecord(record Record) Record {
	out := record
	if record.Op.Clip != nil {
		out.Op.Clip = cloneClipPointer(*record.Op.Clip)
	}
	if record.Op.Effect != nil {
		out.Op.Effect = cloneEffectPointer(*record.Op.Effect)
	}
	if record.Op.Marker != nil {
		marker := *record.Op.Marker
		out.Op.Marker = &marker
	}
	if record.Inverse.Clip != nil {
		out.Inverse.Clip = cloneClipPointer(*record.Inverse.Clip)
	}
	if record.Inverse.Effect != nil {
		out.Inverse.Effect = cloneEffectPointer(*record.Inverse.Effect)
	}
	if record.Inverse.Marker != nil {
		marker := *record.Inverse.Marker
		out.Inverse.Marker = &marker
	}
	return out
}

func parseJournal(data []byte) (logHeader, []Record, int64, Recovery, error) {
	var header logHeader
	var records []Record
	var recovery Recovery

	headerLine, next, hasNewline := nextLine(data, 0)
	if len(bytes.TrimSpace(headerLine)) == 0 {
		return header, nil, 0, recovery, errors.New("journal is empty")
	}
	if err := json.Unmarshal(headerLine, &header); err != nil {
		return header, nil, 0, recovery, fmt.Errorf("invalid journal header: %w", err)
	}
	if header.Format != logFormat {
		return header, nil, 0, recovery, fmt.Errorf("unsupported journal format %q", header.Format)
	}
	if header.Version != logVersion {
		return header, nil, 0, recovery, fmt.Errorf("unsupported journal version %d", header.Version)
	}
	header.Base = header.Base.Normalize()
	if err := header.Base.Validate(); err != nil {
		return header, nil, 0, recovery, fmt.Errorf("invalid base project: %w", err)
	}
	if !hasNewline {
		recovery.RepairedNewline = true
		return header, records, int64(len(data)), recovery, nil
	}

	offset := next
	for offset < len(data) {
		line, after, terminated := nextLine(data, offset)
		if len(bytes.TrimSpace(line)) == 0 {
			if terminated && after == len(data) {
				return header, records, int64(offset), recovery, nil
			}
			return header, nil, 0, recovery, fmt.Errorf("empty journal record at byte %d", offset)
		}
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			if !terminated {
				recovery.TruncatedBytes = int64(len(line))
				return header, records, int64(offset), recovery, nil
			}
			return header, nil, 0, recovery, fmt.Errorf("invalid journal record at byte %d: %w", offset, err)
		}
		if err := validateRecord(record); err != nil {
			if !terminated {
				recovery.TruncatedBytes = int64(len(line))
				return header, records, int64(offset), recovery, nil
			}
			return header, nil, 0, recovery, fmt.Errorf("invalid journal record at byte %d: %w", offset, err)
		}
		records = append(records, record)
		if !terminated {
			recovery.RepairedNewline = true
			return header, records, int64(len(data)), recovery, nil
		}
		offset = after
	}
	return header, records, int64(len(data)), recovery, nil
}

func cloneOperation(op Operation) Operation {
	out := op
	if op.Clip != nil {
		out.Clip = cloneClipPointer(*op.Clip)
	}
	if op.Effect != nil {
		out.Effect = cloneEffectPointer(*op.Effect)
	}
	if op.Marker != nil {
		marker := *op.Marker
		out.Marker = &marker
	}
	return out
}

func nextLine(data []byte, start int) (line []byte, next int, terminated bool) {
	if start >= len(data) {
		return nil, len(data), false
	}
	if index := bytes.IndexByte(data[start:], '\n'); index >= 0 {
		end := start + index
		return data[start:end], end + 1, true
	}
	return data[start:], len(data), false
}

func validateRecord(record Record) error {
	if record.Seq == 0 {
		return errors.New("record sequence must be positive")
	}
	if record.ID == "" {
		return errors.New("record ID is required")
	}
	if !record.Kind.valid() {
		return fmt.Errorf("invalid record kind %q", record.Kind)
	}
	if record.Kind != RecordEdit && strings.TrimSpace(record.Target) == "" {
		return errors.New("undo/redo record target is required")
	}
	if record.Op.Type == "" {
		return errors.New("record operation is required")
	}
	if record.Inverse.Type == "" {
		return errors.New("record inverse operation is required")
	}
	if record.StateHash == "" {
		return errors.New("record state hash is required")
	}
	return nil
}
