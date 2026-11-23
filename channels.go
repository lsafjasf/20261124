package wav

import (
	"fmt"
	"math/bits"
	"strings"
)

// ChannelMappingError describes a channel-mask problem without a file offset.
// Parse wraps it in an Issue whose FirstSampleOffset is the data payload start.
type ChannelMappingError struct {
	Code    string
	Message string
}

func (e *ChannelMappingError) Error() string {
	return e.Message
}

// MapChannels maps every set channel-mask bit, in increasing bit order, to an
// interleaved channel index. A zero mask is accepted only for mono, where it
// means that no speaker position was assigned. Unknown mask bits are returned
// as specified-but-unknown channels and produce an error.
func MapChannels(channelCount uint16, mask uint32) ([]Channel, string, error) {
	if channelCount == 0 {
		return nil, "", &ChannelMappingError{
			Code:    "invalid_channel_count",
			Message: "fmt chunk declares zero channels",
		}
	}

	if mask == 0 {
		if channelCount != 1 {
			return nil, "", &ChannelMappingError{
				Code: "unspecified_channel_mask",
				Message: fmt.Sprintf(
					"channel mask is zero for %d channels; speaker-to-index mapping is unspecified",
					channelCount,
				),
			}
		}
		return []Channel{{
			Index:     0,
			Speaker:   "unspecified",
			ShortName: "M",
			Specified: false,
			Known:     false,
		}}, "mono", nil
	}

	channels := make([]Channel, 0, bits.OnesCount32(mask))
	unknownBits := mask &^ SpeakerKnownMask
	for bit := 0; bit < 32; bit++ {
		maskBit := uint32(1) << bit
		if mask&maskBit == 0 {
			continue
		}
		speaker := Speaker(maskBit)
		value := maskBit
		channels = append(channels, Channel{
			Index:     len(channels),
			Speaker:   speaker.Name(),
			ShortName: speaker.ShortName(),
			MaskBit:   &value,
			Bit:       bit,
			Specified: true,
			Known:     speaker.Known(),
		})
	}

	layout := classifyLayout(channelCount, mask)
	if unknownBits != 0 {
		return channels, layout, &ChannelMappingError{
			Code: "unknown_channel_mask_bits",
			Message: fmt.Sprintf(
				"channel mask 0x%08X contains unknown speaker bits 0x%08X (%s)",
				mask, unknownBits, formatUnknownBits(unknownBits),
			),
		}
	}
	if got := uint16(bits.OnesCount32(mask)); got != channelCount {
		return channels, layout, &ChannelMappingError{
			Code: "channel_mask_count_mismatch",
			Message: fmt.Sprintf(
				"channel mask 0x%08X has %d speaker positions but fmt declares %d channels",
				mask, got, channelCount,
			),
		}
	}
	return channels, layout, nil
}

func classifyLayout(channelCount uint16, mask uint32) string {
	if channelCount == 1 {
		return "mono"
	}
	switch mask {
	case 0x0000003F:
		return "5.1"
	case 0x0000060F:
		return "5.1-side"
	case 0x0000063F:
		return "7.1"
	case 0x000003FF:
		return "7.1-wide"
	}
	return fmt.Sprintf("custom-%d-channel", channelCount)
}

func formatUnknownBits(mask uint32) string {
	var names []string
	for bit := 0; bit < 32; bit++ {
		value := uint32(1) << bit
		if mask&value != 0 {
			names = append(names, fmt.Sprintf("%d", bit))
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return "bit " + strings.Join(names, ", ")
}
