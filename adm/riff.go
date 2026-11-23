// Package adm provides read-only parsing and cross-checking of ADM BWF
// (ITU-R BS.2076) files: RIFF chunk inventory, chna table, axml metadata
// and a timeline probe. It never renders audio and never writes to the
// inspected file.
package adm

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Chunk describes one top-level RIFF chunk.
type Chunk struct {
	ID     string
	Offset int64 // offset of chunk data within the file
	Size   uint32
}

// RIFF is the parsed container structure of a BWF file.
type RIFF struct {
	Chunks []Chunk
	Format FmtInfo
	HasFmt bool
}

// FmtInfo holds the fields of the WAV fmt chunk we care about.
type FmtInfo struct {
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
}

// ParseRIFF reads the chunk inventory and the fmt chunk from r.
// It only reads chunk headers plus the fmt payload; sample data is skipped.
func ParseRIFF(r io.ReadSeeker) (*RIFF, error) {
	var riff RIFF
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("reading RIFF header: %w", err)
	}
	if string(hdr[0:4]) != "RIFF" && string(hdr[0:4]) != "RF64" {
		return nil, fmt.Errorf("not a RIFF/RF64 file (magic %q)", string(hdr[0:4]))
	}
	if string(hdr[8:12]) != "WAVE" {
		return nil, fmt.Errorf("RIFF form type is %q, want WAVE", string(hdr[8:12]))
	}
	for {
		var ch [8]byte
		_, err := io.ReadFull(r, ch[:])
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading chunk header: %w", err)
		}
		size := binary.LittleEndian.Uint32(ch[4:8])
		pos, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		id := string(ch[0:4])
		riff.Chunks = append(riff.Chunks, Chunk{ID: id, Offset: pos, Size: size})
		if id == "fmt " && size >= 16 {
			var fb [16]byte
			if _, err := io.ReadFull(r, fb[:]); err != nil {
				return nil, fmt.Errorf("reading fmt chunk: %w", err)
			}
			riff.Format = FmtInfo{
				AudioFormat:   binary.LittleEndian.Uint16(fb[0:2]),
				NumChannels:   binary.LittleEndian.Uint16(fb[2:4]),
				SampleRate:    binary.LittleEndian.Uint32(fb[4:8]),
				ByteRate:      binary.LittleEndian.Uint32(fb[8:12]),
				BlockAlign:    binary.LittleEndian.Uint16(fb[12:14]),
				BitsPerSample: binary.LittleEndian.Uint16(fb[14:16]),
			}
			riff.HasFmt = true
			if _, err := r.Seek(int64(size)-16+pad(size), io.SeekCurrent); err != nil {
				return nil, err
			}
			continue
		}
		if _, err := r.Seek(int64(size)+pad(size), io.SeekCurrent); err != nil {
			return nil, err
		}
	}
	return &riff, nil
}

func pad(size uint32) int64 {
	if size%2 != 0 {
		return 1
	}
	return 0
}

// Find returns the first chunk with the given ID, or nil.
func (r *RIFF) Find(id string) *Chunk {
	for i := range r.Chunks {
		if r.Chunks[i].ID == id {
			return &r.Chunks[i]
		}
	}
	return nil
}

// ReadChunk returns the full payload of the first chunk with the given ID.
func (r *RIFF) ReadChunk(rs io.ReadSeeker, id string) ([]byte, error) {
	c := r.Find(id)
	if c == nil {
		return nil, fmt.Errorf("chunk %q not present", id)
	}
	if _, err := rs.Seek(c.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, c.Size)
	if _, err := io.ReadFull(rs, buf); err != nil {
		return nil, fmt.Errorf("reading %s payload: %w", id, err)
	}
	return buf, nil
}
