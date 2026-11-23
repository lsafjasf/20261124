package subtitle

import (
	"testing"
	"time"
)

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

func testTracks() []Track {
	return []Track{
		{
			ID: "f-en", Kind: KindForced, Language: "en", AudioLanguage: "en",
			Cues: []Cue{{Start: 0, End: sec(10), Lines: []string{"FORCED: sign text"}, WidthCells: 18}},
		},
		{
			ID: "f-zh", Kind: KindForced, Language: "zh-Hans", AudioLanguage: "en",
			Cues: []Cue{{Start: 0, End: sec(10), Lines: []string{"强制：标牌文字"}, WidthCells: 16}},
		},
		{
			ID: "full-en", Kind: KindFull, Language: "en",
			Cues: []Cue{{Start: 0, End: sec(10), Lines: []string{"hello there"}, WidthCells: 11}},
		},
		{
			ID: "sdh-en", Kind: KindSDH, Language: "en",
			Cues: []Cue{{Start: 0, End: sec(10), Lines: []string{"[door slams]", "hello there"}, WidthCells: 12}},
		},
		{
			ID: "full-zh", Kind: KindFull, Language: "zh-Hans",
			Cues: []Cue{{Start: 0, End: sec(10), Lines: []string{"你好"}, WidthCells: 4}},
		},
	}
}

func prefs(enabled bool, sdh bool, langs ...string) Preferences {
	return Preferences{
		SubtitlesEnabled:   enabled,
		SDHMode:            sdh,
		PreferredLanguages: langs,
	}
}

func state() PlaybackState {
	return PlaybackState{
		AudioLanguage: "en",
		At:            sec(2),
		Readable:      ReadableSpace{WidthCells: 40, HeightLines: 4},
	}
}

func TestDecideRegularSDHPreferred(t *testing.T) {
	decision := Decide(testTracks(), prefs(true, true, "en"), state())
	if decision.Track == nil || decision.Track.ID != "sdh-en" {
		t.Fatalf("expected sdh-en, got %v", decision.Track)
	}
	if decision.Reason != ReasonSDHPreferred {
		t.Fatalf("expected sdh reason, got %q", decision.Reason)
	}
}

func TestDecideRegularFullWhenSDHOff(t *testing.T) {
	decision := Decide(testTracks(), prefs(true, false, "en"), state())
	if decision.Track == nil || decision.Track.ID != "full-en" {
		t.Fatalf("expected full-en, got %v", decision.Track)
	}
}

func TestDecidePreferredLanguageRanking(t *testing.T) {
	decision := Decide(testTracks(), prefs(true, false, "zh-Hans", "en"), state())
	if decision.Track == nil || decision.Track.ID != "full-zh" {
		t.Fatalf("expected full-zh, got %v", decision.Track)
	}
	if decision.Reason != ReasonPreferredLanguage {
		t.Fatalf("expected preferred language reason, got %q", decision.Reason)
	}
}

func TestDecideLanguageFallback(t *testing.T) {
	decision := Decide(testTracks(), prefs(true, false, "ja"), state())
	if decision.Track == nil || decision.Track.ID != "full-en" {
		t.Fatalf("expected fallback full-en, got %v", decision.Track)
	}
	if decision.Reason != ReasonLanguageFallback {
		t.Fatalf("expected fallback reason, got %q", decision.Reason)
	}
}

func TestForcedWinsWhenSubtitlesDisabled(t *testing.T) {
	decision := Decide(testTracks(), prefs(false, false, "zh-Hans"), state())
	if decision.Track == nil || decision.Track.ID != "f-zh" {
		t.Fatalf("expected forced zh track, got %v", decision.Track)
	}
	if decision.Kind != KindForced || decision.Reason != ReasonForcedOnly {
		t.Fatalf("expected forced only, got kind=%q reason=%q", decision.Kind, decision.Reason)
	}
}

func TestForcedWinsWhenNoRegularCue(t *testing.T) {
	tracks := testTracks()
	regular := []string{"full-en", "sdh-en", "full-zh"}
	for i := range tracks {
		for _, id := range regular {
			if tracks[i].ID == id {
				tracks[i].Cues = []Cue{{Start: sec(30), End: sec(40), Lines: []string{"later"}}}
			}
		}
	}
	decision := Decide(tracks, prefs(true, false, "en"), state())
	if decision.Track == nil || decision.Track.ID != "f-en" {
		t.Fatalf("expected forced f-en, got %v", decision.Track)
	}
	if decision.Reason != ReasonForcedOverride {
		t.Fatalf("expected forced override, got %q", decision.Reason)
	}
}

