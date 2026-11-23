package id3

import (
	"bytes"
	"testing"
)

func putSync(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

func frameV4(id string, payload []byte) []byte {
	h := make([]byte, 10)
	copy(h[0:4], id)
	putSyncsafe(uint32(len(payload)), h[4:8])
	return append(h, payload...)
}

func tit2(title string) []byte {
	p := append([]byte{3}, title...)
	return append(p, 0)
}

func chap(id string, start, end uint32, title string) []byte {
	var p []byte
	p = append(p, id...)
	p = append(p, 0)
	p = appendU32(p, start)
	p = appendU32(p, end)
	p = appendU32(p, 0xffffffff)
	p = appendU32(p, 0xffffffff)
	p = append(p, frameV4("TIT2", tit2(title))...)
	return frameV4("CHAP", p)
}

func ctoc(id string, children, flags byte, ids ...string) []byte {
	var p []byte
	p = append(p, id...)
	p = append(p, 0, flags, children)
	for _, c := range ids {
		p = append(p, c...)
		p = append(p, 0)
	}
	return frameV4("CTOC", p)
}

func buildTagV4(frames ...[]byte) []byte {
	var body []byte
	for _, f := range frames {
		body = append(body, f...)
	}
	header := make([]byte, 10)
	copy(header[0:3], "ID3")
	header[3], header[4] = 4, 0
	putSyncsafe(uint32(len(body)), header[6:10])
	out := append(header, body...)
	out = append(out, []byte("AUDIO-BYTES-MUST-STAY")...)
	return out
}

func TestParseChaptersOutOfOrder(t *testing.T) {
	// Frames intentionally shuffled: TIT2 top-level, CTOC before CHAP.
	buf := buildTagV4(
		frameV4("TIT2", tit2("Album title")),
		frameV4("XCOA", []byte{9, 8, 7}), // private/unknown frame
		ctoc("toc", 2, 0x03, "c1", "c2"),
		chap("c1", 0, 500, "Intro"),
		chap("c2", 500, 1000, "Intro"), // duplicate title
	)
	tag, err := Parse(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tag.Version.Major != 4 {
		t.Fatalf("version = %d", tag.Version.Major)
	}
	if string(tag.Audio) != "AUDIO-BYTES-MUST-STAY" {
		t.Fatalf("audio not retained: %q", tag.Audio)
	}
	byID := map[string]*Frame{}
	for _, f := range tag.Frames {
		if f.ID == "CHAP" {
			byID[f.ElementID] = f
		}
	}
	if byID["c1"].Start != 0 || byID["c1"].End != 500 || byID["c1"].Title != "Intro" {
		t.Fatalf("c1 decode wrong: %+v", byID["c1"])
	}
	for _, f := range tag.Frames {
		if f.ID == "CTOC" {
			if len(f.ChildIDs) != 2 || f.ChildIDs[0] != "c1" || f.ChildIDs[1] != "c2" {
				t.Fatalf("ctoc children: %v", f.ChildIDs)
			}
		}
	}
}

func TestWriteBlockPreservesUnknownAndAudio(t *testing.T) {
	unknown := frameV4("TXXX", []byte{3, 'k', 0, 'v', 0})
	buf := buildTagV4(
		ctoc("toc", 1, 0x03, "c1"),
		chap("c1", 10, 20, "Ch"),
		unknown,
	)
	tag, err := Parse(buf)
	if err != nil {
		t.Fatal(err)
	}
	block := WriteBlock(tag, WriteOptions{})
	if !bytes.HasPrefix(block, []byte("ID3\x04\x00")) {
		t.Fatalf("bad header: %x", block[:5])
	}
	// The unknown TXXX wire bytes must appear verbatim in the new block.
	if !bytes.Contains(block, unknown) {
		t.Fatalf("unknown frame not preserved byte-for-byte")
	}
	// Reparse the new block and confirm structure survives.
	tag2, err := Parse(append(block, tag.Audio...))
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if string(tag2.Audio) != "AUDIO-BYTES-MUST-STAY" {
		t.Fatalf("audio changed")
	}
	ids := map[string]bool{}
	for _, f := range tag2.Frames {
		ids[f.ID] = true
	}
	if !ids["CHAP"] || !ids["CTOC"] || !ids["TXXX"] {
		t.Fatalf("frames lost after round trip: %v", ids)
	}
}

func TestParseV23(t *testing.T) {
	chapPayload := []byte("c1\x00")
	chapPayload = appendU32(chapPayload, 100)
	chapPayload = appendU32(chapPayload, 200)
	chapPayload = appendU32(chapPayload, 0xffffffff)
	chapPayload = appendU32(chapPayload, 0xffffffff)
	frameHeader := make([]byte, 10)
	copy(frameHeader[0:4], "CHAP")
	putSync(frameHeader[4:8], uint32(len(chapPayload)))
	body := append(frameHeader, chapPayload...)

	tagHeader := make([]byte, 10)
	copy(tagHeader[0:3], "ID3")
	tagHeader[3], tagHeader[4] = 3, 0
	putSyncsafe(uint32(len(body)), tagHeader[6:10])
	buf := append(append(tagHeader, body...), []byte("PCM?")...)

	tag, err := Parse(buf)
	if err != nil {
		t.Fatal(err)
	}
	f := tag.Frames[0]
	if f.ElementID != "c1" || f.Start != 100 || f.End != 200 {
		t.Fatalf("v2.3 chap decode: %+v", f)
	}
}

func TestTLEN(t *testing.T) {
	if got := TLENMillis("361234"); got != 361234 {
		t.Fatalf("plain tlen: %d", got)
	}
	if got := TLENMillis("01:02:03.500"); got != 3723500 {
		t.Fatalf("hms tlen: %d", got)
	}
}

func TestUnsyncRoundTrip(t *testing.T) {
	data := []byte{1, 0xff, 0xff, 0x00, 0xff, 2}
	enc := Sync(data)
	dec := Unsync(enc)
	if !bytes.Equal(dec, data) {
		t.Fatalf("unsync round trip: %x -> %x", enc, dec)
	}
}

func TestV23TagUnsyncRoundTrip(t *testing.T) {
	// CHAP payload deliberately contains 0xFF so tag-level unsync must escape it.
	payload := []byte("c1\x00")
	payload = appendU32(payload, 5)
	payload = appendU32(payload, 10)
	payload = appendU32(payload, 0xffffffff)
	payload = appendU32(payload, 0xffffffff)
	frameHeader := make([]byte, 10)
	copy(frameHeader[0:4], "CHAP")
	putSync(frameHeader[4:8], uint32(len(payload)))
	rawFrame := append(frameHeader, payload...)
	body := Sync(rawFrame)

	tagHeader := make([]byte, 10)
	copy(tagHeader[0:3], "ID3")
	tagHeader[3], tagHeader[4] = 3, 0
	tagHeader[5] = 0x80 // tag-level unsynchronisation
	putSyncsafe(uint32(len(body)), tagHeader[6:10])
	buf := append(append(tagHeader, body...), []byte("AUDIO")...)

	tag, err := Parse(buf)
	if err != nil {
		t.Fatal(err)
	}
	f := tag.Frames[0]
	if f.StOff != 0xffffffff || f.ElementID != "c1" {
		t.Fatalf("unsynced v2.3 decode: %+v", f)
	}
	block := WriteBlock(tag, WriteOptions{})
	tag2, err := Parse(append(block, tag.Audio...))
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if tag2.Frames[0].StOff != 0xffffffff {
		t.Fatalf("round trip lost 0xFFFFFFFF offsets")
	}
}
