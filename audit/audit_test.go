package audit_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"hdr10audit/audit"
	"hdr10audit/hdr"
	"hdr10audit/internal/fixture"
	"hdr10audit/mp4"
)

func writeFixture(t *testing.T, opts fixture.Options) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := fixture.Write(path, opts); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func baseOptions() fixture.Options {
	m, c := fixture.DefaultMetadata()
	return fixture.Options{
		ContainerMDCV: []hdr.Mastering{m},
		ContainerCLLI: []hdr.ContentLight{c},
		Bitstream: []hdr.Metadata{
			{Mastering: &m, Light: &c},
		},
	}
}

func TestAlignedMetadataUsesBitstream(t *testing.T) {
	path := writeFixture(t, baseOptions())
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK {
		t.Fatalf("aligned file should be OK: %+v", report.Tracks[0].Issues)
	}
	tr := report.Tracks[0]
	if len(tr.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", tr.Conflicts)
	}
	if tr.Effective.MasteringSource != "bitstream" || tr.Effective.LightsSource != "bitstream" {
		t.Fatalf("unexpected effective sources: %+v", tr.Effective)
	}
	if len(tr.Container.Locations) != 2 || len(tr.Bitstream.Locations) != 2 {
		t.Fatalf("occurrences container=%d bitstream=%d",
			len(tr.Container.Locations), len(tr.Bitstream.Locations))
	}
}

func TestConflictReportsBitstreamWinner(t *testing.T) {
	opts := baseOptions()
	streamMaster, streamLight := fixture.DefaultMetadata()
	streamMaster.Primaries[0].X = 14000
	streamLight.MaxCLL = 1200
	streamLight.MaxFALL = 350
	opts.Bitstream = []hdr.Metadata{{Mastering: &streamMaster, Light: &streamLight}}
	path := writeFixture(t, opts)
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatal("conflicting file should not be OK")
	}
	tr := report.Tracks[0]
	if len(tr.Conflicts) != 3 {
		t.Fatalf("want 3 conflicts, got %+v", tr.Conflicts)
	}
	for _, c := range tr.Conflicts {
		if c.Winner != "bitstream" {
			t.Fatalf("winner=%q, want bitstream", c.Winner)
		}
	}
	if tr.Effective.Mastering == nil || tr.Effective.Mastering.Primaries["G"].XRaw != 14000 {
		t.Fatalf("effective mastering did not follow bitstream: %+v", tr.Effective.Mastering)
	}
	if tr.Effective.Light == nil || tr.Effective.Light.MaxCLL != 1200 {
		t.Fatalf("effective CLLI did not follow bitstream: %+v", tr.Effective.Light)
	}
}

func TestMissingFieldsFallBackPerField(t *testing.T) {
	m, c := fixture.DefaultMetadata()
	opts := fixture.Options{
		ContainerMDCV: []hdr.Mastering{m},
		Bitstream:     []hdr.Metadata{{Light: &c}},
	}
	path := writeFixture(t, opts)
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	tr := report.Tracks[0]
	if tr.Effective.MasteringSource != "container" {
		t.Fatalf("mastering source=%q, want container fallback", tr.Effective.MasteringSource)
	}
	if tr.Effective.LightsSource != "bitstream" {
		t.Fatalf("light source=%q, want bitstream", tr.Effective.LightsSource)
	}
	if !hasCode(tr.Issues, "missing_container_metadata") ||
		!hasCode(tr.Issues, "missing_bitstream_metadata") {
		t.Fatalf("missing-field warnings absent: %+v", tr.Issues)
	}
	if report.OK {
		t.Fatal("per-field fallback should not make a missing-file fully OK")
	}
}

func TestCompletelyMissingMetadataIsError(t *testing.T) {
	path := writeFixture(t, fixture.Options{})
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || !hasCode(report.Tracks[0].Issues, "hdr10_metadata_missing") {
		t.Fatalf("missing metadata not reported as error: %+v", report.Tracks[0].Issues)
	}
}

func TestDuplicateBitstreamConflict(t *testing.T) {
	m1, c := fixture.DefaultMetadata()
	m2 := m1
	m2.Primaries[2].X = 35000
	opts := fixture.Options{
		ContainerMDCV: []hdr.Mastering{m1},
		ContainerCLLI: []hdr.ContentLight{c},
		Bitstream: []hdr.Metadata{
			{Mastering: &m1, Light: &c},
			{Mastering: &m2},
		},
	}
	path := writeFixture(t, opts)
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	tr := report.Tracks[0]
	if tr.Effective.Mastering != nil {
		t.Fatal("ambiguous bitstream mastering should not have an effective value")
	}
	if report.OK || !hasCode(tr.Issues, "duplicate_field_conflict") {
		t.Fatalf("duplicate conflict not reported: %+v", tr.Issues)
	}
}

