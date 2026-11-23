package j2k

import (
	"fmt"
	"sort"
)

func (p *parser) buildTiles() {
	grouped := make(map[int][]TilePart)
	for _, part := range p.parts {
		grouped[part.Isot] = append(grouped[part.Isot], part)
	}

	indices := make([]int, 0, len(grouped))
	for index := range grouped {
		indices = append(indices, index)
	}
	sort.Ints(indices)

	for _, index := range indices {
		parts := grouped[index]
		p.checkTilePartSet(index, parts)
		sortTileParts(parts)
		tile := Tile{
			Index: index, GridX: -1, GridY: -1,
			Coding: make([]ComponentCoding, 0), Components: make([]TileComponent, 0),
			TileParts: make([]TilePart, 0), Packets: make([]Packet, 0),
		}
		if p.out.SIZ != nil && p.out.SIZ.TilesX > 0 && index >= 0 &&
			index < p.out.SIZ.TilesX*p.out.SIZ.TilesY {
			tile.GridX = index % p.out.SIZ.TilesX
			tile.GridY = index / p.out.SIZ.TilesX
			p.fillTileGeometry(&tile, p.out.SIZ)
		}
		cp := p.tileCoding[index]
		if cp == nil {
			cp = p.mainCoding
		}
		if cp == nil {
			p.out.addIssue("missing_cod", SeverityError, tileRange(parts),
				"tile %d has no effective COD parameters", index)
		} else {
			tile.Layers = cp.layers
			tile.Coding = append([]ComponentCoding(nil), cp.components...)
			for _, comp := range cp.components {
				if comp.Resolutions > tile.Resolutions {
					tile.Resolutions = comp.Resolutions
				}
			}
		}
		tile.TileParts = parts
		validOrdering := cp != nil && cp.progression >= 0 && cp.progression <= 4 && cp.layers > 0
		if validOrdering && p.out.SIZ != nil && !p.skipPacketIndex {
			p.indexPackets(&tile, cp)
		} else if cp != nil && p.out.SIZ != nil {
			p.out.addIssue("packet_indexes_skipped", SeverityWarning, tileRange(parts),
				"tile %d packet indexes were not built because an unsupported ordering/header feature is present",
				index)
		}
		p.out.Tiles = append(p.out.Tiles, tile)
	}
}

func tileRange(parts []TilePart) ByteRange {
	if len(parts) == 0 {
		return ByteRange{}
	}
	start, end := parts[0].SOT.Range.Start, parts[0].SOT.Range.End
	for _, part := range parts[1:] {
		if part.SOT.Range.Start < start {
			start = part.SOT.Range.Start
		}
		if part.SOT.Range.End > end {
			end = part.SOT.Range.End
		}
	}
	return ByteRange{start, end}
}

func (p *parser) checkTilePartSet(tile int, parts []TilePart) {
	firstByTP := make(map[int]int)
	lastTP := -1
	var last *TilePart
	for i := range parts {
		part := &parts[i]
		if first, exists := firstByTP[part.TPsot]; exists {
			original := ByteRange{parts[first].SOT.Range.Start, parts[first].SOT.Range.End}
			part.DuplicateOf = &original
			issue := Issue{
				Code:     "duplicate_tile_part",
				Severity: SeverityError,
				Message:  fmt.Sprintf("tile %d has duplicate TPsot %d", tile, part.TPsot),
				Range:    part.SOT.Range,
				Related:  []ByteRange{original},
			}
			p.out.Issues = append(p.out.Issues, issue)
		} else {
			firstByTP[part.TPsot] = i
		}
		if lastTP >= 0 && part.TPsot < lastTP {
			issue := Issue{
				Code:     "tile_part_out_of_order",
				Severity: SeverityWarning,
				Message: fmt.Sprintf("tile %d tile-part TPsot %d follows %d",
					tile, part.TPsot, lastTP),
				Range: part.SOT.Range,
			}
			if last != nil {
				issue.Related = []ByteRange{last.SOT.Range}
			}
			p.out.Issues = append(p.out.Issues, issue)
		}
		lastTP = part.TPsot
		last = part
	}

	unique := make([]TilePart, 0, len(firstByTP))
	for _, i := range firstByTP {
		unique = append(unique, parts[i])
	}
	sortTileParts(unique)
	for expected, part := range unique {
		if part.TPsot != expected {
			p.out.addIssue("missing_tile_part", SeverityError, part.SOT.Range,
				"tile %d has TPsot %d where %d was expected", tile, part.TPsot, expected)
		}
	}
	if len(unique) > 0 && unique[0].TNsot != 0 && len(unique) != unique[0].TNsot {
		p.out.addIssue("tile_part_count_mismatch", SeverityError, unique[0].SOT.Range,
			"tile %d declares %d tile-parts but %d unique parts were found",
			tile, unique[0].TNsot, len(unique))
	}
	for _, part := range unique[1:] {
		if part.TNsot != 0 && unique[0].TNsot != 0 && part.TNsot != unique[0].TNsot {
			p.out.addIssue("tile_part_count_mismatch", SeverityWarning, part.SOT.Range,
				"tile %d tile-parts disagree on TNsot", tile)
		}
	}
}

