// Package fixture builds deterministic SDR/HDR review pairs in the VX1
// container. Both versions come from one shared timeline: same scene layout,
// frame numbers, cuts, subtitles and audio sample positions. The HDR encode
// injects documented defects so the inspector can be validated end to end.
package fixture

// Spec holds the shared timeline parameters.
type Spec struct {
	Width       int
	Height      int
	Frames      int
	RateNum     uint32
	RateDen     uint32
	AudioRate   uint32
	AudioPacket int // PCM frames per audio block

	// Defect positions on the HDR timeline (in timeline frame numbers).
	DropFrame         int    // duplicated-encode effect: one timeline frame missing
	BadNumFrame       int    // frame whose declared Num is corrupted
	BadNumValue       uint32 // wrong declared Num
	BadMeta           bool   // wrong color metadata in header
	SubtitleShiftTick int64  // subtitle boundary drift (ticks)
	AudioGapFrame     int    // audio packet starting after this frame drops samples
}

// Default returns the review fixture used by the command and the tests.
func Default() Spec {
	return Spec{
		Width:             160,
		Height:            90,
		Frames:            96, // 4.0 s at 24 fps
		RateNum:           24,
		RateDen:           1,
		AudioRate:         48000,
		AudioPacket:       2000, // 25 packets/sec -> 2000 samples per frame
		DropFrame:         41,   // 1.7083 s: dropped frame
		BadNumFrame:       58,   // 2.4167 s: wrong frame number metadata
		BadNumValue:       700,
		BadMeta:           true,
		SubtitleShiftTick: 2,  // subtitle 2 ticks late
		AudioGapFrame:     41, // same production incident as the drop
	}
}

// SampleRatePerFrame returns how many PCM samples cover one video frame.
func (s Spec) SampleRatePerFrame() int {
	return int(s.AudioRate) * int(s.RateDen) / int(s.RateNum)
}

// SceneCuts lists timeline frame numbers where the scene changes.
func (s Spec) SceneCuts() []int {
	return []int{0, 24, 48, 72}
}

// Subtitles returns the subtitle events of the shared timeline.
func (s Spec) Subtitles() []Sub {
	return []Sub{
		{ID: 1, Start: 0, End: 23, Text: "OPENING TITLE"},
		{ID: 2, Start: 24, End: 47, Text: "SCENE TWO"},
		{ID: 3, Start: 48, End: 71, Text: "SCENE THREE"},
		{ID: 4, Start: 72, End: 95, Text: "END CARD"},
	}
}

// Sub is a timeline-level subtitle event before container encoding.
type Sub struct {
	ID    uint32
	Start int64
	End   int64
	Text  string
}
