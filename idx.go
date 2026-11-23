package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Entry is one subtitle cue from the .idx time table.
type Entry struct {
	Index     int    // ordinal in the idx file (0-based)
	LangIdx   int    // language stream index (sub-stream id = 0x20+LangIdx)
	LangID    string // two-letter id from "id: xx, index: N"
	Timestamp string // raw "HH:MM:SS:mmm"
	FilePos   int64  // byte offset into the .sub file
}

// Idx holds the parsed .idx file.
type Idx struct {
	Size    [2]int // frame size, e.g. 720x480
	Palette [16][3]uint8
	Entries []Entry
}

func parseTimestamp(s string) error {
	parts := strings.Split(s, ":")
	if len(parts) != 4 {
		return fmt.Errorf("bad timestamp %q", s)
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return fmt.Errorf("bad timestamp %q", s)
		}
	}
	return nil
}

// ParseIdx reads a VobSub .idx file.
func ParseIdx(r io.Reader) (*Idx, error) {
	idx := &Idx{}
	langIdx := 0
	langID := ""
	sc := bufio.NewScanner(r)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "size":
			var w, h int
			if _, err := fmt.Sscanf(val, "%dx%d", &w, &h); err != nil {
				return nil, fmt.Errorf("idx line %d: bad size %q", lineNo, val)
			}
			idx.Size = [2]int{w, h}
		case "palette":
			fields := strings.Split(val, ",")
			if len(fields) != 16 {
				return nil, fmt.Errorf("idx line %d: palette needs 16 colors, got %d", lineNo, len(fields))
			}
			for i, f := range fields {
				var r, g, b int
				if _, err := fmt.Sscanf(strings.TrimSpace(f), "%02x%02x%02x", &r, &g, &b); err != nil {
					return nil, fmt.Errorf("idx line %d: bad palette color %q", lineNo, f)
				}
				idx.Palette[i] = [3]uint8{uint8(r), uint8(g), uint8(b)}
			}
		case "id":
			idPart, idxPart, ok := strings.Cut(val, ",")
			n := 0
			if !ok || len(strings.TrimSpace(idPart)) == 0 {
				return nil, fmt.Errorf("idx line %d: bad id %q", lineNo, val)
			}
			if _, err := fmt.Sscanf(strings.TrimSpace(idxPart), "index: %d", &n); err != nil {
				return nil, fmt.Errorf("idx line %d: bad id %q", lineNo, val)
			}
			langID, langIdx = strings.TrimSpace(idPart), n
		case "timestamp":
			// "timestamp: 00:00:01:500, filepos: 000000000"
			ts, rest, _ := strings.Cut(val, ",")
			ts = strings.TrimSpace(ts)
			if err := parseTimestamp(ts); err != nil {
				return nil, fmt.Errorf("idx line %d: %v", lineNo, err)
			}
			var posStr string
			if _, err := fmt.Sscanf(rest, " filepos: %s", &posStr); err != nil {
				return nil, fmt.Errorf("idx line %d: missing filepos", lineNo)
			}
			pos, err := strconv.ParseInt(posStr, 16, 64)
			if err != nil {
				return nil, fmt.Errorf("idx line %d: bad filepos %q", lineNo, posStr)
			}
			idx.Entries = append(idx.Entries, Entry{
				Index:     len(idx.Entries),
				LangIdx:   langIdx,
				LangID:    langID,
				Timestamp: ts,
				FilePos:   pos,
			})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(idx.Entries) == 0 {
		return nil, fmt.Errorf("idx contains no timestamp entries")
	}
	return idx, nil
}
