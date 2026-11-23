package main

import (
	"bytes"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxInt64 = int64(1<<63 - 1)
	minInt64 = int64(-1 << 63)
)

var timingPattern = regexp.MustCompile(`^\{([+-]?[0-9]+)\}\{([+-]?[0-9]+)\}(.*)$`)

// Convert parses MicroDVD data and converts every frame timestamp to
// milliseconds using fps. Invalid UTF-8 aborts the conversion; syntax and
// timeline problems are reported as issues while conversion continues.
func Convert(data []byte, fps float64) (*Result, error) {
	if math.IsNaN(fps) || math.IsInf(fps, 0) || fps <= 0 {
		return nil, errors.New("fps must be a finite number greater than zero")
	}
	if !utf8.Valid(data) {
		offset := firstInvalidUTF8(data)
		return nil, &EncodingError{
			Offset: offset,
			Line:   bytes.Count(data[:offset], []byte{'\n'}) + 1,
		}
	}

	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	result := &Result{
		FPS:    fps,
		Cues:   make([]Cue, 0),
		Issues: make([]Issue, 0),
	}

	for lineIndex, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lineNumber := lineIndex + 1
		match := timingPattern.FindStringSubmatch(line)
		if match == nil {
			result.Issues = append(result.Issues, Issue{
				Code:    "invalid_timing",
				Message: "cue does not start with {start_frame}{end_frame}",
				Line:    lineNumber,
			})
			continue
		}

		startFrame, startErr := strconv.ParseInt(match[1], 10, 64)
		endFrame, endErr := strconv.ParseInt(match[2], 10, 64)
		if startErr != nil || endErr != nil {
			result.Issues = append(result.Issues, Issue{
				Code:    "invalid_timing",
				Message: "frame number is outside the int64 range",
				Line:    lineNumber,
			})
			continue
		}

		startMS, startOK := frameToMilliseconds(startFrame, fps)
		endMS, endOK := frameToMilliseconds(endFrame, fps)
		if !startOK || !endOK {
			result.Issues = append(result.Issues, Issue{
				Code:    "time_overflow",
				Message: "frame timestamp cannot be represented as milliseconds",
				Line:    lineNumber,
			})
			continue
		}

		rawText := match[3]
		stylePrefix, styleTags, remaining := parseStylePrefix(rawText)
		cue := Cue{
			Number:      len(result.Cues) + 1,
			Line:        lineNumber,
			StartFrame:  startFrame,
			EndFrame:    endFrame,
			StartMS:     startMS,
			EndMS:       endMS,
			StylePrefix: stylePrefix,
			StyleTags:   styleTags,
			RawText:     rawText,
			Text:        strings.ReplaceAll(remaining, "|", "\n"),
			RawLine:     line,
		}
		duration, durationOK := intervalDuration(startMS, endMS)
		if durationOK {
			cue.DurationMS = duration
		}

		if startFrame < 0 || endFrame < 0 || startMS < 0 || endMS < 0 {
			cue.Issues = append(cue.Issues, Issue{
				Code:    "negative_time",
				Message: "cue contains a negative frame or millisecond timestamp",
				Line:    lineNumber,
				Cue:     cue.Number,
			})
		}
		if endFrame < startFrame || endMS < startMS {
			cue.Issues = append(cue.Issues, Issue{
				Code:    "negative_interval",
				Message: "cue end precedes its start",
				Line:    lineNumber,
				Cue:     cue.Number,
			})
		}
		if !durationOK {
			cue.Issues = append(cue.Issues, Issue{
				Code:    "duration_overflow",
				Message: "cue duration cannot be represented as int64 milliseconds",
				Line:    lineNumber,
				Cue:     cue.Number,
			})
		}
		result.Issues = append(result.Issues, cue.Issues...)
		result.Cues = append(result.Cues, cue)
	}

	detectOverlaps(result)
	result.CueCount = len(result.Cues)
	result.IssueCount = len(result.Issues)
	return result, nil
}

func firstInvalidUTF8(data []byte) int {
	for offset := 0; offset < len(data); {
		r, size := utf8.DecodeRune(data[offset:])
		if r == utf8.RuneError && size == 1 {
			return offset
		}
		offset += size
	}
	return len(data)
}

func frameToMilliseconds(frame int64, fps float64) (int64, bool) {
	ms := math.Round(float64(frame) * 1000 / fps)
	if math.IsNaN(ms) || math.IsInf(ms, 0) {
		return 0, false
	}
	const int64Limit = float64(1 << 63)
	if ms >= int64Limit || ms < -int64Limit {
		return 0, false
	}
	return int64(ms), true
}

func intervalDuration(start, end int64) (int64, bool) {
	if end < start {
		if start > 0 && end < minInt64+start {
			return 0, false
		}
		return end - start, true
	}
	if start < 0 && end > maxInt64+start {
		return 0, false
	}
	return end - start, true
}

func parseStylePrefix(text string) (string, []StyleTag, string) {
	offset := 0
	tags := make([]StyleTag, 0)
	for offset < len(text) && text[offset] == '{' {
		end := strings.IndexByte(text[offset+1:], '}')
		if end < 0 {
			break
		}
		end += offset + 1
		body := text[offset+1 : end]
		colon := strings.IndexByte(body, ':')
		if colon != 1 || !isASCIILetter(body[0]) {
			break
		}
		tags = append(tags, StyleTag{
			Name:  body[:1],
			Value: body[2:],
			Raw:   text[offset : end+1],
		})
		offset = end + 1
	}
	if len(tags) == 0 {
		return "", nil, text
	}
	return text[:offset], tags, text[offset:]
}

func isASCIILetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
