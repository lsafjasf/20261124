package flacseek

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxMetadataLength = 1<<24 - 1
const maxSeekPoints = maxMetadataLength / 18

// RebuildIndex writes a copy of input to output with a SEEKTABLE derived from
// the actual frame headers. The input is never opened for writing.
func RebuildIndex(input, output string) (*RebuildResult, error) {
	if err := ensureDifferentFile(input, output); err != nil {
		return nil, err
	}
	in, err := os.Open(input)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	inInfo, err := in.Stat()
	if err != nil {
		return nil, err
	}
	if !inInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: input is not a regular file", ErrNotFLAC)
	}
	report, err := Inspect(in, inInfo.Size())
	if err != nil {
		return nil, err
	}
	report.File = input
	if report.AudioState != AudioOK {
		return nil, fmt.Errorf("%w: audio state is %s", ErrAudioCorrupt, report.AudioState)
	}
	if report.IndexState == IndexUnusable {
		return nil, fmt.Errorf("%w: metadata cannot be safely rewritten", ErrAudioCorrupt)
	}

	points := pointsFromFrames(report.Frames, report.AudioOffset)
	metadata, outputAudioOffset, err := rebuildMetadata(report, points)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(output)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(output)+".flacseek-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()

	if err := writeRebuilt(tmp, in, inInfo, report, metadata, outputAudioOffset); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	outReport, err := InspectFile(tmpName)
	if err != nil {
		return nil, fmt.Errorf("verify rebuilt output: %w", err)
	}
	if outReport.AudioState != AudioOK || outReport.IndexState != IndexOK {
		return nil, fmt.Errorf("rebuilt output failed verification: audio=%s index=%s",
			outReport.AudioState, outReport.IndexState)
	}
	afterHash, err := sha256File(input)
	if err != nil {
		return nil, err
	}
	if afterHash != report.SHA256 {
		return nil, fmt.Errorf("input changed during reindex: %s -> %s", report.SHA256, afterHash)
	}
	if err := os.Rename(tmpName, output); err != nil {
		return nil, err
	}
	keep = true
	outReport.File = output
	return &RebuildResult{
		Input: input, Output: output,
		InputSHA256: report.SHA256, OutputSHA256: outReport.SHA256,
		SeekPointCount: len(points), Report: outReport,
	}, nil
}

func ensureDifferentFile(input, output string) error {
	inAbs, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	outAbs, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if filepath.Clean(inAbs) == filepath.Clean(outAbs) {
		return ErrSameFile
	}
	inInfo, err := os.Stat(input)
	if err != nil {
		return err
	}
	if outInfo, err := os.Stat(output); err == nil {
		if os.SameFile(inInfo, outInfo) {
			return ErrSameFile
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func pointsFromFrames(frames []Frame, audioOffset int64) []SeekPoint {
	if len(frames) <= maxSeekPoints {
		points := make([]SeekPoint, len(frames))
		for i, frame := range frames {
			points[i] = SeekPoint{
				SampleNumber: frame.SampleStart,
				Offset:       uint64(frame.Offset - audioOffset),
				FrameSamples: frame.BlockSize,
			}
		}
		return points
	}

	points := make([]SeekPoint, 0, maxSeekPoints)
	stride := (len(frames) + maxSeekPoints - 1) / maxSeekPoints
	for i := 0; i < len(frames) && len(points) < maxSeekPoints-1; i += stride {
		points = append(points, frameSeekPoint(frames[i], audioOffset))
	}
	last := frameSeekPoint(frames[len(frames)-1], audioOffset)
	if len(points) == 0 || points[len(points)-1].SampleNumber != last.SampleNumber {
		points = append(points, last)
	}
	return points
}

func frameSeekPoint(frame Frame, audioOffset int64) SeekPoint {
	return SeekPoint{
		SampleNumber: frame.SampleStart,
		Offset:       uint64(frame.Offset - audioOffset),
		FrameSamples: frame.BlockSize,
	}
}

func encodeSeekTable(points []SeekPoint) []byte {
	data := make([]byte, len(points)*18)
	for i, point := range points {
		entry := data[i*18 : (i+1)*18]
		binary.BigEndian.PutUint64(entry[0:8], point.SampleNumber)
		binary.BigEndian.PutUint64(entry[8:16], point.Offset)
		binary.BigEndian.PutUint16(entry[16:18], point.FrameSamples)
	}
	return data
}

func rebuildMetadata(report *Report, points []SeekPoint) ([]byte, int64, error) {
	seekData := encodeSeekTable(points)
	blocks := make([]MetadataBlock, 0, len(report.Metadata)+1)
	replaced := false
	for _, block := range report.Metadata {
		if block.Type == 3 {
			if !replaced {
				block.Data = seekData
				block.Length = uint32(len(seekData))
				blocks = append(blocks, block)
				replaced = true
			}
			continue
		}
		blocks = append(blocks, block)
	}
	if !replaced {
		insertAt := 1
		if len(blocks) == 0 {
			insertAt = 0
		}
		blocks = append(blocks, MetadataBlock{})
		copy(blocks[insertAt+1:], blocks[insertAt:])
		blocks[insertAt] = MetadataBlock{Type: 3, Length: uint32(len(seekData)), Data: seekData}
	}
	if len(blocks) == 0 || blocks[0].Type != 0 {
		return nil, 0, errors.New("STREAMINFO is not the first metadata block")
	}

	var buf bytes.Buffer
	buf.WriteString("fLaC")
	for i := range blocks {
		block := &blocks[i]
		if len(block.Data) != int(block.Length) || block.Length > maxMetadataLength {
			return nil, 0, fmt.Errorf("metadata block type %d has invalid length %d",
				block.Type, block.Length)
		}
		last := i == len(blocks)-1
		head := [4]byte{block.Type, byte(block.Length >> 16), byte(block.Length >> 8), byte(block.Length)}
		if last {
			head[0] |= 0x80
		}
		buf.Write(head[:])
		buf.Write(block.Data)
	}
	audioOffset := int64(buf.Len())
	return buf.Bytes(), audioOffset, nil
}

func writeRebuilt(out *os.File, in *os.File, info os.FileInfo, report *Report,
	metadata []byte, audioOffset int64) error {
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := out.Write(metadata); err != nil {
		return err
	}
	audioSize := report.FileSize - report.AudioOffset
	src := io.NewSectionReader(in, report.AudioOffset, audioSize)
	if _, err := io.CopyN(out, src, audioSize); err != nil {
		return err
	}
	if got, err := out.Seek(0, io.SeekCurrent); err != nil {
		return err
	} else if got != audioOffset+audioSize {
		return fmt.Errorf("short reindexed file: wrote %d bytes, expected %d",
			got, audioOffset+audioSize)
	}
	return out.Sync()
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
