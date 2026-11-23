package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"opusloss/sim"
)

func writeReport(reports []runReport, scenarios []scenario) error {
	var b strings.Builder
	b.WriteString("# 丢包恢复对比实验（Opus 风格 FEC / 静音插入 / 独立 PLC）\n\n")
	b.WriteString(fmt.Sprintf("- 采样率 %d Hz，帧长 %d 样本（20 ms），共 %d 帧（%.1f s）\n",
		sim.SampleRate, sim.FrameSize, nFrames, float64(nFrames)*sim.FrameSize/float64(sim.SampleRate)))
	b.WriteString("- 纯 Go 标准库实现；接收器无法访问发送端干净源，恢复帧逐样本审计禁止原文回填。\n")
	b.WriteString("- FEC 为发送端低通+8bit 量化的前一帧冗余，由下一帧分组携带。\n\n")
	b.WriteString("状态链图例：`.` 正常 `R` 恢复交叉淡化 `S` 静音 `P` PLC `C` 冷启动PLC `F` FEC修复 `p` FEC回退PLC `s` FEC回退静音\n\n")

	for _, sc := range scenarios {
		b.WriteString("## " + sc.pattern + "\n\n")
		var ev []string
		for _, e := range sc.events {
			if e[0] == e[1] {
				ev = append(ev, fmt.Sprintf("帧%d", e[0]))
			} else {
				ev = append(ev, fmt.Sprintf("帧%d-%d（%d帧）", e[0], e[1], e[1]-e[0]+1))
			}
		}
		b.WriteString("丢包事件：" + strings.Join(ev, "，") + "\n\n")
		b.WriteString("| 策略 | 丢包帧 | 恢复样本数 | MAE | RMSE | 峰值误差 | 分段SNR(dB) | 输出 |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---:|---|\n")
		for _, rep := range reports {
			if rep.Scenario != sc.name {
				continue
			}
			snr := fmt.Sprintf("%.2f", rep.Metrics.SegSNRDB)
			b.WriteString(fmt.Sprintf("| %s | %d | %d | %.5f | %.5f | %.5f | %s | `%s` |\n",
				modeLabel(rep.Mode), rep.Metrics.LostFrames, rep.Metrics.RecoveredSamples,
				rep.Metrics.MAE, rep.Metrics.RMSE, rep.Metrics.PeakAbs, snr, rep.Output))
		}
		b.WriteString("\n状态链（每 token 一帧）：\n\n")
		for _, rep := range reports {
			if rep.Scenario != sc.name {
				continue
			}
			b.WriteString("- " + modeLabel(rep.Mode) + "：`" + rep.StateSequence + "`\n")
		}
		b.WriteString("\n逐帧状态链明细：\n\n")
		for _, rep := range reports {
			if rep.Scenario != sc.name {
				continue
			}
			b.WriteString("**" + modeLabel(rep.Mode) + "**\n\n")
			b.WriteString("| 帧 | 状态 | 丢包 | 恢复样本 | 衰减增益 | 基音滞后 |\n|---:|---|:--:|---:|---:|---:|\n")
			for _, c := range rep.Chain {
				gain := "-"
				if c.Gain != 0 {
					gain = fmt.Sprintf("%.3f", c.Gain)
				}
				lag := "-"
				if c.PitchLag > 0 {
					lag = fmt.Sprintf("%d", c.PitchLag)
				}
				b.WriteString(fmt.Sprintf("| %d | %s | %t | %d | %s | %s |\n",
					c.Index, c.State, c.Lost, c.Samples, gain, lag))
			}
			b.WriteString("\n")
		}
	}
	return os.WriteFile(filepath.Join(outDir, "report.md"), []byte(b.String()), 0o644)
}

func modeLabel(mode string) string {
	switch mode {
	case "fec":
		return "Opus内建FEC"
	case "silence":
		return "静音插入"
	case "plc":
		return "独立PLC"
	default:
		return mode
	}
}
