package master

import (
	"math"
	"path/filepath"
	"testing"
)

func TestPFMRoundTripPreservesTopLeftOrientation(t *testing.T) {
	f := &Frame{
		Width: 2, Height: 2, Shot: "S001", FrameNumber: 7,
		Pixels: []Vec3{
			{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6},
			{0.7, 0.8, 0.9}, {1.1, 1.2, 1.3},
		},
	}
	path := filepath.Join(t.TempDir(), "frame.pfm")
	if err := WritePFM(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPFM(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != f.Width || got.Height != f.Height {
		t.Fatalf("dimensions %dx%d", got.Width, got.Height)
	}
	for i, want := range f.Pixels {
		for c := 0; c < 3; c++ {
			w := float64(float32(want[c]))
			if math.Abs(got.Pixels[i][c]-w) > 1e-7 {
				t.Fatalf("pixel %d channel %d = %f, want %f", i, c, got.Pixels[i][c], w)
			}
		}
	}
}
