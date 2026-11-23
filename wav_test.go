package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

type wavFixture struct {
	channels     uint16
	bits         uint16
	validBits    uint16
	mask         uint32
	subTag       uint32
	blockAlign   uint16
	byteRate     uint32
	data         []byte
	dataDeclared uint32
	badGUIDTail  bool
}

func buildFixture(f wavFixture) []byte {
	if f.bits == 0 {
		f.bits = 16
	}
	if f.subTag == 0 {
		f.subTag = uint32(FormatPCM)
	}
	if f.channels == 0 {
		f.channels = 2
	}
	if f.blockAlign == 0 {
		f.blockAlign = f.channels * uint16(bytesPerSample(f.bits))
	}
	if f.byteRate == 0 {
		f.byteRate = 48000 * uint32(f.blockAlign)
	}
	if f.validBits == 0 {
		f.validBits = f.bits
	}
	if f.dataDeclared == 0 {
		f.dataDeclared = uint32(len(f.data))
	}

	fmtData := make([]byte, 40)
	binary.LittleEndian.PutUint16(fmtData[0:2], FormatExtensible)
	binary.LittleEndian.PutUint16(fmtData[2:4], f.channels)
	binary.LittleEndian.PutUint32(fmtData[4:8], 48000)
	binary.LittleEndian.PutUint32(fmtData[8:12], f.byteRate)
	binary.LittleEndian.PutUint16(fmtData[12:14], f.blockAlign)
	binary.LittleEndian.PutUint16(fmtData[14:16], f.bits)
	binary.LittleEndian.PutUint16(fmtData[16:18], extensibleCBMinSize)
	binary.LittleEndian.PutUint16(fmtData[18:20], f.validBits)
	binary.LittleEndian.PutUint32(fmtData[20:24], f.mask)
	binary.LittleEndian.PutUint32(fmtData[24:28], f.subTag)
	copy(fmtData[28:40], ksDataFormatTail[:])
	if f.badGUIDTail {
		fmtData[39] ^= 0xFF
	}

	body := make([]byte, 0, 8+len(fmtData)+8+len(f.data)+1)
	body = appendChunk(body, "fmt ", fmtData)
	body = appendChunkWithDeclared(body, "data", f.data, f.dataDeclared)

	file := make([]byte, 12, 12+len(body))
	copy(file[0:4], "RIFF")
	binary.LittleEndian.PutUint32(file[4:8], uint32(4+len(body)))
	copy(file[8:12], "WAVE")
	return append(file, body...)
}

func appendChunk(dst []byte, id string, data []byte) []byte {
	return appendChunkWithDeclared(dst, id, data, uint32(len(data)))
}

func appendChunkWithDeclared(dst []byte, id string, data []byte, declared uint32) []byte {
	header := make([]byte, 8)
	copy(header[0:4], id)
	binary.LittleEndian.PutUint32(header[4:8], declared)
	dst = append(dst, header...)
	dst = append(dst, data...)
	if declared&1 != 0 {
		dst = append(dst, 0)
	}
	return dst
}

func mustParse(t *testing.T, file []byte) (*Header, error) {
	t.Helper()
	header, err := Parse(bytes.NewReader(file), int64(len(file)))
	if header == nil {
		t.Fatal("Parse returned a nil header")
	}
	return header, err
}

func findIssue(header *Header, code string) *Issue {
	for i := range header.Issues {
		if header.Issues[i].Code == code {
			return &header.Issues[i]
		}
	}
	return nil
}

func assertValid(t *testing.T, header *Header, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if !header.Valid || len(header.Issues) != 0 {
		t.Fatalf("header is not valid: valid=%v issues=%+v", header.Valid, header.Issues)
	}
}

func assertChannelNames(t *testing.T, channels []Channel, want ...string) {
	t.Helper()
	if len(channels) != len(want) {
		t.Fatalf("got %d channels, want %d: %+v", len(channels), len(want), channels)
	}
	for i := range want {
		if channels[i].Index != i {
			t.Errorf("channel %d has index %d", i, channels[i].Index)
		}
		if channels[i].Speaker != want[i] {
			t.Errorf("channel %d speaker = %q, want %q", i, channels[i].Speaker, want[i])
		}
	}
}

