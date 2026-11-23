package wav

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
)

const noSampleOffset int64 = -1

// ParseBytes parses an in-memory WAV file.
func ParseBytes(data []byte) (*Header, error) {
	return Parse(bytes.NewReader(data), int64(len(data)))
}

// Parse reads only the RIFF structure and format header; it never loads sample
// data and never guesses a channel layout from the amount of data available.
func Parse(r io.ReaderAt, size int64) (*Header, error) {
	h := &Header{
		FileSize: size,
		Chunks:   make([]Chunk, 0, 4),
		Issues:   make([]Issue, 0),
	}
	if size < 12 {
		h.addIssue("error", "truncated_riff_header",
			fmt.Sprintf("file is %d bytes; a RIFF/WAVE header needs at least 12", size),
			noSampleOffset)
		h.finish()
		return h, h.validationError()
	}

	riffHeader, err := readAt(r, 0, 12, size)
	if err != nil {
		h.addIssue("error", "read_error", err.Error(), noSampleOffset)
		h.finish()
		return h, h.validationError()
	}
	if string(riffHeader[0:4]) != "RIFF" {
		h.addIssue("error", "invalid_riff_id",
			fmt.Sprintf("first chunk is %q, want %q", riffHeader[0:4], "RIFF"),
			noSampleOffset)
		h.finish()
		return h, h.validationError()
	}
	if string(riffHeader[8:12]) != "WAVE" {
		h.addIssue("error", "invalid_wave_id",
			fmt.Sprintf("RIFF form type is %q, want %q", riffHeader[8:12], "WAVE"),
			noSampleOffset)
		h.finish()
		return h, h.validationError()
	}

	h.RIFFSize = binary.LittleEndian.Uint32(riffHeader[4:8])
	riffEnd := int64(8) + int64(h.RIFFSize)
	if riffEnd < 12 {
		h.addIssue("error", "invalid_riff_size",
			fmt.Sprintf("RIFF size %d ends before the WAVE form type", h.RIFFSize),
			noSampleOffset)
		riffEnd = 12
	}
	if riffEnd > size {
		h.addIssue("error", "truncated_riff",
			fmt.Sprintf("RIFF declares end offset %d but file has %d bytes", riffEnd, size),
			noSampleOffset)
		riffEnd = size
	}

	var fmtChunk *Chunk
	var fmtBytes []byte
	var dataChunk *Chunk

	pos := int64(12)
	for pos < riffEnd {
		if riffEnd-pos < 8 {
			h.addIssue("error", "truncated_chunk_header",
				fmt.Sprintf("%d trailing byte(s) cannot form a chunk header", riffEnd-pos),
				noSampleOffset)
			break
		}
		chunkHeader, readErr := readAt(r, pos, 8, size)
		if readErr != nil {
			h.addIssue("error", "read_error", readErr.Error(), noSampleOffset)
			break
		}
		id := string(chunkHeader[0:4])
		declaredSize := uint64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		dataOffset := pos + 8

		available := uint64(0)
		if dataOffset < riffEnd && dataOffset < size {
			limit := riffEnd
			if size < limit {
				limit = size
			}
			available = uint64(limit - dataOffset)
			if available > declaredSize {
				available = declaredSize
			}
		}
		chunk := Chunk{
			ID:            id,
			HeaderOffset:  pos,
			DataOffset:    dataOffset,
			DeclaredSize:  declaredSize,
			AvailableSize: available,
			Truncated:     available < declaredSize,
		}
		h.Chunks = append(h.Chunks, chunk)

		if chunk.Truncated && id != "data" {
			h.addIssue("error", "truncated_chunk",
				fmt.Sprintf("chunk %q declares %d data bytes but only %d are available",
					id, declaredSize, available),
				chunkSampleOffset(&chunk))
		}

		switch id {
		case "fmt ":
			if fmtChunk != nil {
				h.addIssue("error", "duplicate_fmt_chunk",
					"more than one fmt chunk is present", noSampleOffset)
				break
			}
			fmtChunk = &chunk
			readSize := available
			if readSize > 4096 {
				readSize = 4096
			}
			fmtBytes, readErr = readAt(r, dataOffset, int(readSize), size)
			if readErr != nil && len(fmtBytes) == 0 {
				h.addIssue("error", "read_error", readErr.Error(), noSampleOffset)
			}
		case "data":
			if dataChunk != nil {
				h.addIssue("error", "duplicate_data_chunk",
					"more than one data chunk is present", dataOffset)
				break
			}
			dataChunk = &chunk
		}

		next := dataOffset + int64(declaredSize)
		if declaredSize&1 != 0 {
			next++
		}
		if next <= pos {
			h.addIssue("error", "invalid_chunk_size",
				fmt.Sprintf("chunk %q has an invalid size %d", id, declaredSize),
				chunkSampleOffset(&chunk))
			break
		}
		pos = next
	}

	if fmtChunk == nil {
		h.addIssue("error", "missing_fmt_chunk", "no fmt chunk is present", noSampleOffset)
	} else {
		h.Format = h.parseFormat(fmtBytes, fmtChunk.DeclaredSize)
		if h.Format != nil {
			h.Extensible = h.Format.Extensible
		}
	}

	if dataChunk == nil {
		h.addIssue("error", "missing_data_chunk", "no data chunk is present", noSampleOffset)
	} else {
		h.Data = &Data{
			Offset:            dataChunk.DataOffset,
			FirstSampleOffset: dataChunk.DataOffset,
			DeclaredSize:      dataChunk.DeclaredSize,
			AvailableSize:     dataChunk.AvailableSize,
		}
		if h.Format != nil {
			h.Data.BlockAlign = h.Format.BlockAlign
		}
	}

	h.validate()
	h.finish()
	return h, h.validationError()
}

