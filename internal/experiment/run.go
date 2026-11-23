package experiment

import (
	"fmt"
	"math"

	"chromaexp/internal/chroma"
)

// Config defines the deterministic experiment.
type Config struct {
	Width  int
	Height int
	BlockW int
	BlockH int
}

// DefaultConfig uses odd dimensions so the last partial block is exercised.
func DefaultConfig() Config {
	return Config{Width: 19, Height: 11, BlockW: 8, BlockH: 8}
}

// RGBDelta is a per-pixel RGB difference.
type RGBDelta struct {
	R float64 `json:"dR"`
	G float64 `json:"dG"`
	B float64 `json:"dB"`
}

// Row is one per-pixel comparison between wrong and corrected reconstruction.
type Row struct {
	X int `json:"x"`
	Y int `json:"y"`

	RefCb   float64 `json:"refCb"`
	RefCr   float64 `json:"refCr"`
	WrongCb float64 `json:"wrongCb"`
	WrongCr float64 `json:"wrongCr"`
	FixedCb float64 `json:"fixedCb"`
	FixedCr float64 `json:"fixedCr"`

	RefRGB   chroma.RGB `json:"refRGB"`
	WrongRGB chroma.RGB `json:"wrongRGB"`
	FixedRGB chroma.RGB `json:"fixedRGB"`

	DeltaCb float64  `json:"dCbWrongMinusFixed"`
	DeltaCr float64  `json:"dCrWrongMinusFixed"`
	Delta   RGBDelta `json:"dRGBWrongMinusFixed"`
	AbsRGB  float64  `json:"absRGBWrongMinusFixed"`

	WrongRefAbsRGB float64 `json:"wrongVsRefAbsRGB"`
	FixedRefAbsRGB float64 `json:"fixedVsRefAbsRGB"`
}

// AxisRow records one axis coordinate and its weights.
type AxisRow struct {
	Position int                `json:"position"`
	Wrong    chroma.AxisMapping `json:"wrong"`
	Fixed    chroma.AxisMapping `json:"fixed"`
	Delta    float64            `json:"coordinateDelta"`
}

// BlockRow records whole-versus-tiled and defect-versus-fixed differences.
type BlockRow struct {
	Index int `json:"index"`
	X0    int `json:"x0"`
	Y0    int `json:"y0"`
	X1    int `json:"x1"`
	Y1    int `json:"y1"`

	WrongWholeVsTiled float64 `json:"wrongWholeVsTiledMax"`
	FixedWholeVsTiled float64 `json:"fixedWholeVsTiledMax"`
	BlockLocalVsFixed float64 `json:"blockLocalVsFixedMax"`
	WrongVsFixed      float64 `json:"wrongVsFixedMax"`
}

// Summary contains scalar acceptance results.
type Summary struct {
	MaxAbsDeltaCb       float64 `json:"maxAbsDeltaCb"`
	MaxAbsDeltaCr       float64 `json:"maxAbsDeltaCr"`
	MaxAbsRGB           float64 `json:"maxAbsRGB"`
	MeanAbsRGB          float64 `json:"meanAbsRGB"`
	WrongMeanRefAbsRGB  float64 `json:"wrongMeanRefAbsRGB"`
	FixedMeanRefAbsRGB  float64 `json:"fixedMeanRefAbsRGB"`
	WrongMaxRefAbsRGB   float64 `json:"wrongMaxRefAbsRGB"`
	FixedMaxRefAbsRGB   float64 `json:"fixedMaxRefAbsRGB"`
	WrongTiledMismatch  float64 `json:"wrongTiledMismatch"`
	FixedTiledMismatch  float64 `json:"fixedTiledMismatch"`
	BlockLocalMismatch  float64 `json:"blockLocalMismatch"`
	MaxLumaDelta        float64 `json:"maxLumaDelta"`
	OddWidthSampleShift float64 `json:"oddWidthSampleShift"`
}

// Result is the complete deterministic experiment output.
type Result struct {
	Config  Config
	Fixture *chroma.Fixture

	WrongYUV   *chroma.YUVImage `json:"-"`
	FixedYUV   *chroma.YUVImage `json:"-"`
	WrongRGB   *chroma.RGBImage `json:"-"`
	FixedRGB   *chroma.RGBImage `json:"-"`
	Reference  *chroma.RGBImage `json:"-"`
	WrongCb    *chroma.Plane    `json:"-"`
	WrongCr    *chroma.Plane    `json:"-"`
	FixedCb    *chroma.Plane    `json:"-"`
	FixedCr    *chroma.Plane    `json:"-"`
	WrongTiled *chroma.Plane    `json:"-"`
	FixedTiled *chroma.Plane    `json:"-"`
	LocalCb    *chroma.Plane    `json:"-"`
	LocalCr    *chroma.Plane    `json:"-"`

	XRows   []AxisRow  `json:"xCoordinates"`
	YRows   []AxisRow  `json:"yCoordinates"`
	Blocks  []BlockRow `json:"blocks"`
	Rows    []Row      `json:"-"`
	Summary Summary    `json:"summary"`
}

