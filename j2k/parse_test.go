package j2k

import (
	"encoding/binary"
	"testing"
)

func marker(code uint16) []byte {
	return []byte{byte(code >> 8), byte(code)}
}

func segment(code uint16, payload []byte) []byte {
	out := marker(code)
	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(payload)+2))
	out = append(out, length[:]...)
	return append(out, payload...)
}

func testSIZ(xsiz, ysiz, xtsiz, ytsiz uint32, comps int) []byte {
	payload := make([]byte, 36+3*comps)
	binary.BigEndian.PutUint16(payload[0:], 0)
	binary.BigEndian.PutUint32(payload[2:], xsiz)
	binary.BigEndian.PutUint32(payload[6:], ysiz)
	binary.BigEndian.PutUint32(payload[14:], 0)
	binary.BigEndian.PutUint32(payload[18:], xtsiz)
	binary.BigEndian.PutUint32(payload[22:], ytsiz)
	binary.BigEndian.PutUint32(payload[30:], 0)
	binary.BigEndian.PutUint16(payload[34:], uint16(comps))
	for i := 0; i < comps; i++ {
		payload[36+3*i] = 7
		payload[37+3*i] = 1
		payload[38+3*i] = 1
	}
	return segment(markerSIZ, payload)
}

func testCOD(progression, layers, decompositions int, precincts []byte) []byte {
	scod := byte(0)
	if precincts != nil {
		scod = 1
	}
	payload := []byte{
		scod, byte(progression),
		byte(layers >> 8), byte(layers),
		0, byte(decompositions), 4, 4, 0, 1,
	}
	payload = append(payload, precincts...)
	return segment(markerCOD, payload)
}

func testTilePart(tile, tpsot, tnsot int, data []byte) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint16(payload[0:], uint16(tile))
	binary.BigEndian.PutUint32(payload[2:], uint32(14+len(data)))
	payload[6] = byte(tpsot)
	payload[7] = byte(tnsot)
	out := segment(markerSOT, payload)
	out = append(out, marker(markerSOD)...)
	return append(out, data...)
}

func testStream(parts ...[]byte) []byte {
	out := append(marker(markerSOC), testSIZ(1, 1, 1, 1, 1)...)
	out = append(out, testCOD(0, 1, 0, nil)...)
	for _, part := range parts {
		out = append(out, part...)
	}
	return append(out, marker(markerEOC)...)
}

func hasIssue(c *Codestream, code string) bool {
	for _, issue := range c.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestParseMinimalAndEmptyPacket(t *testing.T) {
	c, err := Parse(testStream(testTilePart(0, 0, 1, []byte{0x00})))
	if err != nil {
		t.Fatal(err)
	}
	if c.SOC != (ByteRange{0, 2}) || c.EOC == nil || c.SIZ == nil || c.COD == nil {
		t.Fatalf("missing required markers: %+v", c)
	}
	if len(c.Tiles) != 1 || len(c.Tiles[0].TileParts) != 1 {
		t.Fatalf("tiles=%d parts=%d", len(c.Tiles), len(c.Tiles[0].TileParts))
	}
	tile := c.Tiles[0]
	if len(tile.Packets) != 1 {
		t.Fatalf("packets=%d issues=%+v", len(tile.Packets), c.Issues)
	}
	packet := tile.Packets[0]
	if packet.Layer != 0 || packet.Resolution != 0 || packet.Component != 0 || packet.Precinct != 0 {
		t.Fatalf("unexpected packet index: %+v", packet)
	}
	if packet.Header.Len() != 1 || packet.Data.Len() != 0 || !packet.Empty {
		t.Fatalf("unexpected packet ranges: %+v", packet)
	}
	if len(c.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", c.Issues)
	}
}

func TestLayerAndResolutionIndices(t *testing.T) {
	stream := append(marker(markerSOC), testSIZ(1, 1, 1, 1, 1)...)
	stream = append(stream, testCOD(0, 2, 1, nil)...)
	stream = append(stream, testTilePart(0, 0, 1, []byte{0x00, 0x00, 0x00, 0x00})...)
	stream = append(stream, marker(markerEOC)...)
	c := Analyze(stream)
	packets := c.Tiles[0].Packets
	if len(packets) != 4 {
		t.Fatalf("packets=%d issues=%+v", len(packets), c.Issues)
	}
	want := [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}}
	for i, pair := range want {
		if packets[i].Layer != pair[0] || packets[i].Resolution != pair[1] {
			t.Fatalf("packet %d is %+v, want layer=%d res=%d",
				i, packets[i], pair[0], pair[1])
		}
	}
}

