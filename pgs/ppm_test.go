package pgs

import (
	"bytes"
	"image"
	"image/color"
	"strconv"
	"strings"
	"testing"
)

func TestWritePPMCompositesAlpha(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Pix[0], img.Pix[1], img.Pix[2], img.Pix[3] = 128, 64, 0, 128
	img.Pix[4], img.Pix[5], img.Pix[6], img.Pix[7] = 0, 0, 0, 0

	var out bytes.Buffer
	if err := WritePPM(&out, img, color.NRGBA{A: 255}); err != nil {
		t.Fatal(err)
	}
	data := out.Bytes()
	headerEnd := bytes.Index(data, []byte("\n255\n")) + len("\n255\n")
	if headerEnd <= len("\n255\n") {
		t.Fatalf("bad PPM header: %q", data[:min(len(data), 32)])
	}
	if got := strings.TrimSpace(string(data[:headerEnd])); got != "P6\n2 1\n255" {
		t.Fatalf("header = %q", got)
	}
	rgb := data[headerEnd:]
	if len(rgb) != 6 {
		t.Fatalf("payload length = %d, want 6", len(rgb))
	}
	if rgb[0] != 128 || rgb[1] != 64 || rgb[2] != 0 {
		t.Fatalf("alpha-composited pixel = %v", rgb[:3])
	}
	if rgb[3] != 0 || rgb[4] != 0 || rgb[5] != 0 {
		t.Fatalf("transparent pixel = %v", rgb[3:])
	}
}

func TestCheckerBackground(t *testing.T) {
	bg := CheckerBackground(color.NRGBA{R: 200, A: 255}, color.NRGBA{R: 100, A: 255}, 2)
	if got := bg(0, 0); got.R != 200 {
		t.Fatalf("checker (0,0) = %v", got)
	}
	if got := bg(2, 0); got.R != 100 {
		t.Fatalf("checker (2,0) = %v", got)
	}
	if strconv.Itoa(int(bg(3, 1).R)) != "100" {
		t.Fatal("unexpected checker parity")
	}
}

func TestResampleUpscalePreservesPremultipliedAlpha(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := 0; i < 4; i++ {
		src.Pix[i*4+0] = 64
		src.Pix[i*4+1] = 32
		src.Pix[i*4+2] = 16
		src.Pix[i*4+3] = 128
	}
	got := resample(src, 4, 4)
	if got.Bounds().Dx() != 4 || got.Bounds().Dy() != 4 {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	for i := 0; i < len(got.Pix); i += 4 {
		if got.Pix[i] != 64 || got.Pix[i+1] != 32 || got.Pix[i+2] != 16 || got.Pix[i+3] != 128 {
			t.Fatalf("pixel %d = %v", i/4, got.Pix[i:i+4])
		}
	}
}
