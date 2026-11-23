package wav

import "fmt"

const (
	FormatPCM        uint16 = 0x0001
	FormatIEEEFloat  uint16 = 0x0003
	FormatALaw       uint16 = 0x0006
	FormatMuLaw      uint16 = 0x0007
	FormatExtensible uint16 = 0xFFFE
)

const (
	SubFormatPCM       = "PCM"
	SubFormatIEEEFloat = "IEEE_FLOAT"
	SubFormatALaw      = "A_LAW"
	SubFormatMuLaw     = "MU_LAW"
	SubFormatUnknown   = "UNKNOWN"
)

// Speaker identifies a speaker position in a WAVEFORMATEXTENSIBLE channel mask.
type Speaker uint32

const (
	SpeakerFrontLeft          Speaker = 0x00000001
	SpeakerFrontRight         Speaker = 0x00000002
	SpeakerFrontCenter        Speaker = 0x00000004
	SpeakerLowFrequency       Speaker = 0x00000008
	SpeakerBackLeft           Speaker = 0x00000010
	SpeakerBackRight          Speaker = 0x00000020
	SpeakerFrontLeftOfCenter  Speaker = 0x00000040
	SpeakerFrontRightOfCenter Speaker = 0x00000080
	SpeakerBackCenter         Speaker = 0x00000100
	SpeakerSideLeft           Speaker = 0x00000200
	SpeakerSideRight          Speaker = 0x00000400
	SpeakerTopCenter          Speaker = 0x00000800
	SpeakerTopFrontLeft       Speaker = 0x00001000
	SpeakerTopFrontCenter     Speaker = 0x00002000
	SpeakerTopFrontRight      Speaker = 0x00004000
	SpeakerTopBackLeft        Speaker = 0x00008000
	SpeakerTopBackCenter      Speaker = 0x00010000
	SpeakerTopBackRight       Speaker = 0x00020000

	// SpeakerKnownMask contains every position currently assigned by Microsoft.
	SpeakerKnownMask uint32 = 0x0003FFFF
)

type speakerInfo struct {
	name      string
	shortName string
}

var speakerNames = map[Speaker]speakerInfo{
	SpeakerFrontLeft:          {"front-left", "FL"},
	SpeakerFrontRight:         {"front-right", "FR"},
	SpeakerFrontCenter:        {"front-center", "FC"},
	SpeakerLowFrequency:       {"low-frequency", "LFE"},
	SpeakerBackLeft:           {"back-left", "BL"},
	SpeakerBackRight:          {"back-right", "BR"},
	SpeakerFrontLeftOfCenter:  {"front-left-of-center", "FLC"},
	SpeakerFrontRightOfCenter: {"front-right-of-center", "FRC"},
	SpeakerBackCenter:         {"back-center", "BC"},
	SpeakerSideLeft:           {"side-left", "SL"},
	SpeakerSideRight:          {"side-right", "SR"},
	SpeakerTopCenter:          {"top-center", "TC"},
	SpeakerTopFrontLeft:       {"top-front-left", "TFL"},
	SpeakerTopFrontCenter:     {"top-front-center", "TFC"},
	SpeakerTopFrontRight:      {"top-front-right", "TFR"},
	SpeakerTopBackLeft:        {"top-back-left", "TBL"},
	SpeakerTopBackCenter:      {"top-back-center", "TBC"},
	SpeakerTopBackRight:       {"top-back-right", "TBR"},
}

// Name returns the descriptive Windows speaker name.
func (s Speaker) Name() string {
	if info, ok := speakerNames[s]; ok {
		return info.name
	}
	return "unknown"
}

// ShortName returns the conventional abbreviation for a speaker.
func (s Speaker) ShortName() string {
	if info, ok := speakerNames[s]; ok {
		return info.shortName
	}
	return "?"
}

// Known reports whether s is a defined Microsoft speaker position.
func (s Speaker) Known() bool {
	_, ok := speakerNames[s]
	return ok
}

// Channel describes the position assigned to one interleaved channel.
type Channel struct {
	Index     int     `json:"index"`
	Speaker   string  `json:"speaker"`
	ShortName string  `json:"short_name"`
	MaskBit   *uint32 `json:"mask_bit,omitempty"`
	Bit       int     `json:"bit"`
	Specified bool    `json:"specified"`
	Known     bool    `json:"known"`
}

