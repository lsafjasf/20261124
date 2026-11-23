package tagfix

import (
	"bytes"
	"fmt"
)

// Change is the audit record for one synchronized field. A byte offset of -1
// means the field did not exist in the input. BitOffset is counted from the
// most significant bit of the byte.
type Change struct {
	Target        string `json:"target"`
	Old           any    `json:"old"`
	New           any    `json:"new"`
	ByteOffset    int64  `json:"byte_offset"`
	NewByteOffset int64  `json:"new_byte_offset"`
	BitOffset     int    `json:"bit_offset"`
	NewBitOffset  int    `json:"new_bit_offset"`
	Readback      any    `json:"readback"`
	ReadbackOK    bool   `json:"readback_ok"`
	Changed       bool   `json:"changed"`
}

// RollbackReport records whether the original file was preserved or restored.
type RollbackReport struct {
	Required  bool   `json:"required"`
	Performed bool   `json:"performed"`
	Verified  bool   `json:"verified"`
	Detail    string `json:"detail,omitempty"`
}

// Report is the machine-readable result of a build or apply operation.
type Report struct {
	File       string          `json:"file,omitempty"`
	Status     string          `json:"status"`
	Reason     string          `json:"reason,omitempty"`
	Track      int             `json:"track"`
	SampleType string          `json:"sample_entry"`
	Changes    []Change        `json:"changes"`
	Payload    []PayloadReport `json:"media_payload"`
	Rollback   RollbackReport  `json:"rollback"`
}

// Build validates and synchronizes the color tags entirely in memory. It never
// writes to disk and returns no output data when the request is rejected.
func Build(data []byte, tags ColorTags) ([]byte, *Report, error) {
	return BuildTrack(data, tags, 0)
}

// BuildTrack is Build for a specific AVC video track.
func BuildTrack(data []byte, tags ColorTags, trackIndex int) ([]byte, *Report, error) {
	old, err := analyze(data, trackIndex)
	if err != nil {
		report := &Report{
			Status:   "rejected",
			Reason:   err.Error(),
			Track:    trackIndex,
			Rollback: RollbackReport{Verified: true},
		}
		if old != nil {
			report.Track = old.track.index
			report.SampleType = old.track.sampleType
			report.Changes = diffChanges(old, old, tags)
			report.Payload = mergePayload(old.mdat, old.mdat)
		}
		return nil, report, err
	}
	report := &Report{
		Status:     "validated",
		Track:      old.track.index,
		SampleType: old.track.sampleType,
		Payload:    mergePayload(old.mdat, old.mdat),
		Rollback:   RollbackReport{Verified: true},
	}
	if err := tags.Validate(); err != nil {
		report.Status = "rejected"
		report.Reason = err.Error()
		report.Changes = diffChanges(old, old, tags)
		return nil, report, err
	}

	edits := newBoxEdits()
	colr := makeColrBox(tags)
	if old.track.colr != nil {
		edits.replace[old.track.colr] = colr
	} else {
		edits.insert[old.track.sample] = append(edits.insert[old.track.sample], colr)
	}

	replacements := make([][]byte, len(old.sps))
	for i, info := range old.sps {
		nal, patchErr := info.patch(tags)
		if patchErr != nil {
			return rejectBuild(report, old, tags,
				fmt.Errorf("SPS %d: %w", i, patchErr))
		}
		replacements[i] = nal
	}
	avccPayload, err := old.config.rebuild(replacements)
	if err != nil {
		return rejectBuild(report, old, tags, err)
	}
	avccBox, err := replacementBox(old.track.avcc, avccPayload)
	if err != nil {
		return rejectBuild(report, old, tags, err)
	}
	edits.replace[old.track.avcc] = avccBox

	newData, err := renderFile(data, old.top, edits)
	if err != nil {
		return rejectBuild(report, old, tags, err)
	}
	current, err := analyze(newData, trackIndex)
	if err != nil {
		return rejectBuild(report, old, tags, fmt.Errorf("%w: %v", ErrReadbackMismatch, err))
	}
	if err := verifyTags(current, tags); err != nil {
		report.Status = "rejected"
		report.Reason = err.Error()
		report.Changes = diffChanges(old, current, tags)
		report.Payload = mergePayload(old.mdat, current.mdat)
		return nil, report, err
	}
	report.Payload = mergePayload(old.mdat, current.mdat)
	for _, payload := range report.Payload {
		if !payload.Unchanged {
			err := fmt.Errorf("%w: mdat payload digest changed", ErrReadbackMismatch)
			report.Status = "rejected"
			report.Reason = err.Error()
			report.Changes = diffChanges(old, current, tags)
			return nil, report, err
		}
	}
	report.Changes = diffChanges(old, current, tags)
	if bytes.Equal(data, newData) {
		report.Status = "unchanged"
	}
	return newData, report, nil
}

