package adm

import (
	"fmt"
	"sort"
)

// Severity of a Finding.
type Severity int

const (
	Info Severity = iota
	Warn
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	default:
		return "ERROR"
	}
}

// Finding is one problem or observation from the cross-check.
type Finding struct {
	Severity Severity
	Code     string
	Message  string
}

// File is the fully parsed ADM BWF input for the checker.
type File struct {
	RIFF *RIFF
	Chna *Chna     // nil if chna chunk absent
	Doc  *Document // nil if axml chunk absent
}

// Check runs all structural cross-checks and returns findings ordered by
// severity (errors first).
func Check(f *File) []Finding {
	var out []Finding
	if f.RIFF == nil {
		return []Finding{{Error, "NO-RIFF", "file is not a RIFF/WAVE container"}}
	}
	if !f.RIFF.HasFmt {
		out = append(out, Finding{Error, "NO-FMT", "fmt chunk missing"})
	}
	if f.RIFF.Find("data") == nil {
		out = append(out, Finding{Error, "NO-DATA", "data chunk missing"})
	}
	if f.Chna == nil {
		out = append(out, Finding{Error, "NO-CHNA", "chna chunk missing: cannot map ADM UIDs to physical tracks"})
	}
	if f.Doc == nil {
		out = append(out, Finding{Error, "NO-AXML", "axml chunk missing: no ADM metadata to verify"})
		return sortFindings(out)
	}
	out = append(out, checkDuplicates(f)...)
	out = append(out, checkXMLRefs(f.Doc)...)
	if f.Chna != nil {
		out = append(out, checkChnaVsXML(f.Chna, f.Doc)...)
		out = append(out, checkChnaVsFmt(f.Chna, f.RIFF)...)
	}
	out = append(out, checkAttachment(f)...)
	return sortFindings(out)
}

func sortFindings(fs []Finding) []Finding {
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Severity > fs[j].Severity })
	return fs
}

// checkDuplicates flags duplicate IDs/UIDs inside the XML and the chna table.
func checkDuplicates(f *File) []Finding {
	var out []Finding
	seen := map[string]bool{}
	add := func(kind, id string) {
		if id == "" {
			return
		}
		key := kind + ":" + id
		if seen[key] {
			out = append(out, Finding{Error, "DUP-ID",
				fmt.Sprintf("duplicate %s ID %s", kind, id)})
			return
		}
		seen[key] = true
	}
	for _, p := range f.Doc.Programmes {
		add("audioProgramme", p.ID)
	}
	for _, c := range f.Doc.Contents {
		add("audioContent", c.ID)
	}
	for _, o := range f.Doc.Objects {
		add("audioObject", o.ID)
	}
	for _, p := range f.Doc.Packs {
		add("audioPackFormat", p.ID)
	}
	for _, c := range f.Doc.Channels {
		add("audioChannelFormat", c.ID)
	}
	for _, s := range f.Doc.Streams {
		add("audioStreamFormat", s.ID)
	}
	for _, t := range f.Doc.Tracks {
		add("audioTrackFormat", t.ID)
	}
	for _, u := range f.Doc.TrackUIDs {
		add("audioTrackUID", u.UID)
	}
	if f.Chna != nil {
		uidSeen := map[string]uint16{}
		trkSeen := map[uint16]string{}
		for _, e := range f.Chna.Entries {
			if prev, dup := uidSeen[e.UID]; dup {
				out = append(out, Finding{Error, "DUP-CHNA-UID",
					fmt.Sprintf("chna: UID %s assigned to both track %d and track %d", e.UID, prev, e.TrackIndex)})
			}
			uidSeen[e.UID] = e.TrackIndex
			if prev, dup := trkSeen[e.TrackIndex]; dup {
				out = append(out, Finding{Error, "DUP-CHNA-TRACK",
					fmt.Sprintf("chna: track %d has two UIDs (%s and %s)", e.TrackIndex, prev, e.UID)})
			}
			trkSeen[e.TrackIndex] = e.UID
		}
	}
	return out
}
