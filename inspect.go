package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"ac3norm/internal/ac3"
)

type frameDump struct {
	Index          int    `json:"index"`
	Bytes          int    `json:"bytes"`
	SampleRate     int    `json:"sampleRateHz"`
	BitRateKbps    int    `json:"bitrateKbps"`
	Bsid           int    `json:"bsid"`
	Bsmod          int    `json:"bsmod"`
	Acmod          int    `json:"acmod"`
	Lfeon          int    `json:"lfeon"`
	Dialnorm       int    `json:"dialnorm"`
	DialnormBitPos int    `json:"dialnormBitPos"`
	PayloadSHA256  string `json:"payloadSha256"`
}

func runInspect(args []string) error {
	path, err := parseFileFlag(args)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read stream: %w", err)
	}
	stream, err := ac3.ParseStream(raw)
	if err != nil {
		return err
	}

	dumps := make([]frameDump, 0, len(stream.Frames))
	for _, frame := range stream.Frames {
		sum := sha256.Sum256(frame.Data)
		dumps = append(dumps, frameDump{
			Index:          frame.Index,
			Bytes:          len(frame.Data),
			SampleRate:     frame.SampleRate,
			BitRateKbps:    frame.BitRateKbps,
			Bsid:           frame.Bsid,
			Bsmod:          frame.Bsmod,
			Acmod:          frame.Acmod,
			Lfeon:          frame.Lfeon,
			Dialnorm:       frame.Dialnorm,
			DialnormBitPos: frame.DialnormPos,
			PayloadSHA256:  hex.EncodeToString(sum[:]),
		})
	}
	fileSum := sha256.Sum256(raw)
	out := map[string]any{
		"path":         path,
		"streamBytes":  len(raw),
		"frameCount":   len(stream.Frames),
		"streamSha256": hex.EncodeToString(fileSum[:]),
		"frames":       dumps,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
