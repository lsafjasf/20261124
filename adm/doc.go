// Package adm implements the Annotation Data Model (ADM) document and the
// transactional edit rules used by the ADM editor page.
//
// An ADM document annotates an underlying, immutable PCM map. The PCM file is
// referenced by path and SHA-256; the editor never writes to it.
package adm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

// SchemaVersion is the ADM document schema version understood by this package.
const SchemaVersion = 1

// Point is one time-stamped position sample attached to an object.
// Time is Unix time in nanoseconds and must be strictly increasing within an
// object's point list.
type Point struct {
	Time int64   `json:"time"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
}

// Object is an editable annotation entity. Only its name, group assignment and
// position points may be changed through the editor.
type Object struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	GroupID string  `json:"groupId,omitempty"`
	Points  []Point `json:"points"`

	// PCMRef is the identifier of the source geometry inside the PCM map
	// (for example a chunk-local record id). It is immutable metadata.
	PCMRef string `json:"pcmRef"`
}

// Group organizes objects into a forest via ParentID (empty means a root).
type Group struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId,omitempty"`
}

// Document is the on-disk ADM JSON model.
type Document struct {
	SchemaVersion int      `json:"schemaVersion"`
	PCMPath       string   `json:"pcmPath"`
	PCMSHA256     string   `json:"pcmSha256"`
	Groups        []Group  `json:"groups"`
	Objects       []Object `json:"objects"`
}

// Validation errors returned for malformed documents or rejected edits.
var (
	ErrSchemaVersion  = errors.New("adm: unsupported schema version")
	ErrEmptyID        = errors.New("adm: empty id")
	ErrEmptyName      = errors.New("adm: empty name")
	ErrDuplicateID    = errors.New("adm: duplicate id")
	ErrUnknownGroup   = errors.New("adm: reference to unknown group")
	ErrGroupCycle     = errors.New("adm: group hierarchy contains a cycle")
	ErrUnknownObject  = errors.New("adm: reference to unknown object")
	ErrUnknownPCMRef  = errors.New("adm: object references unknown PCM geometry")
	ErrPointsOrder    = errors.New("adm: point timestamps must be strictly increasing")
	ErrDuplicatePoint = errors.New("adm: duplicate point timestamp")
	ErrImmutableField = errors.New("adm: field is outside the editor whitelist")
	ErrNoTransaction  = errors.New("adm: no active transaction")
	ErrPCMChanged     = errors.New("adm: underlying PCM bytes changed")
	ErrReadOnlyOutput = errors.New("adm: output target is not writable")
	ErrResidue        = errors.New("adm: stale value survived commit")
)

// LoadPath reads and parses an ADM document.
func LoadPath(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc := &Document{}
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("adm: decode %s: %w", path, err)
	}
	if err := doc.Validate(nil); err != nil {
		return nil, err
	}
	return doc, nil
}

// Clone returns a deep copy so transactions can mutate without touching the
// committed document.
func (d *Document) Clone() *Document {
	cp := *d
	cp.Groups = append([]Group(nil), d.Groups...)
	cp.Objects = make([]Object, len(d.Objects))
	for i, obj := range d.Objects {
		obj.Points = append([]Point(nil), obj.Points...)
		cp.Objects[i] = obj
	}
	return &cp
}

// Validate checks referential integrity, group hierarchy acyclicity and point
// timestamp ordering. knownPCMRefs may be nil to skip PCM reference checks.
func (d *Document) Validate(knownPCMRefs map[string]struct{}) error {
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: got %d want %d", ErrSchemaVersion, d.SchemaVersion, SchemaVersion)
	}

	groups := make(map[string]int, len(d.Groups))
	for i, g := range d.Groups {
		if g.ID == "" {
			return fmt.Errorf("%w: group at index %d", ErrEmptyID, i)
		}
		if g.Name == "" {
			return fmt.Errorf("%w: group %s", ErrEmptyName, g.ID)
		}
		if _, dup := groups[g.ID]; dup {
			return fmt.Errorf("%w: group %s", ErrDuplicateID, g.ID)
		}
		groups[g.ID] = i
	}
	for _, g := range d.Groups {
		if g.ParentID != "" {
			if _, ok := groups[g.ParentID]; !ok {
				return fmt.Errorf("%w: parent %s of group %s", ErrUnknownGroup, g.ParentID, g.ID)
			}
		}
	}
	if err := checkGroupAcyclic(d.Groups); err != nil {
		return err
	}

	objects := make(map[string]int, len(d.Objects))
	for i, obj := range d.Objects {
		if obj.ID == "" {
			return fmt.Errorf("%w: object at index %d", ErrEmptyID, i)
		}
		if obj.Name == "" {
			return fmt.Errorf("%w: object %s", ErrEmptyName, obj.ID)
		}
		if _, dup := objects[obj.ID]; dup {
			return fmt.Errorf("%w: object %s", ErrDuplicateID, obj.ID)
		}
		objects[obj.ID] = i
		if obj.GroupID != "" {
			if _, ok := groups[obj.GroupID]; !ok {
				return fmt.Errorf("%w: object %s -> group %s", ErrUnknownGroup, obj.ID, obj.GroupID)
			}
		}
		if knownPCMRefs != nil {
			if _, ok := knownPCMRefs[obj.PCMRef]; !ok {
				return fmt.Errorf("%w: object %s -> pcmRef %s", ErrUnknownPCMRef, obj.ID, obj.PCMRef)
			}
		}
		if err := checkPointOrder(obj.ID, obj.Points); err != nil {
			return err
		}
	}
	return nil
}

func checkPointOrder(objectID string, points []Point) error {
	for i := 1; i < len(points); i++ {
		if points[i].Time == points[i-1].Time {
			return fmt.Errorf("%w: object %s time %d", ErrDuplicatePoint, objectID, points[i].Time)
		}
		if points[i].Time < points[i-1].Time {
			return fmt.Errorf("%w: object %s (%d after %d)",
				ErrPointsOrder, objectID, points[i].Time, points[i-1].Time)
		}
	}
	return nil
}

func checkGroupAcyclic(groups []Group) error {
	byID := make(map[string]Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}
	// State: 0=unvisited, 1=on current path, 2=done.
	state := make(map[string]int, len(groups))
	var visit func(id string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("%w: at group %s", ErrGroupCycle, id)
		case 2:
			return nil
		}
		state[id] = 1
		if parent := byID[id].ParentID; parent != "" {
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}

	ids := make([]string, 0, len(groups))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
