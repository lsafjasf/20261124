package flacseek

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func inspectTestBytes(t *testing.T, data []byte) *Report {
	t.Helper()
	report, err := Inspect(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	return report
}

func TestVariableBlockSizesAndValidSeekTable(t *testing.T) {
	specs := variableSpecs()
	file := buildTestFLAC(t, specs, validSeekForAudio(specs))
	report := inspectTestBytes(t, file)

	if report.AudioState != AudioOK {
		t.Fatalf("audio state = %s, issues = %+v", report.AudioState, report.Issues)
	}
	if report.IndexState != IndexOK || report.Repairable {
		t.Fatalf("index state = %s repairable=%v", report.IndexState, report.Repairable)
	}
	if report.FrameCount != len(specs) {
		t.Fatalf("frame count = %d, want %d", report.FrameCount, len(specs))
	}
	if report.DecodedSamples != 5864 {
		t.Fatalf("decoded samples = %d, want 5864", report.DecodedSamples)
	}
	if len(report.SeekChecks) != len(specs) || !seekChecksValid(report.SeekChecks) {
		t.Fatalf("seek checks = %+v", report.SeekChecks)
	}
	if !report.SeekChecks[len(report.SeekChecks)-1].Last {
		t.Fatal("last seek point was not marked")
	}
}

func TestFixedBlockStrategyAllowsShortFinalFrame(t *testing.T) {
	specs := []testFrameSpec{
		{sampleStart: 0, blockSize: 192, payloadByte: 1},
		{sampleStart: 192, blockSize: 192, payloadByte: 2},
		{sampleStart: 384, blockSize: 100, payloadByte: 3},
	}
	audio := buildTestAudio(t, specs)
	streamInfo := makeStreamInfo(192, 192, 44100, 1, 16, 484)
	file := []byte("fLaC")
	file = appendMetadata(file, 0, false, streamInfo)
	file = appendMetadata(file, 3, true, encodeTestSeek(validSeekForAudio(specs)))
	file = append(file, audio...)

	report := inspectTestBytes(t, file)
	if report.AudioState != AudioOK || report.IndexState != IndexOK {
		t.Fatalf("audio=%s index=%s issues=%+v", report.AudioState, report.IndexState, report.Issues)
	}
	if report.DecodedSamples != 484 {
		t.Fatalf("decoded samples = %d", report.DecodedSamples)
	}
}

func TestDuplicateSeekPointsAreRepairable(t *testing.T) {
	specs := variableSpecs()
	points := validSeekForAudio(specs)
	points = append(points[:2], append([]testSeekPoint{points[1]}, points[2:]...)...)
	file := buildTestFLAC(t, specs, points)
	report := inspectTestBytes(t, file)

	if report.AudioState != AudioOK {
		t.Fatalf("audio state = %s", report.AudioState)
	}
	if report.IndexState != IndexRepairable || !report.Repairable {
		t.Fatalf("index state = %s repairable=%v", report.IndexState, report.Repairable)
	}
	if !report.SeekChecks[2].Duplicate {
		t.Fatalf("duplicate point not detected: %+v", report.SeekChecks[2])
	}
	if !report.hasIssuePrefix("seektable.duplicate") {
		t.Fatal("duplicate issue missing")
	}
}

func TestStaleLastSeekPointIsRepairable(t *testing.T) {
	specs := variableSpecs()
	points := validSeekForAudio(specs)
	points[len(points)-1].sample += 123456
	file := buildTestFLAC(t, specs, points)
	report := inspectTestBytes(t, file)

	if report.AudioState != AudioOK {
		t.Fatalf("audio state = %s", report.AudioState)
	}
	if report.IndexState != IndexRepairable || !report.Repairable {
		t.Fatalf("index state = %s repairable=%v", report.IndexState, report.Repairable)
	}
	last := report.SeekChecks[len(report.SeekChecks)-1]
	if !last.Last || last.Valid || !last.OutOfRange || last.SampleOK {
		t.Fatalf("last check = %+v", last)
	}
	if !report.hasIssuePrefix("seektable.last_invalid") {
		t.Fatal("last seek point issue missing")
	}
}

func TestPlaceholdersAreValid(t *testing.T) {
	specs := variableSpecs()
	points := validSeekForAudio(specs)
	points = append(points,
		testSeekPoint{sample: ^uint64(0)},
		testSeekPoint{sample: ^uint64(0)},
	)
	file := buildTestFLAC(t, specs, points)
	report := inspectTestBytes(t, file)

	if report.AudioState != AudioOK || report.IndexState != IndexOK {
		t.Fatalf("audio=%s index=%s issues=%+v", report.AudioState, report.IndexState, report.Issues)
	}
	if !report.SeekChecks[len(report.SeekChecks)-1].Valid {
		t.Fatal("trailing placeholders should be valid")
	}
	if !report.SeekChecks[len(specs)-1].Last {
		t.Fatal("last real point was not marked")
	}
}

func TestTruncatedAudioIsNotRepairable(t *testing.T) {
	specs := []testFrameSpec{
		{sampleStart: 0, blockSize: 192, payloadByte: 1},
		{sampleStart: 192, blockSize: 192, payloadByte: 2},
		{sampleStart: 384, blockSize: 192, payloadByte: 3},
	}
	file := buildTestFLAC(t, specs, nil)
	file = file[:len(file)-4]
	report := inspectTestBytes(t, file)

	if report.AudioState != AudioTruncated {
		t.Fatalf("audio state = %s, issues = %+v", report.AudioState, report.Issues)
	}
	if report.Repairable || report.IndexState != IndexUnusable {
		t.Fatalf("truncated file marked repairable: %+v", report)
	}
}

func TestTruncationAtFrameBoundary(t *testing.T) {
	specs := []testFrameSpec{
		{sampleStart: 0, blockSize: 192, payloadByte: 1},
		{sampleStart: 192, blockSize: 192, payloadByte: 2},
		{sampleStart: 384, blockSize: 192, payloadByte: 3},
	}
	file := buildTestFLAC(t, specs, validSeekForAudio(specs))
	valid := inspectTestBytes(t, file)
	file = file[:valid.Frames[2].Offset]
	report := inspectTestBytes(t, file)

	if report.AudioState != AudioTruncated {
		t.Fatalf("audio state = %s, issues = %+v", report.AudioState, report.Issues)
	}
	if report.FrameCount != 2 || report.Repairable {
		t.Fatalf("frame count=%d repairable=%v", report.FrameCount, report.Repairable)
	}
}

func TestHeaderCRC8FailureIsCorrupt(t *testing.T) {
	specs := variableSpecs()
	file := buildTestFLAC(t, specs, validSeekForAudio(specs))
	valid := inspectTestBytes(t, file)
	second := valid.Frames[1]
	file[int(second.Offset)+second.HeaderSize-1] ^= 0xff

	report := inspectTestBytes(t, file)
	if report.AudioState != AudioCorrupt {
		t.Fatalf("audio state = %s, issues = %+v", report.AudioState, report.Issues)
	}
	if report.Repairable {
		t.Fatal("header CRC failure marked repairable")
	}
}

func TestFrameCRCFailureIsCorrupt(t *testing.T) {
	specs := variableSpecs()
	file := buildTestFLAC(t, specs, validSeekForAudio(specs))
	valid := inspectTestBytes(t, file)
	second := valid.Frames[1]
	file[int(second.Offset)+second.HeaderSize] ^= 0x01

	report := inspectTestBytes(t, file)
	if report.AudioState != AudioCorrupt {
		t.Fatalf("audio state = %s, issues = %+v", report.AudioState, report.Issues)
	}
	if report.Repairable || report.IndexState != IndexUnusable {
		t.Fatalf("corrupt file marked repairable: %s", report.IndexState)
	}
}

func TestMalformedStreamInfoIsReportedNotFatal(t *testing.T) {
	specs := []testFrameSpec{{sampleStart: 0, blockSize: 192, payloadByte: 1}}
	audio := buildTestAudio(t, specs)
	file := []byte("fLaC")
	file = appendMetadata(file, 0, true, make([]byte, 33))
	file = append(file, audio...)

	report, err := Inspect(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatalf("Inspect returned fatal error: %v", err)
	}
	if report.AudioState != AudioCorrupt || report.IndexState != IndexUnusable {
		t.Fatalf("audio=%s index=%s", report.AudioState, report.IndexState)
	}
	if report.Repairable {
		t.Fatal("malformed STREAMINFO marked repairable")
	}
}

func TestMalformedSeekTableCanBeRebuilt(t *testing.T) {
	specs := variableSpecs()
	audio := buildTestAudio(t, specs)
	streamInfo := makeStreamInfo(192, 4096, 44100, 1, 16, 5864)
	file := []byte("fLaC")
	file = appendMetadata(file, 0, false, streamInfo)
	file = appendMetadata(file, 3, true, make([]byte, 17))
	file = append(file, audio...)

	report := inspectTestBytes(t, file)
	if report.AudioState != AudioOK || report.IndexState != IndexRepairable {
		t.Fatalf("audio=%s index=%s issues=%+v", report.AudioState, report.IndexState, report.Issues)
	}
	input := writeTestFile(t, "bad-seek.flac", file)
	output := filepath.Join(t.TempDir(), "fixed-seek.flac")
	result, err := RebuildIndex(input, output)
	if err != nil {
		t.Fatal(err)
	}
	if result.SeekPointCount != len(specs) || result.Report.IndexState != IndexOK {
		t.Fatalf("rebuild result = %+v", result)
	}
}

func TestDuplicateSeekTableBlocksAreRepairable(t *testing.T) {
	specs := variableSpecs()
	audio := buildTestAudio(t, specs)
	streamInfo := makeStreamInfo(192, 4096, 44100, 1, 16, 5864)
	seek := encodeTestSeek(validSeekForAudio(specs))
	file := []byte("fLaC")
	file = appendMetadata(file, 0, false, streamInfo)
	file = appendMetadata(file, 3, false, seek)
	file = appendMetadata(file, 3, true, seek)
	file = append(file, audio...)

	report := inspectTestBytes(t, file)
	if report.AudioState != AudioOK || report.IndexState != IndexRepairable {
		t.Fatalf("audio=%s index=%s issues=%+v", report.AudioState, report.IndexState, report.Issues)
	}
	input := writeTestFile(t, "duplicate-seek.flac", file)
	output := filepath.Join(t.TempDir(), "single-seek.flac")
	result, err := RebuildIndex(input, output)
	if err != nil {
		t.Fatal(err)
	}
	if countMetadataType(result.Report.Metadata, 3) != 1 || result.Report.IndexState != IndexOK {
		t.Fatalf("duplicate SEEKTABLE not removed: %+v", result.Report.Metadata)
	}
}

func TestRebuildWritesNewFileAndPreservesInput(t *testing.T) {
	specs := variableSpecs()
	points := validSeekForAudio(specs)
	points = append(points[:1], append([]testSeekPoint{points[0]}, points[1:]...)...)
	points[len(points)-1].count = 123
	input := writeTestFile(t, "input.flac", buildTestFLAC(t, specs, points))
	before, err := sha256File(input)
	if err != nil {
		t.Fatal(err)
	}
	inputReport, err := InspectFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !inputReport.Repairable {
		t.Fatalf("fixture is not repairable: %+v", inputReport.Issues)
	}
	output := filepath.Join(t.TempDir(), "rebuilt.flac")
	result, err := RebuildIndex(input, output)
	if err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
	after, err := sha256File(input)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("input hash changed: %s -> %s", before, after)
	}
	if result.InputSHA256 != before || result.OutputSHA256 == "" {
		t.Fatalf("bad result hashes: %+v", result)
	}
	if result.Report.AudioState != AudioOK || result.Report.IndexState != IndexOK {
		t.Fatalf("output not clean: audio=%s index=%s", result.Report.AudioState, result.Report.IndexState)
	}
	if result.SeekPointCount != len(specs) || len(result.Report.SeekChecks) != len(specs) {
		t.Fatalf("seek point count = %d", result.SeekPointCount)
	}
	for _, check := range result.Report.SeekChecks {
		if !check.Valid {
			t.Fatalf("invalid rebuilt point: %+v", check)
		}
	}

	inBytes, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	outBytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inBytes[inputReport.AudioOffset:], outBytes[result.Report.AudioOffset:]) {
		t.Fatal("audio bytes changed during rebuild")
	}
}

