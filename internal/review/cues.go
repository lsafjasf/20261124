package review

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var srtTiming = regexp.MustCompile(
	`^\s*(\d{1,3}):(\d{2}):(\d{2})[,.](\d{1,3})\s*-->\s*(\d{1,3}):(\d{2}):(\d{2})[,.](\d{1,3})`,
)

func loadCues(path string) ([]Cue, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".srt":
		return ParseSRTFile(path)
	case ".json":
		return parseJSONCuesFile(path)
	default:
		return nil, fmt.Errorf("unsupported subtitle format %q (expected .srt or .json)", filepath.Ext(path))
	}
}

func ParseSRTFile(path string) ([]Cue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SRT: %w", err)
	}
	cues, err := ParseSRT(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse SRT %s: %w", path, err)
	}
	return cues, nil
}

// ParseSRT parses a conventional SRT stream. Cue end times are exclusive.
func ParseSRT(r io.Reader) ([]Cue, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var blocks [][]string
	var block []string
	firstLine := true
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if firstLine {
			line = strings.TrimPrefix(line, "\ufeff")
			firstLine = false
		}
		if strings.TrimSpace(line) == "" {
			if len(block) > 0 {
				blocks = append(blocks, block)
				block = nil
			}
			continue
		}
		block = append(block, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(block) > 0 {
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		return nil, errors.New("no cues")
	}

	cues := make([]Cue, 0, len(blocks))
	for i, lines := range blocks {
		cue, err := parseSRTBlock(lines, i+1)
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i+1, err)
		}
		cues = append(cues, cue)
	}
	return cues, nil
}

func parseSRTBlock(lines []string, ordinal int) (Cue, error) {
	timingIndex := -1
	for i, line := range lines {
		if srtTiming.MatchString(line) {
			timingIndex = i
			break
		}
	}
	if timingIndex < 0 {
		return Cue{}, errors.New("missing timing line")
	}
	if timingIndex == len(lines)-1 {
		return Cue{}, errors.New("missing subtitle text")
	}

	match := srtTiming.FindStringSubmatch(lines[timingIndex])
	start, err := srtTimeToMS(match[1:5])
	if err != nil {
		return Cue{}, fmt.Errorf("start time: %w", err)
	}
	end, err := srtTimeToMS(match[5:9])
	if err != nil {
		return Cue{}, fmt.Errorf("end time: %w", err)
	}
	if end <= start {
		return Cue{}, errors.New("end time must be after start time")
	}

	id := fmt.Sprintf("cue-%d", ordinal)
	if timingIndex > 0 {
		index := strings.TrimSpace(lines[timingIndex-1])
		if index != "" && !strings.Contains(index, "-->") {
			id = index
		}
	}
	text := strings.TrimSpace(strings.Join(lines[timingIndex+1:], "\n"))
	if text == "" {
		return Cue{}, errors.New("empty subtitle text")
	}
	return Cue{ID: id, StartMs: start, EndMs: end, Text: text}, nil
}

func srtTimeToMS(parts []string) (int64, error) {
	if len(parts) != 4 {
		return 0, errors.New("incomplete timecode")
	}
	hour, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	minute, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, err
	}
	second, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, err
	}
	frac, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return 0, err
	}
	switch len(parts[3]) {
	case 1:
		frac *= 100
	case 2:
		frac *= 10
	}
	if minute > 59 || second > 59 || frac > 999 {
		return 0, errors.New("timecode component out of range")
	}
	return ((hour*60+minute)*60+second)*1000 + frac, nil
}

func parseJSONCuesFile(path string) ([]Cue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read JSON cues: %w", err)
	}
	cues, err := ParseJSONCues(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse JSON cues %s: %w", path, err)
	}
	return cues, nil
}

// ParseJSONCues accepts either a cue array or an object with a "cues" field.
func ParseJSONCues(r io.Reader) ([]Cue, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	if len(data) == 0 {
		return nil, errors.New("empty JSON")
	}

	var wrapper struct {
		Cues []Cue `json:"cues"`
	}
	if data[0] == '[' {
		if err := decodeSingleJSON(data, &wrapper.Cues); err != nil {
			return nil, err
		}
	} else {
		if err := decodeSingleJSON(data, &wrapper); err != nil {
			return nil, err
		}
	}
	if len(wrapper.Cues) == 0 {
		return nil, errors.New("no cues")
	}
	for i := range wrapper.Cues {
		if strings.TrimSpace(wrapper.Cues[i].ID) == "" {
			wrapper.Cues[i].ID = fmt.Sprintf("cue-%d", i+1)
		}
	}
	return wrapper.Cues, nil
}

func decodeSingleJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return ensureEOF(dec)
}