func (p *parser) fillTileGeometry(tile *Tile, siz *SIZ) {
	tilesX := int64(siz.TilesX)
	if tilesX <= 0 {
		return
	}
	gx := int64(tile.GridX)
	gy := int64(tile.GridY)
	gridX0 := int64(siz.XTOsiz) + gx*int64(siz.XTsiz)
	gridY0 := int64(siz.YTOsiz) + gy*int64(siz.YTsiz)
	tile.X0 = maxInt64(gridX0, int64(siz.XOsiz))
	tile.Y0 = maxInt64(gridY0, int64(siz.YOsiz))
	tile.X1 = minInt64(gridX0+int64(siz.XTsiz), int64(siz.Xsiz))
	tile.Y1 = minInt64(gridY0+int64(siz.YTsiz), int64(siz.Ysiz))
	if tile.X1 < tile.X0 {
		tile.X1 = tile.X0
	}
	if tile.Y1 < tile.Y0 {
		tile.Y1 = tile.Y0
	}

	cp := p.tileCoding[tile.Index]
	if cp == nil {
		cp = p.mainCoding
	}
	for i, comp := range siz.Components {
		xr, yr := int64(comp.XRsiz), int64(comp.YRsiz)
		if xr <= 0 || yr <= 0 {
			continue
		}
		tc := TileComponent{
			Index: i,
			XRsiz: int(comp.XRsiz),
			YRsiz: int(comp.YRsiz),
			X0:    ceilDiv64(tile.X0, xr),
			Y0:    ceilDiv64(tile.Y0, yr),
			X1:    ceilDiv64(tile.X1, xr),
			Y1:    ceilDiv64(tile.Y1, yr),
		}
		levels := 1
		precincts := []PrecinctSize{{15, 15}}
		if cp != nil && i < len(cp.components) {
			levels = cp.components[i].Resolutions
			precincts = cp.components[i].Precincts
		}
		if levels < 1 {
			levels = 1
		}
		for r := 0; r < levels; r++ {
			level := levels - 1 - r
			if level < 0 || level > 30 {
				break
			}
			scale := int64(1) << level
			res := Resolution{
				Level: r,
				X0:    ceilDiv64(tc.X0, scale),
				Y0:    ceilDiv64(tc.Y0, scale),
				X1:    ceilDiv64(tc.X1, scale),
				Y1:    ceilDiv64(tc.Y1, scale),
			}
			pdx, pdy := 15, 15
			if r < len(precincts) {
				pdx, pdy = precincts[r].WidthExp, precincts[r].HeightExp
			}
			if pdx >= 0 && pdx < 31 && pdy >= 0 && pdy < 31 && res.X0 != res.X1 && res.Y0 != res.Y1 {
				px0 := floorDivPow2(res.X0, pdx) << pdx
				px1 := ceilDivPow2(res.X1, pdx) << pdx
				py0 := floorDivPow2(res.Y0, pdy) << pdy
				py1 := ceilDivPow2(res.Y1, pdy) << pdy
				res.PrecinctsX = int((px1 - px0) >> pdx)
				res.PrecinctsY = int((py1 - py0) >> pdy)
			}
			tc.Resolutions = append(tc.Resolutions, res)
		}
		tile.Components = append(tile.Components, tc)
	}
}

func ceilDiv64(a, b int64) int64 {
	if b <= 0 {
		return 0
	}
	if a >= 0 {
		return (a + b - 1) / b
	}
	return a / b
}

func floorDivPow2(v int64, exp int) int64 {
	if exp == 0 {
		return v
	}
	return v >> exp
}

func ceilDivPow2(v int64, exp int) int64 {
	if exp == 0 {
		return v
	}
	return (v + (int64(1) << exp) - 1) >> exp
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