func TestRebuildAddsMissingSeekTable(t *testing.T) {
	specs := []testFrameSpec{
		{sampleStart: 0, blockSize: 192, payloadByte: 1},
		{sampleStart: 192, blockSize: 192, payloadByte: 2},
	}
	input := writeTestFile(t, "no-seek.flac", buildTestFLAC(t, specs, nil))
	report, err := InspectFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.IndexState != IndexRepairable || report.SeekPresent {
		t.Fatalf("missing seek table not classified: %+v", report)
	}
	output := filepath.Join(t.TempDir(), "with-seek.flac")
	result, err := RebuildIndex(input, output)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Report.SeekPresent || len(result.Report.SeekChecks) != len(specs) {
		t.Fatalf("seek table was not added: %+v", result.Report)
	}
}

func TestRebuildRejectsCorruptAudioAndSameFile(t *testing.T) {
	specs := variableSpecs()
	file := buildTestFLAC(t, specs, validSeekForAudio(specs))
	base := inspectTestBytes(t, file)
	file[int(base.Frames[0].Offset)+base.Frames[0].HeaderSize] ^= 0x80
	input := writeTestFile(t, "bad.flac", file)
	output := filepath.Join(t.TempDir(), "must-not-exist.flac")
	if _, err := RebuildIndex(input, output); !errors.Is(err, ErrAudioCorrupt) {
		t.Fatalf("RebuildIndex error = %v, want ErrAudioCorrupt", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output exists after failed rebuild: %v", err)
	}
	if _, err := RebuildIndex(input, input); !errors.Is(err, ErrSameFile) {
		t.Fatalf("same-file error = %v", err)
	}
}

func writeTestFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func countMetadataType(blocks []MetadataBlock, blockType byte) int {
	count := 0
	for _, block := range blocks {
		if block.Type == blockType {
			count++
		}
	}
	return count
}
