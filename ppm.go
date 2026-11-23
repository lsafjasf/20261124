package main

import (
	"bufio"
	"os"
	"path/filepath"
)

// Image is a row-major RGB image of display-ready [0,1] triples.
type Image struct {
	Width, Height int
	Pixels        []RGB
}

func newImage(width, height int) *Image {
	return &Image{Width: width, Height: height, Pixels: make([]RGB, width*height)}
}

func (im *Image) set(x, y int, c RGB) {
	im.Pixels[y*im.Width+x] = c
}

func encode16(v float64) []byte {
	v = clamp01(v)
	sample := int(v*65535 + 0.5)
	return []byte{byte(sample >> 8), byte(sample)}
}

func encode8(v float64) int {
	return int(clamp01(v)*255 + 0.5)
}

// WritePPM16 writes a 16-bit-per-channel binary PPM (magic P6, big-endian
// samples as required by the PPM specification).
func WritePPM16(dir, name string, im *Image) (string, error) {
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := w.WriteString("P6\n# gamutcompress evidence, 16-bit big-endian Rec.709 display values\n"); err != nil {
		return "", err
	}
	if _, err := w.WriteString(itoa(im.Width) + " " + itoa(im.Height) + "\n65535\n"); err != nil {
		return "", err
	}
	buf := make([]byte, 0, im.Width*im.Height*6)
	for _, c := range im.Pixels {
		buf = append(buf, encode16(c.R)...)
		buf = append(buf, encode16(c.G)...)
		buf = append(buf, encode16(c.B)...)
	}
	if _, err := w.Write(buf); err != nil {
		return "", err
	}
	return path, w.Flush()
}

// WritePPM8ASCII writes an 8-bit P3 (plain text) PPM so evidence can be
// inspected with any text editor.
func WritePPM8ASCII(dir, name string, im *Image) (string, error) {
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := w.WriteString("P3\n# gamutcompress evidence, 8-bit ASCII\n" +
		itoa(im.Width) + " " + itoa(im.Height) + "\n255\n"); err != nil {
		return "", err
	}
	for y := 0; y < im.Height; y++ {
		for x := 0; x < im.Width; x++ {
			c := im.Pixels[y*im.Width+x]
			if _, err := w.WriteString(itoa(encode8(c.R)) + " " +
				itoa(encode8(c.G)) + " " + itoa(encode8(c.B)) + "\n"); err != nil {
				return "", err
			}
		}
	}
	if err := w.Flush(); err != nil {
		return "", err
	}
	return path, nil
}

// WriteMaskPPM writes red marker pixels for true entries, gray otherwise.
func WriteMaskPPM(dir, name string, width, height int, mark []bool) (string, error) {
	im := newImage(width, height)
	for i, on := range mark {
		if on {
			im.Pixels[i] = RGB{1, 0.05, 0.05}
		} else {
			im.Pixels[i] = RGB{0.12, 0.12, 0.12}
		}
	}
	return WritePPM16(dir, name, im)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(b[pos:])
}
