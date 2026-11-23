package adm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func assertFilesIntact(t *testing.T, f fixture, admBefore, pcmBefore []byte) {
	t.Helper()
	admAfter, err := os.ReadFile(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if string(admAfter) != string(admBefore) {
		t.Fatal("ADM file changed despite rolled-back transaction")
	}
	pcmAfter, err := os.ReadFile(f.pcm)
	if err != nil {
		t.Fatal(err)
	}
	if string(pcmAfter) != string(pcmBefore) {
		t.Fatal("PCM file changed despite rolled-back transaction")
	}
}

func TestRollbackOnOutOfOrderPoints(t *testing.T) {
	f := setup(t)
	admBefore, _ := os.ReadFile(f.adm)
	pcmBefore := pcmBytes(t, f)

	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.RenameObject("o1", "Should Not Persist"); err != nil {
		t.Fatal(err)
	}
	err = tx.SetPoints("o1", []Point{
		{Time: 300, X: 1, Y: 1, Z: 1},
		{Time: 200, X: 2, Y: 2, Z: 2},
	})
	if !errors.Is(err, ErrPointsOrder) {
		t.Fatalf("SetPoints got %v want ErrPointsOrder", err)
	}

	// A second transaction: a valid rename staged together with an invalid
	// point replacement must leave nothing on disk.
	tx2, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx2.RenameObject("o1", "Nope"); err != nil {
		t.Fatal(err)
	}
	err = tx2.SetPoints("o1", []Point{
		{Time: 300, X: 1, Y: 1, Z: 1},
		{Time: 200, X: 2, Y: 2, Z: 2},
	})
	if !errors.Is(err, ErrPointsOrder) {
		t.Fatalf("SetPoints got %v want ErrPointsOrder", err)
	}
	// No accepted change means Commit is a no-op; verify disk again below.
	if err := tx2.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertFilesIntact(t, f, admBefore, pcmBefore)

	// Reopening yields the original values: no residue.
	reloaded, err := LoadPath(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if obj := findObject(t, reloaded, "o1"); obj.Name != "Car" || len(obj.Points) != 2 {
		t.Fatalf("stale data survived: %+v", obj)
	}
}

func TestRollbackOnUnknownPCMReference(t *testing.T) {
	f := setup(t)
	// Rewrite the ADM with a dangling PCM reference, then attempt an edit.
	bad, err := LoadPath(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	bad.Objects[0].PCMRef = "does-not-exist"
	if err := WriteDoc(f.adm, bad); err != nil {
		t.Fatal(err)
	}
	admBefore, _ := os.ReadFile(f.adm)
	pcmBefore := pcmBytes(t, f)

	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.RenameObject("o1", "Temp"); err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if !errors.Is(err, ErrUnknownPCMRef) {
		t.Fatalf("got %v want ErrUnknownPCMRef", err)
	}
	assertFilesIntact(t, f, admBefore, pcmBefore)
}

func TestRollbackOnGroupCycle(t *testing.T) {
	f := setup(t)
	admBefore, _ := os.ReadFile(f.adm)
	pcmBefore := pcmBytes(t, f)

	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	// g2 currently has parent g1; make g1 a child of g2 -> cycle.
	err = tx.SetGroupParent("g1", "g2")
	if !errors.Is(err, ErrGroupCycle) {
		t.Fatalf("got %v want ErrGroupCycle", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit with no effective edits should succeed: %v", err)
	}
	assertFilesIntact(t, f, admBefore, pcmBefore)
}

func TestRollbackOnReadOnlyOutput(t *testing.T) {
	f := setup(t)
	admBefore, _ := os.ReadFile(f.adm)
	pcmBefore := pcmBytes(t, f)

	if err := os.Chmod(f.adm, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f.adm, 0o644) })

	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.RenameObject("o1", "Blocked"); err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if !errors.Is(err, ErrReadOnlyOutput) {
		t.Fatalf("got %v want ErrReadOnlyOutput", err)
	}
	assertFilesIntact(t, f, admBefore, pcmBefore)

	// No temp files left behind in the directory.
	entries, err := os.ReadDir(filepath.Dir(f.adm))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temp file leaked: %s", e.Name())
		}
	}
}

func TestRollbackOnPCMDrift(t *testing.T) {
	f := setup(t)
	admBefore, _ := os.ReadFile(f.adm)

	// Tamper with the PCM after Begin has validated the hash.
	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	pcmData, err := os.ReadFile(f.pcm)
	if err != nil {
		t.Fatal(err)
	}
	pcmData[headerSizeInTest()] ^= 0x01
	if err := os.WriteFile(f.pcm, pcmData, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { /* PCM tamper is the point of the test */ })

	if err := tx.RenameObject("o1", "Blocked"); err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if !errors.Is(err, ErrPCMChanged) {
		t.Fatalf("got %v want ErrPCMChanged", err)
	}
	admAfter, _ := os.ReadFile(f.adm)
	if string(admAfter) != string(admBefore) {
		t.Fatal("ADM changed despite PCM drift")
	}
}

func TestBeginRejectsHashMismatch(t *testing.T) {
	f := setup(t)
	pcmData, _ := os.ReadFile(f.pcm)
	pcmData[headerSizeInTest()] ^= 0x01
	if err := os.WriteFile(f.pcm, pcmData, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Begin(f.adm); !errors.Is(err, ErrPCMChanged) {
		t.Fatalf("got %v want ErrPCMChanged", err)
	}
}

func TestImmutableFieldRejected(t *testing.T) {
	f := setup(t)
	admBefore, _ := os.ReadFile(f.adm)
	pcmBefore := pcmBytes(t, f)

	tx, err := Begin(f.adm)
	if err != nil {
		t.Fatal(err)
	}
	tx.Document().SchemaVersion = 99
	err = tx.Commit()
	if !errors.Is(err, ErrImmutableField) {
		t.Fatalf("got %v want ErrImmutableField", err)
	}
	assertFilesIntact(t, f, admBefore, pcmBefore)
}
