// subdemo runs the selector against a realistic multilingual manifest and
// prints the fallback-chain audit plus renderer hot-swap behavior.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"subselector/player"
	"subselector/subselector"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	tracks := []subselector.Track{
		{ID: "sub-zh-cn", LanguageTag: "zh-CN", Title: "简体中文", IsDefault: true},
		{ID: "sub-zh-tw", LanguageTag: "zh-TW", Title: "繁體中文"},
		{ID: "sub-en-us", LanguageTag: "en-US", Title: "English (US)"},
		{ID: "sub-en-gb", LanguageTag: "en-GB", Title: "English (UK)"},
		{ID: "sub-en-sdh", LanguageTag: "en", Kind: subselector.KindSDH,
			Title: "English SDH"},
		{ID: "sub-ja", LanguageTag: "ja", Title: "日本語"},
	}

	sel, err := subselector.New(tracks, logger)
	if err != nil {
		logger.Error("selector init failed", "error", err)
		os.Exit(1)
	}
	p := player.NewPlayer(sel, logger)
	ctx := context.Background()

	// Simulate playback already in progress.
	p.Play()
	p.Tick(42 * time.Second)

	scenarios := []struct {
		name  string
		prefs []subselector.Preference
		off   bool
	}{
		{"switch to British English",
			[]subselector.Preference{{LanguageTag: "en-GB"}}, false},
		{"request unavailable German, then English",
			[]subselector.Preference{
				{LanguageTag: "de-DE"},
				{LanguageTag: "en"},
			}, false},
		{"request Traditional Chinese (regional)",
			[]subselector.Preference{{LanguageTag: "zh-HK"}}, false},
		{"request Simplified but only Traditional exists at first",
			[]subselector.Preference{{LanguageTag: "zh-CN"}}, false},
		{"turn subtitles off", nil, true},
	}

	for _, sc := range scenarios {
		logger.Info("scenario start", "scenario", sc.name)
		d, err := p.SwitchLanguage(ctx, sc.prefs, sc.off)
		if err != nil {
			logger.Error("switch failed", "error", err)
			os.Exit(1)
		}
		logger.Info("scenario result",
			"scenario", sc.name,
			"track_id", d.TrackID,
			"off", d.Off,
			"reason", d.Reason,
			"audit_stages", len(d.Steps),
			"position_ms", p.Position().Milliseconds(),
			"pipeline_rebuilds", p.PipelineRebuilds(),
			"renderer_swaps", p.RendererSwaps())
	}
}
