package flacseek

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Inspect scans a native FLAC stream without modifying it.
func Inspect(r io.ReaderAt, size int64) (*Report, error) {
	report := &Report{
		FileSize:   size,
		IndexState: IndexOK,
		AudioState: AudioOK,
	}
	if size < 0 {
		return report, errors.New("negative input size")
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, io.NewSectionReader(r, 0, size)); err != nil {
		return report, fmt.Errorf("hash input: %w", err)
	}
	report.SHA256 = hex.EncodeToString(sum.Sum(nil))

	meta, err := parseMetadata(r, size, report)
	if err != nil {
		if errors.Is(err, ErrNotFLAC) {
			report.IndexState = IndexUnusable
			report.AudioState = AudioCorrupt
			report.Repairable = false
			return report, err
		}
		if errors.Is(err, ErrMissingStreamInfo) {
			report.IndexState = IndexUnusable
			report.AudioState = AudioCorrupt
			report.Repairable = false
			return report, nil
		}
		return report, err
	}
	if !meta.complete {
		report.IndexState = IndexUnusable
		report.AudioState = AudioTruncated
		report.Repairable = false
		return report, nil
	}
	report.AudioOffset = meta.audioOffset

	frames, state, scanErr := scanFrames(r, meta.audioOffset, size, meta.streamInfo, report)
	if scanErr != nil {
		return report, scanErr
	}
	report.Frames = frames
	report.FrameCount = len(frames)
	for _, frame := range frames {
		report.DecodedSamples += uint64(frame.BlockSize)
	}
	report.AudioState = state

	validateSeekTable(report, meta.streamInfo, meta.seekBlock)
	finalizeState(report)
	return report, nil
}

// InspectFile scans a FLAC file and records its path.
func InspectFile(path string) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	report, err := Inspect(f, info.Size())
	if report != nil {
		report.File = path
	}
	return report, err
}

type byteSource struct {
	r    io.ReaderAt
	size int64
	base int64
	data []byte
}

func (s *byteSource) at(off int64) (byte, error) {
	if off < 0 || off >= s.size {
		return 0, io.ErrUnexpectedEOF
	}
	if off < s.base || off >= s.base+int64(len(s.data)) {
		n := int64(64 * 1024)
		if left := s.size - off; left < n {
			n = left
		}
		s.data = make([]byte, int(n))
		got, err := s.r.ReadAt(s.data, off)
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		if got != int(n) {
			return 0, io.ErrUnexpectedEOF
		}
		s.base = off
	}
	return s.data[off-s.base], nil
}

func findFrameEnd(r io.ReaderAt, frameStart int64, hdr frameHeader, size int64) (int64, bool, error) {
	headerEnd := frameStart + int64(hdr.Size)
	if size < headerEnd+3 {
		return 0, false, nil
	}
	crc := crc16(hdr.Raw)
	var ring [4]uint16
	src := &byteSource{r: r, size: size, base: -1}
	sawSync := false

	for p := headerEnd; p < size; p++ {
		b, err := src.at(p)
		if err != nil {
			return 0, sawSync, err
		}
		ring[p&3] = crc
		if p == size-2 {
			next, err := src.at(p + 1)
			if err != nil {
				return 0, sawSync, err
			}
			if crc == uint16(b)<<8|uint16(next) {
				return size, sawSync, nil
			}
		}
		if p >= headerEnd+3 && p+1 < size && b == 0xff {
			next, err := src.at(p + 1)
			if err != nil {
				return 0, sawSync, err
			}
			if next&0xfe == 0xf8 {
				sawSync = true
				prev2, err := src.at(p - 2)
				if err != nil {
					return 0, sawSync, err
				}
				prev1, err := src.at(p - 1)
				if err != nil {
					return 0, sawSync, err
				}
				if ring[(p-2)&3] == uint16(prev2)<<8|uint16(prev1) {
					return p, sawSync, nil
				}
			}
		}
		crc = crc16Update(crc, b)
	}
	return 0, sawSync, nil
}

func scanFrames(r io.ReaderAt, start, size int64, info *StreamInfo, report *Report) ([]Frame, AudioState, error) {
	var frames []Frame
	pos := start
	state := AudioOK
	for pos < size {
		hdr, err := parseFrameHeader(r, pos, size, info)
		if err != nil {
			report.addIssue("audio.frame_header", SeverityError, pos,
				"invalid frame header: %v", err)
			return frames, AudioCorrupt, nil
		}
		end, sawSync, err := findFrameEnd(r, pos, hdr, size)
		if err != nil {
			return frames, state, err
		}
		if end == 0 {
			if !sawSync {
				report.addIssue("audio.truncated", SeverityError, pos,
					"audio ends before a complete frame CRC-16 at offset %d", pos)
				return frames, AudioTruncated, nil
			}
			report.addIssue("audio.frame_crc", SeverityError, pos,
				"frame at offset %d has no boundary with a valid CRC-16", pos)
			return frames, AudioCorrupt, nil
		}
		frame := Frame{
			Offset: pos, Size: end - pos, HeaderSize: hdr.Size,
			Blocking: hdr.Blocking, Number: hdr.Number,
			BlockSize: hdr.BlockSize, SampleRate: hdr.SampleRate,
			Channels: hdr.Channels, BitsPerSample: hdr.BitsPerSample,
		}
		if len(frames) == 0 {
			frame.SampleStart = 0
		} else {
			frame.SampleStart = frames[len(frames)-1].SampleStart +
				uint64(frames[len(frames)-1].BlockSize)
		}
		frames = append(frames, frame)
		pos = end
	}

	if !validateFrameSequence(frames, info, report) {
		state = AudioCorrupt
	}
	if info.TotalSamples > 0 {
		switch decoded := samplesIn(frames); {
		case decoded < info.TotalSamples:
			report.addIssue("audio.truncated", SeverityError, start,
				"decoded sample count %d is below STREAMINFO total %d",
				decoded, info.TotalSamples)
			if state != AudioCorrupt {
				state = AudioTruncated
			}
		case decoded > info.TotalSamples:
			report.addIssue("audio.sample_overflow", SeverityError, start,
				"decoded sample count %d exceeds STREAMINFO total %d",
				decoded, info.TotalSamples)
			state = AudioCorrupt
		}
	}
	if len(frames) == 0 {
		report.addIssue("audio.empty", SeverityError, start, "no audio frames found")
		return frames, AudioCorrupt, nil
	}
	return frames, state, nil
}