// ParseFile parses path without reading its audio payload.
func ParseFile(path string) (*Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Parse(f, info.Size())
}

func readAt(r io.ReaderAt, offset int64, length int, size int64) ([]byte, error) {
	if offset < 0 || length < 0 {
		return nil, fmt.Errorf("invalid read range offset=%d length=%d", offset, length)
	}
	if offset > size || int64(length) > size-offset {
		return nil, fmt.Errorf("read range [%d,%d) exceeds file size %d", offset, offset+int64(length), size)
	}
	buf := make([]byte, length)
	n, err := r.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return buf[:n], err
	}
	if n != length {
		return buf[:n], io.ErrUnexpectedEOF
	}
	return buf, nil
}

func chunkSampleOffset(chunk *Chunk) int64 {
	if chunk == nil || chunk.ID != "data" {
		return noSampleOffset
	}
	return chunk.DataOffset
}

func (h *Header) addIssue(severity, code, message string, firstSampleOffset int64) {
	h.Issues = append(h.Issues, Issue{
		Severity:          severity,
		Code:              code,
		Message:           message,
		FirstSampleOffset: firstSampleOffset,
	})
}

func (h *Header) validationError() error {
	if h.Valid {
		return nil
	}
	return &ValidationError{Issues: append([]Issue(nil), h.Issues...)}
}

func (h *Header) finish() {
	dataOffset := noSampleOffset
	if h.Data != nil {
		dataOffset = h.Data.FirstSampleOffset
	}
	for i := range h.Issues {
		if h.Issues[i].FirstSampleOffset == noSampleOffset {
			h.Issues[i].FirstSampleOffset = dataOffset
		}
	}
	sort.SliceStable(h.Issues, func(i, j int) bool {
		left, right := h.Issues[i], h.Issues[j]
		if left.FirstSampleOffset == noSampleOffset {
			return false
		}
		if right.FirstSampleOffset == noSampleOffset {
			return true
		}
		if left.FirstSampleOffset != right.FirstSampleOffset {
			return left.FirstSampleOffset < right.FirstSampleOffset
		}
		return left.Severity == "error" && right.Severity != "error"
	})

	h.Valid = true
	for _, issue := range h.Issues {
		if issue.Severity == "error" {
			h.Valid = false
			break
		}
	}
}
