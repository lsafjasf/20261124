package adm

import "fmt"

// speakerLabelToLayout maps common DirectSpeakers labels to layout speakers.
var speakerLabelToLayout = map[string]string{
	"L": "M+030", "R": "M-030", "C": "M+000",
	"Ls": "M+110", "Rs": "M-110",
	"Lss": "M+090", "Rss": "M-090",
	"LFE1": "LFE1", "LFE": "LFE1",
	"Tfl": "U+045", "Tfr": "U-045", "Tbl": "U+135", "Tbr": "U-135",
	"U+045": "U+045", "U-045": "U-045", "U+135": "U+135", "U-135": "U-135",
	"M+030": "M+030", "M-030": "M-030", "M+000": "M+000",
	"M+110": "M+110", "M-110": "M-110",
}

// panDirectSpeakers maps a DirectSpeakers channel onto the layout.
func panDirectSpeakers(pb *PannedBlock, cf *ChannelFormat, layout Layout, ev func(string)) []float64 {
	energy := make([]float64, len(layout.Speakers))
	label := cf.SpeakerLabel
	if label == "" {
		label = cf.Name
	}
	target, ok := speakerLabelToLayout[label]
	if !ok {
		ev(fmt.Sprintf("无法识别的 speakerLabel %q，该声道能量被丢弃", label))
		return energy
	}
	for i, s := range layout.Speakers {
		if s.Name == target {
			energy[i] = pb.BasePower
			return energy
		}
	}
	ev(fmt.Sprintf("布局 %s 中不存在声道 %s（标签 %q），能量被丢弃",
		layout.Name, target, label))
	return energy
}

// downmixLoRo implements stage S4: LoRo fold-down to stereo.
func downmixLoRo(pb *PannedBlock, layout Layout, res *RenderResult) []float64 {
	out := make([]float64, len(layout.Speakers))
	idx := map[string]int{}
	for i, s := range layout.Speakers {
		idx[s.Short] = i
	}
	get := func(short string) float64 {
		if i, ok := idx[short]; ok {
			return pb.Energy[i]
		}
		return 0
	}
	lt := get("L") + 0.5*(get("C")+get("Ls"))
	rt := get("R") + 0.5*(get("C")+get("Rs"))
	if get("LFE") > 0 {
		pb.Events = append(pb.Events, FoldEvent{Stage: StageDownmix, Object: pb.Object,
			Detail: "LoRo 下混丢弃 LFE 声道能量"})
	}
	for i, s := range layout.Speakers {
		switch s.Short {
		case "L":
			out[i] = lt
		case "R":
			out[i] = rt
		}
	}
	return out
}
