package adm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"admeditor/pcm"
)

// Transaction applies a batch of whitelisted edits atomically. Zero value is
// not usable; construct with Begin.
type Transaction struct {
	admPath string
	pcm     *pcm.File

	original *Document
	working  *Document
	pcmRefs  map[string]struct{}

	active  bool
	changes int
}

// Begin opens the ADM document and its underlying PCM map. The PCM is opened
// read-only and its SHA-256 must match the reference stored in the document.
func Begin(admPath string) (*Transaction, error) {
	doc, err := LoadPath(admPath)
	if err != nil {
		return nil, err
	}

	pcmFile, err := pcm.OpenPath(doc.PCMPath)
	if err != nil {
		if errors.Is(err, pcm.ErrHashMismatch) {
			return nil, fmt.Errorf("%w: %v", ErrPCMChanged, err)
		}
		return nil, fmt.Errorf("adm: open PCM: %w", err)
	}
	sum := pcmFile.SHA256()
	if fmt.Sprintf("%x", sum) != doc.PCMSHA256 {
		return nil, ErrPCMChanged
	}
	refs, err := pcmFile.GeometryRefs()
	if err != nil {
		return nil, fmt.Errorf("adm: index PCM geometry: %w", err)
	}

	return &Transaction{
		admPath:  admPath,
		pcm:      pcmFile,
		original: doc,
		working:  doc.Clone(),
		pcmRefs:  refs,
		active:   true,
	}, nil
}

// Active reports whether the transaction is still open.
func (t *Transaction) Active() bool { return t.active }

// Changes returns the number of accepted whitelisted operations.
func (t *Transaction) Changes() int { return t.changes }

// Rollback discards all working-set edits and closes the transaction. The
// on-disk ADM and the PCM are guaranteed untouched.
func (t *Transaction) Rollback() error {
	if !t.active {
		return ErrNoTransaction
	}
	t.working = t.original.Clone()
	t.changes = 0
	t.active = false
	return nil
}

func (t *Transaction) requireObject(id string) (*Object, error) {
	if !t.active {
		return nil, ErrNoTransaction
	}
	for i := range t.working.Objects {
		if t.working.Objects[i].ID == id {
			return &t.working.Objects[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownObject, id)
}

func (t *Transaction) requireGroup(id string) error {
	for _, g := range t.working.Groups {
		if g.ID == id {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUnknownGroup, id)
}

// RenameObject changes an object name only.
func (t *Transaction) RenameObject(id, name string) error {
	obj, err := t.requireObject(id)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("%w: object %s", ErrEmptyName, id)
	}
	if obj.Name == name {
		return nil
	}
	obj.Name = name
	t.changes++
	return nil
}

// SetObjectGroup reassigns an object to a group. An empty groupID removes it
// from every group.
func (t *Transaction) SetObjectGroup(objectID, groupID string) error {
	obj, err := t.requireObject(objectID)
	if err != nil {
		return err
	}
	if groupID != "" {
		if err := t.requireGroup(groupID); err != nil {
			return err
		}
	}
	if obj.GroupID == groupID {
		return nil
	}
	obj.GroupID = groupID
	t.changes++
	return nil
}

// SetGroupParent changes a group's parent. An empty parentID makes it a root.
func (t *Transaction) SetGroupParent(groupID, parentID string) error {
	if !t.active {
		return ErrNoTransaction
	}
	var target *Group
	for i := range t.working.Groups {
		if t.working.Groups[i].ID == groupID {
			target = &t.working.Groups[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("%w: %s", ErrUnknownGroup, groupID)
	}
	if parentID != "" {
		if err := t.requireGroup(parentID); err != nil {
			return err
		}
	}
	if target.ParentID == parentID {
		return nil
	}
	target.ParentID = parentID
	if err := checkGroupAcyclic(t.working.Groups); err != nil {
		// Immediate rollback of this single edit on the working copy.
		target.ParentID = t.originalGroupParent(groupID)
		return err
	}
	t.changes++
	return nil
}

func (t *Transaction) originalGroupParent(groupID string) string {
	for _, g := range t.original.Groups {
		if g.ID == groupID {
			return g.ParentID
		}
	}
	return ""
}

// SetPoints replaces an object's position points. Timestamps must be strictly
// increasing. Other object fields are unaffected.
func (t *Transaction) SetPoints(objectID string, points []Point) error {
	obj, err := t.requireObject(objectID)
	if err != nil {
		return err
	}
	if err := checkPointOrder(objectID, points); err != nil {
		return err
	}
	next := append([]Point(nil), points...)
	if pointsEqual(obj.Points, next) {
		return nil
	}
	obj.Points = next
	t.changes++
	return nil
}

// AddPoint inserts a position point, keeping the list time-ordered.
func (t *Transaction) AddPoint(objectID string, p Point) error {
	obj, err := t.requireObject(objectID)
	if err != nil {
		return err
	}
	for _, existing := range obj.Points {
		if existing.Time == p.Time {
			return fmt.Errorf("%w: object %s time %d", ErrDuplicatePoint, objectID, p.Time)
		}
	}
	next := make([]Point, 0, len(obj.Points)+1)
	inserted := false
	for _, existing := range obj.Points {
		if !inserted && p.Time < existing.Time {
			next = append(next, p)
			inserted = true
		}
		next = append(next, existing)
	}
	if !inserted {
		next = append(next, p)
	}
	obj.Points = next
	t.changes++
	return nil
}

// DeletePoint removes the position point at the given timestamp.
func (t *Transaction) DeletePoint(objectID string, time int64) error {
	obj, err := t.requireObject(objectID)
	if err != nil {
		return err
	}
	next := make([]Point, 0, len(obj.Points))
	found := false
	for _, existing := range obj.Points {
		if existing.Time == time {
			found = true
			continue
		}
		next = append(next, existing)
	}
	if !found {
		return fmt.Errorf("adm: object %s has no point at time %d", objectID, time)
	}
	obj.Points = next
	t.changes++
	return nil
}

func pointsEqual(a, b []Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ensureWritable verifies the output target can be replaced before any
// mutation is persisted, so a read-only output aborts the whole transaction.
func ensureWritable(path string) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%w: directory %s: %v", ErrReadOnlyOutput, dir, err)
	}
	if info.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf("%w: directory %s not writable", ErrReadOnlyOutput, dir)
	}
	if info, err := os.Stat(path); err == nil {
		if info.Mode().Perm()&0o200 == 0 {
			return fmt.Errorf("%w: file %s is read-only", ErrReadOnlyOutput, path)
		}
		if fh, err := os.OpenFile(path, os.O_WRONLY, 0); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrReadOnlyOutput, path, err)
		} else {
			_ = fh.Close()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s: %v", ErrReadOnlyOutput, path, err)
	}
	return nil
}
