package pcm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// Build encodes chunks into a PCM container byte sequence. It is provided for
// tooling and tests; the ADM edit path never calls it on an existing map.
func Build(chunks []Chunk) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, ErrEmpty
	}

	out := make([]byte, 0, headerSize)
	out = append(out, Magic[:]...)
	out = binary.LittleEndian.AppendUint16(out, Version1)
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(chunks)))

	for _, chunk := range chunks {
		if chunk.Type != ChunkPTS && chunk.Type != ChunkRAW {
			return nil, errors.New("pcm: cannot build unknown chunk type")
		}
		out = binary.LittleEndian.AppendUint32(out, chunk.Type)
		out = binary.LittleEndian.AppendUint32(out, 0)
		out = binary.LittleEndian.AppendUint64(out, uint64(len(chunk.Data)))
		out = append(out, chunk.Data...)
	}
	sum := sha256.Sum256(out)
	out = append(out, sum[:]...)
	return out, nil
}

// WriteFile creates path with a freshly built PCM container. Editor code must
// never use this against an existing map; it exists for test fixtures.
func WriteFile(path string, chunks []Chunk) error {
	data, err := Build(chunks)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SHA256 returns the checksum of the exact on-disk bytes.
func (f *File) SHA256() [32]byte {
	return sha256.Sum256(f.raw)
}

// RawBytes returns a copy of the exact on-disk byte sequence.
func (f *File) RawBytes() []byte {
	out := make([]byte, len(f.raw))
	copy(out, f.raw)
	return out
}

// BytesEqual reports whether data is byte-for-byte identical to the bytes the
// container was parsed from.
func (f *File) BytesEqual(data []byte) bool {
	return bytes.Equal(f.raw, data)
}

// HashFile returns the SHA-256 of any file without interpreting its contents.
func HashFile(path string) ([32]byte, error) {
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
