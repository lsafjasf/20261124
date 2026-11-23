package main

import (
	"fmt"
	"time"

	"subtitle-decider/subtitle"
)

func main() {
	tracks := []subtitle.Track{
		{
			ID: "forced-en", Kind: subtitle.KindForced,
			Language: "zh-Hans", AudioLanguage: "en",
			Cues: []subtitle.Cue{
				{Start: 0, End: 10 * time.Second, Lines: []string{"【强制】路牌：小心慢行"}, WidthCells: 22},
			},
		},
		{
			ID: "full-en", Kind: subtitle.KindFull, Language: "en",
			Cues: []subtitle.Cue{
				{Start: 0, End: 10 * time.Second, Lines: []string{"watch your step out there"}, WidthCells: 24},
			},
		},
		{
			ID: "sdh-en", Kind: subtitle.KindSDH, Language: "en",
			Cues: []subtitle.Cue{
				{Start: 0, End: 10 * time.Second, Lines: []string{"[metal clanking]", "watch your step out there"}, WidthCells: 24},
			},
		},
	}

	prefs := subtitle.Preferences{
		SubtitlesEnabled:   false,
		SDHMode:            false,
		PreferredLanguages: []string{"zh-Hans", "en"},
	}
	decider := subtitle.NewDecider(tracks, prefs)

	state := subtitle.PlaybackState{
		AudioLanguage: "en",
		At:            2 * time.Second,
		Readable:      subtitle.ReadableSpace{WidthCells: 40, HeightLines: 4},
	}

	printDecision("1. 主字幕关闭，强制叙述接管", decider.Decide(state))

	prefs.SubtitlesEnabled = true
	printDecision("2. 开启主字幕，完整字幕优先于强制轨", decider.SetPreferences(prefs))

	prefs.SDHMode = true
	printDecision("3. 开启 SDH 偏好，SDH 轨优先", decider.SetPreferences(prefs))

	prefs.SDHMode = false
	decider.SetPreferences(prefs)
	printDecision("4. 窗口缩到 12x1：常规轨均放不下时保留最高优先轨并标记溢出",
		decider.Resize(subtitle.ReadableSpace{WidthCells: 12, HeightLines: 1}))

	printDecision("5. 主字幕关闭；音轨仍为英语时强制轨在小窗口照样显示（含溢出标记）",
		func() subtitle.Decision {
			prefs.SubtitlesEnabled = false
			return decider.SetPreferences(prefs)
		}())

	printDecision("6. 音轨切成日语，配套英语的强制轨失配后不再显示",
		decider.ChangeAudio("ja"))
}

func printDecision(label string, d subtitle.Decision) {
	if d.Track == nil {
		fmt.Printf("%s\n  -> 不显示任何字幕（reason=%s）\n\n", label, d.Reason)
		return
	}
	fmt.Printf("%s\n  -> 轨道=%s 类型=%s 适配=%v 超行=%d 原因=%s\n\n",
		label, d.Track.ID, d.Kind, d.Fits, d.Overflow, d.Reason)
}
