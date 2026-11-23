package main

import (
	"flag"
	"fmt"
	"os"

	"admcheck/adm"
)

func main() {
	layoutName := flag.String("layout", "0+5+0", "target layout: 0+2+0, 0+5+0, 4+5+0")
	downmix := flag.Bool("downmix", false, "also simulate a LoRo stereo downmix stage")
	outPath := flag.String("o", "", "write report to file instead of stdout")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: admcheck [flags] <adm.xml|bw64.wav>\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	layout, ok := adm.GetLayout(*layoutName)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown layout %q (supported: %v)\n", *layoutName, adm.LayoutNames())
		os.Exit(2)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open input: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	doc, err := adm.Open(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse ADM: %v\n", err)
		os.Exit(1)
	}
	res := adm.Render(doc, layout, *downmix)
	analysis := adm.Analyze(res)
	rows := adm.Timeline(res)

	out := os.Stdout
	if *outPath != "" {
		of, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create output: %v\n", err)
			os.Exit(1)
		}
		defer of.Close()
		out = of
	}
	adm.WriteReport(out, res, analysis, rows)
}