// Run executes all variants against the same synthetic fixture.
func Run(cfg Config) (*Result, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.BlockW <= 0 || cfg.BlockH <= 0 {
		return nil, fmt.Errorf("invalid config %+v", cfg)
	}
	fixture, err := chroma.EdgeFixture(cfg.Width, cfg.Height)
	if err != nil {
		return nil, err
	}
	result := &Result{Config: cfg, Fixture: fixture}

	wrongOpt := chroma.BrokenOptions()
	wrongOpt.BlockW, wrongOpt.BlockH = cfg.BlockW, cfg.BlockH
	fixedOpt := chroma.DefaultOptions()
	fixedOpt.BlockW, fixedOpt.BlockH = cfg.BlockW, cfg.BlockH
	localOpt := chroma.BlockLocalOptions()
	localOpt.BlockW, localOpt.BlockH = cfg.BlockW, cfg.BlockH
	wholeOpt := fixedOpt
	wholeOpt.BlockW, wholeOpt.BlockH = cfg.Width, cfg.Height

	result.WrongCb, err = chroma.UpsamplePlane(fixture.Cb, cfg.Width, cfg.Height, wrongOpt)
	if err != nil {
		return nil, err
	}
	result.WrongCr, err = chroma.UpsamplePlane(fixture.Cr, cfg.Width, cfg.Height, wrongOpt)
	if err != nil {
		return nil, err
	}
	result.FixedCb, err = chroma.UpsamplePlane(fixture.Cb, cfg.Width, cfg.Height, wholeOpt)
	if err != nil {
		return nil, err
	}
	result.FixedCr, err = chroma.UpsamplePlane(fixture.Cr, cfg.Width, cfg.Height, wholeOpt)
	if err != nil {
		return nil, err
	}
	result.WrongTiled, err = chroma.UpsamplePlane(fixture.Cb, cfg.Width, cfg.Height, wrongOpt)
	if err != nil {
		return nil, err
	}
	result.FixedTiled, err = chroma.UpsamplePlane(fixture.Cb, cfg.Width, cfg.Height, fixedOpt)
	if err != nil {
		return nil, err
	}
	result.LocalCb, err = chroma.UpsamplePlane(fixture.Cb, cfg.Width, cfg.Height, localOpt)
	if err != nil {
		return nil, err
	}
	result.LocalCr, err = chroma.UpsamplePlane(fixture.Cr, cfg.Width, cfg.Height, localOpt)
	if err != nil {
		return nil, err
	}

	result.WrongYUV, err = chroma.Combine(fixture.Luma, result.WrongCb, result.WrongCr)
	if err != nil {
		return nil, err
	}
	result.FixedYUV, err = chroma.Combine(fixture.Luma, result.FixedCb, result.FixedCr)
	if err != nil {
		return nil, err
	}
	result.WrongRGB = chroma.YUVToRGB(result.WrongYUV)
	result.FixedRGB = chroma.YUVToRGB(result.FixedYUV)
	result.Reference = fixture.Reference

	result.XRows = axisRows(cfg.Width, fixture.Cb.W)
	result.YRows = axisRows(cfg.Height, fixture.Cb.H)
	result.Rows = buildRows(result)
	result.Blocks, result.Summary = summarize(result)
	return result, nil
}

func axisRows(n, sourceLen int) []AxisRow {
	rows := make([]AxisRow, n)
	for i := range rows {
		wrong := chroma.Axis(i, sourceLen, chroma.LeftTop)
		fixed := chroma.Axis(i, sourceLen, chroma.Centered)
		rows[i] = AxisRow{
			Position: i,
			Wrong:    wrong,
			Fixed:    fixed,
			Delta:    wrong.Coordinate - fixed.Coordinate,
		}
	}
	return rows
}