func TestMapChannelsMono(t *testing.T) {
	channels, layout, err := MapChannels(1, 0)
	if err != nil {
		t.Fatalf("MapChannels(1, 0): %v", err)
	}
	if layout != "mono" {
		t.Fatalf("layout = %q, want mono", layout)
	}
	if len(channels) != 1 || channels[0].Specified || channels[0].Speaker != "unspecified" {
		t.Fatalf("unexpected mono mapping: %+v", channels)
	}

	channels, layout, err = MapChannels(1, uint32(SpeakerFrontCenter))
	if err != nil {
		t.Fatalf("MapChannels mono front-center: %v", err)
	}
	assertChannelNames(t, channels, "front-center")
	if layout != "mono" {
		t.Fatalf("layout = %q, want mono", layout)
	}
}

func TestMapChannels51And71(t *testing.T) {
	channels, layout, err := MapChannels(6, 0x3F)
	if err != nil {
		t.Fatalf("MapChannels 5.1: %v", err)
	}
	if layout != "5.1" {
		t.Fatalf("layout = %q, want 5.1", layout)
	}
	assertChannelNames(t, channels,
		"front-left", "front-right", "front-center",
		"low-frequency", "back-left", "back-right")

	channels, layout, err = MapChannels(8, 0x63F)
	if err != nil {
		t.Fatalf("MapChannels 7.1: %v", err)
	}
	if layout != "7.1" {
		t.Fatalf("layout = %q, want 7.1", layout)
	}
	assertChannelNames(t, channels,
		"front-left", "front-right", "front-center", "low-frequency",
		"back-left", "back-right", "side-left", "side-right")
}

func TestMapChannelsUnknownBitAndCountMismatch(t *testing.T) {
	mask := uint32(SpeakerFrontLeft) | 1<<18
	channels, layout, err := MapChannels(2, mask)
	if err == nil {
		t.Fatal("MapChannels accepted an unknown mask bit")
	}
	var mappingErr *ChannelMappingError
	if !errors.As(err, &mappingErr) || mappingErr.Code != "unknown_channel_mask_bits" {
		t.Fatalf("unexpected error: %v", err)
	}
	if layout != "custom-2-channel" {
		t.Fatalf("layout = %q", layout)
	}
	if len(channels) != 2 || channels[1].Known || channels[1].Bit != 18 || !channels[1].Specified {
		t.Fatalf("unknown bit was not represented separately: %+v", channels)
	}

	_, _, err = MapChannels(2, 0x3F)
	if !errors.As(err, &mappingErr) || mappingErr.Code != "channel_mask_count_mismatch" {
		t.Fatalf("unexpected count-mismatch error: %v", err)
	}

	_, _, err = MapChannels(2, 0)
	if !errors.As(err, &mappingErr) || mappingErr.Code != "unspecified_channel_mask" {
		t.Fatalf("unexpected zero-mask error: %v", err)
	}
}

