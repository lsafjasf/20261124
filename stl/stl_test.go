package stl

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func tc(h, m, s, f uint8) Timecode {
	return Timecode{Hours: h, Minutes: m, Seconds: s, Frames: f}
}

func baseGSI(tnb, tns, tng int) []byte {
	g := bytes.Repeat([]byte{' '}, GSIBlockSize)
	copy(g[0:3], "850")
	copy(g[3:11], "STL25.01")
	g[11] = '1'
	copy(g[12:14], "00")
	copy(g[14:16], "09")
	copy(g[16:48], "TEST PROGRAMME")
	copy(g[238:243], fmt.Sprintf("%05d", tnb))
	copy(g[243:248], fmt.Sprintf("%05d", tns))
	copy(g[248:251], fmt.Sprintf("%03d", tng))
	copy(g[251:253], "40")
	copy(g[253:255], "23")
	g[255] = '1'
	copy(g[256:264], "10000000")
	copy(g[264:272], "10000000")
	g[272] = '1'
	g[273] = '1'
	copy(g[274:277], "FRA")
	return g
}

func makeBlock(sgn uint8, sn uint16, ebn uint8, in, out Timecode, text string) []byte {
	b := make([]byte, TTIBlockSize)
	b[0] = sgn
	binary.LittleEndian.PutUint16(b[1:3], sn)
	b[3] = ebn
	b[4] = 0
	b[5], b[6], b[7], b[8] = in.Hours, in.Minutes, in.Seconds, in.Frames
	b[9], b[10], b[11], b[12] = out.Hours, out.Minutes, out.Seconds, out.Frames
	b[13] = 20
	b[14] = 2
	b[15] = 0
	copy(b[16:], text)
	for i := 16 + len(text); i < len(b); i++ {
		b[i] = unusedSpace
	}
	return b
}

func makeSTL(gsi []byte, blocks ...[]byte) []byte {
	var out bytes.Buffer
	out.Write(gsi)
	for _, b := range blocks {
		out.Write(b)
	}
	return out.Bytes()
}

func categories(r Report) map[string]int {
	m := make(map[string]int)
	for _, issue := range r.Issues {
		m[issue.Category]++
	}
	return m
}

func fieldsFor(r Report, category string) []string {
	var out []string
	for _, issue := range r.Issues {
		if issue.Category == category {
			out = append(out, issue.Field)
		}
	}
	return out
}

func TestValidFileHasNoFindings(t *testing.T) {
	g := baseGSI(2, 2, 1)
	in := tc(10, 0, 0, 0)
	blocks := [][]byte{
		makeBlock(0, 1, 0xFF, in, tc(10, 0, 2, 0), "Hello"),
		makeBlock(0, 2, 0xFF, tc(10, 0, 3, 0), tc(10, 0, 5, 0), "World"),
	}
	r := Parse(makeSTL(g, blocks...)).Validate()
	if len(r.Issues) != 0 {
		t.Fatalf("expected no findings, got %#v", r.Issues)
	}
	if r.Actual.TTIBlocks != 2 || r.Actual.Subtitles != 2 ||
		r.Actual.NonEmptySubtitles != 2 || r.Actual.SubtitleGroups != 1 {
		t.Fatalf("unexpected actual counts: %#v", r.Actual)
	}
}

func TestDeclaredCountsAndRateAreCheckedAgainstTTI(t *testing.T) {
	g := baseGSI(3, 1, 2)
	blocks := [][]byte{
		makeBlock(0, 1, 0xFF, tc(10, 0, 0, 0), tc(10, 0, 2, 0), "One"),
		makeBlock(0, 2, 0xFF, tc(10, 0, 3, 0), tc(10, 0, 5, 0), "Two"),
	}
	blocks[0][8] = 27
	blocks[0][12] = 27
	copy(g[264:272], "10000027")
	cats := categories(Parse(makeSTL(g, blocks...)).Validate())
	for _, want := range []string{CategoryCount, CategoryTimecode, CategoryFormat} {
		if cats[want] == 0 {
			t.Errorf("expected category %q, got %#v", want, cats)
		}
	}
}

func TestDisplayAndLanguageDeclarations(t *testing.T) {
	g := baseGSI(1, 1, 1)
	g[11] = '9'
	copy(g[12:14], "01")
	block := makeBlock(0, 1, 0xFF, tc(10, 0, 0, 0), tc(10, 0, 2, 0), "Hello")
	cats := categories(Parse(makeSTL(g, block)).Validate())
	if cats[CategoryDisplay] == 0 || cats[CategoryLanguage] == 0 {
		t.Fatalf("expected display and language findings, got %#v", cats)
	}
}

func TestEmptySubtitleAndDuplicateBlockAreSeparate(t *testing.T) {
	g := baseGSI(3, 2, 1)
	empty := makeBlock(0, 1, 0xFF, tc(10, 0, 0, 0), tc(10, 0, 1, 0), "")
	text := makeBlock(0, 2, 0xFF, tc(10, 0, 2, 0), tc(10, 0, 3, 0), "Hello")
	dup := append([]byte(nil), text...)
	r := Parse(makeSTL(g, empty, text, dup)).Validate()
	cats := categories(r)
	if cats[CategoryEmpty] != 1 {
		t.Fatalf("expected one empty-subtitle finding, got %#v", r.Issues)
	}
	if cats[CategoryDuplicate] != 1 {
		t.Fatalf("expected one duplicate-block finding, got %#v", r.Issues)
	}
	if r.Actual.EmptySubtitles != 1 || r.Actual.DuplicateBlocks != 1 {
		t.Fatalf("unexpected actual counts: %#v", r.Actual)
	}
}

