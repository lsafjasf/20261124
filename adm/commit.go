package adm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// snapshot records the pre-edit values that must not survive a commit.
type snapshot struct {
	names   map[string]string
	groups  map[string]string // objectID -> groupID
	parents map[string]string // groupID -> parentID
	points  map[string][]Point
}

func takeSnapshot(d *Document) snapshot {
	s := snapshot{
		names:   make(map[string]string, len(d.Objects)),
		groups:  make(map[string]string, len(d.Objects)),
		parents: make(map[string]string, len(d.Groups)),
		points:  make(map[string][]Point, len(d.Objects)),
	}
	for _, obj := range d.Objects {
		s.names[obj.ID] = obj.Name
		s.groups[obj.ID] = obj.GroupID
		s.points[obj.ID] = append([]Point(nil), obj.Points...)
	}
	for _, g := range d.Groups {
		s.parents[g.ID] = g.ParentID
	}
	return s
}

// Commit validates the working copy and persists it atomically. Any failure
// rolls the transaction back and leaves both ADM and PCM files byte-identical
// to their pre-transaction state.
func (t *Transaction) Commit() error {
	if !t.active {
		return ErrNoTransaction
	}

	// 1. Immutable metadata must be identical before any other check, so an
	// out-of-whitelist edit can never be masked by a validation error.
	if err := t.checkImmutable(); err != nil {
		return t.abort(err)
	}

	// 2. Full validation, including point ordering, references and cycles.
	if err := t.working.Validate(t.pcmRefs); err != nil {
		return t.abort(err)
	}

	// 3. Read-only output must abort before any file is touched.
	if err := ensureWritable(t.admPath); err != nil {
		return t.abort(err)
	}

	// 4. Re-hash the PCM straight from disk: zero tolerance for drift.
	diskSum, err := hashPCM(t.original.PCMPath)
	if err != nil {
		return t.abort(err)
	}
	if diskSum != t.original.PCMSHA256 {
		return t.abort(ErrPCMChanged)
	}

	old := takeSnapshot(t.original)
	committedChanges := t.changes
	encoded, err := json.MarshalIndent(t.working, "", "  ")
	if err != nil {
		return t.abort(err)
	}
	encoded = append(encoded, '\n')

	// 5. Write to a sibling temp file, fsync, then atomically rename.
	if err := t.atomicWrite(encoded); err != nil {
		return t.abort(err)
	}

	// 6. Re-parse from disk and verify no stale value remains.
	if err := t.verifyPostCommit(old); err != nil {
		return t.abort(err)
	}

	t.changes = committedChanges
	t.active = false
	return nil
}

func (t *Transaction) abort(cause error) error {
	_ = t.Rollback()
	return cause
}

func (t *Transaction) checkImmutable() error {
	if t.working.SchemaVersion != t.original.SchemaVersion {
		return fmt.Errorf("%w: schemaVersion", ErrImmutableField)
	}
	if t.working.PCMPath != t.original.PCMPath {
		return fmt.Errorf("%w: pcmPath", ErrImmutableField)
	}
	if t.working.PCMSHA256 != t.original.PCMSHA256 {
		return fmt.Errorf("%w: pcmSha256", ErrImmutableField)
	}
	if len(t.working.Objects) != len(t.original.Objects) {
		return fmt.Errorf("%w: object set cannot be added or removed", ErrImmutableField)
	}
	if len(t.working.Groups) != len(t.original.Groups) {
		return fmt.Errorf("%w: group set cannot be added or removed", ErrImmutableField)
	}

	origByID := make(map[string]Object, len(t.original.Objects))
	for _, obj := range t.original.Objects {
		origByID[obj.ID] = obj
	}
	for _, obj := range t.working.Objects {
		prev, ok := origByID[obj.ID]
		if !ok {
			return fmt.Errorf("%w: new object %s", ErrImmutableField, obj.ID)
		}
		if obj.PCMRef != prev.PCMRef {
			return fmt.Errorf("%w: pcmRef of %s", ErrImmutableField, obj.ID)
		}
	}

	origGroups := make(map[string]Group, len(t.original.Groups))
	for _, g := range t.original.Groups {
		origGroups[g.ID] = g
	}
	for _, g := range t.working.Groups {
		prev, ok := origGroups[g.ID]
		if !ok || g.Name != prev.Name {
			return fmt.Errorf("%w: group identity/name %s", ErrImmutableField, g.ID)
		}
	}
	return nil
}

func (t *Transaction) atomicWrite(data []byte) error {
	dir := filepath.Dir(t.admPath)
	tmp, err := os.CreateTemp(dir, ".adm-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: create temp: %v", ErrReadOnlyOutput, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, t.admPath); err != nil {
		cleanup()
		return fmt.Errorf("%w: rename: %v", ErrReadOnlyOutput, err)
	}
	return nil
}

func (t *Transaction) verifyPostCommit(old snapshot) error {
	reloaded, err := LoadPath(t.admPath)
	if err != nil {
		return err
	}

	onDisk, err := os.ReadFile(t.admPath)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(reloaded, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if !bytes.Equal(encoded, onDisk) {
		return fmt.Errorf("%w: on-disk document not canonical", ErrResidue)
	}

	wantByName := make(map[string]Object, len(t.working.Objects))
	for _, obj := range t.working.Objects {
		wantByName[obj.ID] = obj
	}
	for _, got := range reloaded.Objects {
		want := wantByName[got.ID]
		if oldName := old.names[got.ID]; oldName != want.Name {
			if got.Name != want.Name || got.Name == oldName {
				return fmt.Errorf("%w: old name of %s", ErrResidue, got.ID)
			}
		}
		if oldGroup := old.groups[got.ID]; oldGroup != want.GroupID {
			if got.GroupID != want.GroupID || got.GroupID == oldGroup {
				return fmt.Errorf("%w: old group link of %s", ErrResidue, got.ID)
			}
		}
		if oldPoints := old.points[got.ID]; !pointsEqual(oldPoints, want.Points) {
			if !pointsEqual(got.Points, want.Points) || pointsEqual(got.Points, oldPoints) {
				return fmt.Errorf("%w: old points of %s", ErrResidue, got.ID)
			}
		}
	}
	wantParents := make(map[string]Group, len(t.working.Groups))
	for _, g := range t.working.Groups {
		wantParents[g.ID] = g
	}
	for _, got := range reloaded.Groups {
		want := wantParents[got.ID]
		if oldParent := old.parents[got.ID]; oldParent != want.ParentID {
			if got.ParentID != want.ParentID || got.ParentID == oldParent {
				return fmt.Errorf("%w: old parent of %s", ErrResidue, got.ID)
			}
		}
	}

	pcmSum, err := hashPCM(reloaded.PCMPath)
	if err != nil {
		return err
	}
	if pcmSum != reloaded.PCMSHA256 {
		return ErrPCMChanged
	}

	t.working = reloaded
	t.original = reloaded.Clone()
	t.changes = 0
	return nil
}

func hashPCM(path string) (string, error) {
	sum, err := sha256File(path)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum[:]), nil
}

func sha256File(path string) ([32]byte, error) {
	fh, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return [32]byte{}, err
	}
	defer fh.Close()

	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
