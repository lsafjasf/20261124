package master

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// ReadPFM reads a color Portable Float Map. PFM files store rows bottom-up;
// this function converts them to the top-down convention used by Frame.
func ReadPFM(path string) (*Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return decodePFM(bufio.NewReaderSize(f, 1<<20))
}

func decodePFM(r *bufio.Reader) (*Frame, error) {
	magic, err := nextPFMToken(r)
	if err != nil {
		return nil, fmt.Errorf("read PFM magic: %w", err)
	}
	if magic != "PF" {
		return nil, fmt.Errorf("unsupported PFM type %q (only color PF is supported)", magic)
	}
	width, err := nextPFMInt(r)
	if err != nil {
		return nil, fmt.Errorf("read PFM width: %w", err)
	}
	height, err := nextPFMInt(r)
	if err != nil {
		return nil, fmt.Errorf("read PFM height: %w", err)
	}
	if width <= 0 || height <= 0 || width > 32768 || height > 32768 {
		return nil, fmt.Errorf("invalid PFM dimensions %dx%d", width, height)
	}
	scaleToken, err := nextPFMToken(r)
	if err != nil {
		return nil, fmt.Errorf("read PFM scale: %w", err)
	}
	scale, err := strconv.ParseFloat(scaleToken, 64)
	if err != nil || scale == 0 {
		return nil, fmt.Errorf("invalid PFM scale %q", scaleToken)
	}
	littleEndian := scale < 0
	rowBytes := width * 3 * 4
	row := make([]byte, rowBytes)
	frame := &Frame{Width: width, Height: height, Pixels: make([]Vec3, width*height)}
	for fileRow := 0; fileRow < height; fileRow++ {
		if _, err := io.ReadFull(r, row); err != nil {
			return nil, fmt.Errorf("read PFM row %d: %w", fileRow, err)
		}
		y := height - 1 - fileRow
		for x := 0; x < width; x++ {
			var channels [3]float64
			for c := 0; c < 3; c++ {
				off := (x*3 + c) * 4
				var bits uint32
				if littleEndian {
					bits = binary.LittleEndian.Uint32(row[off : off+4])
				} else {
					bits = binary.BigEndian.Uint32(row[off : off+4])
				}
				v := float64(math.Float32frombits(bits))
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return nil, fmt.Errorf("non-finite pixel at x=%d y=%d channel=%d", x, y, c)
				}
				channels[c] = v
			}
			frame.Pixels[y*width+x] = Vec3{channels[0], channels[1], channels[2]}
		}
	}
	return frame, nil
}

func nextPFMToken(r *bufio.Reader) (string, error) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if isPFMSpace(b) {
			continue
		}
		if b == '#' {
			if _, err := r.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
				return "", err
			}
			continue
		}
		var token strings.Builder
		token.WriteByte(b)
		for {
			b, err = r.ReadByte()
			if errors.Is(err, io.EOF) {
				return token.String(), nil
			}
			if err != nil {
				return "", err
			}
			if isPFMSpace(b) {
				return token.String(), nil
			}
			token.WriteByte(b)
		}
	}
}

func nextPFMInt(r *bufio.Reader) (int, error) {
	token, err := nextPFMToken(r)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(token)
}

func isPFMSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// WritePFM writes a color PFM with little-endian float samples.
func WritePFM(path string, f *Frame) error {
	if err := f.Validate(); err != nil {
		return err
	}
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()
	w := bufio.NewWriterSize(out, 1<<20)
	if _, err := fmt.Fprintf(w, "PF\n%d %d\n-1.0\n", f.Width, f.Height); err != nil {
		return err
	}
	var word [4]byte
	for y := f.Height - 1; y >= 0; y-- {
		for x := 0; x < f.Width; x++ {
			v := f.At(x, y)
			for c := 0; c < 3; c++ {
				binary.LittleEndian.PutUint32(word[:], math.Float32bits(float32(v[c])))
				if _, err := w.Write(word[:]); err != nil {
					return err
				}
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
