package experiment

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"strconv"

	"chromaexp/internal/chroma"
)

// Report is the stable JSON description of the experiment.
type Report struct {
	Schema    string        `json:"schema"`
	Fixture   ReportFixture `json:"fixture"`
	Policy    ReportPolicy  `json:"policy"`
	XRows     []AxisRow     `json:"xCoordinates"`
	YRows     []AxisRow     `json:"yCoordinates"`
	Blocks    []BlockRow    `json:"blocks"`
	Summary   Summary       `json:"summary"`
	InputHash string        `json:"inputYUVSHA256"`
	RefHash   string        `json:"referenceRGBSHA256"`
}

// ReportFixture identifies the deterministic test image.
type ReportFixture struct {
	Name         string `json:"name"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	ChromaWidth  int    `json:"chromaWidth"`
	ChromaHeight int    `json:"chromaHeight"`
	BlockWidth   int    `json:"blockWidth"`
	BlockHeight  int    `json:"blockHeight"`
}

// ReportPolicy records the non-negotiable processing rules.
type ReportPolicy struct {
	ChromaConvention   string `json:"chromaConvention"`
	BlockOrigin        string `json:"blockOrigin"`
	GlobalDesaturation bool   `json:"globalDesaturation"`
	LumaModified       bool   `json:"lumaModified"`
}

// NewReport builds a JSON-ready report.
func NewReport(r *Result) Report {
	return Report{
		Schema: "chroma-halfpixel-v1",
		Fixture: ReportFixture{
			Name:         r.Fixture.Name,
			Width:        r.Fixture.W,
			Height:       r.Fixture.H,
			ChromaWidth:  r.Fixture.Cb.W,
			ChromaHeight: r.Fixture.Cb.H,
			BlockWidth:   r.Config.BlockW,
			BlockHeight:  r.Config.BlockH,
		},
		Policy: ReportPolicy{
			ChromaConvention:   chroma.Centered.String(),
			BlockOrigin:        chroma.GlobalOrigin.String(),
			GlobalDesaturation: false,
			LumaModified:       false,
		},
		XRows:     r.XRows,
		YRows:     r.YRows,
		Blocks:    r.Blocks,
		Summary:   r.Summary,
		InputHash: hashYUV(r.Fixture.Original),
		RefHash:   hashRGB(r.Fixture.Reference),
	}
}

// WriteJSON writes the report with stable indentation.
func WriteJSON(w io.Writer, r *Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(NewReport(r))
}

// WritePixelsCSV writes one row per output pixel.
func WritePixelsCSV(w io.Writer, r *Result) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	header := []string{
		"x", "y", "ref_cb", "ref_cr", "wrong_cb", "wrong_cr", "fixed_cb", "fixed_cr",
		"d_cb", "d_cr", "ref_r", "ref_g", "ref_b", "wrong_r", "wrong_g", "wrong_b",
		"fixed_r", "fixed_g", "fixed_b", "d_r", "d_g", "d_b", "abs_rgb",
		"wrong_vs_ref_abs_rgb", "fixed_vs_ref_abs_rgb",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, p := range r.Rows {
		row := []string{
			strconv.Itoa(p.X), strconv.Itoa(p.Y),
			f(p.RefCb), f(p.RefCr), f(p.WrongCb), f(p.WrongCr), f(p.FixedCb), f(p.FixedCr),
			f(p.DeltaCb), f(p.DeltaCr),
			f(p.RefRGB.R), f(p.RefRGB.G), f(p.RefRGB.B),
			f(p.WrongRGB.R), f(p.WrongRGB.G), f(p.WrongRGB.B),
			f(p.FixedRGB.R), f(p.FixedRGB.G), f(p.FixedRGB.B),
			f(p.Delta.R), f(p.Delta.G), f(p.Delta.B), f(p.AbsRGB),
			f(p.WrongRefAbsRGB), f(p.FixedRefAbsRGB),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}

// WriteCoordinatesCSV writes the independent X/Y coordinate and weight audit.
func WriteCoordinatesCSV(w io.Writer, r *Result) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	header := []string{
		"axis", "position", "wrong_coord", "wrong_luma_position", "wrong_i0", "wrong_i1",
		"wrong_w0", "wrong_w1", "fixed_coord", "fixed_luma_position", "fixed_i0", "fixed_i1",
		"fixed_w0", "fixed_w1", "coord_delta",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	writeAxis := func(axis string, rows []AxisRow) error {
		for _, p := range rows {
			row := []string{
				axis, strconv.Itoa(p.Position),
				f(p.Wrong.Coordinate), f(2*p.Wrong.Coordinate + 1),
				strconv.Itoa(p.Wrong.I0), strconv.Itoa(p.Wrong.I1),
				f(p.Wrong.W0), f(p.Wrong.W1),
				f(p.Fixed.Coordinate), f(2*p.Fixed.Coordinate + 1),
				strconv.Itoa(p.Fixed.I0), strconv.Itoa(p.Fixed.I1),
				f(p.Fixed.W0), f(p.Fixed.W1), f(p.Delta),
			}
			if err := cw.Write(row); err != nil {
				return err
			}
		}
		return nil
	}
	if err := writeAxis("x", r.XRows); err != nil {
		return err
	}
	if err := writeAxis("y", r.YRows); err != nil {
		return err
	}
	return cw.Error()
}

// WriteBlocksCSV writes the tile-by-tile comparison.
func WriteBlocksCSV(w io.Writer, r *Result) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	header := []string{
		"index", "x0", "y0", "x1", "y1", "wrong_whole_vs_tiled_max",
		"fixed_whole_vs_tiled_max", "block_local_vs_fixed_max", "wrong_vs_fixed_max",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, b := range r.Blocks {
		row := []string{
			strconv.Itoa(b.Index), strconv.Itoa(b.X0), strconv.Itoa(b.Y0),
			strconv.Itoa(b.X1), strconv.Itoa(b.Y1),
			f(b.WrongWholeVsTiled), f(b.FixedWholeVsTiled),
			f(b.BlockLocalVsFixed), f(b.WrongVsFixed),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}

// WritePNG encodes a display RGB image.
func WritePNG(w io.Writer, src *chroma.RGBImage) error {
	dst := image.NewNRGBA(image.Rect(0, 0, src.W, src.H))
	for y := 0; y < src.H; y++ {
		for x := 0; x < src.W; x++ {
			p := src.At(x, y)
			dst.SetNRGBA(x, y, color.NRGBA{
				R: chroma.Clamp8(p.R),
				G: chroma.Clamp8(p.G),
				B: chroma.Clamp8(p.B),
				A: 255,
			})
		}
	}
	return png.Encode(w, dst)
}

// WriteDiffPNG encodes signed wrong-minus-fixed RGB differences with 8x gain.
func WriteDiffPNG(w io.Writer, src *RGBImageDelta) error {
	dst := image.NewNRGBA(image.Rect(0, 0, src.W, src.H))
	for y := 0; y < src.H; y++ {
		for x := 0; x < src.W; x++ {
			d := src.At(x, y)
			dst.SetNRGBA(x, y, color.NRGBA{
				R: chroma.Clamp8(128 + 8*d.R),
				G: chroma.Clamp8(128 + 8*d.G),
				B: chroma.Clamp8(128 + 8*d.B),
				A: 255,
			})
		}
	}
	return png.Encode(w, dst)
}

// RGBImageDelta is a float RGB difference image.
type RGBImageDelta struct {
	W   int
	H   int
	Pix []RGBDelta
}

// At returns the delta at x, y.
func (im *RGBImageDelta) At(x, y int) RGBDelta {
	return im.Pix[y*im.W+x]
}

// DiffImage returns wrong minus fixed RGB.
func DiffImage(r *Result) *RGBImageDelta {
	dst := &RGBImageDelta{W: r.Fixture.W, H: r.Fixture.H, Pix: make([]RGBDelta, len(r.Rows))}
	for i, p := range r.Rows {
		dst.Pix[i] = p.Delta
	}
	return dst
}

func f(v float64) string {
	if math.Abs(v) < 0.0000005 {
		v = 0
	}
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func hashYUV(src *chroma.YUVImage) string {
	h := sha256.New()
	for _, p := range src.Pix {
		writeFloat(h, p.Y)
		writeFloat(h, p.Cb)
		writeFloat(h, p.Cr)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashRGB(src *chroma.RGBImage) string {
	h := sha256.New()
	for _, p := range src.Pix {
		writeFloat(h, p.R)
		writeFloat(h, p.G)
		writeFloat(h, p.B)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeFloat(w io.Writer, v float64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
	_, _ = w.Write(buf[:])
}

// Verify makes accidental policy changes fail loudly.
func Verify(r *Result) error {
	if r.Summary.MaxLumaDelta != 0 {
		return fmt.Errorf("luma changed by %g", r.Summary.MaxLumaDelta)
	}
	if r.Summary.WrongTiledMismatch != 0 || r.Summary.FixedTiledMismatch != 0 {
		return fmt.Errorf("global-origin tiling changed output: wrong=%g fixed=%g",
			r.Summary.WrongTiledMismatch, r.Summary.FixedTiledMismatch)
	}
	if r.Summary.OddWidthSampleShift != 0 {
		return fmt.Errorf("odd-width sample center shifted by %g", r.Summary.OddWidthSampleShift)
	}
	return nil
}
