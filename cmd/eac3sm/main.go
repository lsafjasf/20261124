// Command eac3sm replays a synthetic E-AC-3 program-switch scenario and
// prints every state transition and per-frame event, demonstrating that a
// new program is never decoded with the previous program's channel mode.
package main

import (
	"fmt"

	"eac3sm"
)

func intPtr(v int) *int { return &v }

func main() {
	m := eac3sm.NewMachine()
	stereo := eac3sm.Spec{FSCod: 0, Acmod: 2, Compr: intPtr(10)}
	surround := eac3sm.Spec{FSCod: 0, Acmod: 7, LFE: true}
	rate44 := eac3sm.Spec{FSCod: 1, Acmod: 7, LFE: true}
	type step struct {
		label string
		spec  eac3sm.Spec
		hint  eac3sm.Hint
	}
	steps := []step{
		{"program A 2/0 48k", stereo, eac3sm.Hint{}},
		{"program A 2/0 48k", stereo, eac3sm.Hint{}},
		{"program A 2/0 48k", stereo, eac3sm.Hint{}},
		{"boundary -> program B 3/2+LFE", surround, eac3sm.Hint{ProgramBoundary: true}},
		{"program B 3/2+LFE", surround, eac3sm.Hint{}},
		{"program B 3/2+LFE", surround, eac3sm.Hint{}},
		{"corrupt frame", eac3sm.Spec{FSCod: 0, Acmod: 7, LFE: true, Corrupt: true}, eac3sm.Hint{}},
		{"program B 3/2+LFE", surround, eac3sm.Hint{}},
		{"rate change 44.1k", rate44, eac3sm.Hint{}},
		{"rate change 44.1k", rate44, eac3sm.Hint{}},
		{"DRC profile change", eac3sm.Spec{FSCod: 1, Acmod: 7, LFE: true, Compr: intPtr(24)}, eac3sm.Hint{}},
	}

	fmt.Println("== frame events ==")
	for _, s := range steps {
		for _, ev := range m.Feed(eac3sm.SynthFrame(s.spec), s.hint) {
			fmt.Printf("frame %02d  %-26s  -> %-16s %s %s\n",
				ev.Frame, s.label, ev.Type, ev.Config, ev.Detail)
		}
	}

	fmt.Println("\n== state transition records ==")
	for _, tr := range m.Transitions() {
		fmt.Println(tr)
	}
}