func TestDuplicateContainerBoxIsReported(t *testing.T) {
	m, c := fixture.DefaultMetadata()
	opts := fixture.Options{
		ContainerMDCV: []hdr.Mastering{m, m},
		ContainerCLLI: []hdr.ContentLight{c},
		Bitstream:     []hdr.Metadata{{Mastering: &m, Light: &c}},
	}
	path := writeFixture(t, opts)
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(report.Tracks[0].Issues, "duplicate_metadata") {
		t.Fatalf("duplicate container box not reported: %+v", report.Tracks[0].Issues)
	}
}

func TestUnitAndRangeErrors(t *testing.T) {
	m, c := fixture.DefaultMetadata()
	m.Primaries[0].X = 60000
	m.MaxLuminance = 200000000
	c.MaxCLL = 60000
	c.MaxFALL = 61000
	opts := fixture.Options{
		ContainerMDCV: []hdr.Mastering{m},
		ContainerCLLI: []hdr.ContentLight{c},
		Bitstream:     []hdr.Metadata{{Mastering: &m, Light: &c}},
	}
	path := writeFixture(t, opts)
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	issues := report.Tracks[0].Issues
	for _, code := range []string{
		"chromaticity_out_of_range", "luminance_out_of_range",
		"max_cll_out_of_range", "max_fall_out_of_range",
	} {
		if !hasCode(issues, code) {
			t.Fatalf("missing issue %s: %+v", code, issues)
		}
	}
}

func TestScaledUnitErrorIsDetected(t *testing.T) {
	m, c := fixture.DefaultMetadata()
	m.Primaries[0] = hdr.Point{X: 708, Y: 292}
	m.WhitePoint = hdr.Point{X: 156, Y: 164}
	m.MaxLuminance = 1000
	opts := fixture.Options{
		ContainerMDCV: []hdr.Mastering{m},
		ContainerCLLI: []hdr.ContentLight{c},
		Bitstream:     []hdr.Metadata{{Mastering: &m, Light: &c}},
	}
	path := writeFixture(t, opts)
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	issues := report.Tracks[0].Issues
	if !hasCode(issues, "chromaticity_unit_suspected") ||
		!hasCode(issues, "luminance_unit_suspected") {
		t.Fatalf("scaled unit errors not detected: %+v", issues)
	}
}

func TestFixOnlyChangesContainerMetadata(t *testing.T) {
	opts := baseOptions()
	streamMaster, streamLight := fixture.DefaultMetadata()
	streamMaster.Primaries[0].X = 14000
	streamMaster.MaxLuminance = 8000000
	streamLight.MaxCLL = 1200
	streamLight.MaxFALL = 350
	opts.Bitstream = []hdr.Metadata{{Mastering: &streamMaster, Light: &streamLight}}
	path := writeFixture(t, opts)
	before, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "fixed.mp4")
	result, err := audit.FixContainerMetadata(path, out)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || !result.MediaUnchanged || result.Patches != 2 {
		t.Fatalf("unexpected fix result: %+v", result)
	}
	after, err := audit.Analyze(out)
	if err != nil {
		t.Fatal(err)
	}
	if after.MediaPayloadHash != before.MediaPayloadHash {
		t.Fatalf("media payload changed: before=%s after=%s",
			before.MediaPayloadHash, after.MediaPayloadHash)
	}
	if len(after.Tracks[0].Conflicts) != 0 || !after.OK {
		t.Fatalf("fixed file still has conflicts: %+v", after.Tracks[0].Conflicts)
	}
	if after.Tracks[0].Container.Locations[0].Mastering.Primaries["G"].XRaw != 14000 {
		t.Fatalf("container was not patched: %+v",
			after.Tracks[0].Container.Locations[0].Mastering)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(original) {
		t.Fatal("read-only source file was modified")
	}
	fixed, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) != len(original) {
		t.Fatalf("file size changed: %d -> %d", len(original), len(fixed))
	}
	allowed := metadataRanges(t, path)
	for i := range original {
		if original[i] != fixed[i] && !inRanges(int64(i), allowed) {
			t.Fatalf("non-metadata byte changed at %d", i)
		}
	}
	if bytes.Equal(original, fixed) {
		t.Fatal("expected metadata bytes to change")
	}
}

func TestFragmentedMP4BitstreamMetadata(t *testing.T) {
	opts := baseOptions()
	data, err := fixture.BuildFragmented(opts)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fragmented.mp4")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := audit.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.Tracks[0].Effective.MasteringSource != "bitstream" {
		t.Fatalf("fragmented audit failed: ok=%t issues=%+v",
			report.OK, report.Tracks[0].Issues)
	}
}

func hasCode(issues []hdr.Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func metadataRanges(t *testing.T, path string) []mp4.Range {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	info, err := mp4.NewFile(f, st.Size()).Inspect()
	if err != nil {
		t.Fatal(err)
	}
	var ranges []mp4.Range
	for _, track := range info.Tracks {
		for _, desc := range track.Descriptions {
			for _, box := range desc.MetadataBox {
				ranges = append(ranges, mp4.Range{
					Start: box.PayloadStart, Size: box.PayloadSize,
				})
			}
		}
	}
	return ranges
}

func inRanges(off int64, ranges []mp4.Range) bool {
	for _, r := range ranges {
		if off >= r.Start && off < r.Start+r.Size {
			return true
		}
	}
	return false
}
