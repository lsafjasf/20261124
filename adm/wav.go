package adm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Open reads an ADM source: either a plain XML file or a BW64/RF64/WAVE
// file carrying an axml chunk.
func Open(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) >= 12 && (string(data[0:4]) == "RIFF" || string(data[0:4]) == "RF64" || string(data[0:4]) == "BW64") {
		xmlData, err := extractChunk(data, "axml")
		if err != nil {
			return nil, err
		}
		return Parse(bytes.NewReader(xmlData))
	}
	return Parse(bytes.NewReader(data))
}

func extractChunk(data []byte, id string) ([]byte, error) {
	off := 12
	for off+8 <= len(data) {
		chunkID := string(data[off : off+4])
		size := int64(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		body := off + 8
		if chunkID == id {
			end := body + int(size)
			if end > len(data) {
				end = len(data)
			}
			return data[body:end], nil
		}
		// Chunks are word-aligned.
		next := body + int(size)
		if size%2 == 1 {
			next++
		}
		if next <= off {
			break
		}
		off = next
	}
	return nil, fmt.Errorf("no %s chunk found to extract", id)
}
