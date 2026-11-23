package subtime

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// SRTCue is a parsed subtitle cue. Start and End are exact seconds, so an SRT
// file can be mapped repeatedly without accumulating float or millisecond
// rounding error.
type SRTCue struct {
	Number int      `json:"number"`
	Start  Rational `json:"start"`
	End    Rational `json:"end"`
	Text   string   `json:"text"`
}

// ParseSRT parses SubRip cues. Duplicate cues and duplicate timestamps are
// retained. Blank lines separate cues; the first line may be a numeric cue
// identifier or the timing line.
func ParseSRT(reader io.Reader) ([]SRTCue, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, strings.TrimSuffix(scanner.Text(), "\r"))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("subtime: reading SRT: %w", err)
	}

	var cues []SRTCue
	if len(lines) > 0 {
		lines[0] = strings.TrimPrefix(lines[0], "\ufeff")
	}
	for i := 0; i < len(lines); {
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		if i == len(lines) {
			break
		}

		number := len(cues) + 1
		lineNumber := i + 1
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, "-->") {
			parsed, err := strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("subtime: SRT line %d: expected cue number or timing line", lineNumber)
			}
			number = parsed
			i++
			if i == len(lines) {
				return nil, fmt.Errorf("subtime: SRT cue %d has no timing line", number)
			}
			line = strings.TrimSpace(lines[i])
			lineNumber = i + 1
		}

		start, end, err := parseSRTTiming(line)
		if err != nil {
			return nil, fmt.Errorf("subtime: SRT line %d: %w", lineNumber, err)
		}
		i++

		var textLines []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			textLines = append(textLines, lines[i])
			i++
		}
		cues = append(cues, SRTCue{
			Number: number,
			Start:  start,
			End:    end,
			Text:   strings.Join(textLines, "\n"),
		})
	}
	return cues, nil
}

func parseSRTTiming(line string) (Rational, Rational, error) {
	left, right, ok := strings.Cut(line, "-->")
	if !ok {
		return Rational{}, Rational{}, fmt.Errorf("invalid timing line %q", line)
	}
	right = strings.TrimSpace(right)
	if fields := strings.Fields(right); len(fields) > 0 {
		right = fields[0]
	}
	start, err := ParseClock(strings.TrimSpace(left))
	if err != nil {
		return Rational{}, Rational{}, err
	}
	end, err := ParseClock(right)
	if err != nil {
		return Rational{}, Rational{}, err
	}
	return start, end, nil
}

// SRTFormatOptions controls the only lossy operation in SRT output:
// converting exact seconds to integer milliseconds.
type SRTFormatOptions struct {
	MillisecondSnap     SnapPolicy `json:"millisecond_snap"`
	EnsureEndAfterStart bool       `json:"ensure_end_after_start"`
}

// WriteSRT writes cues with an explicit millisecond snap policy. When
// EnsureEndAfterStart is true, an out point that would collapse is advanced by
// exactly one millisecond.
func WriteSRT(writer io.Writer, cues []SRTCue, options SRTFormatOptions) error {
	if options.MillisecondSnap == SnapNone {
		return fmt.Errorf("subtime: explicit millisecond snap policy is required")
	}
	buffer := bufio.NewWriter(writer)
	for i, cue := range cues {
		startMillis, err := SnapClockMillis(cue.Start, options.MillisecondSnap)
		if err != nil {
			return fmt.Errorf("subtime: cue %d in point: %w", cue.Number, err)
		}
		endMillis, err := SnapClockMillis(cue.End, options.MillisecondSnap)
		if err != nil {
			return fmt.Errorf("subtime: cue %d out point: %w", cue.Number, err)
		}
		if endMillis <= startMillis {
			if !options.EnsureEndAfterStart {
				return fmt.Errorf(
					"subtime: cue %d collapses to %d ms; enable EnsureEndAfterStart or choose another snap policy",
					cue.Number, startMillis,
				)
			}
			endMillis = startMillis + 1
		}
		startText, _ := FormatClockMillis(startMillis)
		endText, _ := FormatClockMillis(endMillis)
		number := cue.Number
		if number == 0 {
			number = i + 1
		}
		if _, err := fmt.Fprintf(buffer, "%d\n%s --> %s\n%s\n", number, startText, endText, cue.Text); err != nil {
			return fmt.Errorf("subtime: writing SRT: %w", err)
		}
		if i != len(cues)-1 {
			if _, err := fmt.Fprintln(buffer); err != nil {
				return fmt.Errorf("subtime: writing SRT: %w", err)
			}
		}
	}
	return buffer.Flush()
}

// MappedSRTCue pairs the original cue with exact target points and the
// explicitly resolved target frames.
type MappedSRTCue struct {
	Source   SRTCue           `json:"source"`
	Mapping  IntervalMapping  `json:"mapping"`
	Resolved ResolvedInterval `json:"resolved"`
	Output   SRTCue           `json:"output"`
}

// MapSRTCues maps cues onto target frames without rounding intermediate
// values. Resolve is called with the supplied, explicit policy. Repeated
// timestamps remain separate entries.
func MapSRTCues(cues []SRTCue, sourceRate, targetRate Rate, policy IntervalPolicy) ([]MappedSRTCue, error) {
	return MapSRTCuesThrough(cues, sourceRate, []Rate{targetRate}, policy)
}

// MapSRTCuesThrough maps cues through an exact rate chain whose last rate is
// the output grid. Intermediate rates are never snapped. Repeated timestamps
// remain separate entries.
func MapSRTCuesThrough(cues []SRTCue, sourceRate Rate, path []Rate, policy IntervalPolicy) ([]MappedSRTCue, error) {
	if err := sourceRate.Validate(); err != nil {
		return nil, err
	}
	if len(path) == 0 {
		return nil, fmt.Errorf("subtime: SRT mapping path must contain a target rate")
	}
	targetRate := path[len(path)-1]
	if err := targetRate.Validate(); err != nil {
		return nil, err
	}
	for _, rate := range path {
		if err := rate.Validate(); err != nil {
			return nil, err
		}
	}
	output := make([]MappedSRTCue, len(cues))
	for i, cue := range cues {
		start, err := PointAtTime(cue.Start, sourceRate)
		if err != nil {
			return nil, fmt.Errorf("subtime: cue %d in point: %w", cue.Number, err)
		}
		end, err := PointAtTime(cue.End, sourceRate)
		if err != nil {
			return nil, fmt.Errorf("subtime: cue %d out point: %w", cue.Number, err)
		}
		source := Interval{ID: strconv.Itoa(cue.Number), Start: start, End: end}
		projected, err := ProjectInterval(source, path...)
		if err != nil {
			return nil, fmt.Errorf("subtime: cue %d: %w", cue.Number, err)
		}
		mapping, err := MapInterval(projected, targetRate)
		if err != nil {
			return nil, fmt.Errorf("subtime: cue %d: %w", cue.Number, err)
		}
		mapping.Source = source
		resolved, err := mapping.Resolve(policy)
		if err != nil {
			return nil, fmt.Errorf("subtime: cue %d: %w", cue.Number, err)
		}
		startSeconds, err := resolved.Start.Seconds()
		if err != nil {
			return nil, err
		}
		endSeconds, err := resolved.End.Seconds()
		if err != nil {
			return nil, err
		}
		output[i] = MappedSRTCue{
			Source:   cue,
			Mapping:  mapping,
			Resolved: resolved,
			Output: SRTCue{
				Number: cue.Number,
				Start:  startSeconds,
				End:    endSeconds,
				Text:   cue.Text,
			},
		}
	}
	return output, nil
}
