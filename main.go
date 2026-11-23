package main

import (
	"fmt"
	"os"
	"path/filepath"

	"opusloss/audio"
	"opusloss/sim"
)

const (
	outDir  = "output"
	nFrames = 60 // 60 * 20 ms = 1.2 s
	seed    = 20261124
)

type scenario struct {
	name    string
	pattern string
	mask    sim.Mask
	events  [][2]int
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	clean := audio.GenerateSpeechLike(sim.SampleRate, nFrames*sim.FrameSize, seed)
	frames := audio.Frames(clean, sim.FrameSize)
	if err := audio.WriteWAV16(filepath.Join(outDir, "source_clean.wav"), sim.SampleRate, clean); err != nil {
		return err
	}

	scenarios := []scenario{
		{
			name:    "isolated",
			pattern: "孤立丢包（单帧，间隔足够，FEC 可修复）",
			mask:    sim.IsolatedMask(6, 18, 34, 49),
		},
		{
			name:    "burst",
			pattern: "连续丢包（两段 4 帧突发，共 8 帧）",
			mask:    mergeMask(sim.BurstMask(10, 4), sim.BurstMask(40, 4)),
		},
	}
	for sc := range scenarios {
		scenarios[sc].events = scenarios[sc].mask.Events(nFrames)
	}
	if err := writeMasks(scenarios); err != nil {
		return err
	}

	packets := sim.BuildPackets(frames)
	modes := []sim.Mode{sim.ModeFEC, sim.ModeSilence, sim.ModePLC}
	var reports []runReport
	for _, sc := range scenarios {
		channel := sim.ApplyChannel(packets, sc.mask)
		for _, mode := range modes {
			rep, err := runOne(sc, mode, channel, frames)
			if err != nil {
				return err
			}
			reports = append(reports, rep)
		}
	}
	if err := writeJSON(reports); err != nil {
		return err
	}
	return writeReport(reports, scenarios)
}

func mergeMask(a, b sim.Mask) sim.Mask {
	m := sim.Mask{}
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}

func runOne(sc scenario, mode sim.Mode, channel []*sim.Packet, clean [][]float64) (runReport, error) {
	rcv := sim.NewReceiver(mode)
	reconstructed := rcv.Run(channel)
	if err := sim.AuditNoBackfill(reconstructed, clean, rcv.Chain()); err != nil {
		return runReport{}, fmt.Errorf("%s/%s: %w", sc.name, mode, err)
	}
	metrics := sim.ErrorMetrics(reconstructed, clean, rcv.Chain())
	flat := audio.Flatten(reconstructed)
	file := filepath.Join(outDir, fmt.Sprintf("%s_%s.wav", sc.name, mode))
	if err := audio.WriteWAV16(file, sim.SampleRate, flat); err != nil {
		return runReport{}, err
	}
	rep := runReport{
		Scenario: sc.name, Pattern: sc.pattern, Mode: string(mode),
		Output: file, Metrics: metrics,
		StateSequence: sim.StateSequence(rcv.Chain()),
		Chain:         rcv.Chain(),
		Events:        sc.events,
	}
	fmt.Printf("%-8s mode=%-7s lostFrames=%d recoveredSamples=%d MAE=%.5f RMSE=%.5f segSNR=%.2fdB\n",
		sc.name, mode, metrics.LostFrames, metrics.RecoveredSamples,
		metrics.MAE, metrics.RMSE, metrics.SegSNRDB)
	return rep, nil
}
