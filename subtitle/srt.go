package subtitle

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// SRTCue is a millisecond-resolution subtitle event in an SRT document.
type SRTCue struct {
	Index   int
	StartMS int64
	EndMS   int64
	Text    string
}

// FormatTimestamp renders milliseconds as HH:MM:SS,mmm.
func FormatTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1000
	milli := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, milli)
}

// ParseTimestamp parses HH:MM:SS,mmm (and the dot variant).
func ParseTimestamp(s string) (int64, error) {
	s = strings.TrimSpace(strings.Replace(s, ".", ",", 1))
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	h, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	m, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	secParts := strings.Split(strings.TrimSpace(parts[2]), ",")
	if len(secParts) != 2 {
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(secParts[0]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	msText := strings.TrimSpace(secParts[1])
	ms, err := strconv.ParseInt(msText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	switch len(msText) {
	case 1:
		ms *= 100
	case 2:
		ms *= 10
	case 3:
	default:
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	return h*3_600_000 + m*60_000 + sec*1000 + ms, nil
}

// FormatSRT renders cues in deterministic SRT form.
func FormatSRT(cues []SRTCue) string {
	var b strings.Builder
	for i, cue := range cues {
		idx := cue.Index
		if idx == 0 {
			idx = i + 1
		}
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n",
			idx, FormatTimestamp(cue.StartMS), FormatTimestamp(cue.EndMS), strings.TrimSpace(cue.Text))
	}
	return b.String()
}

// ParseSRT reads a conventional SRT document.
func ParseSRT(r io.Reader) ([]SRTCue, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var blocks [][]string
	var block []string
	flush := func() {
		if len(block) == 0 {
			return
		}
		blocks = append(blocks, block)
		block = nil
	}
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "\ufeff")
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		block = append(block, line)
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	cues := make([]SRTCue, 0, len(blocks))
	for i, lines := range blocks {
		cue, err := parseSRTBlock(lines)
		if err != nil {
			return nil, fmt.Errorf("cue block %d: %w", i+1, err)
		}
		cue.Index = i + 1
		cues = append(cues, cue)
	}
	return cues, nil
}

func parseSRTBlock(lines []string) (SRTCue, error) {
	arrow := -1
	for i, line := range lines {
		if strings.Contains(line, "-->") {
			arrow = i
			break
		}
	}
	if arrow < 0 {
		return SRTCue{}, fmt.Errorf("missing --> line")
	}
	parts := strings.SplitN(lines[arrow], "-->", 2)
	start, err := ParseTimestamp(parts[0])
	if err != nil {
		return SRTCue{}, err
	}
	endFields := strings.Fields(strings.TrimSpace(parts[1]))
	if len(endFields) == 0 {
		return SRTCue{}, fmt.Errorf("missing end timestamp")
	}
	end, err := ParseTimestamp(endFields[0])
	if err != nil {
		return SRTCue{}, err
	}
	return SRTCue{
		StartMS: start,
		EndMS:   end,
		Text:    strings.Join(lines[arrow+1:], "\n"),
	}, nil
}
