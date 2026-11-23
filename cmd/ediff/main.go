package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"ediff/internal/diff"
	"ediff/internal/project"
	"ediff/internal/report"
)

func main() {
	format := flag.String("format", "text", "输出格式：text 或 json")
	outPath := flag.String("out", "", "报告输出路径，缺省输出到 stdout")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"用法: %s [选项] <base.json> <target.json>\n\n剪辑工程合并前的可核查变更单生成器\n\n选项:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	base, err := project.LoadFile(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	target, err := project.LoadFile(flag.Arg(1))
	if err != nil {
		fatal(err)
	}

	r := diff.Compare(base, target)

	w := os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		w = f
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fatal(err)
		}
	case "text":
		if err := report.WriteText(w, r); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("unknown format %q", *format))
	}

	// 退出码：存在冲突时返回 1，便于流水线门禁。
	if r.Summary.Conflicts > 0 {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ediff:", err)
	os.Exit(2)
}
