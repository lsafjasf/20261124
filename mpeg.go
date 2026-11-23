package vobsub

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	privateStream1 = 0xBD
	maxScanWindow  = 1 << 20
)

// SubFile is a random-access VobSub .sub program stream.
type SubFile struct {
	Path string
	Size int64
	f    *os.File
	SHA  string
}

// OpenSub opens a .sub file.
func OpenSub(path string) (*SubFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	sha, err := hashFile(path)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &SubFile{Path: path, Size: info.Size(), f: f, SHA: sha}, nil
}

// Close closes the underlying file.
func (s *SubFile) Close() error {
	if s == nil || s.f == nil {
		return nil
	}
	return s.f.Close()
}

// SPUAt extracts one SPU beginning at or shortly after filepos.
func (s *SubFile) SPUAt(filepos int64) ([]byte, error) {
	if filepos < 0 || filepos >= s.Size {
		return nil, fmt.Errorf("filepos %#x outside .sub (%d bytes)", filepos, s.Size)
	}
	window, err := s.readWindow(filepos, 96*1024)
	if err != nil {
		return nil, err
	}
	spu, found, err := findSPU(window)
	if err != nil {
		return nil, err
	}
	if found {
		return spu, nil
	}
	window, err = s.readWindow(filepos, maxScanWindow)
	if err != nil {
		return nil, err
	}
	spu, found, err = findSPU(window)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("no MPEG-PS private-stream packet in %#x..%#x", filepos, filepos+int64(len(window)))
	}
	return spu, nil
}

func (s *SubFile) readWindow(off, n int64) ([]byte, error) {
	if off+n > s.Size {
		n = s.Size - off
	}
	if n <= 0 {
		return nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, n)
	if _, err := s.f.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf, nil
}

func findSPU(data []byte) ([]byte, bool, error) {
	// Some rare tools make filepos point at the substream byte rather than
	// the MPEG pack/PES start. Accept that shape before scanning.
	if len(data) >= 6 && data[0] >= 0x20 && data[0] <= 0x3f {
		if size := int(binary.BigEndian.Uint16(data[1:3])); size >= 4 && size <= len(data)-1 {
			return append([]byte(nil), data[1:1+size]...), true, nil
		}
	}

	code := []byte{0, 0, 1, privateStream1}
	cursor := 0
	var firstErr error
	for {
		rel := bytes.Index(data[cursor:], code)
		if rel < 0 {
			return nil, false, firstErr
		}
		pos := cursor + rel
		spu, ok, err := parsePES(data[pos:])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			cursor = pos + 4
			continue
		}
		if ok {
			return spu, true, nil
		}
		cursor = pos + 4
	}
}

func parsePES(pes []byte) ([]byte, bool, error) {
	if len(pes) < 9 {
		return nil, false, nil
	}
	length := int(binary.BigEndian.Uint16(pes[4:6]))
	payload := pes[6:]
	if length > 0 && length < len(payload) {
		payload = payload[:length]
	}
	if len(payload) < 3 {
		return nil, false, nil
	}

	// MPEG-2 PES header: 10xxxxxx, flags, header length.
	if payload[0]&0xC0 == 0x80 {
		headerLen := int(payload[2])
		if 3+headerLen > len(payload) {
			return nil, false, errors.New("PES header length exceeds packet")
		}
		payload = payload[3+headerLen:]
	}
	if len(payload) < 3 {
		return nil, false, nil
	}
	start := 0
	if payload[0] >= 0x20 && payload[0] <= 0x3f {
		start = 1
	}
	if len(payload)-start < 4 {
		return nil, false, nil
	}
	size := int(binary.BigEndian.Uint16(payload[start : start+2]))
	if size < 4 {
		return nil, false, fmt.Errorf("invalid SPU size %d", size)
	}
	if start+size > len(payload) {
		return nil, false, fmt.Errorf("SPU size %d exceeds PES payload %d", size, len(payload)-start)
	}
	return append([]byte(nil), payload[start:start+size]...), true, nil
}
