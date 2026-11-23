// Package audio contains dependency-free PCM and WAV helpers.
package audio

import (
	"encoding/binary"
	"errors"
	"os"
)

// WriteWAV16 writes mono 16-bit little-endian PCM as a canonical WAV file.
func WriteWAV16(path string, sampleRate int, samples []float64) error {
	dataBytes := 2 * len(samples)
	buf := make([]byte, 44+dataBytes)

	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataBytes))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // PCM chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM format
	binary.LittleEndian.PutUint16(buf[22:24], 1)  // mono
	binary.LittleEndian.PutUint32(buf[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(sampleRate*2)) // byte rate
	binary.LittleEndian.PutUint16(buf[32:34], 2)                    // block align
	binary.LittleEndian.PutUint16(buf[34:36], 16)                   // bits per sample
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataBytes))

	for i, s := range samples {
		v := int16(Clamp(round(s*32767.0), -32768, 32767))
		binary.LittleEndian.PutUint16(buf[44+2*i:46+2*i], uint16(v))
	}
	return os.WriteFile(path, buf, 0o644)
}

// ReadWAV16 reads a mono 16-bit PCM WAV file and returns float64 samples.
func ReadWAV16(path string) (sampleRate int, samples []float64, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, err
	}
	if len(raw) < 44 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return 0, nil, errors.New("not a RIFF/WAVE file")
	}
	pos := 12
	var data []byte
	for pos+8 <= len(raw) {
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(raw) {
			return 0, nil, errors.New("truncated chunk: " + id)
		}
		if id == "fmt " {
			if size < 16 ||
				binary.LittleEndian.Uint16(raw[body:body+2]) != 1 ||
				binary.LittleEndian.Uint16(raw[body+2:body+4]) != 1 ||
				binary.LittleEndian.Uint16(raw[body+14:body+16]) != 16 {
				return 0, nil, errors.New("only mono PCM 16-bit WAV is supported")
			}
			sampleRate = int(binary.LittleEndian.Uint32(raw[body+4 : body+8]))
		}
		if id == "data" {
			data = raw[body : body+size]
		}
		pos = body + size + size%2
	}
	if sampleRate == 0 || data == nil {
		return 0, nil, errors.New("missing fmt/data chunk")
	}
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	samples = make([]float64, len(data)/2)
	for i := range samples {
		samples[i] = float64(int16(binary.LittleEndian.Uint16(data[2*i:2*i+2]))) / 32768.0
	}
	return sampleRate, samples, nil
}

// Frames splits samples into fixed-size frames; the final partial frame is dropped.
func Frames(samples []float64, size int) [][]float64 {
	n := len(samples) / size
	out := make([][]float64, n)
	for i := 0; i < n; i++ {
		out[i] = samples[i*size : (i+1)*size]
	}
	return out
}

// Flatten concatenates frames into one slice.
func Flatten(frames [][]float64) []float64 {
	if len(frames) == 0 {
		return nil
	}
	size := len(frames[0])
	out := make([]float64, 0, len(frames)*size)
	for _, f := range frames {
		out = append(out, f...)
	}
	return out
}

func Clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func round(x float64) float64 {
	if x < 0 {
		return float64(int64(x - 0.5))
	}
	return float64(int64(x + 0.5))
}