func samplesIn(frames []Frame) uint64 {
	var n uint64
	for _, frame := range frames {
		n += uint64(frame.BlockSize)
	}
	return n
}

func validateFrameSequence(frames []Frame, info *StreamInfo, report *Report) bool {
	if len(frames) == 0 {
		return false
	}
	valid := true
	blocking := frames[0].Blocking
	minBlock, maxBlock := frames[0].BlockSize, frames[0].BlockSize
	for i, frame := range frames {
		if frame.Blocking != blocking {
			report.addIssue("audio.blocking_mixed", SeverityError, frame.Offset,
				"frame %d uses a different blocking strategy", i)
			valid = false
		}
		if i == 0 {
			if frame.Number != 0 {
				report.addIssue("audio.first_sample", SeverityError, frame.Offset,
					"first frame does not start at sample 0 (encoded number %d)", frame.Number)
				valid = false
			}
		} else if blocking == 0 {
			if frame.Number != uint64(i) {
				report.addIssue("audio.frame_number", SeverityError, frame.Offset,
					"fixed-block frame number %d, expected %d", frame.Number, i)
				valid = false
			}
		} else {
			expected := frames[i-1].SampleStart + uint64(frames[i-1].BlockSize)
			if frame.Number != expected {
				report.addIssue("audio.sample_number", SeverityError, frame.Offset,
					"sample number %d, expected %d", frame.Number, expected)
				valid = false
			}
		}
		if i+1 < len(frames) && frame.BlockSize < minBlock {
			minBlock = frame.BlockSize
		}
		if frame.BlockSize > maxBlock {
			maxBlock = frame.BlockSize
		}
		if frame.SampleRate != info.SampleRate {
			report.addIssue("audio.sample_rate", SeverityError, frame.Offset,
				"frame sample rate %d, STREAMINFO says %d", frame.SampleRate, info.SampleRate)
			valid = false
		}
		if frame.Channels != info.Channels {
			report.addIssue("audio.channels", SeverityError, frame.Offset,
				"frame has %d channels, STREAMINFO says %d", frame.Channels, info.Channels)
			valid = false
		}
		if frame.BitsPerSample != info.BitsPerSample {
			report.addIssue("audio.bits_per_sample", SeverityError, frame.Offset,
				"frame has %d bits per sample, STREAMINFO says %d",
				frame.BitsPerSample, info.BitsPerSample)
			valid = false
		}
	}
	if blocking == 0 {
		for i := 0; i+1 < len(frames); i++ {
			if frames[i].BlockSize != frames[0].BlockSize {
				report.addIssue("audio.fixed_block_size", SeverityError, frames[i].Offset,
					"fixed-block stream has block size %d, expected %d",
					frames[i].BlockSize, frames[0].BlockSize)
				valid = false
			}
		}
		if len(frames) > 1 && frames[len(frames)-1].BlockSize > frames[0].BlockSize {
			report.addIssue("audio.fixed_last_block", SeverityError,
				frames[len(frames)-1].Offset, "final fixed-block frame is larger than earlier frames")
			valid = false
		}
	}
	if (len(frames) > 1 && minBlock < info.MinBlockSize) || maxBlock > info.MaxBlockSize {
		report.addIssue("audio.block_range", SeverityWarning, frames[0].Offset,
			"observed block sizes %d..%d fall outside STREAMINFO range %d..%d",
			minBlock, maxBlock, info.MinBlockSize, info.MaxBlockSize)
	}
	return valid
}

func finalizeState(report *Report) {
	audioOK := report.AudioState == AudioOK
	if !report.SeekPresent {
		report.IndexState = IndexRepairable
		report.addIssue("seektable.missing", SeverityWarning, report.AudioOffset,
			"file has no SEEKTABLE; a new index can be written without changing audio")
	} else if audioOK && (!seekChecksValid(report.SeekChecks) ||
		report.hasIssuePrefix("seektable.") || report.hasIssue("metadata.seektable_duplicate")) {
		report.IndexState = IndexRepairable
	}
	if !audioOK {
		report.IndexState = IndexUnusable
	}
	if report.metadataUnusable() {
		report.IndexState = IndexUnusable
	}
	report.Repairable = audioOK && report.IndexState == IndexRepairable
}

func seekChecksValid(checks []SeekPointCheck) bool {
	for _, check := range checks {
		if !check.Valid {
			return false
		}
	}
	return true
}

func (r *Report) hasIssuePrefix(prefix string) bool {
	for _, issue := range r.Issues {
		if strings.HasPrefix(issue.Code, prefix) && issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r *Report) hasIssue(code string) bool {
	for _, issue := range r.Issues {
		if issue.Code == code && issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r *Report) metadataUnusable() bool {
	unusable := map[string]bool{
		"metadata.streaminfo_order":     true,
		"metadata.streaminfo_duplicate": true,
		"metadata.streaminfo_invalid":   true,
		"metadata.invalid_type":         true,
	}
	for _, issue := range r.Issues {
		if issue.Severity == SeverityError && unusable[issue.Code] {
			return true
		}
	}
	return false
}
