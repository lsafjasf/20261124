package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"hdr10audit/hdr"
	"hdr10audit/internal/fixture"
)

func main() {
	out := flag.String("out", "testdata/batch", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}
	for name, opts := range cases() {
		path := filepath.Join(*out, name+".mp4")
		if err := fixture.Write(path, opts); err != nil {
			fatal(err)
		}
		fmt.Println(path)
	}
	m, c := fixture.DefaultMetadata()
	frag := fixture.Options{
		ContainerMDCV: []hdr.Mastering{m},
		ContainerCLLI: []hdr.ContentLight{c},
		Bitstream:     []hdr.Metadata{{Mastering: &m, Light: &c}},
	}
	data, err := fixture.BuildFragmented(frag)
	if err != nil {
		fatal(err)
	}
	path := filepath.Join(*out, "fragmented.mp4")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatal(err)
	}
	fmt.Println(path)
}

func cases() map[string]fixture.Options {
	m1, c1 := fixture.DefaultMetadata()
	m2 := m1
	m2.Primaries[0].X = 14000
	m2.MaxLuminance = 8000000
	c2 := hdr.ContentLight{MaxCLL: 1200, MaxFALL: 350}

	badM := m1
	badM.Primaries[2].X = 60000
	badM.MaxLuminance = 200000000
	badC := hdr.ContentLight{MaxCLL: 60000, MaxFALL: 61000}

	dupM := m1
	dupM.Primaries[2].Y = 17000
	return map[string]fixture.Options{
		"aligned": {
			ContainerMDCV: []hdr.Mastering{m1},
			ContainerCLLI: []hdr.ContentLight{c1},
			Bitstream:     []hdr.Metadata{{Mastering: &m1, Light: &c1}},
		},
		"conflict": {
			ContainerMDCV: []hdr.Mastering{m1},
			ContainerCLLI: []hdr.ContentLight{c1},
			Bitstream:     []hdr.Metadata{{Mastering: &m2, Light: &c2}},
		},
		"missing-fields": {
			ContainerMDCV: []hdr.Mastering{m1},
			Bitstream:     []hdr.Metadata{{Light: &c1}},
		},
		"duplicate-sei": {
			ContainerMDCV: []hdr.Mastering{m1},
			ContainerCLLI: []hdr.ContentLight{c1},
			Bitstream: []hdr.Metadata{
				{Mastering: &m1, Light: &c1},
				{Mastering: &dupM},
			},
		},
		"duplicate-box": {
			ContainerMDCV: []hdr.Mastering{m1, m1},
			ContainerCLLI: []hdr.ContentLight{c1, c1},
			Bitstream:     []hdr.Metadata{{Mastering: &m1, Light: &c1}},
		},
		"unit-errors": {
			ContainerMDCV: []hdr.Mastering{badM},
			ContainerCLLI: []hdr.ContentLight{badC},
			Bitstream:     []hdr.Metadata{{Mastering: &badM, Light: &badC}},
		},
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
