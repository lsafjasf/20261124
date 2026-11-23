package flacseek

func validateSeekTable(report *Report, info *StreamInfo, seekBlock int) {
	if !report.SeekPresent {
		return
	}
	frameByOffset := make(map[uint64]Frame, len(report.Frames))
	frameBySample := make(map[uint64]Frame, len(report.Frames))
	for _, frame := range report.Frames {
		relative := uint64(frame.Offset - report.AudioOffset)
		frameByOffset[relative] = frame
		frameBySample[frame.SampleStart] = frame
	}

	checks := make([]SeekPointCheck, len(report.SeekPoints))
	seenSamples := make(map[uint64]int)
	seenOffsets := make(map[uint64]int)
	lastReal := -1
	prevSample := uint64(0)
	havePrev := false
	placeholderSeen := false
	for i, point := range report.SeekPoints {
		check := SeekPointCheck{Index: i, Point: point}
		if point.Placeholder {
			if point.Offset != 0 || point.FrameSamples != 0 {
				check.Valid = false
				report.addIssue("seektable.placeholder", SeverityError,
					seekPointOffset(report, seekBlock, i),
					"placeholder seek point %d is malformed or not at the end", i)
			} else {
				check.Valid = true
			}
			placeholderSeen = true
			checks[i] = check
			continue
		}
		if placeholderSeen {
			check.Valid = false
			report.addIssue("seektable.order", SeverityError,
				seekPointOffset(report, seekBlock, i),
				"seek point %d follows a placeholder", i)
		}
		lastReal = i

		if prev, ok := seenSamples[point.SampleNumber]; ok {
			check.Duplicate = true
			check.Valid = false
			report.addIssue("seektable.duplicate_sample", SeverityError,
				seekPointOffset(report, seekBlock, i),
				"seek point %d repeats sample %d from point %d",
				i, point.SampleNumber, prev)
		} else {
			seenSamples[point.SampleNumber] = i
		}
		if prev, ok := seenOffsets[point.Offset]; ok {
			check.Duplicate = true
			check.Valid = false
			report.addIssue("seektable.duplicate_offset", SeverityError,
				seekPointOffset(report, seekBlock, i),
				"seek point %d repeats frame offset %d from point %d",
				i, point.Offset, prev)
		} else {
			seenOffsets[point.Offset] = i
		}
		if havePrev && point.SampleNumber <= prevSample {
			check.Valid = false
			report.addIssue("seektable.order", SeverityError,
				seekPointOffset(report, seekBlock, i),
				"seek points are not strictly increasing at index %d", i)
		}
		prevSample = point.SampleNumber
		havePrev = true

		frame, found := frameByOffset[point.Offset]
		check.FrameFound = found
		check.OffsetOK = found
		if found {
			check.SampleOK = frame.SampleStart == point.SampleNumber
			check.BlockSizeOK = frame.BlockSize == point.FrameSamples
		}
		if _, found := frameBySample[point.SampleNumber]; found && !check.SampleOK {
			report.addIssue("seektable.sample_mismatch", SeverityError,
				seekPointOffset(report, seekBlock, i),
				"seek point %d claims sample %d but offset %d is frame sample %d",
				i, point.SampleNumber, point.Offset, frame.SampleStart)
		}
		if found && !check.BlockSizeOK {
			report.addIssue("seektable.block_size_mismatch", SeverityError,
				seekPointOffset(report, seekBlock, i),
				"seek point %d claims %d samples, frame has %d",
				i, point.FrameSamples, frame.BlockSize)
		}
		if !found {
			report.addIssue("seektable.offset_mismatch", SeverityError,
				seekPointOffset(report, seekBlock, i),
				"seek point %d offset %d is not a frame boundary", i, point.Offset)
		}
		if point.FrameSamples == 0 {
			check.BlockSizeOK = false
			check.Valid = false
			report.addIssue("seektable.zero_block", SeverityError,
				seekPointOffset(report, seekBlock, i),
				"seek point %d has a zero frame sample count", i)
		}
		if info.TotalSamples > 0 {
			end := point.SampleNumber + uint64(point.FrameSamples)
			if point.SampleNumber >= info.TotalSamples ||
				uint64(point.FrameSamples) > info.TotalSamples-point.SampleNumber {
				check.OutOfRange = true
				check.Valid = false
				report.addIssue("seektable.out_of_range", SeverityError,
					seekPointOffset(report, seekBlock, i),
					"seek point %d spans samples %d..%d beyond total %d",
					i, point.SampleNumber, end, info.TotalSamples)
			}
		}
		if check.SampleOK && check.BlockSizeOK && check.OffsetOK && !check.Duplicate &&
			!check.OutOfRange {
			check.Valid = true
		}
		checks[i] = check
	}
	report.SeekChecks = checks

	if lastReal >= 0 {
		checks[lastReal].Last = true
		if !checks[lastReal].Valid {
			report.addIssue("seektable.last_invalid", SeverityError,
				seekPointOffset(report, seekBlock, lastReal),
				"last non-placeholder seek point %d does not identify a valid frame", lastReal)
		}
	}
}

func seekPointOffset(report *Report, seekBlock, index int) int64 {
	if seekBlock < 0 || seekBlock >= len(report.Metadata) {
		return report.AudioOffset
	}
	return report.Metadata[seekBlock].Offset + 4 + int64(index*18)
}