// Chunk records the location of a RIFF chunk for auditing.
type Chunk struct {
	ID            string `json:"id"`
	HeaderOffset  int64  `json:"header_offset"`
	DataOffset    int64  `json:"data_offset"`
	DeclaredSize  uint64 `json:"declared_size"`
	AvailableSize uint64 `json:"available_size"`
	Truncated     bool   `json:"truncated"`
}

// SubFormat is the decoded KSDATAFORMAT_SUBTYPE_* GUID.
type SubFormat struct {
	GUID         string `json:"guid"`
	GUIDBytes    string `json:"guid_bytes"`
	FormatTag    uint32 `json:"format_tag"`
	FormatTagHex string `json:"format_tag_hex"`
	Name         string `json:"name"`
	Known        bool   `json:"known"`
	ValidTail    bool   `json:"valid_tail"`
}

// Extensible holds the WAVEFORMATEXTENSIBLE-specific fields.
type Extensible struct {
	ValidBitsPerSample uint16    `json:"valid_bits_per_sample"`
	ChannelMask        uint32    `json:"channel_mask"`
	ChannelMaskHex     string    `json:"channel_mask_hex"`
	UnknownMaskBits    uint32    `json:"unknown_mask_bits"`
	UnknownMaskBitsHex string    `json:"unknown_mask_bits_hex"`
	SubFormat          SubFormat `json:"subformat"`
}

// Format is the parsed fmt chunk.
type Format struct {
	Tag           uint16      `json:"tag"`
	TagHex        string      `json:"tag_hex"`
	Name          string      `json:"name"`
	Channels      uint16      `json:"channels"`
	SampleRate    uint32      `json:"sample_rate"`
	ByteRate      uint32      `json:"byte_rate"`
	BlockAlign    uint16      `json:"block_align"`
	BitsPerSample uint16      `json:"bits_per_sample"`
	Extensible    *Extensible `json:"extensible,omitempty"`
	ChunkSize     uint64      `json:"chunk_size"`
}

// Data describes the data chunk and the interleaved frame count derived from it.
// FrameCount uses the declared chunk size; AvailableFrames uses bytes actually
// present in the file. Channels are never inferred from either value.
type Data struct {
	Offset               int64  `json:"offset"`
	FirstSampleOffset    int64  `json:"first_sample_offset"`
	DeclaredSize         uint64 `json:"declared_size"`
	AvailableSize        uint64 `json:"available_size"`
	BlockAlign           uint16 `json:"block_align"`
	FrameCount           uint64 `json:"frame_count"`
	AvailableFrames      uint64 `json:"available_frames"`
	InterleavedSamples   uint64 `json:"interleaved_samples"`
	AvailableSamples     uint64 `json:"available_samples"`
	TrailingPartialBytes uint64 `json:"trailing_partial_bytes"`
}

// Issue is a validation finding. FirstSampleOffset is absolute and points at
// the first interleaved sample that cannot be interpreted safely.
type Issue struct {
	Severity          string `json:"severity"`
	Code              string `json:"code"`
	Message           string `json:"message"`
	FirstSampleOffset int64  `json:"first_sample_offset"`
}

// Header is the complete, reviewable result of parsing a WAV header.
type Header struct {
	FileSize   int64       `json:"file_size"`
	RIFFSize   uint32      `json:"riff_size"`
	Format     *Format     `json:"format,omitempty"`
	Extensible *Extensible `json:"extensible,omitempty"`
	Layout     string      `json:"layout"`
	Channels   []Channel   `json:"channels"`
	Data       *Data       `json:"data,omitempty"`
	Chunks     []Chunk     `json:"chunks"`
	Issues     []Issue     `json:"issues"`
	Valid      bool        `json:"valid"`
}

func formatName(tag uint16) string {
	switch tag {
	case FormatPCM:
		return "PCM"
	case FormatIEEEFloat:
		return "IEEE_FLOAT"
	case FormatALaw:
		return "A_LAW"
	case FormatMuLaw:
		return "MU_LAW"
	case FormatExtensible:
		return "WAVE_FORMAT_EXTENSIBLE"
	default:
		return fmt.Sprintf("UNKNOWN_0x%04X", tag)
	}
}

func bytesPerSample(bits uint16) uint32 {
	if bits == 0 {
		return 0
	}
	return (uint32(bits) + 7) / 8
}
