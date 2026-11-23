package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"opusloss/sim"
)

type runReport struct {
	Scenario      string            `json:"scenario"`
	Pattern       string            `json:"pattern"`
	Mode          string            `json:"mode"`
	Output        string            `json:"output_wav"`
	Metrics       sim.Metrics       `json:"metrics"`
	StateSequence string            `json:"state_sequence"`
	Chain         []sim.FrameResult `json:"state_chain"`
	Events        [][2]int          `json:"loss_events"`
}

func writeMasks(scenarios []scenario) error {
	for _, sc := range scenarios {
		path := filepath.Join(outDir, "mask_"+sc.name+".txt")
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(f, "# %s\n# frame index, state (1=lost)\n", sc.pattern)
		if err != nil {
			f.Close()
			return err
		}
		for i := 0; i < nFrames; i++ {
			state := 0
			if sc.mask[i] {
				state = 1
			}
			if _, err := fmt.Fprintf(f, "%d %d\n", i, state); err != nil {
				f.Close()
				return err
			}
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(reports []runReport) error {
	path := filepath.Join(outDir, "results.json")
	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