func buildRows(result *Result) []Row {
	f := result.Fixture
	rows := make([]Row, 0, f.W*f.H)
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			i := y*f.W + x
			ref := f.Reference.Pix[i]
			wr := result.WrongRGB.Pix[i]
			fr := result.FixedRGB.Pix[i]
			d := RGBDelta{R: wr.R - fr.R, G: wr.G - fr.G, B: wr.B - fr.B}
			rows = append(rows, Row{
				X:              x,
				Y:              y,
				RefCb:          f.Original.Pix[i].Cb,
				RefCr:          f.Original.Pix[i].Cr,
				WrongCb:        result.WrongCb.Pix[i],
				WrongCr:        result.WrongCr.Pix[i],
				FixedCb:        result.FixedCb.Pix[i],
				FixedCr:        result.FixedCr.Pix[i],
				RefRGB:         ref,
				WrongRGB:       wr,
				FixedRGB:       fr,
				DeltaCb:        result.WrongCb.Pix[i] - result.FixedCb.Pix[i],
				DeltaCr:        result.WrongCr.Pix[i] - result.FixedCr.Pix[i],
				Delta:          d,
				AbsRGB:         absRGB(d),
				WrongRefAbsRGB: absRGB(RGBDelta{wr.R - ref.R, wr.G - ref.G, wr.B - ref.B}),
				FixedRefAbsRGB: absRGB(RGBDelta{fr.R - ref.R, fr.G - ref.G, fr.B - ref.B}),
			})
		}
	}
	return rows
}

func summarize(result *Result) ([]BlockRow, Summary) {
	f := result.Fixture
	var blocks []BlockRow
	index := 0
	chroma.ForEachBlock(f.W, f.H, result.Config.BlockW, result.Config.BlockH, func(b chroma.Block) {
		br := BlockRow{Index: index, X0: b.X0, Y0: b.Y0, X1: b.X1, Y1: b.Y1}
		for y := b.Y0; y < b.Y1; y++ {
			for x := b.X0; x < b.X1; x++ {
				i := y*f.W + x
				br.WrongWholeVsTiled = math.Max(br.WrongWholeVsTiled,
					math.Abs(result.WrongCb.Pix[i]-result.WrongTiled.Pix[i]))
				br.FixedWholeVsTiled = math.Max(br.FixedWholeVsTiled,
					math.Abs(result.FixedCb.Pix[i]-result.FixedTiled.Pix[i]))
				br.BlockLocalVsFixed = math.Max(br.BlockLocalVsFixed,
					math.Abs(result.LocalCb.Pix[i]-result.FixedCb.Pix[i]))
				br.WrongVsFixed = math.Max(br.WrongVsFixed,
					math.Abs(result.WrongCb.Pix[i]-result.FixedCb.Pix[i]))
			}
		}
		blocks = append(blocks, br)
		index++
	})

	var s Summary
	for _, row := range result.Rows {
		s.MaxAbsDeltaCb = math.Max(s.MaxAbsDeltaCb, math.Abs(row.DeltaCb))
		s.MaxAbsDeltaCr = math.Max(s.MaxAbsDeltaCr, math.Abs(row.DeltaCr))
		s.MaxAbsRGB = math.Max(s.MaxAbsRGB, row.AbsRGB)
		s.MeanAbsRGB += row.AbsRGB
		s.WrongMeanRefAbsRGB += row.WrongRefAbsRGB
		s.FixedMeanRefAbsRGB += row.FixedRefAbsRGB
		s.WrongMaxRefAbsRGB = math.Max(s.WrongMaxRefAbsRGB, row.WrongRefAbsRGB)
		s.FixedMaxRefAbsRGB = math.Max(s.FixedMaxRefAbsRGB, row.FixedRefAbsRGB)
	}
	n := float64(len(result.Rows))
	s.MeanAbsRGB /= n
	s.WrongMeanRefAbsRGB /= n
	s.FixedMeanRefAbsRGB /= n
	for _, b := range blocks {
		s.WrongTiledMismatch = math.Max(s.WrongTiledMismatch, b.WrongWholeVsTiled)
		s.FixedTiledMismatch = math.Max(s.FixedTiledMismatch, b.FixedWholeVsTiled)
		s.BlockLocalMismatch = math.Max(s.BlockLocalMismatch, b.BlockLocalVsFixed)
	}
	for i, y := range f.Luma.Pix {
		s.MaxLumaDelta = math.Max(s.MaxLumaDelta,
			math.Abs(result.WrongYUV.Pix[i].Y-y))
		s.MaxLumaDelta = math.Max(s.MaxLumaDelta,
			math.Abs(result.FixedYUV.Pix[i].Y-y))
	}
	s.OddWidthSampleShift = oddWidthShift(f.W, f.Cb.W)
	return blocks, s
}

func absRGB(d RGBDelta) float64 {
	return math.Abs(d.R) + math.Abs(d.G) + math.Abs(d.B)
}

func oddWidthShift(width, chromaWidth int) float64 {
	if width%2 == 0 {
		return 0
	}
	m := chroma.Axis(width-1, chromaWidth, chroma.Centered)
	reconstructed := 2*m.Coordinate + 1
	expected := float64(width-1) + 0.5
	return math.Abs(reconstructed - expected)
}
