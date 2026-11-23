package compare

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"hdrthumb/fixtures"
	"hdrthumb/hdr"
)

func TestCompressedScenesPreserveHue(t *testing.T) {
	for _, scene := range fixtures.All(96, 64) {
		for _, preset := range hdr.Presets() {
			out, _ := hdr.CompressFrame(scene.Frame, preset)
			m := Measure(scene.Frame, out)
			if m.ChromaticPixels == 0 {
				t.Fatalf("%s/%s: no chromatic pixels measured", scene.Name, preset.Name)
			}
			if m.HueShiftMaxDeg > 1e-5 {
				t.Fatalf("%s/%s: max hue shift %.9f degrees", scene.Name, preset.Name, m.HueShiftMaxDeg)
			}
		}
	}
}

func TestNaiveClampDistortsHueMuchMore(t *testing.T) {
	results := Run(fixtures.All(96, 64), hdr.Presets())
	for _, r := range results {
		if r.Naive.HueShiftMeanDeg < 5 {
			t.Fatalf("%s/%s: naive hue shift only %.3f degrees", r.Scene, r.Preset, r.Naive.HueShiftMeanDeg)
		}
		if r.Naive.HueShiftMeanDeg <= r.Compressed.HueShiftMeanDeg+1 {
			t.Fatalf("%s/%s: naive %.3f, compressed %.6f", r.Scene, r.Preset,
				r.Naive.HueShiftMeanDeg, r.Compressed.HueShiftMeanDeg)
		}
	}
}

func TestAllCompressedPixelsAreDisplayable(t *testing.T) {
	for _, scene := range fixtures.All(64, 48) {
		for _, preset := range hdr.Presets() {
			out, _ := hdr.CompressFrame(scene.Frame, preset)
			for i, c := range out.Pixels {
				if !c.IsFinite() || !c.InGamut() {
					t.Fatalf("%s/%s pixel %d = %v", scene.Name, preset.Name, i, c)
				}
			}
		}
	}
}

func TestStrongerPresetsFoldHighlightsMore(t *testing.T) {
	in := hdr.RGB{R: 2, G: 1, B: 0.5}
	mild := hdr.Compress(in, hdr.Mild).Color.Luminance()
	balanced := hdr.Compress(in, hdr.Balanced).Color.Luminance()
	strong := hdr.Compress(in, hdr.Strong).Color.Luminance()
	if !(mild > balanced && balanced > strong) {
		t.Fatalf("luminance order mild=%.6f balanced=%.6f strong=%.6f", mild, balanced, strong)
	}
}

func TestWriteStrip(t *testing.T) {
	first := hdr.NewFrame(8, 4)
	second := hdr.NewFrame(8, 4)
	for i := range first.Pixels {
		first.Pixels[i] = hdr.RGB{R: 1, G: 0.5, B: 0.25}
		second.Pixels[i] = hdr.RGB{R: 0.1, G: 0.2, B: 0.3}
	}
	path := filepath.Join(t.TempDir(), "strip.png")
	if err := WriteStrip(path, []hdr.Frame{first, second}, 2); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != 18 {
		t.Fatalf("strip width = %d, want 18", got)
	}
	if got := img.Bounds().Dy(); got != 4 {
		t.Fatalf("strip height = %d, want 4", got)
	}
	red, green, blue, _ := img.At(8, 0).RGBA()
	if red>>8 != 16 || green>>8 != 16 || blue>>8 != 16 {
		t.Fatalf("separator = (%d,%d,%d), want (16,16,16)", red>>8, green>>8, blue>>8)
	}
}
