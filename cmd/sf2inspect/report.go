package main

import (
	"fmt"
	"strings"

	"sf2inspect/internal/sf2"
)

func printText(rep *sf2.Report) {
	fmt.Printf("== Resolved preset -> instrument -> sample (%d zones) ==\n", len(rep.Resolved))
	for _, r := range rep.Resolved {
		fmt.Printf("preset[%d] %-20s bank %3d preset %3d -> inst[%d] %-16s -> sample[%d] %s\n",
			r.PresetIndex, r.PresetName, r.Bank, r.Preset,
			r.InstrumentIndex, r.InstrumentName, r.SampleIndex, r.SampleName)
		fmt.Printf("    key %3d..%3d  vel %3d..%3d  root %s\n",
			r.KeyRange.Low, r.KeyRange.High, r.VelRange.Low, r.VelRange.High,
			rootLabel(r.RootKey))
		fmt.Printf("    loop declared [%d,%d) effective [%d,%d) mode %d\n",
			r.Loop.DeclaredStart, r.Loop.DeclaredEnd,
			r.Loop.EffectiveStart, r.Loop.EffectiveEnd, r.Loop.Mode)
		if len(r.Generators) > 0 {
			fmt.Printf("    gens: %s\n", formatGens(r.Generators))
		}
	}

	fmt.Printf("\n== Diagnostics (%d) ==\n", len(rep.Diagnostics))
	if len(rep.Diagnostics) == 0 {
		fmt.Println("none")
		return
	}
	for _, d := range rep.Diagnostics {
		fmt.Printf("[%s] %-18s %-24s %s\n",
			strings.ToUpper(d.Severity), d.Category, d.Location, d.Message)
	}
}

func rootLabel(key int) string {
	if key == 255 {
		return "unspecified"
	}
	if key < 0 || key > 127 {
		return fmt.Sprintf("ILLEGAL(%d)", key)
	}
	return fmt.Sprintf("%d", key)
}

func formatGens(gens []sf2.GenValue) string {
	parts := make([]string, 0, len(gens))
	for _, g := range gens {
		tag := ""
		if !g.Known {
			tag = " (unknown, raw preserved)"
		}
		parts = append(parts, fmt.Sprintf("%s=%d%s", g.Name, g.Amount, tag))
	}
	return strings.Join(parts, ", ")
}
