package adm

import (
	"errors"
	"testing"
)

func TestValidateDocument(t *testing.T) {
	base := func() *Document {
		return &Document{
			SchemaVersion: SchemaVersion,
			PCMPath:       "map.pcm",
			PCMSHA256:     "abc",
			Groups: []Group{
				{ID: "g1", Name: "One"},
				{ID: "g2", Name: "Two", ParentID: "g1"},
			},
			Objects: []Object{
				{ID: "o1", Name: "Car", GroupID: "g1", PCMRef: "geo-1",
					Points: []Point{{Time: 1}, {Time: 2}}},
			},
		}
	}

	t.Run("valid", func(t *testing.T) {
		if err := base().Validate(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("schema", func(t *testing.T) {
		d := base()
		d.SchemaVersion = 7
		if !errors.Is(d.Validate(nil), ErrSchemaVersion) {
			t.Fatal("want ErrSchemaVersion")
		}
	})
	t.Run("duplicate group", func(t *testing.T) {
		d := base()
		d.Groups = append(d.Groups, Group{ID: "g1", Name: "Dup"})
		if !errors.Is(d.Validate(nil), ErrDuplicateID) {
			t.Fatal("want ErrDuplicateID")
		}
	})
	t.Run("unknown parent", func(t *testing.T) {
		d := base()
		d.Groups[1].ParentID = "ghost"
		if !errors.Is(d.Validate(nil), ErrUnknownGroup) {
			t.Fatal("want ErrUnknownGroup")
		}
	})
	t.Run("cycle", func(t *testing.T) {
		d := base()
		d.Groups[0].ParentID = "g2"
		if !errors.Is(d.Validate(nil), ErrGroupCycle) {
			t.Fatal("want ErrGroupCycle")
		}
	})
	t.Run("unknown object group", func(t *testing.T) {
		d := base()
		d.Objects[0].GroupID = "ghost"
		if !errors.Is(d.Validate(nil), ErrUnknownGroup) {
			t.Fatal("want ErrUnknownGroup")
		}
	})
	t.Run("unknown pcm ref", func(t *testing.T) {
		d := base()
		refs := map[string]struct{}{"other": {}}
		if !errors.Is(d.Validate(refs), ErrUnknownPCMRef) {
			t.Fatal("want ErrUnknownPCMRef")
		}
	})
	t.Run("duplicate timestamp", func(t *testing.T) {
		d := base()
		d.Objects[0].Points = []Point{{Time: 5}, {Time: 5}}
		if !errors.Is(d.Validate(nil), ErrDuplicatePoint) {
			t.Fatal("want ErrDuplicatePoint")
		}
	})
	t.Run("empty name", func(t *testing.T) {
		d := base()
		d.Objects[0].Name = ""
		if !errors.Is(d.Validate(nil), ErrEmptyName) {
			t.Fatal("want ErrEmptyName")
		}
	})
}

func TestCloneIsDeep(t *testing.T) {
	d := &Document{
		SchemaVersion: SchemaVersion,
		Groups:        []Group{{ID: "g1", Name: "One"}},
		Objects: []Object{{
			ID: "o1", Name: "Car", Points: []Point{{Time: 1, X: 1}},
		}},
	}
	cp := d.Clone()
	cp.Objects[0].Points[0].X = 99
	cp.Groups[0].Name = "Changed"
	if d.Objects[0].Points[0].X != 1 || d.Groups[0].Name != "One" {
		t.Fatal("clone shares backing storage with original")
	}
}
