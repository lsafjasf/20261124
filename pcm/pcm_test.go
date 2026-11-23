package pcm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"
)

func sampleChunks() []Chunk {
	return []Chunk{
		{Type: ChunkPTS, Data: EncodePTS([]Sample{
			{Ref: "geo-1", X: 1, Y: 2, Z: 3},
			{Ref: "geo-2", X: 4, Y: 5, Z: 6},
		})},
		{Type: ChunkRAW, Data: []byte{0xde, 0xad, 0xbe, 0xef}},
	}
}

func TestBuildParseRoundTrip(t *testing.T) {
	data, err := Build(sampleChunks())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Version != Version1 || len(f.Chunks) != 2 {
		t.Fatalf("unexpected header: %+v", f)
	}
	samples, err := ParsePTS(f.Chunks[0].Data)
	if err != nil {
		t.Fatalf("ParsePTS: %v", err)
	}
	if samples[1].Ref != "geo-2" || samples[1].Z != 6 {
		t.Fatalf("bad sample: %+v", samples[1])
	}
	refs, err := f.GeometryRefs()
	if err != nil {
		t.Fatalf("GeometryRefs: %v", err)
	}
	if _, ok := refs["geo-1"]; !ok {
		t.Fatal("missing geo-1 ref")
	}
	if !f.BytesEqual(data) {
		t.Fatal("raw bytes not retained exactly")
	}
}

func TestParseRejectsCorruption(t *testing.T) {
	good, err := Build(sampleChunks())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("truncated", func(t *testing.T) {
		if _, err := Parse(good[:headerSize]); !errors.Is(err, ErrTruncated) {
			t.Fatalf("got %v want ErrTruncated", err)
		}
	})
	t.Run("bad magic", func(t *testing.T) {
		bad := bytes.Clone(good)
		bad[0] = 'X'
		if _, err := Parse(bad); !errors.Is(err, ErrBadMagic) {
			t.Fatalf("got %v want ErrBadMagic", err)
		}
	})
	t.Run("hash mismatch", func(t *testing.T) {
		bad := bytes.Clone(good)
		bad[headerSize] ^= 0xff
		if _, err := Parse(bad); !errors.Is(err, ErrHashMismatch) {
			t.Fatalf("got %v want ErrHashMismatch", err)
		}
	})
	t.Run("trailing bytes", func(t *testing.T) {
		bad := make([]byte, 0, len(good)+1)
		bad = append(bad, good[:len(good)-hashSize]...)
		bad = append(bad, 0)
		sum := sha256.Sum256(bad)
		bad = append(bad, sum[:]...)
		if _, err := Parse(bad); !errors.Is(err, ErrTrailingBytes) {
			t.Fatalf("got %v want ErrTrailingBytes", err)
		}
	})
	t.Run("unknown chunk type", func(t *testing.T) {
		bad := bytes.Clone(good)
		binary.LittleEndian.PutUint32(bad[headerSize:], 99)
		// recompute hash so the parser reaches chunk validation
		sum := sha256.Sum256(bad[:len(bad)-hashSize])
		copy(bad[len(bad)-hashSize:], sum[:])
		if _, err := Parse(bad); !errors.Is(err, ErrUnknownChunk) {
			t.Fatalf("got %v want ErrUnknownChunk", err)
		}
	})
}

func TestBuildRejectsEmptyAndUnknownType(t *testing.T) {
	if _, err := Build(nil); !errors.Is(err, ErrEmpty) {
		t.Fatalf("got %v want ErrEmpty", err)
	}
	if _, err := Build([]Chunk{{Type: 99}}); err == nil {
		t.Fatal("expected error for unknown chunk type")
	}
}

func TestPTSRejectsMalformed(t *testing.T) {
	if _, err := ParsePTS(nil); !errors.Is(err, ErrBadPTS) {
		t.Fatalf("got %v want ErrBadPTS", err)
	}
	good := EncodePTS([]Sample{{Ref: "r", X: 1, Y: 2, Z: 3}})
	if _, err := ParsePTS(good[:len(good)-3]); !errors.Is(err, ErrBadPTS) {
		t.Fatalf("got %v want ErrBadPTS", err)
	}
}
