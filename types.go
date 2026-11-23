package truehd

import "fmt"

// ErrorKind keeps transport damage separate from truncated input.
type ErrorKind string

const (
	HeaderError  ErrorKind = "header_error"
	PayloadError ErrorKind = "payload_error"
	EOFError     ErrorKind = "eof_error"
)

// Issue describes one unusable span of the input. RawEnd is exclusive.
// A header issue may cover several packets because its framing was lost;
// payload and EOF issues normally cover exactly one packet or tail.
type Issue struct {
	Kind         ErrorKind `json:"kind"`
	PacketIndex  int       `json:"packet_index"`
	RawStart     int64     `json:"raw_start"`
	RawEnd       int64     `json:"raw_end"`
	DeclaredSize int       `json:"declared_size,omitempty"`
	Reason       string    `json:"reason"`
	RecoveredAt  int64     `json:"recovered_at,omitempty"`
}

// Frame is a complete access unit whose header and available payload checks
// passed. Packet framing is therefore trustworthy for this span.
type Frame struct {
	Index           int     `json:"index"`
	PacketIndex     int     `json:"packet_index"`
	RawStart        int64   `json:"raw_start"`
	RawEnd          int64   `json:"raw_end"`
	Size            int     `json:"size"`
	MajorSync       bool    `json:"major_sync"`
	SampleRate      int     `json:"sample_rate"`
	Samples         int     `json:"samples"`
	Channels        int     `json:"channels"`
	Substreams      int     `json:"substreams"`
	PayloadChecked  bool    `json:"payload_checked"`
	DurationSeconds float64 `json:"duration_seconds"`
}

// PlayableRange is a contiguous run of complete frames in the input bytes.
// DecodedStartSeconds is a timeline for the playable output, not a container
// timestamp.
type PlayableRange struct {
	RawStart            int64   `json:"raw_start"`
	RawEnd              int64   `json:"raw_end"`
	FirstFrameIndex     int     `json:"first_frame_index"`
	FrameCount          int     `json:"frame_count"`
	Samples             int64   `json:"samples"`
	DecodedStartSeconds float64 `json:"decoded_start_seconds"`
	DecodedEndSeconds   float64 `json:"decoded_end_seconds"`
}

// Report is the complete diagnostic result for one TrueHD elementary stream.
type Report struct {
	FileSize                int64           `json:"file_size"`
	Complete                bool            `json:"complete"`
	Truncated               bool            `json:"truncated"`
	HeaderErrors            int             `json:"header_errors"`
	PayloadErrors           int             `json:"payload_errors"`
	EOFErrors               int             `json:"eof_errors"`
	UncheckedPayloadFrames  int             `json:"unchecked_payload_frames"`
	Frames                  []Frame         `json:"frames"`
	Issues                  []Issue         `json:"issues"`
	DiscardedPackets        []Issue         `json:"discarded_packets"`
	PlayableRanges          []PlayableRange `json:"playable_ranges"`
	PlayableBytes           int64           `json:"playable_bytes"`
	PlayableSamples         int64           `json:"playable_samples"`
	PlayableDurationSeconds float64         `json:"playable_duration_seconds"`
	DiscardedBytes          int64           `json:"discarded_bytes"`
}

func (r *Report) String() string {
	return fmt.Sprintf("%d frames, %d playable range(s), %d header/payload/eof errors (%d/%d/%d)",
		len(r.Frames), len(r.PlayableRanges), r.HeaderErrors+r.PayloadErrors+r.EOFErrors,
		r.HeaderErrors, r.PayloadErrors, r.EOFErrors)
}
