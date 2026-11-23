package edl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ParseOptions controls CMX 3600 parsing. Rate is the fallback rate when the
// file has no FRAME RATE/FPS header. DefaultMode is used for colon-separated
// timecodes before an FCM header is seen; semicolon-separated timecodes always
// force DF.
type ParseOptions struct {
	Rate        Rate
	DefaultMode TimecodeMode
}

type TransitionClass string

const (
	TransitionCut      TransitionClass = "cut"
	TransitionDissolve TransitionClass = "dissolve"
	TransitionWipe     TransitionClass = "wipe"
	TransitionKey      TransitionClass = "key"
	TransitionFade     TransitionClass = "fade"
	TransitionUnknown  TransitionClass = "unknown"
)

// Transition describes the EDL transition tokens for one event.
type Transition struct {
	Code           string          `json:"code"`
	Class          TransitionClass `json:"class"`
	DurationFrames int64           `json:"durationFrames,omitempty"`
	Tokens         []string        `json:"tokens,omitempty"`
	Supported      bool            `json:"supported"`
}

// Event is one parsed EDL edit. The Timecode fields keep their original
// wrapped labels; absolute fields and timeline placement are derived later.
type Event struct {
	Number     int        `json:"number"`
	Line       int        `json:"line"`
	Reel       string     `json:"reel"`
	Track      string     `json:"track"`
	Transition Transition `json:"transition"`

	SourceIn  Timecode `json:"sourceIn"`
	SourceOut Timecode `json:"sourceOut"`
	RecordIn  Timecode `json:"recordIn"`
	RecordOut Timecode `json:"recordOut"`

	SourceInAbs  int64 `json:"sourceInAbsoluteFrame"`
	SourceOutAbs int64 `json:"sourceOutAbsoluteFrame"`
	RecordInAbs  int64 `json:"recordInAbsoluteFrame"`
	RecordOutAbs int64 `json:"recordOutAbsoluteFrame"`

	TimelineIn  int64 `json:"recordTimelineInFrame"`
	TimelineOut int64 `json:"recordTimelineOutFrame"`

	SourceDurationFrames int64 `json:"sourceDurationFrames"`
	RecordDurationFrames int64 `json:"recordDurationFrames"`
	SourceRecordOffset   int64 `json:"sourceToRecordOffsetFrames"`
	AbsoluteSRCOffset    int64 `json:"absoluteSourceToRecordOffsetFrames"`

	ClipName   string   `json:"clipName,omitempty"`
	EffectName string   `json:"effectName,omitempty"`
	Comments   []string `json:"comments,omitempty"`
}

// EDLFile is a parsed CMX 3600 file before record-order expansion.
type EDLFile struct {
	Path        string       `json:"path"`
	Title       string       `json:"title"`
	Rate        Rate         `json:"-"`
	FrameRate   string       `json:"frameRate"`
	FPS         string       `json:"fps"`
	Mode        TimecodeMode `json:"mode"`
	MixedModes  bool         `json:"mixedModes"`
	Events      []Event      `json:"events"`
	Warnings    []string     `json:"warnings,omitempty"`
	SourceLines int          `json:"sourceLines"`
	headerRate  bool
	currentMode TimecodeMode
	currentRate Rate
}

// ParseFile opens and parses one EDL file.
func ParseFile(path string, opts ParseOptions) (*EDLFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseEDL(f, path, opts)
}

// ParseEDL parses one CMX 3600 EDL.
func ParseEDL(r io.Reader, path string, opts ParseOptions) (*EDLFile, error) {
	rate := opts.Rate
	if !rate.Valid() {
		rate = Rate2997
	}
	mode := opts.DefaultMode
	if mode == "" {
		mode = NDF
	}
	f := &EDLFile{
		Path:        path,
		Rate:        rate,
		currentMode: mode,
		currentRate: rate,
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	lineNo := 0
	var last *Event
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if handled, err := parseHeader(trimmed, f); handled {
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "*") {
			if last != nil {
				parseComment(trimmed, last, f.currentRate, f.currentMode)
			}
			continue
		}
		if isEventLine(trimmed) {
			ev, err := parseEventLine(trimmed, lineNo, f.currentRate, f.currentMode)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
			f.Events = append(f.Events, ev)
			last = &f.Events[len(f.Events)-1]
			continue
		}
		f.Warnings = append(f.Warnings, fmt.Sprintf("line %d ignored: %s", lineNo, trimmed))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: read: %w", path, err)
	}
	f.SourceLines = lineNo
	if len(f.Events) == 0 {
		return nil, fmt.Errorf("%s: no EDL events found", path)
	}
	f.Rate = f.Events[0].RecordIn.Rate
	f.FrameRate = f.Rate.String()
	f.FPS = f.Rate.Decimal()
	f.Mode = dominantMode(f.Events)
	f.MixedModes = hasMixedModes(f.Events)
	return f, nil
}

