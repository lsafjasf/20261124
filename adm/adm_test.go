package adm

import (
	"strings"
	"testing"
)

func baseFile() *File {
	return &File{
		RIFF: &RIFF{
			HasFmt: true,
			Format: FmtInfo{NumChannels: 1, SampleRate: 48000, ByteRate: 96000},
			Chunks: []Chunk{{ID: "fmt "}, {ID: "axml"}, {ID: "chna"}, {ID: "data", Size: 96000}},
		},
		Chna: &Chna{
			NumTracks: 1,
			NumUIDs:   1,
			Entries: []ChnaEntry{
				{TrackIndex: 1, UID: "ATU_00000001", TrackRef: "AT_00030001_01", PackRef: "AP_00030001"},
			},
		},
		Doc: &Document{
			Objects: []AudioObject{{
				ID: "AO_1001", Name: "Obj",
				PackRefIDs:   []string{"AP_00030001"},
				TrackUIDRefs: []string{"ATU_00000001"},
			}},
			Packs: []PackFormat{{
				ID: "AP_00030001", TypeDef: "Objects",
				ChannelRefIDs: []string{"AC_00030001"},
			}},
			Channels: []ChannelFormat{{
				ID: "AC_00030001", Name: "Obj", TypeDef: "Objects",
				BlockFormats: []BlockFormat{{
					ID: "AB_00030001_00000001", RTime: "00:00:00.00000", Duration: "00:00:01.00000",
					Positions: []Position{{Coordinate: "azimuth", Value: 0}},
				}},
			}},
			Tracks:    []TrackFormat{{ID: "AT_00030001_01"}},
			TrackUIDs: []TrackUID{{UID: "ATU_00000001", TrackRefID: "AT_00030001_01", PackRefID: "AP_00030001"}},
		},
	}
}

func codes(fs []Finding) map[string]int {
	m := map[string]int{}
	for _, f := range fs {
		m[f.Code]++
	}
	return m
}

func TestCleanFileHasNoErrors(t *testing.T) {
	for _, f := range Check(baseFile()) {
		if f.Severity == Error {
			t.Fatalf("unexpected error finding: %+v", f)
		}
	}
}

func TestMissingRefDetected(t *testing.T) {
	f := baseFile()
	f.Doc.Objects[0].PackRefIDs = []string{"AP_00039999"}
	got := codes(Check(f))
	if got["MISSING-REF"] == 0 {
		t.Fatal("expected MISSING-REF for dangling pack reference")
	}
}

func TestDuplicateUIDDetected(t *testing.T) {
	f := baseFile()
	f.Chna.Entries = append(f.Chna.Entries,
		ChnaEntry{TrackIndex: 1, UID: "ATU_00000001", TrackRef: "AT_00030001_01"})
	f.Chna.NumUIDs = 2
	got := codes(Check(f))
	if got["DUP-CHNA-UID"] == 0 || got["DUP-CHNA-TRACK"] == 0 {
		t.Fatalf("expected duplicate chna findings, got %v", got)
	}
	f2 := baseFile()
	f2.Doc.TrackUIDs = append(f2.Doc.TrackUIDs, TrackUID{UID: "ATU_00000001"})
	if codes(Check(f2))["DUP-ID"] == 0 {
		t.Fatal("expected DUP-ID for repeated audioTrackUID in axml")
	}
}

func TestXMLObjectWithoutAudioDetected(t *testing.T) {
	f := baseFile()
	// Object declared in XML whose UID has no chna row.
	f.Doc.TrackUIDs = append(f.Doc.TrackUIDs, TrackUID{UID: "ATU_00000002"})
	f.Doc.Objects = append(f.Doc.Objects, AudioObject{
		ID: "AO_1002", Name: "Ghost",
		PackRefIDs:   []string{"AP_00030001"},
		TrackUIDRefs: []string{"ATU_00000002"},
	})
	got := codes(Check(f))
	if got["OBJ-UNATTACHED"] == 0 || got["UID-NOT-IN-CHNA"] == 0 {
		t.Fatalf("expected unattached-object findings, got %v", got)
	}
	// Object with no track reference at all.
	f2 := baseFile()
	f2.Doc.Objects[0].TrackUIDRefs = nil
	if codes(Check(f2))["OBJ-NO-TRACK"] == 0 {
		t.Fatal("expected OBJ-NO-TRACK for object without audioTrackUIDRef")
	}
}

func TestChnaOrphanAndTrackCount(t *testing.T) {
	f := baseFile()
	f.Chna.Entries[0].UID = "ATU_DEADBEEF"
	got := codes(Check(f))
	if got["CHNA-ORPHAN"] == 0 || got["UID-NOT-IN-CHNA"] == 0 {
		t.Fatalf("expected chna/xml mismatch findings, got %v", got)
	}
	f2 := baseFile()
	f2.RIFF.Format.NumChannels = 2
	if codes(Check(f2))["TRACK-COUNT"] == 0 {
		t.Fatal("expected TRACK-COUNT mismatch")
	}
}

func TestTimelineGapAndOverlap(t *testing.T) {
	f := baseFile()
	f.Doc.Channels[0].BlockFormats = []BlockFormat{
		{ID: "AB_1", RTime: "00:00:00.00000", Duration: "00:00:00.40000"},
		{ID: "AB_2", RTime: "00:00:00.60000", Duration: "00:00:00.40000"},
	}
	_, fs := ProbeTimeline(f)
	if codes(fs)["BLK-GAP"] == 0 {
		t.Fatal("expected BLK-GAP")
	}
	f.Doc.Channels[0].BlockFormats[1].RTime = "00:00:00.30000"
	_, fs = ProbeTimeline(f)
	if codes(fs)["BLK-OVERLAP"] == 0 {
		t.Fatal("expected BLK-OVERLAP")
	}
}

func TestParseADMDuration(t *testing.T) {
	d, ok, err := ParseADMDuration("00:01:01.50000")
	if err != nil || !ok || d.Seconds() != 61.5 {
		t.Fatalf("got %v %v %v", d, ok, err)
	}
	if _, _, err := ParseADMDuration("junk"); err == nil {
		t.Fatal("expected error for junk time")
	}
}

func TestLoadMinimalFixture(t *testing.T) {
	f, err := Load("../testdata/minimal.bwf")
	if err != nil {
		t.Fatal(err)
	}
	for _, fnd := range Check(f) {
		if fnd.Severity == Error {
			t.Fatalf("fixture should be clean, got %+v", fnd)
		}
	}
	tl, _ := ProbeTimeline(f)
	if len(tl) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(tl))
	}
	var mover *ObjectTimeline
	for i := range tl {
		if tl[i].ObjectName == "Mover" {
			mover = &tl[i]
		}
	}
	if mover == nil || len(mover.Tracks) != 1 || mover.Tracks[0] != 2 {
		t.Fatalf("mover object not resolved to track 2: %+v", mover)
	}
	if len(mover.Channels) != 1 || len(mover.Channels[0].Blocks) != 2 {
		t.Fatalf("expected 1 channel with 2 blocks: %+v", mover.Channels)
	}
	if got := mover.Channels[0].Blocks[1].Positions["azimuth"]; got != 30 {
		t.Fatalf("expected azimuth 30 in second block, got %v", got)
	}
	if !strings.Contains(mover.ObjectID, "AO_") {
		t.Fatal("unexpected object ID")
	}
}