func TestForcedRequiresAudioLanguageMatch(t *testing.T) {
	playback := state()
	playback.AudioLanguage = "ja"
	decision := Decide(testTracks(), prefs(false, false), playback)
	if decision.Track != nil {
		t.Fatalf("expected no track for mismatched audio, got %q", decision.Track.ID)
	}
}

func TestNoCueMeansNone(t *testing.T) {
	playback := state()
	playback.At = sec(20)
	decision := Decide(testTracks(), prefs(true, true, "en"), playback)
	if decision.Track != nil || decision.Reason != ReasonNone {
		t.Fatalf("expected none decision, got %+v", decision)
	}
}

func TestRegularTrackSkippedWhenUnreadable(t *testing.T) {
	tracks := testTracks()
	for i := range tracks {
		if tracks[i].ID == "full-en" || tracks[i].ID == "sdh-en" {
			tracks[i].Cues[0].Lines = []string{"a reasonably long english sentence"}
			tracks[i].Cues[0].WidthCells = 36
		}
	}
	playback := state()
	playback.Readable = ReadableSpace{WidthCells: 8, HeightLines: 1}
	decision := Decide(tracks, prefs(true, false, "en"), playback)
	if decision.Track == nil || decision.Track.ID != "full-zh" {
		t.Fatalf("expected compact full-zh fallback, got %v", decision.Track)
	}
	if !decision.Fits {
		t.Fatalf("expected selected cue to fit, got fits=%v", decision.Fits)
	}
}

func TestForcedShownEvenWhenOverflowing(t *testing.T) {
	tracks := testTracks()
	for i := range tracks {
		if tracks[i].ID == "f-en" {
			tracks[i].Cues[0].Lines = []string{"line one", "line two", "line three"}
		}
	}
	playback := state()
	playback.Readable = ReadableSpace{WidthCells: 40, HeightLines: 1}
	decision := Decide(tracks, prefs(false, false), playback)
	if decision.Track == nil || decision.Track.ID != "f-en" {
		t.Fatalf("expected forced track despite overflow, got %v", decision.Track)
	}
	if decision.Fits || decision.Overflow != 2 {
		t.Fatalf("expected overflow=2 and fits=false, got fits=%v overflow=%d", decision.Fits, decision.Overflow)
	}
}

func TestAllRegularsUnreadableKeepsTopCandidate(t *testing.T) {
	tracks := testTracks()
	for i := range tracks {
		if tracks[i].Kind == KindFull || tracks[i].Kind == KindSDH {
			tracks[i].Cues[0].Lines = []string{"a very long line that cannot fit"}
			tracks[i].Cues[0].WidthCells = 34
		}
	}
	playback := state()
	playback.Readable = ReadableSpace{WidthCells: 8, HeightLines: 1}
	decision := Decide(tracks, prefs(true, false, "en"), playback)
	if decision.Track == nil || decision.Track.ID != "full-en" {
		t.Fatalf("expected top-priority full-en retained, got %v", decision.Track)
	}
	if decision.Fits || decision.Overflow == 0 {
		t.Fatalf("expected overflow marker, got fits=%v overflow=%d", decision.Fits, decision.Overflow)
	}
	if decision.Kind == KindForced {
		t.Fatalf("forced track must not take over while regular cues are active")
	}
}

func TestRecomputeOnAudioAndResize(t *testing.T) {
	decider := NewDecider(testTracks(), prefs(false, false, "zh-Hans"))
	first := decider.Decide(state())
	if first.Track == nil || first.Track.ID != "f-zh" {
		t.Fatalf("expected f-zh, got %v", first.Track)
	}

	switched := decider.ChangeAudio("ja")
	if switched.Track != nil {
		t.Fatalf("expected no forced track after audio switch, got %q", switched.Track.ID)
	}

	withSubs := decider.SetPreferences(prefs(true, false, "en"))
	if withSubs.Track == nil || withSubs.Track.ID != "full-en" {
		t.Fatalf("expected full-en after enabling subtitles, got %v", withSubs.Track)
	}

	resized := decider.Resize(ReadableSpace{WidthCells: 6, HeightLines: 1})
	if resized.Track == nil || resized.Track.ID != "full-zh" {
		t.Fatalf("expected compact full-zh after resize, got %v", resized.Track)
	}
	if !resized.Fits {
		t.Fatalf("expected compact track to fit, got %+v", resized)
	}
}
