package flacseek

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var (
	ErrNotFLAC           = errors.New("not a native FLAC stream")
	ErrMissingStreamInfo = errors.New("STREAMINFO metadata block is missing")
	ErrNoAudio           = errors.New("FLAC stream contains no audio frames")
	ErrAudioCorrupt      = errors.New("audio frames are corrupt or truncated")
	ErrSameFile          = errors.New("output path refers to the input file")
)

type metadataResult struct {
	streamInfo  *StreamInfo
	audioOffset int64
	complete    bool
	seekBlock   int
}

func readAt(r io.ReaderAt, off int64, n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("negative read length %d", n)
	}
	buf := make([]byte, n)
	got, err := r.ReadAt(buf, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if got != n {
		return nil, io.ErrUnexpectedEOF
	}
	return buf, nil
}

func parseMetadata(r io.ReaderAt, size int64, report *Report) (metadataResult, error) {
	result := metadataResult{seekBlock: -1}
	if size < 4 {
		return result, fmt.Errorf("%w: file is shorter than the fLaC marker", ErrNotFLAC)
	}
	marker, err := readAt(r, 0, 4)
	if err != nil {
		return result, err
	}
	if string(marker) != "fLaC" {
		return result, ErrNotFLAC
	}

	off := int64(4)
	for blockIndex := 0; ; blockIndex++ {
		if size-off < 4 {
			report.addIssue("metadata.truncated", SeverityError, off,
				"metadata header is truncated at offset %d", off)
			return result, nil
		}
		header, err := readAt(r, off, 4)
		if err != nil {
			return result, err
		}
		last := header[0]&0x80 != 0
		blockType := header[0] & 0x7f
		length := uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
		dataOff := off + 4
		if int64(length) > size-dataOff {
			report.addIssue("metadata.truncated", SeverityError, dataOff,
				"metadata block type %d claims %d bytes, only %d remain",
				blockType, length, size-dataOff)
			return result, nil
		}
		data := make([]byte, int(length))
		if length > 0 {
			data, err = readAt(r, dataOff, int(length))
			if err != nil {
				return result, err
			}
		}
		report.Metadata = append(report.Metadata, MetadataBlock{
			Offset: off, Type: blockType, Length: length, Last: last, Data: data,
		})

		switch blockType {
		case 0:
			if blockIndex != 0 {
				report.addIssue("metadata.streaminfo_order", SeverityError, off,
					"STREAMINFO must be the first metadata block")
			}
			if result.streamInfo != nil {
				report.addIssue("metadata.streaminfo_duplicate", SeverityError, off,
					"multiple STREAMINFO blocks are not allowed")
			} else {
				info, parseErr := parseStreamInfo(data)
				if parseErr != nil {
					report.addIssue("metadata.streaminfo_invalid", SeverityError, dataOff,
						"invalid STREAMINFO: %v", parseErr)
				} else {
					result.streamInfo = info
					report.StreamInfo = info
				}
			}
		case 3:
			if result.seekBlock < 0 {
				result.seekBlock = blockIndex
				parseSeekTable(data, dataOff, report)
			} else {
				report.addIssue("metadata.seektable_duplicate", SeverityError, off,
					"multiple SEEKTABLE blocks are not allowed")
			}
		case 127:
			report.addIssue("metadata.invalid_type", SeverityError, off,
				"metadata block type 127 is invalid")
		}

		off = dataOff + int64(length)
		if last {
			result.audioOffset = off
			result.complete = true
			break
		}
		if off >= size {
			report.addIssue("metadata.missing_last", SeverityError, off,
				"metadata ended without a last-metadata-block flag")
			return result, nil
		}
	}

	if result.streamInfo == nil {
		report.addIssue("metadata.streaminfo_missing", SeverityError, 4,
			"STREAMINFO metadata block is missing")
		return result, ErrMissingStreamInfo
	}
	return result, nil
}

func parseStreamInfo(data []byte) (*StreamInfo, error) {
	if len(data) != 34 {
		return nil, fmt.Errorf("expected 34 bytes, got %d", len(data))
	}
	minFrame := uint32(data[4])<<16 | uint32(data[5])<<8 | uint32(data[6])
	maxFrame := uint32(data[7])<<16 | uint32(data[8])<<8 | uint32(data[9])
	v := binary.BigEndian.Uint64(data[10:18])
	info := &StreamInfo{
		MinBlockSize:  binary.BigEndian.Uint16(data[0:2]),
		MaxBlockSize:  binary.BigEndian.Uint16(data[2:4]),
		MinFrameSize:  minFrame,
		MaxFrameSize:  maxFrame,
		SampleRate:    uint32(v >> 44),
		Channels:      uint8((v>>41)&0x7) + 1,
		BitsPerSample: uint8((v>>36)&0x1f) + 1,
		TotalSamples:  v & 0xfffffffff,
		MD5:           hex.EncodeToString(data[18:34]),
	}
	if info.SampleRate == 0 {
		return nil, errors.New("sample rate is zero")
	}
	if info.MinBlockSize == 0 || info.MaxBlockSize == 0 {
		return nil, errors.New("block size is zero")
	}
	if info.MinBlockSize > info.MaxBlockSize {
		return nil, fmt.Errorf("minimum block size %d exceeds maximum %d",
			info.MinBlockSize, info.MaxBlockSize)
	}
	if info.BitsPerSample < 4 {
		return nil, fmt.Errorf("bits per sample %d is below 4", info.BitsPerSample)
	}
	return info, nil
}

func parseSeekTable(data []byte, off int64, report *Report) {
	report.SeekPresent = true
	if len(data)%18 != 0 {
		report.addIssue("seektable.length", SeverityError, off,
			"SEEKTABLE length %d is not a multiple of 18", len(data))
		return
	}
	for i := 0; i+18 <= len(data); i += 18 {
		entry := data[i : i+18]
		point := SeekPoint{
			SampleNumber: binary.BigEndian.Uint64(entry[0:8]),
			Offset:       binary.BigEndian.Uint64(entry[8:16]),
			FrameSamples: binary.BigEndian.Uint16(entry[16:18]),
		}
		if point.SampleNumber == ^uint64(0) {
			point.Placeholder = true
		}
		report.SeekPoints = append(report.SeekPoints, point)
	}
}
