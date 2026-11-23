package j2k

import "sort"

type packetID struct {
	Layer      int
	Resolution int
	Component  int
	Precinct   int
	keyX       int64
	keyY       int64
}

func packetSequence(tile *Tile, cp *codingParams) []packetID {
	if tile == nil || cp == nil || cp.layers <= 0 {
		return nil
	}
	maxRes := 0
	for i := range tile.Components {
		if i < len(cp.components) && cp.components[i].Resolutions > maxRes {
			maxRes = cp.components[i].Resolutions
		}
	}
	if maxRes == 0 {
		return nil
	}
	byCompRes := make([][][]packetID, len(tile.Components))
	for ci := range tile.Components {
		byCompRes[ci] = make([][]packetID, maxRes)
		if ci >= len(cp.components) {
			continue
		}
		cc := &cp.components[ci]
		tc := &tile.Components[ci]
		for r := 0; r < cc.Resolutions && r < len(tc.Resolutions); r++ {
			res := &tc.Resolutions[r]
			count := res.PrecinctsX * res.PrecinctsY
			if count <= 0 {
				continue
			}
			pdx, pdy := 15, 15
			if r < len(cc.Precincts) {
				pdx, pdy = cc.Precincts[r].WidthExp, cc.Precincts[r].HeightExp
			}
			level := cc.Resolutions - 1 - r
			if level < 0 || level > 30 || pdx < 0 || pdy < 0 || pdx > 30 || pdy > 30 {
				continue
			}
			pxStart := floorDivPow2(res.X0, pdx) << pdx
			pyStart := floorDivPow2(res.Y0, pdy) << pdy
			for p := 0; p < count; p++ {
				px := pxStart + int64(p%res.PrecinctsX)*(int64(1)<<pdx)
				py := pyStart + int64(p/res.PrecinctsX)*(int64(1)<<pdy)
				for l := 0; l < cp.layers; l++ {
					byCompRes[ci][r] = append(byCompRes[ci][r], packetID{
						Layer: l, Resolution: r, Component: ci, Precinct: p,
						keyX: (px << level) * int64(maxInt(tc.XRsiz, 1)),
						keyY: (py << level) * int64(maxInt(tc.YRsiz, 1)),
					})
				}
			}
		}
	}

	var result []packetID
	switch cp.progression {
	case 0: // LRCP
		for l := 0; l < cp.layers; l++ {
			for r := 0; r < maxRes; r++ {
				for ci := range byCompRes {
					for _, packet := range byCompRes[ci][r] {
						if packet.Layer == l {
							result = append(result, packet)
						}
					}
				}
			}
		}
	case 1: // RLCP
		for r := 0; r < maxRes; r++ {
			for l := 0; l < cp.layers; l++ {
				for ci := range byCompRes {
					for _, packet := range byCompRes[ci][r] {
						if packet.Layer == l {
							result = append(result, packet)
						}
					}
				}
			}
		}
	default:
		for ci := range byCompRes {
			for r := range byCompRes[ci] {
				result = append(result, byCompRes[ci][r]...)
			}
		}
		sort.SliceStable(result, func(i, j int) bool {
			a, b := result[i], result[j]
			switch cp.progression {
			case 2: // RPCL
				if a.Resolution != b.Resolution {
					return a.Resolution < b.Resolution
				}
				if a.keyY != b.keyY {
					return a.keyY < b.keyY
				}
				if a.keyX != b.keyX {
					return a.keyX < b.keyX
				}
				if a.Component != b.Component {
					return a.Component < b.Component
				}
				if a.Precinct != b.Precinct {
					return a.Precinct < b.Precinct
				}
				return a.Layer < b.Layer
			case 3: // PCRL
				if a.keyY != b.keyY {
					return a.keyY < b.keyY
				}
				if a.keyX != b.keyX {
					return a.keyX < b.keyX
				}
				if a.Component != b.Component {
					return a.Component < b.Component
				}
				if a.Resolution != b.Resolution {
					return a.Resolution < b.Resolution
				}
				if a.Precinct != b.Precinct {
					return a.Precinct < b.Precinct
				}
				return a.Layer < b.Layer
			case 4: // CPRL
				if a.Component != b.Component {
					return a.Component < b.Component
				}
				if a.keyY != b.keyY {
					return a.keyY < b.keyY
				}
				if a.keyX != b.keyX {
					return a.keyX < b.keyX
				}
				if a.Resolution != b.Resolution {
					return a.Resolution < b.Resolution
				}
				if a.Precinct != b.Precinct {
					return a.Precinct < b.Precinct
				}
				return a.Layer < b.Layer
			default:
				return false
			}
		})
	}
	return result
}
