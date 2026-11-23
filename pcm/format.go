// Package pcm defines the binary Point Cloud Map (PCM) container format.
//
// The ADM editor never mutates PCM data: it only opens files read-only,
// parses them for validation, and verifies byte-for-byte identity through
// cryptographic hashes.
//
// On-disk layout (all integers little-endian):
//
//	magic        [4]byte   "PCM1"
//	version      uint16    currently 1
//	flags        uint16    reserved, must be zero
//	chunkCount   uint32
//	chunks[chunkCount]:
//	    chunkType uint32   (1=PTS, 2=RAW; other values rejected)
//	    reserved  uint32   must be zero
//	    length    uint64
//	    data      [length]byte
//	sha256      [32]byte  SHA-256 over every byte preceding this field
//
// PTS chunks contain encoded position samples; RAW chunks are opaque payload.
// The editor treats both as opaque byte ranges and never rewrites them.
package pcm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

// Magic is the 4-byte file signature of a PCM container.
var Magic = [4]byte{'P', 'C', 'M', '1'}

const (
	// Version1 is the only supported container version.
	Version1 uint16 = 1

	// ChunkPTS holds encoded position/timestamp samples.
	ChunkPTS uint32 = 1
	// ChunkRAW holds an opaque binary payload.
	ChunkRAW uint32 = 2

	headerSize      = 4 + 2 + 2 + 4
	chunkHeaderSize = 4 + 4 + 8
	hashSize        = sha256.Size
)

// Chunk is one opaque data block inside a PCM container.
type Chunk struct {
	Type uint32
	Data []byte
}

// File is a parsed PCM container.
type File struct {
	Version uint16
	Chunks  []Chunk

	// raw is the exact on-disk byte sequence, retained so that identity
	// checks never depend on re-encoding.
	raw []byte
}

var (
	ErrTruncated     = errors.New("pcm: truncated file")
	ErrBadMagic      = errors.New("pcm: bad magic")
	ErrUnsupported   = errors.New("pcm: unsupported version or flags")
	ErrUnknownChunk  = errors.New("pcm: unknown chunk type")
	ErrTrailingBytes = errors.New("pcm: trailing bytes after hash")
	ErrHashMismatch  = errors.New("pcm: sha256 mismatch")
	ErrEmpty         = errors.New("pcm: file has no chunks")
)

// Parse decodes a PCM container and verifies its trailing SHA-256 checksum.
func Parse(data []byte) (*File, error) {
	if len(data) < headerSize+hashSize {
		return nil, ErrTruncated
	}
	if !bytes.Equal(data[:4], Magic[:]) {
		return nil, ErrBadMagic
	}
	version := binary.LittleEndian.Uint16(data[4:6])
	flags := binary.LittleEndian.Uint16(data[6:8])
	if version != Version1 || flags != 0 {
		return nil, fmt.Errorf("%w: version=%d flags=%d", ErrUnsupported, version, flags)
	}

	wantHash := data[len(data)-hashSize:]
	gotSum := sha256.Sum256(data[:len(data)-hashSize])
	if !bytes.Equal(gotSum[:], wantHash) {
		return nil, ErrHashMismatch
	}

	chunkCount := binary.LittleEndian.Uint32(data[8:12])
	offset := headerSize
	chunks := make([]Chunk, 0, chunkCount)
	for i := uint32(0); i < chunkCount; i++ {
		if offset+chunkHeaderSize > len(data)-hashSize {
			return nil, ErrTruncated
		}
		chunkType := binary.LittleEndian.Uint32(data[offset : offset+4])
		reserved := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		length := binary.LittleEndian.Uint64(data[offset+8 : offset+16])
		if reserved != 0 || (chunkType != ChunkPTS && chunkType != ChunkRAW) {
			return nil, fmt.Errorf("%w: type=%d reserved=%d", ErrUnknownChunk, chunkType, reserved)
		}
		offset += chunkHeaderSize
		if uint64(offset)+length > uint64(len(data)-hashSize) {
			return nil, ErrTruncated
		}
		chunkData := data[offset : offset+int(length)]
		chunks = append(chunks, Chunk{Type: chunkType, Data: chunkData})
		offset += int(length)
	}
	if offset != len(data)-hashSize {
		return nil, ErrTrailingBytes
	}
	if len(chunks) == 0 {
		return nil, ErrEmpty
	}

	raw := make([]byte, len(data))
	copy(raw, data)
	return &File{Version: version, Chunks: chunks, raw: raw}, nil
}

// OpenPath opens a PCM file read-only and parses it. The file handle is
// never opened with write permission by the editor.
func OpenPath(path string) (*File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	data := make([]byte, info.Size())
	if _, err := readFull(f, data); err != nil {
		return nil, err
	}
	return Parse(data)
}

func readFull(f *os.File, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := f.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
