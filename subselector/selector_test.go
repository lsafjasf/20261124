package subselector

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func manifest() []Track {
	return []Track{
		{ID: "zh-cn", LanguageTag: "zh-CN", Title: "简体中文", IsDefault: true},
		{ID: "zh-tw", LanguageTag: "zh-TW", Title: "繁體中文"},
		{ID: "en-us", LanguageTag: "en-US", Title: "English (US)"},
		{ID: "en-gb", LanguageTag: "en-GB", Title: "English (UK)"},
		{ID: "en-sdh", LanguageTag: "en", Kind: KindSDH, Title: "English SDH"},
		{ID: "en-forced", LanguageTag: "en", Kind: KindForced, Title: "English forced"},
		{ID: "ja", LanguageTag: "ja", Title: "日本語"},
		{ID: "fr-comment", LanguageTag: "fr", Kind: KindCommentary, Title: "French commentary"},
	}
}

func pick(t *testing.T, tracks []Track, prefs []Preference, off bool) Decision {
	t.Helper()
	sel, err := New(tracks, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return sel.Select(context.Background(), prefs, off)
}

func TestExactEnglishSelection(t *testing.T) {
	d := pick(t, manifest(),
		[]Preference{{LanguageTag: "en-GB"}}, false)
	if d.TrackID != "en-gb" {
		t.Fatalf("got %q, want en-gb; steps=%+v", d.TrackID, d.Steps)
	}
	if d.Off || len(d.Steps) != 1 || d.Steps[0].Confidence != "exact" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestMissingLanguageFallsBackToDefault(t *testing.T) {
	d := pick(t, manifest(),
		[]Preference{{LanguageTag: "de-DE"}}, false)
	if d.TrackID != "zh-cn" {
		t.Fatalf("got %q, want default zh-cn; steps=%+v", d.TrackID, d.Steps)
	}
	stages := []string{d.Steps[0].Stage, d.Steps[1].Stage}
	if stages[0] != "preference" || stages[1] != "default" {
		t.Fatalf("fallback chain = %v, want preference->default", stages)
	}
	if d.Steps[0].Confidence != "none" {
		t.Fatalf("missing language step should be none: %+v", d.Steps[0])
	}
}

func TestMultiplePreferencesChain(t *testing.T) {
	d := pick(t, manifest(),
		[]Preference{
			{LanguageTag: "ko"},
			{LanguageTag: "en-US"},
		}, false)
	if d.TrackID != "en-us" {
		t.Fatalf("got %q, want en-us; steps=%+v", d.TrackID, d.Steps)
	}
	if len(d.Steps) != 2 || d.Steps[0].TrackID != "" {
		t.Fatalf("expected two logged stages, first missing: %+v", d.Steps)
	}
}

func TestSameLanguageMultipleVersions(t *testing.T) {
	tracks := manifest()

	normal := pick(t, tracks,
		[]Preference{{LanguageTag: "en"}}, false)
	if normal.TrackID != "en-us" {
		t.Fatalf("normal preference got %q, want en-us", normal.TrackID)
	}

	sdh := pick(t, tracks,
		[]Preference{{LanguageTag: "en", PreferSDH: true}}, false)
	if sdh.TrackID != "en-sdh" {
		t.Fatalf("SDH preference got %q, want en-sdh", sdh.TrackID)
	}

	forced := pick(t, tracks,
		[]Preference{{LanguageTag: "en", AllowForced: true}}, false)
	if forced.TrackID == "en-forced" {
		t.Fatalf("forced tracks must not outrank full dialogue tracks")
	}

	onlyForced := pick(t, []Track{
		{ID: "f1", LanguageTag: "en", Kind: KindForced},
	}, []Preference{{LanguageTag: "en", AllowForced: true}}, false)
	if onlyForced.TrackID != "f1" {
		t.Fatalf("got %q, want f1 when forced is allowed", onlyForced.TrackID)
	}

	blocked := pick(t, []Track{
		{ID: "f1", LanguageTag: "en", Kind: KindForced},
	}, []Preference{{LanguageTag: "en"}}, false)
	if blocked.TrackID != "" || !blocked.Off {
		t.Fatalf("forced track must be skipped when not allowed: %+v", blocked)
	}
}

func TestRegionalFallback(t *testing.T) {
	d := pick(t, manifest(),
		[]Preference{{LanguageTag: "en-AU"}}, false)
	if d.TrackID != "en-us" && d.TrackID != "en-gb" {
		t.Fatalf("got %q, want an en-* regional track", d.TrackID)
	}

	tw := pick(t, manifest(),
		[]Preference{{LanguageTag: "zh-TW"}}, false)
	if tw.TrackID != "zh-tw" {
		t.Fatalf("got %q, want zh-tw", tw.TrackID)
	}

	// Simplified request must never resolve to Traditional.
	cn := pick(t, manifest(),
		[]Preference{{LanguageTag: "zh-CN"}}, false)
	if cn.TrackID != "zh-cn" {
		t.Fatalf("got %q, want zh-cn", cn.TrackID)
	}

	noSimplified := pick(t, []Track{
		{ID: "zh-tw", LanguageTag: "zh-TW"},
		{ID: "en", LanguageTag: "en"},
	}, []Preference{{LanguageTag: "zh-CN"}, {LanguageTag: "en"}}, false)
	if noSimplified.TrackID != "en" {
		t.Fatalf("simplified must not fall back to traditional: %+v",
			noSimplified)
	}
}

func TestDeterministicAcrossRuns(t *testing.T) {
	tracks := []Track{
		{ID: "a", LanguageTag: "en"},
		{ID: "b", LanguageTag: "en"},
		{ID: "c", LanguageTag: "en"},
	}
	var first string
	for i := 0; i < 20; i++ {
		d := pick(t, tracks,
			[]Preference{{LanguageTag: "en"}}, false)
		if i == 0 {
			first = d.TrackID
		} else if d.TrackID != first {
			t.Fatalf("selection flapped: %q vs %q", first, d.TrackID)
		}
	}
	if first != "a" {
		t.Fatalf("tie should keep manifest order, got %q", first)
	}
}

func TestSubtitlesOffAndEmptyManifest(t *testing.T) {
	d := pick(t, manifest(), nil, true)
	if !d.Off || d.TrackID != "" {
		t.Fatalf("subtitles off request not honored: %+v", d)
	}

	empty := pick(t, nil, []Preference{{LanguageTag: "en"}}, false)
	if !empty.Off || len(empty.Warnings) == 0 {
		t.Fatalf("empty manifest should warn and disable: %+v", empty)
	}

	bad := pick(t, manifest(),
		[]Preference{{LanguageTag: "???"}, {LanguageTag: "ja"}}, false)
	if bad.TrackID != "ja" || len(bad.Warnings) != 1 {
		t.Fatalf("invalid preference should be skipped with warning: %+v", bad)
	}
}

func TestCommentaryNeverAutoSelected(t *testing.T) {
	d := pick(t, []Track{
		{ID: "c1", LanguageTag: "fr", Kind: KindCommentary},
	}, []Preference{{LanguageTag: "fr"}}, false)
	if !d.Off || d.TrackID != "" {
		t.Fatalf("commentary track auto-selected: %+v", d)
	}
}

func TestInvalidTrackTagRejectedAtConstruction(t *testing.T) {
	if _, err := New([]Track{{ID: "x", LanguageTag: "123"}},
		testLogger()); err == nil {
		t.Fatal("expected construction error for invalid track tag")
	}
}
