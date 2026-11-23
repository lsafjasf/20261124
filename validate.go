package wav

import (
	"fmt"
	"math"
)

func (h *Header) validate() {
	h.validateFormat()
	h.validateChannels()
	h.validateData()
}

func (h *Header) validateFormat() {
	f := h.Format
	if f == nil {
		return
	}

	if f.Channels == 0 {
		h.addIssue("error", "invalid_channel_count",
			"fmt chunk declares zero channels", noSampleOffset)
	}
	if f.SampleRate == 0 {
		h.addIssue("error", "invalid_sample_rate",
			"fmt chunk declares a zero sample rate", noSampleOffset)
	}

	sampleBytes := bytesPerSample(f.BitsPerSample)
	if f.BitsPerSample == 0 || sampleBytes == 0 {
		h.addIssue("error", "invalid_bits_per_sample",
			fmt.Sprintf("fmt chunk declares %d bits per sample", f.BitsPerSample),
			noSampleOffset)
	}
	if f.BlockAlign == 0 {
		h.addIssue("error", "invalid_block_align",
			"fmt chunk declares a zero block alignment", noSampleOffset)
	}

	if f.Channels != 0 && sampleBytes != 0 {
		expected := uint64(f.Channels) * uint64(sampleBytes)
		if expected > math.MaxUint16 || uint64(f.BlockAlign) != expected {
			h.addIssue("error", "block_align_channel_mismatch",
				fmt.Sprintf(
					"block_align is %d but %d channels at %d bits/sample need %d bytes",
					f.BlockAlign, f.Channels, f.BitsPerSample, expected,
				),
				noSampleOffset)
		}
	}

	if f.SampleRate != 0 && f.BlockAlign != 0 {
		expected := uint64(f.SampleRate) * uint64(f.BlockAlign)
		if expected > math.MaxUint32 || uint64(f.ByteRate) != expected {
			h.addIssue("error", "byte_rate_mismatch",
				fmt.Sprintf(
					"byte_rate is %d but %d Hz times %d-byte frames is %d",
					f.ByteRate, f.SampleRate, f.BlockAlign, expected,
				),
				noSampleOffset)
		}
	}

	tag := f.Tag
	if f.Tag == FormatExtensible {
		if f.Extensible == nil {
			return
		}
		tag = uint16(f.Extensible.SubFormat.FormatTag)
		if !f.Extensible.SubFormat.ValidTail {
			h.addIssue("error", "invalid_subformat_guid",
				fmt.Sprintf("subformat GUID %s does not have the Microsoft KSDATAFORMAT tail",
					f.Extensible.SubFormat.GUID),
				noSampleOffset)
		}
		if !f.Extensible.SubFormat.Known {
			h.addIssue("error", "unsupported_subformat",
				fmt.Sprintf("subformat GUID %s has unsupported format tag 0x%08X",
					f.Extensible.SubFormat.GUID, f.Extensible.SubFormat.FormatTag),
				noSampleOffset)
		}
		if f.Extensible.ValidBitsPerSample > f.BitsPerSample {
			h.addIssue("error", "invalid_valid_bits",
				fmt.Sprintf("valid bits per sample is %d but container width is %d",
					f.Extensible.ValidBitsPerSample, f.BitsPerSample),
				noSampleOffset)
		}
	}

	if f.BitsPerSample == 0 {
		return
	}
	switch tag {
	case FormatPCM:
		if f.BitsPerSample%8 != 0 || f.BitsPerSample > 64 {
			h.addIssue("error", "unsupported_pcm_width",
				fmt.Sprintf("PCM container width %d bits is not a whole number of bytes up to 64",
					f.BitsPerSample),
				noSampleOffset)
		}
	case FormatIEEEFloat:
		if f.BitsPerSample != 32 && f.BitsPerSample != 64 {
			h.addIssue("error", "unsupported_float_width",
				fmt.Sprintf("IEEE float requires 32 or 64 bits per sample, got %d",
					f.BitsPerSample),
				noSampleOffset)
		}
	case FormatALaw, FormatMuLaw:
		if f.BitsPerSample != 8 {
			h.addIssue("error", "unsupported_law_width",
				fmt.Sprintf("A-law/mu-law requires 8 bits per sample, got %d",
					f.BitsPerSample),
				noSampleOffset)
		}
	default:
		if f.Tag != FormatExtensible || f.Extensible != nil {
			h.addIssue("error", "unsupported_format_tag",
				fmt.Sprintf("unsupported format tag 0x%04X", tag),
				noSampleOffset)
		}
	}
}

func (h *Header) validateChannels() {
	f := h.Format
	if f == nil {
		return
	}

	if f.Tag == FormatExtensible {
		if f.Extensible == nil {
			h.Channels = unspecifiedChannels(f.Channels)
			h.Layout = "unspecified"
			return
		}
		channels, layout, err := MapChannels(f.Channels, f.Extensible.ChannelMask)
		h.Channels = channels
		h.Layout = layout
		if err != nil {
			if mappingErr, ok := err.(*ChannelMappingError); ok {
				h.addIssue("error", mappingErr.Code, mappingErr.Message, noSampleOffset)
			} else {
				h.addIssue("error", "channel_mapping_error", err.Error(), noSampleOffset)
			}
		}
		return
	}

	h.Channels = unspecifiedChannels(f.Channels)
	if f.Channels == 1 {
		h.Layout = "mono"
	} else {
		h.Layout = fmt.Sprintf("unspecified-%d-channel", f.Channels)
	}
}

func unspecifiedChannels(count uint16) []Channel {
	channels := make([]Channel, int(count))
	for i := range channels {
		channels[i] = Channel{
			Index:     i,
			Speaker:   "unspecified",
			ShortName: "?",
			Specified: false,
			Known:     false,
		}
	}
	return channels
}

func (h *Header) validateData() {
	d := h.Data
	if d == nil {
		return
	}
	if d.BlockAlign == 0 {
		return
	}

	align := uint64(d.BlockAlign)
	d.FrameCount = d.DeclaredSize / align
	d.AvailableFrames = d.AvailableSize / align
	d.TrailingPartialBytes = d.DeclaredSize % align
	if h.Format != nil {
		d.InterleavedSamples = d.FrameCount * uint64(h.Format.Channels)
		d.AvailableSamples = d.AvailableFrames * uint64(h.Format.Channels)
	}

	if d.TrailingPartialBytes != 0 {
		offset := d.Offset + int64(d.FrameCount*align)
		h.addIssue("error", "partial_data_frame",
			fmt.Sprintf(
				"data declares %d bytes, which is not a multiple of block_align %d; %d byte(s) remain after the last complete frame",
				d.DeclaredSize, d.BlockAlign, d.TrailingPartialBytes,
			),
			offset)
	}

	if d.AvailableSize < d.DeclaredSize {
		firstBadFrame := d.AvailableFrames * align
		h.addIssue("error", "truncated_data_chunk",
			fmt.Sprintf(
				"data declares %d bytes but only %d are present; channel count is not inferred from available bytes",
				d.DeclaredSize, d.AvailableSize,
			),
			d.Offset+int64(firstBadFrame))
	} else if partial := d.AvailableSize % align; partial != 0 {
		h.addIssue("error", "partial_data_frame",
			fmt.Sprintf(
				"only %d byte(s) remain after the last complete frame in the available data",
				partial,
			),
			d.Offset+int64(d.AvailableSize-partial))
	}
}
