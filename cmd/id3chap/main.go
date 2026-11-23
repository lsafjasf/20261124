package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"id3chap/internal/chapters"
	"id3chap/internal/id3"
	"id3chap/internal/mp3"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "inspect":
		err = inspectCmd(os.Args[2:])
	case "retag":
		err = retagCmd(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `id3chap - ID3v2 chapter tree validator

usage:
  id3chap inspect [--duration-ms N] [--tlen] file.mp3
  id3chap retag --out new.mp3 [--drop ID]... [--padding N] file.mp3
`)
}

func load(path string) (*id3.Tag, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return id3.Parse(buf)
}

func inspectCmd(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	durationOverride := fs.Uint("duration-ms", 0, "override audio duration in milliseconds")
	useTLEN := fs.Bool("tlen", false, "trust TLEN frame for duration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("inspect requires exactly one input file")
	}
	tag, err := load(fs.Arg(0))
	if err != nil {
		return err
	}

	var tlen uint32
	if *useTLEN {
		for _, f := range tag.Frames {
			if f.ID == "TLEN" {
				tlen = id3.TLENMillis(f.Text)
			}
		}
	}
	durationMS, from := mp3.DurationMillis(tag.Audio, tlen)
	if *durationOverride > 0 {
		durationMS, from = uint32(*durationOverride), "flag"
	}

	report := chapters.Validate(tag, durationMS, from)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func retagCmd(args []string) error {
	fs := flag.NewFlagSet("retag", flag.ContinueOnError)
	out := fs.String("out", "", "output file (required)")
	padding := fs.Int("padding", 0, "padding zero bytes inside the new tag")
	drop := multiFlag{}
	fs.Var(&drop, "drop", "frame id to omit (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("retag requires --out and exactly one input file")
	}

	buf, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	tag, err := id3.Parse(buf)
	if err != nil {
		return err
	}
	audioBefore := sha256.Sum256(tag.Audio)

	block := id3.WriteBlock(tag, id3.WriteOptions{
		DropFrames: drop.set(),
		Padding:    *padding,
	})
	audioAfter := sha256.Sum256(tag.Audio)
	if audioBefore != audioAfter {
		return fmt.Errorf("internal error: audio payload changed during retag")
	}

	outBuf := make([]byte, 0, len(block)+len(tag.Audio))
	outBuf = append(outBuf, block...)
	outBuf = append(outBuf, tag.Audio...)
	if err := os.WriteFile(*out, outBuf, 0o644); err != nil {
		return err
	}

	result := map[string]any{
		"input":           fs.Arg(0),
		"output":          *out,
		"old_tag_bytes":   tag.TagSize,
		"new_tag_bytes":   len(block),
		"audio_bytes":     len(tag.Audio),
		"audio_sha256":    hex.EncodeToString(audioBefore[:]),
		"audio_preserved": true,
		"frames_written":  countFrames(tag, drop.set()),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func countFrames(tag *id3.Tag, drop map[string]bool) int {
	n := 0
	for _, f := range tag.Frames {
		if !drop[f.ID] {
			n++
		}
	}
	return n
}

type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint([]string(*m)) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
func (m multiFlag) set() map[string]bool {
	out := map[string]bool{}
	for _, v := range m {
		out[v] = true
	}
	return out
}
