package j2k

type bandPrecinctGeometry struct {
	cw    int
	ch    int
	empty bool
}

func (p *parser) geometryForBand(tile *Tile, cp *codingParams, comp, res, precinct, band int) bandPrecinctGeometry {
	if comp < 0 || comp >= len(tile.Components) || comp >= len(cp.components) {
		return bandPrecinctGeometry{empty: true}
	}
	tc := &tile.Components[comp]
	cc := &cp.components[comp]
	if res < 0 || res >= len(tc.Resolutions) || res >= cc.Resolutions {
		return bandPrecinctGeometry{empty: true}
	}
	rr := &tc.Resolutions[res]
	if precinct < 0 || precinct >= rr.PrecinctsX*rr.PrecinctsY {
		return bandPrecinctGeometry{empty: true}
	}
	level := cc.Resolutions - 1 - res
	if level < 0 || level > 30 {
		return bandPrecinctGeometry{empty: true}
	}
	pdx, pdy := 15, 15
	if res < len(cc.Precincts) {
		pdx, pdy = cc.Precincts[res].WidthExp, cc.Precincts[res].HeightExp
	}
	if pdx < 0 || pdy < 0 || pdx > 30 || pdy > 30 {
		return bandPrecinctGeometry{empty: true}
	}

	var bx0, by0, bx1, by1 int64
	var cbgXExp, cbgYExp int
	var tlcbgX, tlcbgY int64
	if res == 0 {
		bx0, by0, bx1, by1 = rr.X0, rr.Y0, rr.X1, rr.Y1
		cbgXExp, cbgYExp = pdx, pdy
		tlcbgX = floorDivPow2(rr.X0, pdx) << pdx
		tlcbgY = floorDivPow2(rr.Y0, pdy) << pdy
	} else {
		bandNo := band + 1
		if bandNo < 1 || bandNo > 3 {
			return bandPrecinctGeometry{empty: true}
		}
		x0b := int64(bandNo & 1)
		y0b := int64(bandNo >> 1)
		den := int64(1) << (level + 1)
		bx0 = ceilDiv64(tc.X0-(x0b<<level), den)
		by0 = ceilDiv64(tc.Y0-(y0b<<level), den)
		bx1 = ceilDiv64(tc.X1-(x0b<<level), den)
		by1 = ceilDiv64(tc.Y1-(y0b<<level), den)
		if pdx == 0 || pdy == 0 {
			return bandPrecinctGeometry{empty: true}
		}
		cbgXExp, cbgYExp = pdx-1, pdy-1
		tlcbgX = ceilDivPow2(floorDivPow2(rr.X0, pdx)<<pdx, 1)
		tlcbgY = ceilDivPow2(floorDivPow2(rr.Y0, pdy)<<pdy, 1)
	}
	if bx0 >= bx1 || by0 >= by1 {
		return bandPrecinctGeometry{empty: true}
	}
	cbgW := int64(1) << cbgXExp
	cbgH := int64(1) << cbgYExp
	px0 := maxInt64(tlcbgX+int64(precinct%rr.PrecinctsX)*cbgW, bx0)
	px1 := minInt64(tlcbgX+int64(precinct%rr.PrecinctsX+1)*cbgW, bx1)
	py0 := maxInt64(tlcbgY+int64(precinct/rr.PrecinctsX)*cbgH, by0)
	py1 := minInt64(tlcbgY+int64(precinct/rr.PrecinctsX+1)*cbgH, by1)
	if px0 >= px1 || py0 >= py1 {
		return bandPrecinctGeometry{empty: true}
	}
	cbXExp := minInt(cc.CodeBlockWidthExp, cbgXExp)
	cbYExp := minInt(cc.CodeBlockHeightExp, cbgYExp)
	if cbXExp < 0 || cbYExp < 0 || cbXExp > 30 || cbYExp > 30 {
		return bandPrecinctGeometry{empty: true}
	}
	cw := int((ceilDivPow2(px1, cbXExp) - floorDivPow2(px0, cbXExp)))
	ch := int((ceilDivPow2(py1, cbYExp) - floorDivPow2(py0, cbYExp)))
	if cw <= 0 || ch <= 0 {
		return bandPrecinctGeometry{empty: true}
	}
	return bandPrecinctGeometry{cw: cw, ch: ch}
}

func numBands(res int) int {
	if res == 0 {
		return 1
	}
	return 3
}
