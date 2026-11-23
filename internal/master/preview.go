package master

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// WritePreviewPair writes review images, not color-managed deliverables. Both
// previews use the same absolute-luminance review curve so equal middle gray
// remains visibly equal while HDR highlight separation stays inspectable.
func WritePreviewPair(dir, shot string, frame int, sdr, hdr *Master, cfg Config) error {
	if sdr.Width != hdr.Width || sdr.Height != hdr.Height {
		return fmt.Errorf("SDR/HDR dimensions differ")
	}
	shotDir := filepath.Join(dir, shot)
	if err := os.MkdirAll(shotDir, 0o755); err != nil {
		return err
	}
	sdrImg := renderPreview(sdr, "SDR", cfg)
	hdrImg := renderPreview(hdr, "HDR10", cfg)
	base := fmt.Sprintf("%s_%04d", shot, frame)
	if err := writePNG(filepath.Join(shotDir, base+"_sdr.png"), sdrImg); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(shotDir, base+"_hdr10.png"), hdrImg); err != nil {
		return err
	}
	return writePNG(filepath.Join(shotDir, base+"_compare.png"), compareImages(sdrImg, hdrImg))
}

func renderPreview(m *Master, profile string, cfg Config) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, m.Width, m.Height))
	denom := math.Log1p(cfg.HDRPeakNits / 4.0)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			codes := m.Pixel(x, y)
			var linearNits Vec3
			var rgb [3]uint8
			for c := 0; c < 3; c++ {
				signal := DequantizeLegal10(codes[c], cfg)
				if profile == "SDR" {
					linearNits[c] = BT1886DecodeGamma(signal, cfg.SDRGamma) * cfg.SDRPeakNits
				} else {
					linearNits[c] = PQDecode(signal)
				}
			}
			if profile == "HDR10" {
				target := Luminance2020(linearNits)
				linearNits = MulMat3(rec2020ToRec709, linearNits)
				linearNits = FitGamut(linearNits, target, 0, cfg.HDRPeakNits)
			}
			for c := 0; c < 3; c++ {
				nits := linearNits[c]
				norm := math.Log1p(max(nits, 0)/4.0) / denom
				rgb[c] = uint8(math.Round(255 * sRGBEncode(clamp(norm, 0, 1))))
			}
			img.SetRGBA(x, y, color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255})
		}
	}
	return img
}

func sRGBEncode(linear float64) float64 {
	if linear <= 0.0031308 {
		return 12.92 * linear
	}
	return 1.055*math.Pow(linear, 1/2.4) - 0.055
}

func compareImages(a, b *image.RGBA) *image.RGBA {
	const gap = 8
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, w*2+gap, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetRGBA(x, y, a.RGBAAt(x, y))
			out.SetRGBA(x+w+gap, y, b.RGBAAt(x, y))
		}
	}
	for y := 0; y < h; y++ {
		for x := w; x < w+gap; x++ {
			out.SetRGBA(x, y, color.RGBA{R: 48, G: 48, B: 48, A: 255})
		}
	}
	return out
}

func writePNG(path string, img image.Image) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
