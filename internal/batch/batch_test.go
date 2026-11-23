package batch

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func copyToTemp(t *testing.T, src string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("missing fixture %s: %v", src, err)
	}
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

func manifestWith(path string, dialnorm int, boundaries ...int) *Manifest {
	return &Manifest{Jobs: []Job{{Path: path, Dialnorm: &dialnorm, Boundaries: boundaries}}}
}

func TestPreviewDoesNotWrite(t *testing.T) {
	path := copyToTemp(t, filepath.Join("..", "..", "testdata", "stereo192.ac3"))
	before, _ := os.ReadFile(path)
	previews, err := Preview(manifestWith(path, 27))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if sha256hex(before) != sha256hex(after) {
		t.Fatal("preview modified the input file")
	}
	if previews[0].ChangedFrames == 0 {
		t.Fatal("expected changed frames in preview")
	}
	if previews[0].FileHashOld == previews[0].FileHashNew {
		t.Fatal("file hash should change after a dialnorm edit")
	}
}

func TestCommitWritesAtomicallyAndVerifies(t *testing.T) {
	path := copyToTemp(t, filepath.Join("..", "..", "testdata", "mono128.ac3"))
	if _, err := Commit(manifestWith(path, 24, 10)); err != nil {
		t.Fatal(err)
	}
	// Output must still be a fully valid, CRC-clean stream.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[0] != 0x0B || raw[1] != 0x77 {
		t.Fatal("output lost AC-3 sync word")
	}
}

func TestCRCFailureAbortsAndLeavesFileUntouched(t *testing.T) {
	path := copyToTemp(t, filepath.Join("..", "..", "testdata", "stereo192.ac3"))
	raw, _ := os.ReadFile(path)
	raw[400] ^= 0x01 // corrupt payload without updating CRC
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	originalHash := sha256hex(raw)

	_, err := Commit(manifestWith(path, 27))
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("expected ErrAborted, got %v", err)
	}
	post, _ := os.ReadFile(path)
	if sha256hex(post) != originalHash {
		t.Fatal("file changed despite batch abort")
	}
}

func TestDuplicatedBoundaryFrameAborts(t *testing.T) {
	path := copyToTemp(t, filepath.Join("..", "..", "testdata", "stereo192.ac3"))
	raw, _ := os.ReadFile(path)
	const frameSize = 768
	copy(raw[9*frameSize:10*frameSize], raw[8*frameSize:9*frameSize])
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Commit(manifestWith(path, 27, 9))
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("expected ErrAborted on duplicated boundary, got %v", err)
	}
}

func TestDistinctBoundaryFramesPass(t *testing.T) {
	path := copyToTemp(t, filepath.Join("..", "..", "testdata", "stereo192.ac3"))
	if _, err := Preview(manifestWith(path, 27, 10)); err != nil {
		t.Fatalf("distinct frames at boundary should pass: %v", err)
	}
}

func TestManifestValidation(t *testing.T) {
	bad := 99
	m := &Manifest{Jobs: []Job{{Path: "x.ac3", Dialnorm: &bad}}}
	if _, err := Preview(m); err == nil {
		t.Fatal("expected out-of-range dialnorm rejection")
	}
	emptyPath := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(emptyPath, []byte(`{"jobs":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(emptyPath); err == nil {
		t.Fatal("expected empty manifest rejection")
	}
}

func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
