package webvtt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Parse reads WebVTT blocks, REGION settings, STYLE declarations, and cues.
func Parse(r io.Reader) (*Document, error) {
	doc := &Document{
		Regions: make(map[string]Region),
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var block []string
	lineNo := 0
	headerSeen := false
	flush := func() error {
		if len(block) == 0 {
			return nil
		}
		lines := block
		block = nil
		return parseBlock(doc, lines)
	}

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		if !headerSeen {
			if strings.TrimSpace(line) != "WEBVTT" && !strings.HasPrefix(strings.TrimSpace(line), "WEBVTT ") {
				return nil, fmt.Errorf("line %d: expected WEBVTT header", lineNo)
			}
			headerSeen = true
			continue
		}
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
			continue
		}
		block = append(block, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !headerSeen {
		return nil, errors.New("empty input: expected WEBVTT header")
	}
	if err := flush(); err != nil {
		return nil, fmt.Errorf("line %d: %w", lineNo, err)
	}

	for i := range doc.Cues {
		doc.Cues[i].Classes = cueClasses(doc.Cues[i].Text)
	}
	return doc, nil
}

func parseBlock(doc *Document, lines []string) error {
	first := strings.TrimSpace(lines[0])
	switch {
	case strings.HasPrefix(first, "NOTE"):
		return nil
	case first == "STYLE":
		return parseStyle(doc, lines[1:])
	case first == "REGION":
		region, err := parseRegion(lines[1:])
		if err != nil {
			return err
		}
		doc.Regions[region.ID] = region
		return nil
	}

	cueIndex := 0
	timingIndex := 0
	for timingIndex < len(lines) && !strings.Contains(lines[timingIndex], "-->") {
		timingIndex++
	}
	if timingIndex == len(lines) {
		return fmt.Errorf("block does not contain a cue timing line")
	}
	if timingIndex > 1 {
		return fmt.Errorf("unexpected lines before cue timing")
	}
	cueIndex = timingIndex

	cue, err := parseCue(lines, cueIndex, timingIndex)
	if err != nil {
		return err
	}
	cue.SourceIndex = len(doc.Cues)
	if cue.ID == "" {
		cue.ID = fmt.Sprintf("cue-%d", len(doc.Cues)+1)
		cue.GeneratedID = true
	}
	doc.Cues = append(doc.Cues, cue)
	return nil
}

func parseCue(lines []string, cueIndex, timingIndex int) (Cue, error) {
	cue := Cue{}
	if cueIndex == 1 {
		cue.ID = strings.TrimSpace(lines[0])
	}
	start, end, settings, err := parseTiming(strings.TrimSpace(lines[timingIndex]))
	if err != nil {
		return cue, err
	}
	cue.Start = start
	cue.End = end
	cue.Settings = settings
	cue.Text = append([]string(nil), lines[timingIndex+1:]...)
	if len(cue.Text) == 0 {
		cue.Text = []string{""}
	}
	return cue, nil
}

func parseTiming(line string) (time.Duration, time.Duration, CueSettings, error) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return 0, 0, CueSettings{}, fmt.Errorf("invalid cue timing %q", line)
	}
	start, err := parseTimestamp(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, CueSettings{}, err
	}
	right := strings.Fields(strings.TrimSpace(parts[1]))
	if len(right) == 0 {
		return 0, 0, CueSettings{}, fmt.Errorf("missing cue end time")
	}
	end, err := parseTimestamp(right[0])
	if err != nil {
		return 0, 0, CueSettings{}, err
	}
	if end < start {
		return 0, 0, CueSettings{}, fmt.Errorf("cue ends before it starts")
	}
	settings, err := parseCueSettings(right[1:])
	if err != nil {
		return 0, 0, CueSettings{}, err
	}
	return start, end, settings, nil
}