func TestNonEmptyPacketBodyLength(t *testing.T) {
	c := Analyze(testStream(testTilePart(0, 0, 1, []byte{0xe2, 'A', 'B'})))
	if len(c.Tiles) != 1 || len(c.Tiles[0].Packets) != 1 {
		t.Fatalf("packets=%+v issues=%+v", c.Tiles, c.Issues)
	}
	packet := c.Tiles[0].Packets[0]
	if packet.Empty || packet.CodeBlockBytes != 2 || packet.Data.Len() != 2 {
		t.Fatalf("unexpected packet: %+v", packet)
	}
	if len(c.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", c.Issues)
	}
}

func TestBitReaderConsumesNineBits(t *testing.T) {
	r := newBitReader([]byte{0xe2, 0x00}, 0, 2)
	for i := 0; i < 9; i++ {
		if _, err := r.readBit(); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.consumed(); got != 2 {
		t.Fatalf("consumed=%d", got)
	}
}

func TestDuplicateTilePartRange(t *testing.T) {
	part := testTilePart(0, 0, 1, []byte{0x00})
	stream := append(marker(markerSOC), testSIZ(1, 1, 1, 1, 1)...)
	stream = append(stream, testCOD(0, 1, 0, nil)...)
	stream = append(stream, part...)
	secondStart := len(stream)
	stream = append(stream, part...)
	stream = append(stream, marker(markerEOC)...)
	c := Analyze(stream)
	var duplicate *Issue
	for i := range c.Issues {
		if c.Issues[i].Code == "duplicate_tile_part" {
			duplicate = &c.Issues[i]
		}
	}
	if duplicate == nil {
		t.Fatalf("no duplicate issue: %+v", c.Issues)
	}
	if duplicate.Range.Start != secondStart || len(duplicate.Related) != 1 {
		t.Fatalf("bad duplicate ranges: %+v (second start %d)", duplicate, secondStart)
	}
}

func TestPsotOverflowPointsAtLengthField(t *testing.T) {
	part := testTilePart(0, 0, 1, []byte{0x00})
	binary.BigEndian.PutUint32(part[6:], 9999)
	stream := append(marker(markerSOC), testSIZ(1, 1, 1, 1, 1)...)
	stream = append(stream, testCOD(0, 1, 0, nil)...)
	partStart := len(stream)
	stream = append(stream, part...)
	stream = append(stream, marker(markerEOC)...)
	c := Analyze(stream)
	var overflow *Issue
	for i := range c.Issues {
		if c.Issues[i].Code == "length_overflow" {
			overflow = &c.Issues[i]
			break
		}
	}
	if overflow == nil {
		t.Fatalf("no length overflow: %+v", c.Issues)
	}
	if overflow.Range.Start != partStart+6 || overflow.Range.End != partStart+10 {
		t.Fatalf("bad overflow range: %+v", overflow.Range)
	}
}

func TestTruncatedSIZAndMissingEOC(t *testing.T) {
	stream := append(marker(markerSOC), marker(markerSIZ)...)
	stream = append(stream, 0x00, 0x64, 0x00)
	c := Analyze(stream)
	if !hasIssue(c, "length_overflow") {
		t.Fatalf("no SIZ length overflow: %+v", c.Issues)
	}
	if !hasIssue(c, "missing_eoc") {
		t.Fatalf("no missing EOC: %+v", c.Issues)
	}
}

func TestTileGridIndex(t *testing.T) {
	stream := append(marker(markerSOC), testSIZ(2, 1, 1, 1, 1)...)
	stream = append(stream, testCOD(0, 1, 0, nil)...)
	stream = append(stream, testTilePart(1, 0, 1, []byte{0x00})...)
	stream = append(stream, marker(markerEOC)...)
	c := Analyze(stream)
	if len(c.Tiles) != 1 || c.Tiles[0].Index != 1 || c.Tiles[0].GridX != 1 || c.Tiles[0].GridY != 0 {
		t.Fatalf("unexpected tiles: %+v", c.Tiles)
	}
	if c.Tiles[0].X0 != 1 || c.Tiles[0].X1 != 2 {
		t.Fatalf("unexpected tile geometry: %+v", c.Tiles[0])
	}
}

func TestPacketsContinueAcrossTileParts(t *testing.T) {
	stream := append(marker(markerSOC), testSIZ(1, 1, 1, 1, 1)...)
	stream = append(stream, testCOD(0, 2, 0, nil)...)
	stream = append(stream, testTilePart(0, 0, 2, []byte{0x00})...)
	stream = append(stream, testTilePart(0, 1, 2, []byte{0x00})...)
	stream = append(stream, marker(markerEOC)...)
	c := Analyze(stream)
	if len(c.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", c.Issues)
	}
	if len(c.Tiles[0].Packets) != 2 {
		t.Fatalf("packets=%+v", c.Tiles[0].Packets)
	}
	if c.Tiles[0].Packets[0].Layer != 0 || c.Tiles[0].Packets[1].Layer != 1 {
		t.Fatalf("packet layers: %+v", c.Tiles[0].Packets)
	}
}

func TestOutOfOrderTilePartsAreReported(t *testing.T) {
	stream := append(marker(markerSOC), testSIZ(1, 1, 1, 1, 1)...)
	stream = append(stream, testCOD(0, 2, 0, nil)...)
	stream = append(stream, testTilePart(0, 1, 2, []byte{0x00})...)
	stream = append(stream, testTilePart(0, 0, 2, []byte{0x00})...)
	stream = append(stream, marker(markerEOC)...)
	c := Analyze(stream)
	if !hasIssue(c, "tile_part_out_of_order") {
		t.Fatalf("no ordering issue: %+v", c.Issues)
	}
	if c.Tiles[0].TileParts[0].TPsot != 0 || c.Tiles[0].TileParts[1].TPsot != 1 {
		t.Fatalf("parts were not sorted: %+v", c.Tiles[0].TileParts)
	}
}

func TestTruncatedTileHeaderMarker(t *testing.T) {
	stream := append(marker(markerSOC), testSIZ(1, 1, 1, 1, 1)...)
	stream = append(stream, testCOD(0, 1, 0, nil)...)
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[2:], 13)
	stream = append(stream, segment(markerSOT, payload)...)
	stream = append(stream, 0x00)
	stream = append(stream, marker(markerEOC)...)
	c := Analyze(stream)
	if !hasIssue(c, "truncated_marker") {
		t.Fatalf("no truncated marker issue: %+v", c.Issues)
	}
}

