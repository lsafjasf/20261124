package videoedit

import "encoding/json"

// EntryKind identifies the inverse operation stored in an undo entry.
type EntryKind string

const (
	EntryUpdateClip EntryKind = "updateClip"
	EntryInsertClip EntryKind = "insertClip"
	EntryDeleteClip EntryKind = "deleteClip"
)

// JournalEntry is one recoverable write. Replaying entries in reverse order
// restores the project to its pre-application state.
type JournalEntry struct {
	Kind      EntryKind `json:"kind"`
	TrackID   string    `json:"trackId"`
	ClipID    string    `json:"clipId"`
	ClipIndex int       `json:"clipIndex"`
	Before    *Clip     `json:"before,omitempty"`
	After     *Clip     `json:"after,omitempty"`
	SourceOp  int       `json:"sourceOp"`
	Note      string    `json:"note,omitempty"`
}

// Journal is the undo log produced while applying a change order.
type Journal struct {
	OrderID string         `json:"orderId"`
	Entries []JournalEntry `json:"entries"`
}

// Marshal serializes the journal to JSON.
func (j *Journal) Marshal() ([]byte, error) {
	return json.MarshalIndent(j, "", "  ")
}

// ParseJournal decodes a persisted undo log.
func ParseJournal(data []byte) (*Journal, error) {
	var journal Journal
	if err := json.Unmarshal(data, &journal); err != nil {
		return nil, err
	}
	return &journal, nil
}

func (j *Journal) append(entry JournalEntry) {
	j.Entries = append(j.Entries, entry)
}

// Rollback replays the journal in reverse order on the given project. It is
// used internally when an application fails and can also restore a journal
// that was persisted after a successful run.
func (j *Journal) Rollback(project *Project) error {
	for i := len(j.Entries) - 1; i >= 0; i-- {
		entry := j.Entries[i]
		trackIndex := refOfTrack(project, entry.TrackID)
		if trackIndex < 0 {
			return errTrackMissing(entry.TrackID)
		}
		switch entry.Kind {
		case EntryUpdateClip:
			if entry.Before == nil {
				return errJournalInconsistent(entry, "missing before snapshot")
			}
			idx := clipIndex(project.Tracks[trackIndex], entry.ClipID)
			if idx < 0 {
				return errJournalInconsistent(entry, "clip vanished")
			}
			project.Tracks[trackIndex].Clips[idx] = *entry.Before
		case EntryDeleteClip:
			if entry.Before == nil {
				return errJournalInconsistent(entry, "missing deleted snapshot")
			}
			project.Tracks[trackIndex].Clips = insertClip(
				project.Tracks[trackIndex].Clips, entry.ClipIndex, *entry.Before)
		case EntryInsertClip:
			idx := clipIndex(project.Tracks[trackIndex], entry.ClipID)
			if idx < 0 {
				return errJournalInconsistent(entry, "inserted clip missing")
			}
			project.Tracks[trackIndex].Clips = removeClip(project.Tracks[trackIndex].Clips, idx)
		default:
			return errJournalInconsistent(entry, "unknown entry kind")
		}
	}
	return nil
}

func clipIndex(track Track, clipID string) int {
	for i := range track.Clips {
		if track.Clips[i].ID == clipID {
			return i
		}
	}
	return -1
}

func insertClip(clips []Clip, index int, clip Clip) []Clip {
	if index >= len(clips) {
		return append(clips, clip)
	}
	clips = append(clips, Clip{})
	copy(clips[index+1:], clips[index:])
	clips[index] = clip
	return clips
}

func removeClip(clips []Clip, index int) []Clip {
	return append(clips[:index], clips[index+1:]...)
}