func parseTimestamp(value string) (time.Duration, error) {
	fields := strings.Split(value, ":")
	if len(fields) < 2 || len(fields) > 3 {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}
	hour := 0
	minuteField := fields[0]
	secondField := fields[1]
	if len(fields) == 3 {
		var err error
		hour, err = strconv.Atoi(fields[0])
		if err != nil || hour < 0 {
			return 0, fmt.Errorf("invalid hours in %q", value)
		}
		minuteField = fields[1]
		secondField = fields[2]
	}
	minute, err := strconv.Atoi(minuteField)
	if err != nil || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("invalid minutes in %q", value)
	}
	secParts := strings.Split(secondField, ".")
	if len(secParts) != 2 {
		return 0, fmt.Errorf("invalid seconds in %q", value)
	}
	sec, err := strconv.Atoi(secParts[0])
	if err != nil || sec < 0 || sec > 59 {
		return 0, fmt.Errorf("invalid seconds in %q", value)
	}
	if len(secParts[1]) != 3 {
		return 0, fmt.Errorf("milliseconds must have three digits in %q", value)
	}
	ms, err := strconv.Atoi(secParts[1])
	if err != nil || ms < 0 || ms > 999 {
		return 0, fmt.Errorf("invalid milliseconds in %q", value)
	}
	return time.Duration(hour)*time.Hour +
		time.Duration(minute)*time.Minute +
		time.Duration(sec)*time.Second +
		time.Duration(ms)*time.Millisecond, nil
}

// ParseTimestamp accepts WebVTT hh:mm:ss.mmm or mm:ss.mmm timestamps.
func ParseTimestamp(value string) (time.Duration, error) {
	return parseTimestamp(value)
}

func parseCueSettings(fields []string) (CueSettings, error) {
	settings := CueSettings{}
	for _, field := range fields {
		key, value, ok := strings.Cut(field, ":")
		if !ok || key == "" || value == "" {
			return settings, fmt.Errorf("invalid cue setting %q", field)
		}
		switch key {
		case "region":
			settings.Region = value
		case "line":
			settings.Line = value
		case "position":
			settings.Position = value
		case "size":
			settings.Size = value
		case "align":
			settings.Align = value
		case "vertical":
			settings.Vertical = value
		}
	}
	return settings, nil
}

func parseRegion(lines []string) (Region, error) {
	region := Region{
		Width:          100,
		Lines:          3,
		RegionAnchor:   Point{X: 0, Y: 100},
		ViewportAnchor: Point{X: 0, Y: 100},
		Defined:        true,
	}
	for _, line := range lines {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "id":
			region.ID = value
		case "width":
			parsed, err := parsePercent(value)
			if err != nil {
				return region, fmt.Errorf("region width: %w", err)
			}
			region.Width = parsed
		case "lines":
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				return region, fmt.Errorf("region lines must be a positive integer")
			}
			region.Lines = parsed
		case "regionanchor":
			parsed, err := parsePoint(value)
			if err != nil {
				return region, fmt.Errorf("regionanchor: %w", err)
			}
			region.RegionAnchor = parsed
		case "viewportanchor":
			parsed, err := parsePoint(value)
			if err != nil {
				return region, fmt.Errorf("viewportanchor: %w", err)
			}
			region.ViewportAnchor = parsed
		case "scroll":
			region.Scroll = value
		}
	}
	if region.ID == "" {
		return region, errors.New("REGION block is missing an id")
	}
	return region, nil
}

func parsePoint(value string) (Point, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return Point{}, fmt.Errorf("expected x%%,y%%")
	}
	x, err := parsePercent(strings.TrimSpace(parts[0]))
	if err != nil {
		return Point{}, err
	}
	y, err := parsePercent(strings.TrimSpace(parts[1]))
	if err != nil {
		return Point{}, err
	}
	return Point{X: x, Y: y}, nil
}

func parsePercent(value string) (float64, error) {
	if !strings.HasSuffix(value, "%") {
		return 0, fmt.Errorf("expected percentage, got %q", value)
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
	if err != nil || n < 0 || n > 100 {
		return 0, fmt.Errorf("percentage out of range %q", value)
	}
	return n, nil
}

func cueClasses(text []string) []string {
	seen := make(map[string]bool)
	var classes []string
	for _, line := range text {
		for _, attr := range strings.Split(line, "<") {
			if !strings.HasPrefix(attr, "c.") {
				continue
			}
			tag := attr
			if end := strings.IndexAny(tag, "> \t"); end >= 0 {
				tag = tag[:end]
			}
			for _, class := range strings.Split(strings.TrimPrefix(tag, "c."), ".") {
				if class != "" && !seen[class] {
					seen[class] = true
					classes = append(classes, class)
				}
			}
		}
	}
	return classes
}

func stripMarkup(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		var b strings.Builder
		inTag := false
		for _, r := range line {
			switch r {
			case '<':
				inTag = true
			case '>':
				inTag = false
			default:
				if !inTag {
					b.WriteRune(r)
				}
			}
		}
		out = append(out, strings.TrimSpace(b.String()))
	}
	return out
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}
