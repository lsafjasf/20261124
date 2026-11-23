package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	outDir := flag.String("o", "", "directory for PNG previews (default: alongside .idx)")
	lang := flag.Int("lang", -1, "only process entries of this language stream index")
	ascii := flag.Bool("ascii", false, "print ASCII preview of each subtitle to stdout")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: vobsubtool [flags] file.idx\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	idxPath := flag.Arg(0)
	subPath := strings.TrimSuffix(idxPath, filepath.Ext(idxPath)) + ".sub"

	idxFile, err := os.Open(idxPath)
	if err != nil {
		fatal("open idx: %v", err)
	}
	idx, err := ParseIdx(idxFile)
	idxFile.Close()
	if err != nil {
		fatal("parse idx: %v", err)
	}
	subFile, err := os.Open(subPath)
	if err != nil {
		fatal("open sub: %v", err)
	}
	defer subFile.Close()
	st, err := subFile.Stat()
	if err != nil {
		fatal("stat sub: %v", err)
	}
	subSize := st.Size()
	dir := *outDir
	if dir == "" {
		dir = filepath.Dir(idxPath)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal("mkdir: %v", err)
	}
	fmt.Printf("idx: %s  sub: %s (%d bytes)  frame %dx%d  entries %d\n",
		idxPath, subPath, subSize, idx.Size[0], idx.Size[1], len(idx.Entries))

	failures := 0
	for _, e := range idx.Entries {
		if *lang >= 0 && e.LangIdx != *lang {
			continue
		}
		spu, start, end, err := readSPU(subFile, e, subSize)
		if err != nil {
			failures++
			fmt.Printf("ERROR %v\n", err)
			continue
		}
		sub, err := decodeSPU(e, spu, start, end)
		if err != nil {
			failures++
			fmt.Printf("ERROR %v\n", err)
			continue
		}
		if err := writePreview(dir, sub, &idx.Palette); err != nil {
			failures++
			fmt.Printf("ERROR %v\n", entryErr(e, "writing preview: %v", err))
			continue
		}
		fmt.Printf("OK    %s -> %s\n", Describe(sub), previewPath(dir, sub))
		if *ascii {
			fmt.Print(ASCIIArt(sub))
		}
	}
	if failures > 0 {
		fmt.Printf("%d of %d entries failed\n", failures, len(idx.Entries))
		os.Exit(1)
	}
}

func previewPath(dir string, sub *Subtitle) string {
	name := fmt.Sprintf("%04d_%s.png", sub.Entry.Index, strings.ReplaceAll(sub.Entry.Timestamp, ":", "-"))
	return filepath.Join(dir, name)
}

func writePreview(dir string, sub *Subtitle, palette *[16][3]uint8) error {
	f, err := os.Create(previewPath(dir, sub))
	if err != nil {
		return err
	}
	defer f.Close()
	return RenderPNG(f, sub, palette)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vobsubtool: "+format+"\n", args...)
	os.Exit(2)
}
