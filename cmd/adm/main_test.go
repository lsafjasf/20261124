package main

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func fileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		t.Fatal(err)
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func TestCLIEndToEnd(t *testing.T) {
	dir := t.TempDir()
	pcmPath := filepath.Join(dir, "map.pcm")
	admPath := filepath.Join(dir, "doc.json")

	if err := cmdInit([]string{pcmPath, admPath, "o1", "geo-1"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	pcmBefore := fileHash(t, pcmPath)

	// Successful transaction across all whitelisted edit kinds.
	if err := cmdRename([]string{admPath, "o1", "Van"}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := cmdAddPoint([]string{admPath, "o1", "150", "1", "2", "3"}); err != nil {
		t.Fatalf("add-point: %v", err)
	}
	if err := cmdSetGroup([]string{admPath, "o1", "-"}); err != nil {
		t.Fatalf("set-group: %v", err)
	}

	if got := fileHash(t, pcmPath); got != pcmBefore {
		t.Fatal("PCM changed after successful edits")
	}

	// Failing transaction: out-of-order points roll back, ADM stays intact.
	admBefore, err := os.ReadFile(admPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmdSetPoints([]string{
		admPath, "o1", "900:1:1:1,200:2:2:2",
	}); err == nil {
		t.Fatal("expected ErrPointsOrder")
	}
	admAfter, err := os.ReadFile(admPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(admBefore) != string(admAfter) {
		t.Fatal("ADM residue after rolled-back CLI transaction")
	}
	if got := fileHash(t, pcmPath); got != pcmBefore {
		t.Fatal("PCM changed after rolled-back transaction")
	}

	if err := cmdVerify([]string{admPath}); err != nil {
		t.Fatalf("verify: %v", err)
	}
}
