package colorxform

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestDepth16KeepsFullPrecision guards against the 16-bit path being
// truncated to the high byte: the output must match a full-precision
// conversion of the exact 16-bit input, not of its high byte.
func TestDepth16KeepsFullPrecision(t *testing.T) {
	img := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	img.SetNRGBA64(0, 0, color.NRGBA64{100, 100, 100, 0xffff})
	out, err := ConvertImage(img, Options{OutputDepth: 16, ClampOutput: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, _ := out.At(0, 0).RGBA()
	cr, _, _ := ConvertColor(100.0/65535, 100.0/65535, 100.0/65535, 0)
	want := uint32(cr*65535 + 0.5)
	if got != want {
		t.Fatalf("16-bit path imprecise: got %d, want %d", got, want)
	}
	truncated, _, _ := ConvertColor(0, 0, 0, 0) // high-byte-only path would see 0
	if got == uint32(truncated*65535+0.5) {
		t.Fatalf("16-bit path dropped the low byte: got %d", got)
	}
}

func TestDepth8Endpoints(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 0xff})
	img.SetNRGBA(1, 0, color.NRGBA{255, 255, 255, 0xff})
	out, err := ConvertImage(img, Options{OutputDepth: 8, ClampOutput: true})
	if err != nil {
		t.Fatal(err)
	}
	r0, _, _, _ := out.At(0, 0).RGBA()
	r1, _, _, _ := out.At(1, 0).RGBA()
	if r0 != 0 || r1 != 0xffff {
		t.Fatalf("8-bit endpoints wrong: black -> %d, white -> %d", r0, r1)
	}
}

func TestBadDepthRejected(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if _, err := ConvertImage(img, Options{OutputDepth: 12}); err == nil {
		t.Fatal("expected error for unsupported depth")
	}
}

// TestGoldenSamples converts the checked-in sRGB regression samples and
// compares pixel-exact against the expected Rec.709 outputs. Regenerate
// with: go run ./cmd/regen
func TestGoldenSamples(t *testing.T) {
	dir := filepath.Join("..", "testdata")
	names := []string{"dark_gradient_16", "gray_ramp_8", "saturated_patches_16"}
	for _, name := range names {
		src := decodePNG(t, filepath.Join(dir, name+"_srgb.png"))
		for _, depth := range []int{8, 16} {
			wantFile := filepath.Join(dir, name+"_expected_rec709_"+itoa(depth)+".png")
			want := decodePNG(t, wantFile)
			got, err := ConvertImage(src, Options{OutputDepth: depth, ClampOutput: true})
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, got); err != nil {
				t.Fatal(err)
			}
			back, err := png.Decode(&buf)
			if err != nil {
				t.Fatal(err)
			}
			compareImages(t, name, depth, back, want)
		}
	}
}

func itoa(v int) string {
	if v == 8 {
		return "8"
	}
	return "16"
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func compareImages(t *testing.T, name string, depth int, got, want image.Image) {
	t.Helper()
	b := want.Bounds()
	if got.Bounds() != b {
		t.Fatalf("%s/%d-bit: bounds differ: %v vs %v", name, depth, got.Bounds(), b)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			gr, gg, gb, ga := got.At(x, y).RGBA()
			wr, wg, wb, wa := want.At(x, y).RGBA()
			if gr != wr || gg != wg || gb != wb || ga != wa {
				t.Fatalf("%s/%d-bit: pixel (%d,%d): got (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					name, depth, x, y, gr, gg, gb, ga, wr, wg, wb, wa)
			}
		}
	}
}
