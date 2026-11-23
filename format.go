package wav

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	extensibleFixedSize = 40
	extensibleCBMinSize = 22
)

var ksDataFormatTail = [12]byte{
	0x00, 0x00, 0x10, 0x00,
	0x80, 0x00, 0x00, 0xAA,
	0x00, 0x38, 0x9B, 0x71,
}

func (h *Header) parseFormat(data []byte, declaredSize uint64) *Format {
	if declaredSize < 16 || len(data) < 16 {
		h.addIssue("error", "truncated_fmt_chunk",
			fmt.Sprintf("fmt chunk declares %d bytes; at least 16 are required", declaredSize),
			noSampleOffset)
		return nil
	}

	format := &Format{
		Tag:           binary.LittleEndian.Uint16(data[0:2]),
		Channels:      binary.LittleEndian.Uint16(data[2:4]),
		SampleRate:    binary.LittleEndian.Uint32(data[4:8]),
		ByteRate:      binary.LittleEndian.Uint32(data[8:12]),
		BlockAlign:    binary.LittleEndian.Uint16(data[12:14]),
		BitsPerSample: binary.LittleEndian.Uint16(data[14:16]),
		ChunkSize:     declaredSize,
	}
	format.TagHex = fmt.Sprintf("0x%04X", format.Tag)
	format.Name = formatName(format.Tag)

	if format.Tag != FormatExtensible {
		return format
	}

	if declaredSize < extensibleFixedSize {
		h.addIssue("error", "truncated_extensible_fmt",
			fmt.Sprintf("WAVE_FORMAT_EXTENSIBLE fmt chunk has %d bytes; at least %d are required",
				declaredSize, extensibleFixedSize),
			noSampleOffset)
		return format
	}
	if len(data) < extensibleFixedSize {
		h.addIssue("error", "truncated_extensible_fmt",
			fmt.Sprintf("only %d of %d required extensible fmt bytes are available",
				len(data), extensibleFixedSize),
			noSampleOffset)
		return format
	}

	cbSize := binary.LittleEndian.Uint16(data[16:18])
	if cbSize < extensibleCBMinSize {
		h.addIssue("error", "invalid_extensible_cb_size",
			fmt.Sprintf("cbSize is %d; WAVEFORMATEXTENSIBLE requires at least %d",
				cbSize, extensibleCBMinSize),
			noSampleOffset)
	}
	if uint64(cbSize)+18 > declaredSize {
		h.addIssue("error", "extensible_cb_size_overflow",
			fmt.Sprintf("cbSize %d requires %d fmt bytes but the chunk declares %d",
				cbSize, uint64(cbSize)+18, declaredSize),
			noSampleOffset)
	}

	subFormat := parseSubFormatGUID(data[24:40])
	format.Extensible = &Extensible{
		ValidBitsPerSample: binary.LittleEndian.Uint16(data[18:20]),
		ChannelMask:        binary.LittleEndian.Uint32(data[20:24]),
		SubFormat:          subFormat,
	}
	format.Extensible.ChannelMaskHex = fmt.Sprintf("0x%08X", format.Extensible.ChannelMask)
	format.Extensible.UnknownMaskBits = format.Extensible.ChannelMask &^ SpeakerKnownMask
	format.Extensible.UnknownMaskBitsHex = fmt.Sprintf("0x%08X", format.Extensible.UnknownMaskBits)
	return format
}

// ParseSubFormatGUID decodes the 16-byte GUID from a WAVEFORMATEXTENSIBLE fmt
// chunk. The first four bytes are the little-endian format tag; the remaining
// bytes must match the Microsoft KSDATAFORMAT tail.
func ParseSubFormatGUID(guid []byte) (SubFormat, error) {
	if len(guid) != 16 {
		return SubFormat{}, fmt.Errorf("subformat GUID is %d bytes; want 16", len(guid))
	}
	return parseSubFormatGUID(guid), nil
}

func parseSubFormatGUID(guid []byte) SubFormat {
	tag := binary.LittleEndian.Uint32(guid[0:4])
	result := SubFormat{
		GUID:         formatGUID(guid),
		GUIDBytes:    fmt.Sprintf("%X", guid),
		FormatTag:    tag,
		FormatTagHex: fmt.Sprintf("0x%08X", tag),
		ValidTail:    bytes.Equal(guid[4:16], ksDataFormatTail[:]),
	}
	switch tag {
	case uint32(FormatPCM):
		result.Name = SubFormatPCM
		result.Known = true
	case uint32(FormatIEEEFloat):
		result.Name = SubFormatIEEEFloat
		result.Known = true
	case uint32(FormatALaw):
		result.Name = SubFormatALaw
		result.Known = true
	case uint32(FormatMuLaw):
		result.Name = SubFormatMuLaw
		result.Known = true
	default:
		result.Name = SubFormatUnknown
	}
	return result
}

func formatGUID(guid []byte) string {
	return fmt.Sprintf("%08X-%04X-%04X-%s-%s",
		binary.LittleEndian.Uint32(guid[0:4]),
		binary.LittleEndian.Uint16(guid[4:6]),
		binary.LittleEndian.Uint16(guid[6:8]),
		fmt.Sprintf("%02X%02X", guid[8], guid[9]),
		fmt.Sprintf("%02X%02X%02X%02X%02X%02X", guid[10], guid[11], guid[12], guid[13], guid[14], guid[15]),
	)
}
