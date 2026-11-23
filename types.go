package flacseek

import "fmt"

// AudioState describes whether the audio frames themselves are usable.
type AudioState string

const (
	AudioOK        AudioState = "ok"
	AudioCorrupt   AudioState = "corrupt"
	AudioTruncated AudioState = "truncated"
)

// IndexState describes whether a new SEEKTABLE can repair the file.
type IndexState string

const (
	IndexOK         IndexState = "ok"
	IndexRepairable IndexState = "repairable"
	IndexUnusable   IndexState = "unusable"
)

// Severity classifies an issue found during a scan.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Issue is a single diagnostic. Offset is absolute in the input file.
type Issue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Offset   int64    `json:"offset,omitempty"`
	Message  string   `json:"message"`
}

// StreamInfo is the decoded STREAMINFO metadata block.
type StreamInfo struct {
	MinBlockSize  uint16 `json:"min_block_size"`
	MaxBlockSize  uint16 `json:"max_block_size"`
	MinFrameSize  uint32 `json:"min_frame_size"`
	MaxFrameSize  uint32 `json:"max_frame_size"`
	SampleRate    uint32 `json:"sample_rate"`
	Channels      uint8  `json:"channels"`
	BitsPerSample uint8  `json:"bits_per_sample"`
	TotalSamples  uint64 `json:"total_samples"`
	MD5           string `json:"md5"`
}

// MetadataBlock is a metadata block retained for index rebuilding.
type MetadataBlock struct {
	Offset int64  `json:"offset"`
	Type   byte   `json:"type"`
	Length uint32 `json:"length"`
	Last   bool   `json:"last"`
	Data   []byte `json:"-"`
}

// SeekPoint is one raw SEEKTABLE entry.
type SeekPoint struct {
	SampleNumber uint64 `json:"sample_number"`
	Offset       uint64 `json:"offset"`
	FrameSamples uint16 `json:"frame_samples"`
	Placeholder  bool   `json:"placeholder,omitempty"`
}

// Frame is one CRC-validated audio frame.
type Frame struct {
	Offset        int64  `json:"offset"`
	Size          int64  `json:"size"`
	HeaderSize    int    `json:"header_size"`
	Blocking      byte   `json:"blocking"`
	Number        uint64 `json:"number"`
	SampleStart   uint64 `json:"sample_start"`
	BlockSize     uint16 `json:"block_size"`
	SampleRate    uint32 `json:"sample_rate"`
	Channels      uint8  `json:"channels"`
	BitsPerSample uint8  `json:"bits_per_sample"`
}

// SeekPointCheck compares one seek point with the scanned frame index.
type SeekPointCheck struct {
	Index       int       `json:"index"`
	Point       SeekPoint `json:"point"`
	Last        bool      `json:"last"`
	FrameFound  bool      `json:"frame_found"`
	SampleOK    bool      `json:"sample_ok"`
	BlockSizeOK bool      `json:"block_size_ok"`
	OffsetOK    bool      `json:"offset_ok"`
	Duplicate   bool      `json:"duplicate"`
	OutOfRange  bool      `json:"out_of_range"`
	Valid       bool      `json:"valid"`
}

// Report is the complete result of scanning a native FLAC file.
type Report struct {
	File           string           `json:"file,omitempty"`
	FileSize       int64            `json:"file_size"`
	SHA256         string           `json:"sha256,omitempty"`
	StreamInfo     *StreamInfo      `json:"stream_info,omitempty"`
	Metadata       []MetadataBlock  `json:"metadata"`
	AudioOffset    int64            `json:"audio_offset"`
	Frames         []Frame          `json:"frames,omitempty"`
	FrameCount     int              `json:"frame_count"`
	DecodedSamples uint64           `json:"decoded_samples"`
	SeekPresent    bool             `json:"seektable_present"`
	SeekPoints     []SeekPoint      `json:"seek_points,omitempty"`
	SeekChecks     []SeekPointCheck `json:"seek_checks,omitempty"`
	IndexState     IndexState       `json:"index_state"`
	AudioState     AudioState       `json:"audio_state"`
	Repairable     bool             `json:"repairable"`
	Issues         []Issue          `json:"issues"`
}

// RebuildResult describes an index-only rewrite.
type RebuildResult struct {
	Input          string  `json:"input"`
	Output         string  `json:"output"`
	InputSHA256    string  `json:"input_sha256"`
	OutputSHA256   string  `json:"output_sha256"`
	SeekPointCount int     `json:"seek_point_count"`
	Report         *Report `json:"output_report"`
}

func (r *Report) addIssue(code string, severity Severity, offset int64, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{
		Code: code, Severity: severity, Offset: offset,
		Message: fmt.Sprintf(format, args...),
	})
}
