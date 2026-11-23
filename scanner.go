package truehd

import (
	"fmt"
	"sort"
)

// Scan parses a complete raw TrueHD elementary stream held in memory. Damage
// is reported in the result rather than being returned as an error, so callers
// can still use every complete frame before the damage.
func Scan(data []byte) *Report {
	s := &scanner{data: data}
	s.run()
	return s.report()
}

// Analyze is an alias kept for callers that prefer the diagnostic name.
func Analyze(data []byte) *Report {
	return Scan(data)
}

type scanner struct {
	data      []byte
	cfg       *streamConfig
	frames    []Frame
	issues    []Issue
	packetSeq int
}

func (s *scanner) run() {
	pos := 0
	for pos < len(s.data) {
		fh, failure := inspectFrame(s.data, pos, s.cfg)
		if failure != nil {
			if failure.truncated {
				s.addIssue(EOFError, pos, len(s.data), 0, "truncated input: "+failure.reason)
				return
			}
			if declared, ok := auLength(s.data, pos); ok && pos+declared > len(s.data) && len(s.data)-pos == 4 {
				s.addIssue(EOFError, pos, len(s.data), declared,
					"truncated input after access-unit header")
				return
			}
			next, ok := s.recoverHeader(pos, failure.reason)
			if !ok {
				return
			}
			pos = next
			continue
		}

		auLen, _ := auLength(s.data, pos)
		if pos+auLen > len(s.data) {
			s.addIssue(EOFError, pos, len(s.data), auLen,
				fmt.Sprintf("truncated access unit: declared %d bytes, only %d available", auLen, len(s.data)-pos))
			return
		}

		payloadReason, checked, ok := validatePayload(s.data, pos, fh)
		if !ok {
			s.addIssue(PayloadError, pos, pos+auLen, auLen, payloadReason)
			s.packetSeq++
			pos += auLen
			continue
		}

		cfg := fh.Config
		s.cfg = cfg
		frame := Frame{
			Index:           len(s.frames),
			PacketIndex:     s.packetSeq,
			RawStart:        int64(pos),
			RawEnd:          int64(pos + auLen),
			Size:            auLen,
			MajorSync:       fh.Major,
			SampleRate:      cfg.SampleRate,
			Samples:         cfg.SamplesPerFrame,
			Channels:        cfg.Channels,
			Substreams:      cfg.NumSubstreams,
			PayloadChecked:  checked,
			DurationSeconds: float64(cfg.SamplesPerFrame) / float64(cfg.SampleRate),
		}
		s.frames = append(s.frames, frame)
		s.packetSeq++
		pos += auLen
	}
}

func (s *scanner) recoverHeader(pos int, reason string) (int, bool) {
	declared, _ := auLength(s.data, pos)
	next, ok := findResync(s.data, pos+1, s.cfg)
	end := len(s.data)
	recoveredAt := int64(0)
	if ok {
		end = next
		recoveredAt = int64(next)
	}
	s.addIssue(HeaderError, pos, end, declared, reason)
	s.packetSeq++
	if !ok {
		return 0, false
	}
	s.issues[len(s.issues)-1].RecoveredAt = recoveredAt
	return next, true
}

func (s *scanner) addIssue(kind ErrorKind, start, end, declared int, reason string) {
	if start < 0 {
		start = 0
	}
	if end > len(s.data) {
		end = len(s.data)
	}
	if end < start {
		end = start
	}
	s.issues = append(s.issues, Issue{
		Kind:         kind,
		PacketIndex:  s.packetSeq,
		RawStart:     int64(start),
		RawEnd:       int64(end),
		DeclaredSize: declared,
		Reason:       reason,
	})
}

func validatePayload(data []byte, packet int, fh *frameHeader) (string, bool, bool) {
	checked := false
	for i, sub := range fh.Subs {
		if !sub.CheckData {
			continue
		}
		checked = true
		if sub.Start < packet || sub.End > len(data) || sub.End < sub.Start {
			return fmt.Sprintf("substream %d payload range is invalid", i), checked, false
		}
		payload := data[sub.Start:sub.End]
		if !validSubstreamCheck(payload) {
			return fmt.Sprintf("substream %d parity/CRC check failed", i), checked, false
		}
	}
	return "", checked, true
}

