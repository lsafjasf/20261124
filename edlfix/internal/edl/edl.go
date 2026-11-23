package edl

import (
	"strings"

	"edlfix/internal/timecode"
)

// FCM is the frame counting mode declared by an FCM section header.
type FCM int

const (
	FCMUnspecified FCM = iota
	FCMNonDrop
	FCMDrop
)

// TokenRef points at one timecode token inside an event line.
type TokenRef struct {
	TC       timecode.Timecode
	Field    string // "SRC_IN", "SRC_OUT", "REC_IN", "REC_OUT"
	TokenIdx int    // 0..3 in field order
}

// Event is one CMX 3600 decision: its source line plus the four timecodes.
type Event struct {
	LineIndex     int // zero-based index in Document.Lines
	EventNum      string
	Reel          string
	TrackType     string // V, A, B etc.
	Transition    string // C, W, D etc.
	FCM           FCM    // FCM section active for this event
	Tokens        []TokenRef
	EventRepaired bool
}

// MarkRepaired flags that this event's line was rewritten by the repair pass.
func (e *Event) MarkRepaired() { e.EventRepaired = true }

// Line is one physical input line kept verbatim for output reconstruction.
type Line struct {
	Text    string
	IsEvent bool
	Event   *Event
}

// Document is a parsed CMX 3600 EDL with all original text preserved.
type Document struct {
	Lines  []Line
	Events []*Event
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isSeparator(b byte) bool {
	return b == ':' || b == ';' || b == '.' || b == ','
}

// looksLikeTimecode reports whether s[start:start+11] is HH?MM?SS?FF with
// fixed separator slots and digits elsewhere.
func looksLikeTimecode(s string, start int) bool {
	if start+11 > len(s) {
		return false
	}
	for i := 0; i < 11; i++ {
		b := s[start+i]
		switch i {
		case 2, 5:
			if b != ':' {
				return false
			}
		case 8:
			if !isSeparator(b) {
				return false
			}
		default:
			if !isDigit(b) {
				return false
			}
		}
	}
	return true
}

// boundaryOK makes sure the token is not glued to other word characters.
func boundaryOK(s string, start, end int) bool {
	if start > 0 {
		prev := s[start-1]
		if isDigit(prev) || isSeparator(prev) {
			return false
		}
	}
	if end < len(s) {
		next := s[end]
		if isDigit(next) || isSeparator(next) {
			return false
		}
	}
	return true
}

func extractTimecodes(line string) []TokenRef {
	var refs []TokenRef
	for i := 0; i+11 <= len(line); {
		if looksLikeTimecode(line, i) && boundaryOK(line, i, i+11) {
			tc, err := timecode.Parse(line[i:i+11], i)
			if err == nil {
				refs = append(refs, TokenRef{TC: tc, TokenIdx: len(refs)})
				i += 11
				continue
			}
		}
		i++
	}
	fields := []string{"SRC_IN", "SRC_OUT", "REC_IN", "REC_OUT"}
	for i := range refs {
		if i < len(fields) {
			refs[i].Field = fields[i]
		}
	}
	return refs
}

func parseFCM(line string) FCM {
	upper := strings.ToUpper(strings.TrimSpace(line))
	if !strings.HasPrefix(upper, "FCM:") {
		return FCMUnspecified
	}
	body := strings.TrimSpace(strings.TrimPrefix(upper, "FCM:"))
	switch {
	case strings.Contains(body, "DROP") && strings.Contains(body, "NON"):
		return FCMNonDrop
	case strings.Contains(body, "DROP"):
		return FCMDrop
	default:
		return FCMUnspecified
	}
}

// Parse splits raw EDL text into lines and events while keeping every byte of
// non-event content so repair output can preserve layout exactly.
func Parse(raw string) *Document {
	doc := &Document{}
	activeFCM := FCMUnspecified
	physicalLines := strings.Split(raw, "\n")
	for idx, text := range physicalLines {
		line := Line{Text: text}
		if fcm := parseFCM(text); fcm != FCMUnspecified {
			activeFCM = fcm
			doc.Lines = append(doc.Lines, line)
			continue
		}
		refs := extractTimecodes(text)
		if len(refs) == 4 && isEventHeader(text) {
			ev := buildEvent(idx, text, refs, activeFCM)
			line.IsEvent = true
			line.Event = ev
			doc.Events = append(doc.Events, ev)
		}
		doc.Lines = append(doc.Lines, line)
	}
	return doc
}

func isEventHeader(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "TITLE") {
		return false
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 4 {
		return false
	}
	for _, c := range fields[0] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func buildEvent(idx int, line string, refs []TokenRef, fcm FCM) *Event {
	fields := strings.Fields(strings.TrimSpace(line))
	ev := &Event{LineIndex: idx, FCM: fcm, Tokens: refs}
	if len(fields) >= 1 {
		ev.EventNum = fields[0]
	}
	if len(fields) >= 2 {
		ev.Reel = fields[1]
	}
	if len(fields) >= 3 {
		ev.Transition = fields[2]
	}
	if len(fields) >= 4 {
		ev.TrackType = fields[3]
	}
	return ev
}

// ReplaceToken rewrites one timecode token in place. Because every canonical
// label is exactly 11 bytes, column alignment and all surrounding text survive.
func ReplaceToken(line string, ref TokenRef, rendered string) string {
	if len(rendered) != 11 || ref.TC.ByteStart+11 > len(line) {
		return line
	}
	return line[:ref.TC.ByteStart] + rendered + line[ref.TC.ByteStart+11:]
}
