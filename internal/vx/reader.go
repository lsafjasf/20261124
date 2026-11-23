package vx

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

var magic = []byte("VX1:")

const (
	blockVideo    = 'V'
	blockAudio    = 'A'
	blockSubtitle = 'S'
	blockEnd      = 'E'
)

// DecodeError records a parse-level problem without aborting the scan so the
// reviewer can still inspect every block that was readable.
type DecodeError struct {
	Offset int64
	Msg    string
}

func (e DecodeError) Error() string { return fmt.Sprintf("vx: offset %d: %s", e.Offset, e.Msg) }

// Read parses a complete VX1 stream. Structural errors are returned; frame
// payload problems are collected and attached to the returned File where
// possible via DecodeError wrapping.
func Read(r io.Reader) (*File, error) {
	br := &countReader{r: r}

	prefix := make([]byte, 5)
	if _, err := io.ReadFull(br, prefix); err != nil {
		return nil, fmt.Errorf("vx: read magic: %w", err)
	}
	if !bytes.Equal(prefix[:4], magic) {
		return nil, fmt.Errorf("vx: bad magic %q", prefix[:4])
	}
	if prefix[4] != 1 {
		return nil, fmt.Errorf("vx: unsupported version %d", prefix[4])
	}

	var headerLen uint32
	if err := binary.Read(br, binary.LittleEndian, &headerLen); err != nil {
		return nil, fmt.Errorf("vx: read header length: %w", err)
	}
	headerJSON := make([]byte, headerLen)
	if _, err := io.ReadFull(br, headerJSON); err != nil {
		return nil, fmt.Errorf("vx: read header: %w", err)
	}
	var header Header
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("vx: parse header json: %w", err)
	}
	f := &File{Header: header, HeaderJSON: headerJSON}

	plane := header.Width * header.Height
	var errs []error
	for {
		tag := make([]byte, 1)
		if _, err := io.ReadFull(br, tag); err != nil {
			return f, joinErrors(errs, err)
		}
		var length uint32
		if err := binary.Read(br, binary.LittleEndian, &length); err != nil {
			return f, joinErrors(errs, fmt.Errorf("vx: read block length: %w", err))
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(br, payload); err != nil {
			return f, joinErrors(errs, fmt.Errorf("vx: read block payload: %w", err))
		}

		switch tag[0] {
		case blockEnd:
			return f, joinErrors(errs, nil)
		case blockVideo:
			frame, err := decodeFrame(payload, plane)
			if err != nil {
				errs = append(errs, DecodeError{Offset: br.n, Msg: "video frame: " + err.Error()})
				continue
			}
			frame.Index = len(f.Frames)
			f.Frames = append(f.Frames, *frame)
		case blockAudio:
			packet, err := decodeAudio(payload)
			if err != nil {
				errs = append(errs, DecodeError{Offset: br.n, Msg: "audio packet: " + err.Error()})
				continue
			}
			packet.Index = len(f.Audio)
			f.Audio = append(f.Audio, *packet)
		case blockSubtitle:
			var sub Subtitle
			if err := json.Unmarshal(payload, &sub); err != nil {
				errs = append(errs, DecodeError{Offset: br.n, Msg: "subtitle: " + err.Error()})
				continue
			}
			f.Subtitles = append(f.Subtitles, sub)
		default:
			errs = append(errs, DecodeError{Offset: br.n, Msg: fmt.Sprintf("unknown block tag %q", tag[0])})
		}
	}
}

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	k, err := c.r.Read(p)
	c.n += int64(k)
	return k, err
}

func joinErrors(errs []error, tail error) error {
	if tail != nil {
		errs = append(errs, tail)
	}
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("%v (+%d more)", errs[0], len(errs)-1)
}
