// edlfix diagnoses and repairs DF/NDF timecode inconsistencies in CMX 3600
// EDL files. It never applies a fixed seconds offset: drop-frame drift is
// non-linear and accumulates per segment, so every label is recomputed from
// its absolute frame count at the referenced media frame rate.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"edlfix/internal/edl"
	"edlfix/internal/rate"
	"edlfix/internal/repair"
)

type config struct {
	input      string
	output     string
	defaultFPS string
	ratesFile  string
	reportJSON bool
	dryRun     bool
}

func main() {
	cfg := parseFlags()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "edlfix:", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.input, "input", "", "输入 EDL 文件路径（必填）")
	flag.StringVar(&cfg.output, "output", "", "修复后 EDL 输出路径；缺省为 <input>.fixed.edl")
	flag.StringVar(&cfg.defaultFPS, "fps", "", "缺省媒体帧率，如 29.97、30000/1001、25；未在 -rates 中声明的 reel 使用")
	flag.StringVar(&cfg.ratesFile, "rates", "", "reel 帧率映射文件，每行 `REEL=帧率`，# 开头为注释")
	flag.BoolVar(&cfg.reportJSON, "json", false, "以 JSON 输出诊断结果")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "只诊断不写文件")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout, _ io.Writer) error {
	if cfg.input == "" {
		return fmt.Errorf("缺少 -input 参数")
	}
	raw, err := os.ReadFile(cfg.input)
	if err != nil {
		return fmt.Errorf("读取输入失败: %w", err)
	}
	opts, err := buildOptions(cfg)
	if err != nil {
		return err
	}
	doc := edl.Parse(string(raw))
	if len(doc.Events) == 0 {
		return fmt.Errorf("未在 %s 中识别到任何 EDL 事件", cfg.input)
	}
	res := repair.Run(doc, opts)

	if cfg.reportJSON {
		return writeJSON(stdout, cfg, res)
	}
	printReport(stdout, cfg, res)
	if cfg.dryRun {
		return nil
	}
	outPath := cfg.output
	if outPath == "" {
		outPath = cfg.input + ".fixed.edl"
	}
	if err := os.WriteFile(outPath, []byte(res.RepairedText), 0o644); err != nil {
		return fmt.Errorf("写入输出失败: %w", err)
	}
	fmt.Fprintf(stdout, "\n修复后 EDL 已写入: %s\n", outPath)
	return nil
}

func buildOptions(cfg config) (repair.Options, error) {
	opts := repair.Options{ReelRates: map[string]rate.Rate{}}
	if cfg.ratesFile != "" {
		data, err := os.ReadFile(cfg.ratesFile)
		if err != nil {
			return opts, fmt.Errorf("读取帧率映射失败: %w", err)
		}
		if err := parseRatesFile(string(data), opts.ReelRates); err != nil {
			return opts, err
		}
	}
	if cfg.defaultFPS != "" {
		r, err := rate.Resolve(cfg.defaultFPS)
		if err != nil {
			return opts, fmt.Errorf("缺省帧率无效: %w", err)
		}
		opts.DefaultRate = &r
	}
	if opts.DefaultRate == nil && len(opts.ReelRates) == 0 {
		return opts, fmt.Errorf("必须提供 -fps 或 -rates，否则无法按引用媒体帧率重算")
	}
	return opts, nil
}

func parseRatesFile(content string, dst map[string]rate.Rate) error {
	for lineNo, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return fmt.Errorf("帧率映射第 %d 行格式错误（应为 REEL=帧率）: %q", lineNo+1, line)
		}
		reel := strings.TrimSpace(line[:eq])
		r, err := rate.Resolve(strings.TrimSpace(line[eq+1:]))
		if err != nil {
			return fmt.Errorf("帧率映射第 %d 行: %w", lineNo+1, err)
		}
		dst[reel] = r
	}
	return nil
}

func printReport(w io.Writer, cfg config, res *repair.Result) {
	fmt.Fprintln(w, "================ EDL DF/NDF 诊断修复报告 ================")
	fmt.Fprintf(w, "输入文件: %s\n", cfg.input)
	fmt.Fprintf(w, "事件总数: %d | 已修复: %d | 原样保留: %d\n",
		res.TotalEvents, res.RepairedEvents, res.SkippedEvents)

	fmt.Fprintln(w, "\n-- 引用媒体帧率 --")
	if len(res.ReelRates) == 0 {
		fmt.Fprintln(w, "  （无可用帧率信息）")
	}
	for _, name := range repair.ReelRatesSorted(res) {
		r := res.ReelRates[name]
		conv := "NDF"
		if r.DropCapable {
			conv = "DF"
		}
		fmt.Fprintf(w, "  reel %-12s 帧率 %-10s 规范标注: %s\n", name, r.Input, conv)
	}

	if res.FirstInconsistent != nil {
		f := res.FirstInconsistent
		fmt.Fprintln(w, "\n-- 首个不一致时间码 --")
		fmt.Fprintf(w, "  位置: 第 %d 行 第 %d 列，事件 %s（reel %s，字段 %s）\n",
			f.Line, f.Column, f.EventNum, f.Reel, f.Field)
		fmt.Fprintf(w, "  原文: %s  ->  修复: %s（帧号 %d，帧率 %s）\n",
			f.Original, f.Repaired, f.FrameCount, f.Rate)
		fmt.Fprintf(w, "  原因: %s\n", f.Message)
	} else {
		fmt.Fprintln(w, "\n未发现 DF/NDF 不一致。")
	}

	if len(res.Findings) > 1 {
		fmt.Fprintf(w, "\n-- 其余不一致条目（共 %d 处）--\n", len(res.Findings)-1)
		for _, f := range res.Findings[1:] {
			fmt.Fprintf(w, "  第%d行 事件%s %s: %s -> %s [%s]\n",
				f.Line, f.EventNum, f.Field, f.Original, f.Repaired, f.Code)
		}
	}

	if len(res.Skipped) > 0 {
		fmt.Fprintln(w, "\n-- 原样保留的条目 --")
		for _, s := range res.Skipped {
			fmt.Fprintf(w, "  第%d行 事件%s（reel %s）[%s]: %s\n",
				s.Line, s.EventNum, s.Reel, s.Code, s.Message)
		}
	}
}

func writeJSON(w io.Writer, cfg config, res *repair.Result) error {
	payload := struct {
		Input             string              `json:"input"`
		FirstInconsistent *repair.Finding     `json:"firstInconsistent,omitempty"`
		Findings          []repair.Finding    `json:"findings"`
		Skipped           []repair.SkipReason `json:"skipped"`
		ReelRates         map[string]string   `json:"reelRates"`
		RepairedEvents    int                 `json:"repairedEvents"`
		SkippedEvents     int                 `json:"skippedEvents"`
		TotalEvents       int                 `json:"totalEvents"`
		RepairedText      string              `json:"repairedText,omitempty"`
	}{
		Input:             cfg.input,
		FirstInconsistent: res.FirstInconsistent,
		Findings:          res.Findings,
		Skipped:           res.Skipped,
		ReelRates:         map[string]string{},
		RepairedEvents:    res.RepairedEvents,
		SkippedEvents:     res.SkippedEvents,
		TotalEvents:       res.TotalEvents,
	}
	if cfg.dryRun {
		payload.RepairedText = res.RepairedText
	}
	for name, r := range res.ReelRates {
		payload.ReelRates[name] = r.Input
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
