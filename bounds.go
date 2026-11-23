package au

// inspect computes the payload interval without changing any source bytes.
// Header byte counts are treated as claims: Data includes only bytes that are
// physically present, inside the claimed interval, and part of a complete
// interleaved sample frame.
func (f *File) inspect() {
	offset := uint64(f.Header.DataOffset)
	total := uint64(len(f.Raw))
	available := uint64(0)
	if offset < total {
		available = total - offset
	}
	f.AvailableBytes = available
	f.OffsetPastEOF = offset > total

	declared := available
	if f.Header.DataSize != UnknownDataSize {
		declared = uint64(f.Header.DataSize)
	}
	f.DeclaredBytes = declared
	f.SizeMismatch = f.Header.DataSize != UnknownDataSize && declared != available

	f.Format.BlockAlign = blockAlign(f.Format, f.Header.Channels)
	if f.Format.Kind == KindUnknown || f.Format.BlockAlign == 0 {
		// Without a sample width there is no frame boundary to enforce.
		usable := min(declared, available)
		f.Data = slice(f.Raw, offset, usable)
		f.ExtraBytes = slice(f.Raw, offset+usable, available-usable)
		f.Truncated = f.Header.DataSize != UnknownDataSize && declared > available
		return
	}

	block := f.Format.BlockAlign
	f.DeclaredFrames = declared / block
	f.AvailableFrames = available / block
	f.Frames = min(f.DeclaredFrames, f.AvailableFrames)

	usable := f.Frames * block
	f.Data = slice(f.Raw, offset, usable)
	f.ExtraBytes = slice(f.Raw, offset+usable, available-usable)

	// Only bytes that belong to whole, physically present frames are usable.
	// Bytes after such a frame cannot establish another complete frame.
	f.PartialBytes = available % block
	f.IncompleteTail = f.PartialBytes != 0
	f.Truncated = f.Header.DataSize != UnknownDataSize && declared > available
}

func blockAlign(f Format, channels uint32) uint64 {
	if f.BytesPerSample == 0 {
		return 0
	}
	return uint64(f.BytesPerSample) * uint64(channels)
}

// DeclaredSampleCount is the number of interleaved sample values claimed by a
// known encoding, including all channels. It returns zero for unknown
// encodings because their frame size cannot be established.
func (f *File) DeclaredSampleCount() uint64 {
	if f.Format.Kind == KindUnknown {
		return 0
	}
	return f.DeclaredFrames * uint64(f.Header.Channels)
}

// AvailableSampleCount is the number of interleaved sample values in complete
// frames physically present after DataOffset. It excludes a partial tail.
func (f *File) AvailableSampleCount() uint64 {
	if f.Format.Kind == KindUnknown {
		return 0
	}
	return f.AvailableFrames * uint64(f.Header.Channels)
}

// SampleCount is the number of interleaved sample values retained in Data.
func (f *File) SampleCount() uint64 {
	if f.Format.Kind == KindUnknown {
		return 0
	}
	return f.Frames * uint64(f.Header.Channels)
}

func slice(raw []byte, offset, length uint64) []byte {
	if length == 0 || offset >= uint64(len(raw)) {
		return nil
	}
	remaining := uint64(len(raw)) - offset
	if length > remaining {
		length = remaining
	}
	return raw[offset : offset+length]
}