func parseHeader(line string, f *EDLFile) (bool, error) {
	plain := strings.TrimSpace(strings.TrimPrefix(line, "*"))
	colon := strings.IndexByte(plain, ':')
	if colon < 0 {
		return false, nil
	}
	key := strings.ToUpper(strings.TrimSpace(plain[:colon]))
	value := strings.TrimSpace(plain[colon+1:])
	switch key {
	case "TITLE":
		if f.Title == "" {
			f.Title = value
		}
		return true, nil
	case "FCM":
		mode, err := ParseTimecodeMode(value)
		if err != nil {
			return true, err
		}
		f.currentMode = mode
		return true, nil
	case "FPS", "FRAME RATE", "FRAMERATE":
		rate, err := ParseRate(value)
		if err != nil {
			return true, err
		}
		f.currentRate = rate
		f.headerRate = true
		return true, nil
	default:
		return false, nil
	}
}

func isEventLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return false
	}
	if _, err := strconv.Atoi(fields[0]); err != nil {
		return false
	}
	count := 0
	for _, field := range fields[3:] {
		if looksLikeTimecode(field) {
			count++
		}
	}
	return count >= 4
}

func parseEventLine(line string, lineNo int, rate Rate, defaultMode TimecodeMode) (Event, error) {
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return Event{}, fmt.Errorf("event line has too few fields")
	}
	number, err := strconv.Atoi(fields[0])
	if err != nil {
		return Event{}, fmt.Errorf("event number: %w", err)
	}
	tcIndexes := make([]int, 0, 4)
	for i := 3; i < len(fields); i++ {
		if looksLikeTimecode(fields[i]) {
			tcIndexes = append(tcIndexes, i)
		}
	}
	if len(tcIndexes) != 4 {
		return Event{}, fmt.Errorf("expected four timecodes, found %d", len(tcIndexes))
	}
	first := tcIndexes[0]
	if first < 3 {
		return Event{}, fmt.Errorf("missing transition field")
	}
	tcs := make([]Timecode, 4)
	for i, idx := range tcIndexes {
		tc, err := ParseTimecode(fields[idx], rate, defaultMode)
		if err != nil {
			return Event{}, fmt.Errorf("timecode %d: %w", i+1, err)
		}
		tcs[i] = tc
	}
	return Event{
		Number:     number,
		Line:       lineNo,
		Reel:       fields[1],
		Track:      fields[2],
		Transition: parseTransition(fields[3:first]),
		SourceIn:   tcs[0],
		SourceOut:  tcs[1],
		RecordIn:   tcs[2],
		RecordOut:  tcs[3],
	}, nil
}

func parseTransition(tokens []string) Transition {
	t := Transition{Tokens: append([]string(nil), tokens...)}
	if len(tokens) == 0 {
		t.Code = "C"
		t.Class = TransitionCut
		t.Supported = true
		return t
	}
	t.Code = strings.ToUpper(tokens[0])
	for _, token := range tokens[1:] {
		if n, err := strconv.ParseInt(token, 10, 64); err == nil {
			if n >= 0 {
				t.DurationFrames = n
			}
			break
		}
	}
	switch {
	case t.Code == "C":
		t.Class = TransitionCut
	case t.Code == "D":
		t.Class = TransitionDissolve
	case strings.HasPrefix(t.Code, "W"):
		t.Class = TransitionWipe
	case t.Code == "K" || strings.HasPrefix(t.Code, "K"):
		t.Class = TransitionKey
	case t.Code == "F" || strings.HasPrefix(t.Code, "FI") || strings.HasPrefix(t.Code, "FO"):
		t.Class = TransitionFade
	default:
		t.Class = TransitionUnknown
	}
	t.Supported = t.Class == TransitionCut
	return t
}

func parseComment(line string, ev *Event, rate Rate, mode TimecodeMode) {
	ev.Comments = append(ev.Comments, line)
	plain := strings.TrimSpace(strings.TrimPrefix(line, "*"))
	colon := strings.IndexByte(plain, ':')
	if colon < 0 {
		return
	}
	key := strings.ToUpper(strings.TrimSpace(plain[:colon]))
	value := strings.TrimSpace(plain[colon+1:])
	switch key {
	case "FROM CLIP NAME", "TO CLIP NAME", "CLIP NAME":
		ev.ClipName = value
	case "EFFECT NAME":
		ev.EffectName = value
	case "DURATION":
		if ev.Transition.DurationFrames == 0 {
			if tc, err := ParseTimecode(value, rate, mode); err == nil && tc.Frames > 0 {
				ev.Transition.DurationFrames = tc.Frames
			}
		}
	}
}

func dominantMode(events []Event) TimecodeMode {
	df, ndf := 0, 0
	for _, ev := range events {
		for _, tc := range []Timecode{ev.SourceIn, ev.SourceOut, ev.RecordIn, ev.RecordOut} {
			if tc.Mode == DF {
				df++
			} else {
				ndf++
			}
		}
	}
	if df > ndf {
		return DF
	}
	return NDF
}

func hasMixedModes(events []Event) bool {
	seenDF, seenNDF := false, false
	for _, ev := range events {
		for _, tc := range []Timecode{ev.SourceIn, ev.SourceOut, ev.RecordIn, ev.RecordOut} {
			if tc.Mode == DF {
				seenDF = true
			} else {
				seenNDF = true
			}
		}
	}
	return seenDF && seenNDF
}
