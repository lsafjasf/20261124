package player

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"subselector/subselector"
)

func testPlayer(t *testing.T) *Player {
	t.Helper()
	tracks := []subselector.Track{
		{ID: "zh-cn", LanguageTag: "zh-CN", IsDefault: true},
		{ID: "zh-tw", LanguageTag: "zh-TW"},
		{ID: "en-us", LanguageTag: "en-US"},
		{ID: "en-gb", LanguageTag: "en-GB"},
	}
	sel, err := subselector.New(tracks,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	return NewPlayer(sel, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestSwitchPreservesPositionAndPipeline(t *testing.T) {
	p := testPlayer(t)
	ctx := context.Background()

	first, err := p.SwitchLanguage(ctx,
		[]subselector.Preference{{LanguageTag: "zh-CN"}}, false)
	if err != nil || first.TrackID != "zh-cn" {
		t.Fatalf("initial switch: %+v err=%v", first, err)
	}

	p.Play()
	p.Tick(10 * time.Second)
	positionBefore := p.Position()

	second, err := p.SwitchLanguage(ctx,
		[]subselector.Preference{{LanguageTag: "en-GB"}}, false)
	if err != nil || second.TrackID != "en-gb" {
		t.Fatalf("english switch: %+v err=%v", second, err)
	}

	if p.Position() != positionBefore {
		t.Fatalf("position changed: before=%v after=%v",
			positionBefore, p.Position())
	}
	if !p.IsPlaying() {
		t.Fatal("playback stopped during subtitle switch")
	}
	if p.PipelineRebuilds() != 0 {
		t.Fatalf("video pipeline rebuilt %d times", p.PipelineRebuilds())
	}
	if p.RendererSwaps() != 2 {
		t.Fatalf("renderer swaps=%d, want 2", p.RendererSwaps())
	}
	if p.ActiveTrackID() != "en-gb" {
		t.Fatalf("active track=%q, want en-gb", p.ActiveTrackID())
	}
}

func TestRepeatedSameDecisionIsNoOp(t *testing.T) {
	p := testPlayer(t)
	ctx := context.Background()
	prefs := []subselector.Preference{{LanguageTag: "en-US"}}

	if _, err := p.SwitchLanguage(ctx, prefs, false); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SwitchLanguage(ctx, prefs, false); err != nil {
		t.Fatal(err)
	}
	if p.RendererSwaps() != 1 {
		t.Fatalf("re-applying same track must not swap: swaps=%d",
			p.RendererSwaps())
	}
}

func TestCuesSurviveThroughHotSwap(t *testing.T) {
	p := testPlayer(t)
	ctx := context.Background()

	if _, err := p.SwitchLanguage(ctx,
		[]subselector.Preference{{LanguageTag: "zh-CN"}}, false); err != nil {
		t.Fatal(err)
	}
	cues := []Cue{
		{5 * time.Second, 8 * time.Second, "你好"},
	}
	if err := p.LoadCues("zh-cn", cues); err != nil {
		t.Fatal(err)
	}

	p.Play()
	p.Tick(6 * time.Second)
	if got := p.CueAt(); got != "你好" {
		t.Fatalf("cue=%q, want 你好", got)
	}

	if _, err := p.SwitchLanguage(ctx,
		[]subselector.Preference{{LanguageTag: "en-US"}}, false); err != nil {
		t.Fatal(err)
	}
	if p.Position() != 6*time.Second {
		t.Fatalf("position moved after switch: %v", p.Position())
	}
	if err := p.LoadCues("en-us", []Cue{
		{5 * time.Second, 8 * time.Second, "Hello"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := p.CueAt(); got != "Hello" {
		t.Fatalf("cue=%q, want Hello", got)
	}
}

func TestSwitchingOffKeepsPipelineIntact(t *testing.T) {
	p := testPlayer(t)
	ctx := context.Background()
	if _, err := p.SwitchLanguage(ctx,
		[]subselector.Preference{{LanguageTag: "en"}}, false); err != nil {
		t.Fatal(err)
	}
	p.Play()
	p.Tick(3 * time.Second)

	d, err := p.SwitchLanguage(ctx, nil, true)
	if err != nil || !d.Off {
		t.Fatalf("off decision: %+v err=%v", d, err)
	}
	if p.ActiveTrackID() != "" || p.CueAt() != "" {
		t.Fatal("subtitles should render nothing when off")
	}
	if p.Position() != 3*time.Second || !p.IsPlaying() {
		t.Fatal("off switch disturbed playback")
	}
	if p.PipelineRebuilds() != 0 {
		t.Fatal("turning subtitles off rebuilt the video pipeline")
	}
}

func TestLoadCuesRejectsInactiveTrack(t *testing.T) {
	p := testPlayer(t)
	ctx := context.Background()
	if _, err := p.SwitchLanguage(ctx,
		[]subselector.Preference{{LanguageTag: "zh-CN"}}, false); err != nil {
		t.Fatal(err)
	}
	if err := p.LoadCues("en-us", nil); err == nil {
		t.Fatal("expected error loading cues into inactive track")
	}
}
