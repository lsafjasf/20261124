package vobsub

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
)

// ParseIndex reads a VobSub .idx sidecar.
func ParseIndex(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	idx := &Index{Path: path}
	idx.RawSHA256, err = hashFile(path)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNo := 0
	sawSize := false
	sawPalette := false
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "size":
			w, h, err := parseSize(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
			idx.Frame = image.Pt(w, h)
			sawSize = true
		case "palette":
			if err := parsePalette(value, &idx.Palette); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
			sawPalette = true
		case "id":
			idx.LanguageID = value
		case "langidx":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid langidx %q", path, lineNo, value)
			}
			idx.Language = n
		case "timestamp":
			entry, err := parseTimestampLine(value)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
			if len(idx.Timestamps) > 0 && entry.StartMS < idx.Timestamps[len(idx.Timestamps)-1].StartMS {
				return nil, fmt.Errorf("%s:%d: timestamps are not monotonic", path, lineNo)
			}
			idx.Timestamps = append(idx.Timestamps, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if !sawSize {
		return nil, fmt.Errorf("%s: missing size", path)
	}
	if !sawPalette {
		return nil, fmt.Errorf("%s: missing palette", path)
	}
	if idx.Frame.X <= 0 || idx.Frame.Y <= 0 {
		return nil, fmt.Errorf("%s: invalid frame size %dx%d", path, idx.Frame.X, idx.Frame.Y)
	}
	if len(idx.Timestamps) == 0 {
		return nil, fmt.Errorf("%s: no timestamps", path)
	}
	return idx, nil
}

func parseSize(value string) (int, int, error) {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	wText, hText, ok := strings.Cut(value, "x")
	if !ok {
		return 0, 0, fmt.Errorf("size %q must be WIDTHxHEIGHT", value)
	}
	w, err := strconv.Atoi(wText)
	if err != nil {
		return 0, 0, fmt.Errorf("size width %q", wText)
	}
	h, err := strconv.Atoi(hText)
	if err != nil {
		return 0, 0, fmt.Errorf("size height %q", hText)
	}
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return 0, 0, fmt.Errorf("size %dx%d out of range", w, h)
	}
	return w, h, nil
}

func parsePalette(value string, dst *[16]RGB) error {
	fields := strings.Split(value, ",")
	if len(fields) < 16 {
		return fmt.Errorf("palette has %d entries, need 16", len(fields))
	}
	for i := 0; i < 16; i++ {
		text := strings.TrimSpace(fields[i])
		text = strings.TrimPrefix(text, "#")
		if len(text) != 6 {
			return fmt.Errorf("palette entry %d %q is not RRGGBB", i, text)
		}
		n, err := strconv.ParseUint(text, 16, 32)
		if err != nil {
			return fmt.Errorf("palette entry %d %q: %w", i, text, err)
		}
		dst[i] = RGB{uint8(n >> 16), uint8(n >> 8), uint8(n)}
	}
	return nil
}

func parseTimestampLine(value string) (TimestampEntry, error) {
	parts := strings.Split(value, ",")
	if len(parts) < 2 {
		return TimestampEntry{}, errors.New("timestamp needs time and filepos")
	}
	start, err := ParseMS(strings.TrimSpace(parts[0]))
	if err != nil {
		return TimestampEntry{}, err
	}
	var fileposText string
	for _, part := range parts[1:] {
		key, val, ok := strings.Cut(part, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "filepos") {
			fileposText = strings.TrimSpace(val)
			break
		}
	}
	if fileposText == "" {
		return TimestampEntry{}, errors.New("timestamp missing filepos")
	}
	fileposText = strings.TrimPrefix(strings.ToLower(fileposText), "0x")
	filepos, err := strconv.ParseInt(fileposText, 16, 64)
	if err != nil || filepos < 0 {
		return TimestampEntry{}, fmt.Errorf("invalid filepos %q", fileposText)
	}
	return TimestampEntry{StartMS: start, FilePos: filepos, Raw: value}, nil
}
