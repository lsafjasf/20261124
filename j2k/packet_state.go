package j2k

type segmentState struct {
	passes    int
	maxPasses int
}

type codeBlockState struct {
	segments   []segmentState
	numLenBits int
}

type bandPrecState struct {
	incl   *tagTree
	imsb   *tagTree
	blocks []codeBlockState
	cw     int
	ch     int
}

type bandKey struct {
	component  int
	resolution int
	precinct   int
	band       int
}

type packetScanState struct {
	tile   *Tile
	coding *codingParams
	bands  map[bandKey]*bandPrecState
}

func newPacketScanState(tile *Tile, cp *codingParams) *packetScanState {
	return &packetScanState{tile: tile, coding: cp, bands: make(map[bandKey]*bandPrecState)}
}

func (p *parser) getBandPrecinctState(state *packetScanState, id packetID, band int) (*bandPrecState, bool) {
	g := p.geometryForBand(state.tile, state.coding, id.Component, id.Resolution, id.Precinct, band)
	if g.empty {
		return nil, true
	}
	key := bandKey{id.Component, id.Resolution, id.Precinct, band}
	if cached := state.bands[key]; cached != nil {
		if id.Layer == 0 {
			cached.reset()
		}
		return cached, true
	}
	if g.cw <= 0 || g.ch <= 0 {
		return nil, true
	}
	if g.cw > 1<<20 || g.ch > 1<<20 || g.cw > (1<<24)/g.ch {
		p.out.addIssue("resource_limit", SeverityError,
			ByteRange{state.tile.TileParts[0].SOT.Range.Start, state.tile.TileParts[0].SOT.Range.End},
			"precinct has too many code-blocks (%dx%d)", g.cw, g.ch)
		return nil, false
	}
	blocks := g.cw * g.ch
	bs := &bandPrecState{
		incl:   newTagTree(g.cw, g.ch),
		imsb:   newTagTree(g.cw, g.ch),
		blocks: make([]codeBlockState, blocks),
		cw:     g.cw,
		ch:     g.ch,
	}
	state.bands[key] = bs
	return bs, true
}

func (s *bandPrecState) reset() {
	if s.incl != nil {
		s.incl.reset()
	}
	if s.imsb != nil {
		s.imsb.reset()
	}
	for i := range s.blocks {
		s.blocks[i] = codeBlockState{}
	}
}

func segmentLimit(style uint8, first bool, previous *segmentState) int {
	if style&0x04 != 0 {
		return 1
	}
	if style&0x01 != 0 {
		if first {
			return 10
		}
		if previous != nil && (previous.maxPasses == 1 || previous.maxPasses == 10) {
			return 2
		}
		return 1
	}
	return 109
}

func appendSegment(block *codeBlockState, style uint8) {
	first := len(block.segments) == 0
	var previous *segmentState
	if !first {
		previous = &block.segments[len(block.segments)-1]
	}
	block.segments = append(block.segments, segmentState{
		maxPasses: segmentLimit(style, first, previous),
	})
}