func TestTruncatedPacketHeaderRange(t *testing.T) {
	c := Analyze(testStream(testTilePart(0, 0, 1, []byte{0xc0})))
	var issue *Issue
	for i := range c.Issues {
		if c.Issues[i].Code == "truncated_packet_header" {
			issue = &c.Issues[i]
		}
	}
	if issue == nil {
		t.Fatalf("no truncated packet header issue: %+v", c.Issues)
	}
	if issue.Range.Len() == 0 {
		t.Fatalf("empty issue range: %+v", issue)
	}
}

func TestPacketBodyLengthOverflow(t *testing.T) {
	c := Analyze(testStream(testTilePart(0, 0, 1, []byte{0xe2})))
	var issue *Issue
	for i := range c.Issues {
		if c.Issues[i].Code == "length_overflow" {
			issue = &c.Issues[i]
			break
		}
	}
	if issue == nil || len(issue.Related) == 0 {
		t.Fatalf("no packet length overflow with related range: %+v", c.Issues)
	}
	if issue.Range.Start >= issue.Range.End {
		t.Fatalf("bad issue range: %+v", issue)
	}
}

func TestTileHeaderCODOverride(t *testing.T) {
	stream := append(marker(markerSOC), testSIZ(1, 1, 1, 1, 1)...)
	header := testCOD(0, 1, 0, nil)
	data := []byte{0x00}
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[2:], uint32(12+len(header)+2+len(data)))
	payload[7] = 1
	tilePart := segment(markerSOT, payload)
	tilePart = append(tilePart, header...)
	tilePart = append(tilePart, marker(markerSOD)...)
	tilePart = append(tilePart, data...)
	stream = append(stream, tilePart...)
	stream = append(stream, marker(markerEOC)...)
	c := Analyze(stream)
	if len(c.Tiles) != 1 || c.Tiles[0].Layers != 1 || len(c.Tiles[0].Packets) != 1 {
		t.Fatalf("tile COD was not applied: %+v issues=%+v", c.Tiles, c.Issues)
	}
}