func TestExtensionBlockNumbersAreSequential(t *testing.T) {
	g := baseGSI(2, 1, 1)
	first := makeBlock(0, 4, 0x01, tc(10, 0, 0, 0), tc(10, 0, 2, 0), strings.Repeat("a", 20))
	last := makeBlock(0, 4, 0xFF, tc(10, 0, 0, 0), tc(10, 0, 2, 0), "tail")
	r := Parse(makeSTL(g, first, last)).Validate()
	if fieldsFor(r, CategoryBlockNo) == nil {
		t.Fatalf("expected block-number finding, got %#v", r.Issues)
	}
	if !strings.Contains(r.Issues[0].Message, "expected extension block 0x00") {
		t.Fatalf("unexpected message: %s", r.Issues[0].Message)
	}
}

func TestValidExtensionBlocksHaveNoFindings(t *testing.T) {
	g := baseGSI(2, 1, 1)
	firstText := strings.Repeat("a", 37) + "\x8a" + strings.Repeat("b", 37) + "\x8a" + strings.Repeat("c", 36)
	if len(firstText) != TextFieldSize {
		t.Fatalf("test setup: first text block is %d bytes", len(firstText))
	}
	first := makeBlock(0, 1, 0x00, tc(10, 0, 0, 0), tc(10, 0, 2, 0), firstText)
	last := makeBlock(0, 1, 0xFF, tc(10, 0, 0, 0), tc(10, 0, 2, 0), "tail")
	r := Parse(makeSTL(g, first, last)).Validate()
	if len(r.Issues) != 0 {
		t.Fatalf("expected valid extension blocks, got %#v", r.Issues)
	}
}

func TestSubtitleNumbersAreSequentialWithinGroup(t *testing.T) {
	g := baseGSI(2, 2, 1)
	blocks := [][]byte{
		makeBlock(0, 1, 0xFF, tc(10, 0, 0, 0), tc(10, 0, 2, 0), "One"),
		makeBlock(0, 3, 0xFF, tc(10, 0, 3, 0), tc(10, 0, 5, 0), "Three"),
	}
	r := Parse(makeSTL(g, blocks...)).Validate()
	if fields := fieldsFor(r, CategoryBlockNo); strings.Join(fields, ",") != "SN" {
		t.Fatalf("expected an SN finding, got %#v", r.Issues)
	}
}

func TestOverlongTextIsReportedWithoutTruncationAdvice(t *testing.T) {
	g := baseGSI(1, 1, 1)
	copy(g[251:253], "20")
	text := strings.Repeat("中", 36) + "    "
	block := makeBlock(0, 1, 0xFF, tc(10, 0, 0, 0), tc(10, 0, 2, 0), text)
	raw := makeSTL(g, block)
	snapshot := append([]byte(nil), raw...)
	r := Parse(raw).Validate()
	cats := categories(r)
	for _, want := range []string{CategoryOverlong, CategoryText} {
		if cats[want] == 0 {
			t.Errorf("expected category %q, got %#v", want, cats)
		}
	}
	for _, issue := range r.Issues {
		if issue.Category == CategoryOverlong {
			lower := strings.ToLower(issue.Suggestion)
			if !strings.Contains(lower, "preserv") && !strings.Contains(lower, "never truncate") {
				t.Errorf("overlong suggestion must preserve characters: %q", issue.Suggestion)
			}
		}
	}
	if !bytes.Equal(raw, snapshot) {
		t.Fatal("validation modified the input bytes")
	}
}

func TestTimecodeOrderAndRanges(t *testing.T) {
	g := baseGSI(1, 1, 1)
	block := makeBlock(0, 1, 0xFF, tc(255, 255, 255, 255), tc(9, 59, 59, 0), "Hello")
	r := Parse(makeSTL(g, block)).Validate()
	fields := fieldsFor(r, CategoryTimecode)
	got := strings.Join(fields, ",")
	if !strings.Contains(got, "TCI") || !strings.Contains(got, "TCO") {
		t.Fatalf("expected TCI and TCO findings, got %v", fields)
	}
	if !strings.Contains(r.Issues[0].Message, "255") {
		t.Fatalf("time-code formatting lost the invalid value: %s", r.Issues[0].Message)
	}
}

func TestStructuralDamageStillProducesAReport(t *testing.T) {
	r := Parse([]byte("short")).Validate()
	if r.Summary.Errors == 0 || categories(r)[CategoryStructure] == 0 {
		t.Fatalf("expected a structural finding, got %#v", r)
	}

	g := baseGSI(0, 0, 0)
	r = Parse(append(g, []byte{1, 2, 3}...)).Validate()
	if categories(r)[CategoryStructure] == 0 {
		t.Fatalf("expected a trailing-fragment finding, got %#v", r)
	}
}