func findResync(data []byte, from int, cfg *streamConfig) (int, bool) {
	if from < 0 {
		from = 0
	}
	for p := from; p+4 <= len(data); p++ {
		if auLen, ok := auLength(data, p); !ok || auLen < 4 {
			continue
		} else if p+auLen > len(data) && majorPrefixLen(data, p+4) != len(majorMagic) {
			continue
		}

		if majorPrefixLen(data, p+4) == len(majorMagic) {
			fh, failure := inspectFrame(data, p, nil)
			if failure != nil {
				continue
			}
			auLen, _ := auLength(data, p)
			if p+auLen > len(data) {
				return p, true
			}
			if _, _, ok := validatePayload(data, p, fh); ok {
				return p, true
			}
			// A valid major header still marks a trustworthy new boundary
			// even when this packet's own payload is damaged.
			return p, true
		}

		if cfg == nil {
			continue
		}
		auLen, _ := auLength(data, p)
		if p+auLen > len(data) {
			continue
		}
		fh, failure := inspectFrame(data, p, cfg)
		if failure != nil || fh.Major {
			continue
		}
		if _, _, ok := validatePayload(data, p, fh); !ok {
			continue
		}
		if plausibleBoundary(data, p+auLen, cfg) {
			return p, true
		}
	}
	return 0, false
}

func plausibleBoundary(data []byte, pos int, cfg *streamConfig) bool {
	if pos >= len(data) {
		return true
	}
	fh, failure := inspectFrame(data, pos, cfg)
	if failure != nil {
		return failure.truncated
	}
	if _, _, ok := validatePayload(data, pos, fh); !ok {
		return false
	}
	return true
}

func (s *scanner) report() *Report {
	frames := append([]Frame(nil), s.frames...)
	issues := append([]Issue(nil), s.issues...)
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].RawStart == issues[j].RawStart {
			return issues[i].RawEnd < issues[j].RawEnd
		}
		return issues[i].RawStart < issues[j].RawStart
	})

	r := &Report{
		FileSize:         int64(len(s.data)),
		Frames:           frames,
		Issues:           issues,
		DiscardedPackets: []Issue{},
		PlayableRanges:   []PlayableRange{},
	}
	if r.Frames == nil {
		r.Frames = []Frame{}
	}
	if r.Issues == nil {
		r.Issues = []Issue{}
	}
	for _, issue := range issues {
		r.DiscardedBytes += issue.RawEnd - issue.RawStart
		r.DiscardedPackets = append(r.DiscardedPackets, issue)
		switch issue.Kind {
		case HeaderError:
			r.HeaderErrors++
		case PayloadError:
			r.PayloadErrors++
		case EOFError:
			r.EOFErrors++
			r.Truncated = true
		}
	}
	r.PlayableRanges, r.PlayableBytes, r.PlayableSamples = buildPlayable(frames, r)
	r.Complete = len(issues) == 0 && len(frames) > 0 &&
		frames[0].RawStart == 0 && frames[len(frames)-1].RawEnd == r.FileSize &&
		r.PlayableBytes == r.FileSize
	return r
}

func buildPlayable(frames []Frame, r *Report) ([]PlayableRange, int64, int64) {
	ranges := []PlayableRange{}
	var totalBytes, totalSamples int64
	var decoded float64

	for i := 0; i < len(frames); {
		first := i
		start := frames[i].RawStart
		samples := int64(0)
		startSeconds := decoded
		for i < len(frames) {
			f := frames[i]
			if i > first && f.RawStart != frames[i-1].RawEnd {
				break
			}
			samples += int64(f.Samples)
			decoded += f.DurationSeconds
			totalBytes += int64(f.Size)
			i++
		}
		end := frames[i-1].RawEnd
		totalSamples += samples
		ranges = append(ranges, PlayableRange{
			RawStart:            start,
			RawEnd:              end,
			FirstFrameIndex:     first,
			FrameCount:          i - first,
			Samples:             samples,
			DecodedStartSeconds: startSeconds,
			DecodedEndSeconds:   decoded,
		})
	}

	r.PlayableDurationSeconds = decoded
	for _, f := range frames {
		if !f.PayloadChecked {
			r.UncheckedPayloadFrames++
		}
	}
	return ranges, totalBytes, totalSamples
}