func TestParseExtensibleLayouts(t *testing.T) {
	tests := []struct {
		name       string
		channels   uint16
		mask       uint32
		data       []byte
		layout     string
		speakers   []string
		frameCount uint64
		samples    uint64
	}{
		{
			name:       "mono with zero mask",
			channels:   1,
			mask:       0,
			data:       make([]byte, 4),
			layout:     "mono",
			speakers:   []string{"unspecified"},
			frameCount: 2,
			samples:    2,
		},
		{
			name:       "5.1",
			channels:   6,
			mask:       0x3F,
			data:       make([]byte, 24),
			layout:     "5.1",
			speakers:   []string{"front-left", "front-right", "front-center", "low-frequency", "back-left", "back-right"},
			frameCount: 2,
			samples:    12,
		},
		{
			name:       "7.1",
			channels:   8,
			mask:       0x63F,
			data:       make([]byte, 32),
			layout:     "7.1",
			speakers:   []string{"front-left", "front-right", "front-center", "low-frequency", "back-left", "back-right", "side-left", "side-right"},
			frameCount: 2,
			samples:    16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := buildFixture(wavFixture{
				channels: tt.channels,
				mask:     tt.mask,
				data:     tt.data,
			})
			header, err := mustParse(t, file)
			assertValid(t, header, err)

			if header.Layout != tt.layout {
				t.Fatalf("layout = %q, want %q", header.Layout, tt.layout)
			}
			assertChannelNames(t, header.Channels, tt.speakers...)
			if header.Data == nil {
				t.Fatal("data chunk was not parsed")
			}
			if header.Data.FirstSampleOffset != 68 {
				t.Fatalf("first sample offset = %d, want 68", header.Data.FirstSampleOffset)
			}
			if header.Data.FrameCount != tt.frameCount {
				t.Fatalf("frame count = %d, want %d", header.Data.FrameCount, tt.frameCount)
			}
			if header.Data.InterleavedSamples != tt.samples {
				t.Fatalf("interleaved samples = %d, want %d", header.Data.InterleavedSamples, tt.samples)
			}
			if !header.Format.Extensible.SubFormat.Known || !header.Format.Extensible.SubFormat.ValidTail {
				t.Fatalf("unexpected subformat: %+v", header.Format.Extensible.SubFormat)
			}
			if header.Format.Extensible.SubFormat.GUID != "00000001-0000-0010-8000-00AA00389B71" {
				t.Fatalf("GUID = %q", header.Format.Extensible.SubFormat.GUID)
			}
		})
	}
}

func TestParseMaskMismatchReportsFirstSample(t *testing.T) {
	file := buildFixture(wavFixture{
		channels: 2,
		mask:     0x3F,
		data:     make([]byte, 8),
	})
	header, err := mustParse(t, file)
	if err == nil || header.Valid {
		t.Fatal("expected a channel-mask validation error")
	}
	issue := findIssue(header, "channel_mask_count_mismatch")
	if issue == nil {
		t.Fatalf("missing channel_mask_count_mismatch issue: %+v", header.Issues)
	}
	if header.Data == nil || issue.FirstSampleOffset != header.Data.FirstSampleOffset {
		t.Fatalf("issue offset = %d, first sample = %+v", issue.FirstSampleOffset, header.Data)
	}
	if len(header.Channels) != 6 {
		t.Fatalf("mapped %d channels from mask, want 6", len(header.Channels))
	}
	if header.Data.FrameCount != 2 {
		t.Fatalf("frame count = %d; it must come from block_align, not mask count", header.Data.FrameCount)
	}
}

func TestParseUnknownMaskBit(t *testing.T) {
	mask := uint32(SpeakerFrontLeft) | 1<<18
	file := buildFixture(wavFixture{
		channels: 2,
		mask:     mask,
		data:     make([]byte, 8),
	})
	header, err := mustParse(t, file)
	if err == nil || header.Valid {
		t.Fatal("expected an unknown-mask-bit validation error")
	}
	issue := findIssue(header, "unknown_channel_mask_bits")
	if issue == nil {
		t.Fatalf("missing unknown_channel_mask_bits issue: %+v", header.Issues)
	}
	if issue.FirstSampleOffset != 68 {
		t.Fatalf("first sample offset = %d, want 68", issue.FirstSampleOffset)
	}
	if header.Extensible == nil || header.Extensible.UnknownMaskBits != 1<<18 {
		t.Fatalf("unknown bits = %+v", header.Extensible)
	}
	if len(header.Channels) != 2 || header.Channels[1].Speaker != "unknown" ||
		header.Channels[1].Bit != 18 || header.Channels[1].Known {
		t.Fatalf("unknown channel was not kept distinct: %+v", header.Channels)
	}
}

