// Package vx implements VX1, a tiny self-contained media container used for
// frame-accurate SDR/HDR review. It is parsed with the Go standard library
// only; no external demuxer or decoder is involved.
//
// Layout (little-endian):
//
//	magic "VX1:" | version u8
//	header_json_len u32 | header JSON
//	blocks ...
//	  'V' u8 | len u32 | video frame payload
//	  'A' u8 | len u32 | audio packet payload
//	  'S' u8 | len u32 | subtitle payload
//	  'E' u8 | len u32(=0)
package vx

// Color describes the declared color metadata of a video stream.
type Color struct {
	Primaries string `json:"primaries"` // bt709 | bt2020
	Transfer  string `json:"transfer"`  // bt1886 | pq
	Matrix    string `json:"matrix"`    // bt709 | bt2020ncl
	Range     string `json:"range"`     // limited | full
}

// Audio describes the declared PCM audio stream.
type Audio struct {
	SampleRate uint32 `json:"sample_rate"`
	Channels   uint16 `json:"channels"`
	Depth      uint16 `json:"depth"` // bits per sample, only 16 is supported
}

// Header is the JSON document stored at the start of a VX1 file.
type Header struct {
	Version uint16 `json:"version"`
	Title   string `json:"title"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	RateNum uint32 `json:"rate_num"`
	RateDen uint32 `json:"rate_den"`
	Format  string `json:"pixel_format"` // only yuv444p8
	Color   Color  `json:"color"`
	Audio   Audio  `json:"audio"`
	Notes   string `json:"notes,omitempty"`
}

// Frame is one decoded video frame. The planes use limited-range 8-bit
// samples regardless of transfer function.
type Frame struct {
	Index   int    // parse order (0-based), assigned by the reader
	Num     uint32 // frame number declared in the bitstream
	PTS     int64  // presentation tick declared in the bitstream
	Key     bool   // declared keyframe flag
	Cut     bool   // declared scene-cut flag
	Y, U, V []byte // yuv444p8 planes, each Width*Height
}

// AudioPacket is one contiguous block of interleaved PCM samples.
type AudioPacket struct {
	Index       int
	SampleStart uint64 // first sample position declared on the audio timeline
	Samples     []int16
}

// Subtitle is one subtitle event with tick-granularity boundaries.
type Subtitle struct {
	ID    uint32 `json:"id"`
	Start int64  `json:"start_tick"`
	End   int64  `json:"end_tick"`
	Text  string `json:"text"`
}

// File is a fully parsed VX1 stream in block order.
type File struct {
	Header     Header
	HeaderJSON []byte
	Frames     []Frame
	Audio      []AudioPacket
	Subtitles  []Subtitle
}

// TickRate returns frames-per-second as a floating point number.
func (h Header) TickRate() float64 {
	if h.RateDen == 0 || h.RateNum == 0 {
		return 0
	}
	return float64(h.RateNum) / float64(h.RateDen)
}

// TickToSeconds converts a timeline tick to seconds.
func (h Header) TickToSeconds(tick int64) float64 {
	if h.RateNum == 0 {
		return 0
	}
	return float64(tick) * float64(h.RateDen) / float64(h.RateNum)
}