func rejectBuild(report *Report, old *analysis, tags ColorTags, err error) ([]byte, *Report, error) {
	report.Status = "rejected"
	report.Reason = err.Error()
	report.Changes = diffChanges(old, old, tags)
	report.Payload = mergePayload(old.mdat, old.mdat)
	return nil, report, err
}

func verifyTags(current *analysis, want ColorTags) error {
	if current.colr == nil || !current.colr.valid || !current.colr.tags.equal(want) {
		got := any(nil)
		if current.colr != nil && current.colr.valid {
			got = current.colr.tags.String()
		}
		return fmt.Errorf("%w: container has %v, want %s",
			ErrReadbackMismatch, got, want)
	}
	for i, info := range current.sps {
		got, complete := info.tags()
		if !complete {
			return fmt.Errorf("%w: SPS %d has incomplete color VUI", ErrReadbackMismatch, i)
		}
		if !got.equal(want) {
			return fmt.Errorf("%w: SPS %d has %s, want %s",
				ErrReadbackMismatch, i, got, want)
		}
	}
	return nil
}

func mergePayload(old, current []PayloadReport) []PayloadReport {
	out := make([]PayloadReport, len(old))
	for i := range old {
		out[i] = old[i]
		if i < len(current) {
			out[i].NewByteOffset = current[i].ByteOffset
			out[i].NewSHA256 = current[i].SHA256
			out[i].Unchanged = old[i].SHA256 == current[i].SHA256 &&
				old[i].Size == current[i].Size
		} else {
			out[i].NewByteOffset = -1
			out[i].Unchanged = false
		}
	}
	return out
}

func diffChanges(old, current *analysis, expected ColorTags) []Change {
	var changes []Change
	oldContainer := old.containerSource()
	currentContainer := current.containerSource()
	changes = append(changes, sourceChanges(
		"container.colr", oldContainer, currentContainer, expected)...)
	oldBitstream := old.bitstreamSources()
	currentBitstream := current.bitstreamSources()
	for i := range currentBitstream {
		source := fmt.Sprintf("bitstream.avcC.sps[%d].vui", i)
		var oldSource tagSource
		if i < len(oldBitstream) {
			oldSource = oldBitstream[i]
		}
		changes = append(changes, sourceChanges(source, oldSource, currentBitstream[i], expected)...)
	}
	return changes
}

func sourceChanges(target string, old, current tagSource, expected ColorTags) []Change {
	var changes []Change
	for _, name := range []string{fieldPrimaries, fieldTransfer, fieldMatrix, fieldFullRange} {
		want := expectedValue(name, expected)
		oldField, oldFound := lookupField(old, name)
		newField, newFound := lookupField(current, name)
		readback := any(nil)
		if newFound {
			readback = newField.Value
		}
		change := Change{
			Target:        target + "." + name,
			New:           want,
			ByteOffset:    -1,
			NewByteOffset: -1,
			Readback:      readback,
			ReadbackOK:    newFound && valuesEqual(readback, want),
			Changed:       !oldFound || !valuesEqual(oldField.Value, want),
		}
		if oldFound {
			change.Old = oldField.Value
			change.ByteOffset = oldField.ByteOffset
			change.BitOffset = oldField.BitOffset
		}
		if newFound {
			change.NewByteOffset = newField.ByteOffset
			change.NewBitOffset = newField.BitOffset
		}
		changes = append(changes, change)
	}
	return changes
}

func lookupField(source tagSource, name string) (fieldSnapshot, bool) {
	for _, field := range source.Fields {
		if field.Field == name {
			return field, true
		}
	}
	return fieldSnapshot{}, false
}

func expectedValue(name string, tags ColorTags) any {
	switch name {
	case fieldPrimaries:
		return tags.Primaries
	case fieldTransfer:
		return tags.Transfer
	case fieldMatrix:
		return tags.Matrix
	case fieldFullRange:
		return tags.FullRange
	default:
		return nil
	}
}

func valuesEqual(a, b any) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}
