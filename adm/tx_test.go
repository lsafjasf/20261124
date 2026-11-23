package adm

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"admeditor/pcm"
)

type fixture struct {
	dir     string
	adm     string
	pcm     string
	pcmHash string
}

func setup(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	pcmPath := filepath.Join(dir, "map.pcm")
	admPath := filepath.Join(dir, "doc.adm.json")

	chunks := []pcm.Chunk{{
		Type: pcm.ChunkPTS,
		Data: pcm.EncodePTS([]pcm.Sample{
			{Ref: "geo-1", X: 1, Y: 2, Z: 3},
			{Ref: "geo-2", X: 4, Y: 5, Z: 6},
		}),
	}}
	if err := pcm.WriteFile(pcmPath, chunks); err != nil {
		t.Fatal(err)
	}
	pcmFile, err := pcm.OpenPath(pcmPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := pcmFile.SHA256()
	doc := &Document{
		SchemaVersion: SchemaVersion,
		PCMPath:       pcmPath,
		PCMSHA256:     hex.EncodeToString(sum[:]),
		Groups: []Group{
			{ID: "g1", Name: "Group One"},
			{ID: "g2", Name: "Group Two", ParentID: "g1"},
		},
		Objects: []Object{
			{
				ID: "o1", Name: "Car", GroupID: "g1", PCMRef: "geo-1",
				Points: []Point{
					{Time: 100, X: 1, Y: 1, Z: 1},
					{Time: 200, X: 2, Y: 2, Z: 2},
				},
			},
			{ID: "o2", Name: "Truck", GroupID: "g2", PCMRef: "geo-2"},
		},
	}
	if err := WriteDoc(admPath, doc); err != nil {
		t.Fatal(err)
	}
	return fixture{dir: dir, adm: admPath, pcm: pcmPath, pcmHash: hex.EncodeToString(sum[:])}
}

func pcmBytes(t *testing.T, f fixture) []byte {
	t.Helper()
	data, err := os.ReadFile(f.pcm)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCommitAllWhitelistedEdits(t *testing.T) {
	f := setup(t)
	before := pcmBytes(t, f)

	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.RenameObject("o1", "Sports Car"); err != nil {
		t.Fatal(err)
	}
	if err := tx.SetObjectGroup("o1", "g2"); err != nil {
		t.Fatal(err)
	}
	if err := tx.SetGroupParent("g2", ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.SetPoints("o1", []Point{
		{Time: 50, X: 0, Y: 0, Z: 0},
		{Time: 150, X: 9, Y: 8, Z: 7},
	}); err != nil {
		t.Fatal(err)
	}
	if tx.Changes() != 4 {
		t.Fatalf("Changes=%d want 4", tx.Changes())
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	reloaded, err := LoadPath(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	obj := findObject(t, reloaded, "o1")
	if obj.Name != "Sports Car" {
		t.Fatalf("name=%q", obj.Name)
	}
	if obj.GroupID != "g2" {
		t.Fatalf("group=%q", obj.GroupID)
	}
	if len(obj.Points) != 2 || obj.Points[0].Time != 50 || obj.Points[1].X != 9 {
		t.Fatalf("points=%+v", obj.Points)
	}
	g2 := findGroup(t, reloaded, "g2")
	if g2.ParentID != "" {
		t.Fatalf("parent=%q want empty", g2.ParentID)
	}
	// Immutable metadata survives.
	if obj.PCMRef != "geo-1" || reloaded.PCMSHA256 != f.pcmHash {
		t.Fatal("immutable field changed")
	}
	after := pcmBytes(t, f)
	if string(after) != string(before) {
		t.Fatal("PCM bytes changed after commit")
	}
}

func TestAddAndDeletePoint(t *testing.T) {
	f := setup(t)
	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.AddPoint("o1", Point{Time: 150, X: 5, Y: 5, Z: 5}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadPath(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	obj := findObject(t, reloaded, "o1")
	if len(obj.Points) != 3 || obj.Points[1].Time != 150 {
		t.Fatalf("points=%+v", obj.Points)
	}

	tx2, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx2.DeletePoint("o1", 100); err != nil {
		t.Fatal(err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	reloaded, _ = LoadPath(f.adm)
	obj = findObject(t, reloaded, "o1")
	if len(obj.Points) != 2 || obj.Points[0].Time != 150 {
		t.Fatalf("points after delete=%+v", obj.Points)
	}
}

func findObject(t *testing.T, d *Document, id string) Object {
	t.Helper()
	for _, obj := range d.Objects {
		if obj.ID == id {
			return obj
		}
	}
	t.Fatalf("object %s not found", id)
	return Object{}
}

func findGroup(t *testing.T, d *Document, id string) Group {
	t.Helper()
	for _, g := range d.Groups {
		if g.ID == id {
			return g
		}
	}
	t.Fatalf("group %s not found", id)
	return Group{}
}

func TestUnknownObjectAndGroupReferences(t *testing.T) {
	f := setup(t)
	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.RenameObject("missing", "x"); !errors.Is(err, ErrUnknownObject) {
		t.Fatalf("got %v want ErrUnknownObject", err)
	}
	if err := tx.SetObjectGroup("o1", "nope"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("got %v want ErrUnknownGroup", err)
	}
	if err := tx.SetGroupParent("nope", "g1"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("got %v want ErrUnknownGroup", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}