func TestParsePartialFrameDoesNotInferChannels(t *testing.T) {
	file := buildFixture(wavFixture{
		channels: 6,
		mask:     0x3F,
		data:     make([]byte, 13),
	})
	header, err := mustParse(t, file)
	if err == nil || header.Valid {
		t.Fatal("expected a partial-frame validation error")
	}
	issue := findIssue(header, "partial_data_frame")
	if issue == nil {
		t.Fatalf("missing partial_data_frame issue: %+v", header.Issues)
	}
	if header.Data == nil {
		t.Fatal("data chunk was not parsed")
	}
	if issue.FirstSampleOffset != header.Data.Offset+12 {
		t.Fatalf("issue offset = %d, want %d", issue.FirstSampleOffset, header.Data.Offset+12)
	}
	if len(header.Channels) != 6 {
		t.Fatalf("channel count changed to %d based on data length", len(header.Channels))
	}
	if header.Data.FrameCount != 1 || header.Data.InterleavedSamples != 6 || header.Data.TrailingPartialBytes != 1 {
		t.Fatalf("unexpected derived data counts: %+v", header.Data)
	}
}

func TestParseTruncatedDataReportsUnalignedFrame(t *testing.T) {
	fullData := make([]byte, 24)
	file := buildFixture(wavFixture{
		channels:     6,
		mask:         0x3F,
		data:         fullData[:20],
		dataDeclared: 24,
	})
	header, err := mustParse(t, file)
	if err == nil || header.Valid {
		t.Fatal("expected a truncated-data validation error")
	}
	issue := findIssue(header, "truncated_data_chunk")
	if issue == nil {
		t.Fatalf("missing truncated_data_chunk issue: %+v", header.Issues)
	}
	if header.Data == nil {
		t.Fatal("data chunk was not parsed")
	}
	wantOffset := header.Data.Offset + 12
	if issue.FirstSampleOffset != wantOffset {
		t.Fatalf("issue offset = %d, want %d", issue.FirstSampleOffset, wantOffset)
	}
	if len(header.Channels) != 6 {
		t.Fatalf("channel count changed to %d based on truncated data", len(header.Channels))
	}
}

func TestParseBlockAlignMismatchReportsFirstSample(t *testing.T) {
	file := buildFixture(wavFixture{
		channels:   6,
		mask:       0x3F,
		blockAlign: 10,
		data:       make([]byte, 20),
	})
	header, err := mustParse(t, file)
	if err == nil || header.Valid {
		t.Fatal("expected a block-align validation error")
	}
	issue := findIssue(header, "block_align_channel_mismatch")
	if issue == nil {
		t.Fatalf("missing block_align_channel_mismatch issue: %+v", header.Issues)
	}
	if header.Data == nil || issue.FirstSampleOffset != header.Data.FirstSampleOffset {
		t.Fatalf("issue offset = %d, data = %+v", issue.FirstSampleOffset, header.Data)
	}
}

func TestParseBadSubformatGUID(t *testing.T) {
	file := buildFixture(wavFixture{
		channels:    2,
		mask:        0x3,
		data:        make([]byte, 8),
		badGUIDTail: true,
	})
	header, err := mustParse(t, file)
	if err == nil || header.Valid {
		t.Fatal("expected an invalid-GUID validation error")
	}
	issue := findIssue(header, "invalid_subformat_guid")
	if issue == nil {
		t.Fatalf("missing invalid_subformat_guid issue: %+v", header.Issues)
	}
	if header.Extensible == nil || header.Extensible.SubFormat.ValidTail {
		t.Fatalf("invalid GUID tail was not recorded: %+v", header.Extensible)
	}
	if issue.FirstSampleOffset != 68 {
		t.Fatalf("first sample offset = %d, want 68", issue.FirstSampleOffset)
	}
}

func TestParseSubFormatGUIDRequires16Bytes(t *testing.T) {
	if _, err := ParseSubFormatGUID(make([]byte, 15)); err == nil {
		t.Fatal("ParseSubFormatGUID accepted 15 bytes")
	}
}
