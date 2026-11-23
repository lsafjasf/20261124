package sf2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// riffChunk describes one RIFF chunk after the outer sfbk container has been
// walked. Data is the raw chunk payload; the parser never holds more than one
// file in memory at a time.
type riffChunk struct {
	ID   string
	Data []byte
}

// Parse reads a complete SoundFont 2 file and decodes the seven tables the
// inspector needs. Missing optional chunks yield empty tables rather than an
// error; structural corruption of an existing chunk returns an error.
func Parse(r io.Reader) (*File, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read sf2: %w", err)
	}
	chunks, err := walkRIFF(raw)
	if err != nil {
		return nil, err
	}

	f := &File{}
	for _, c := range chunks {
		switch c.ID {
		case "phdr":
			if f.Presets, err = parsePHDR(c.Data); err != nil {
				return nil, err
			}
		case "pbag":
			if f.PBags, err = parseBags[PresetBag](c.Data); err != nil {
				return nil, err
			}
		case "pgen":
			if f.PGens, err = parseGens(c.Data); err != nil {
				return nil, err
			}
		case "inst":
			if f.Insts, err = parseINST(c.Data); err != nil {
				return nil, err
			}
		case "ibag":
			if f.IBags, err = parseBags[InstBag](c.Data); err != nil {
				return nil, err
			}
		case "igen":
			if f.IGens, err = parseGens(c.Data); err != nil {
				return nil, err
			}
		case "shdr":
			if f.Samples, err = parseSHDR(c.Data); err != nil {
				return nil, err
			}
		case "smpl":
			f.SmplFrames = uint32(len(c.Data)) / 2
		}
	}
	return f, nil
}

// walkRIFF validates the outer "RIFF .... sfbk" container and flattens the
// chunks nested inside LIST chunks (typically INFO, sdta, pdta).
func walkRIFF(raw []byte) ([]riffChunk, error) {
	if len(raw) < 12 {
		return nil, errors.New("file shorter than RIFF header")
	}
	if string(raw[0:4]) != "RIFF" {
		return nil, errors.New("missing RIFF header")
	}
	size := binary.LittleEndian.Uint32(raw[4:8])
	if int(size)+8 > len(raw) {
		return nil, fmt.Errorf("RIFF size %d exceeds file length %d", size, len(raw))
	}
	if string(raw[8:12]) != "sfbk" {
		return nil, fmt.Errorf("unexpected RIFF form type %q, want sfbk", string(raw[8:12]))
	}

	var out []riffChunk
	payload := raw[12 : 8+size]
	for pos := 0; pos+8 <= len(payload); {
		id := string(payload[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(payload[pos+4 : pos+8]))
		dataStart := pos + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > len(payload) {
			return nil, fmt.Errorf("chunk %q overruns RIFF container", id)
		}
		body := payload[dataStart:dataEnd]
		if id == "LIST" {
			if len(body) < 4 {
				return nil, errors.New("truncated LIST chunk")
			}
			listChunks, err := flattenLIST(body)
			if err != nil {
				return nil, err
			}
			out = append(out, listChunks...)
		} else {
			out = append(out, riffChunk{ID: id, Data: body})
		}
		// Chunks are word aligned; a single pad byte follows odd sizes.
		pos = dataEnd + chunkSize%2
	}
	return out, nil
}

func flattenLIST(body []byte) ([]riffChunk, error) {
	listType := string(body[0:4])
	var out []riffChunk
	pos := 4
	for pos+8 <= len(body) {
		id := string(body[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(body[pos+4 : pos+8]))
		dataStart := pos + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > len(body) {
			return nil, fmt.Errorf("LIST %q chunk %q overruns container", listType, id)
		}
		out = append(out, riffChunk{ID: id, Data: body[dataStart:dataEnd]})
		pos = dataEnd + chunkSize%2
	}
	return out, nil
}

func fixedString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func parsePHDR(data []byte) ([]PresetHeader, error) {
	const rec = 38
	if len(data)%rec != 0 {
		return nil, fmt.Errorf("phdr size %d is not a multiple of %d", len(data), rec)
	}
	out := make([]PresetHeader, 0, len(data)/rec)
	for off := 0; off < len(data); off += rec {
		r := data[off:]
		out = append(out, PresetHeader{
			Name:         fixedString(r[0:20]),
			Preset:       binary.LittleEndian.Uint16(r[20:22]),
			Bank:         binary.LittleEndian.Uint16(r[22:24]),
			PresetBagIdx: binary.LittleEndian.Uint16(r[24:26]),
			Library:      binary.LittleEndian.Uint32(r[26:30]),
			Genre:        binary.LittleEndian.Uint32(r[30:34]),
			Morphology:   binary.LittleEndian.Uint32(r[34:38]),
		})
	}
	return out, nil
}

func parseINST(data []byte) ([]InstHeader, error) {
	const rec = 22
	if len(data)%rec != 0 {
		return nil, fmt.Errorf("inst size %d is not a multiple of %d", len(data), rec)
	}
	out := make([]InstHeader, 0, len(data)/rec)
	for off := 0; off < len(data); off += rec {
		r := data[off:]
		out = append(out, InstHeader{
			Name:       fixedString(r[0:20]),
			InstBagIdx: binary.LittleEndian.Uint16(r[20:22]),
		})
	}
	return out, nil
}
