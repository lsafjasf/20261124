package vobsub

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeRunForms(t *testing.T) {
	tests := []struct {
		name    string
		bytes   []byte
		wantRun int
		wantCol uint8
	}{
		{name: "4-bit", bytes: []byte{0x50}, wantRun: 1, wantCol: 1},
		{name: "8-bit", bytes: []byte{0x13}, wantRun: 4, wantCol: 3},
		{name: "12-bit", bytes: []byte{0x04, 0x10}, wantRun: 16, wantCol: 1},
		{name: "fill-rest", bytes: []byte{0x00, 0x02}, wantRun: fillRest, wantCol: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			br := nibbleReader{data: tc.bytes}
			run, col, err := decodeRun(&br)
			if err != nil {
				t.Fatal(err)
			}
			if run != tc.wantRun || col != tc.wantCol {
				t.Fatalf("got run=%d colour=%d, want %d/%d", run, col, tc.wantRun, tc.wantCol)
			}
		})
	}
}

func TestFillRestRow(t *testing.T) {
	r := &Raster{Width: 8, Height: 1, Pixels: make([]uint8, 8)}
	// Four leading zero nibbles select the fill-rest form; the final
	// low nibble is the colour index.
	if err := decodeField([]byte{0x00, 0x02}, r, 0); err != nil {
		t.Fatal(err)
	}
	for i, pixel := range r.Pixels {
		if pixel != 2 {
			t.Fatalf("pixel %d=%d, want fill colour 2", i, pixel)
		}
	}
}

func TestParseASSColourIsBGR(t *testing.T) {
	if got := parsePrimaryColor(`{\c&H0000FF&}`); got != "#FF0000" {
		t.Fatalf("got %s, want #FF0000", got)
	}
}

func TestWriteASSPreservesNonWhitePrimary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "colour.ass")
	cue := &Cue{
		Index: 1, StartMS: 1000, EndMS: 2000, Migratable: true,
		State:  State{Rect: Rect{X: 12, Y: 34, Width: 20, Height: 10}},
		Colors: []VisibleColour{{Index: 1, RGB: "#123456", Alpha: 255, Area: 10, Dominant: true}},
	}
	if _, err := WriteASS(path, image.Pt(720, 480), []*Cue{cue}, ASSOptions{}); err != nil {
		t.Fatal(err)
	}
	ass, err := ParseASS(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(ass.Events) != 1 {
		t.Fatalf("got %d events", len(ass.Events))
	}
	if ass.Events[0].Color != "#123456" {
		t.Fatalf("colour round trip got %s", ass.Events[0].Color)
	}
}

func TestParseASSTime(t *testing.T) {
	ms, err := parseASSTime("1:02:03.45")
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(3_723_450); ms != want {
		t.Fatalf("got %d, want %d", ms, want)
	}
	if _, err := parseASSTime("0:00:99.00"); err == nil {
		t.Fatal("invalid seconds accepted")
	}
}

func TestDemoSPUDecode(t *testing.T) {
	bitmap := makeDemoBitmap(64, 16, true)
	rect := Rect{X: 100, Y: 390, Width: 64, Height: 16}
	raw, err := buildDemoSPU(bitmap, rect, 1500, false, false)
	if err != nil {
		t.Fatal(err)
	}
	spu, err := DecodeSPU(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !spu.HasStart || !spu.HasStop || spu.Animation {
		t.Fatalf("unexpected flags: start=%v stop=%v animation=%v", spu.HasStart, spu.HasStop, spu.Animation)
	}
	if spu.MainState.Rect != rect {
		t.Fatalf("rect=%+v, want %+v", spu.MainState.Rect, rect)
	}
	raster, err := DecodeRaster(spu.Raw, spu.MainState)
	if err != nil {
		t.Fatal(err)
	}
	if len(raster.Pixels) != len(bitmap) {
		t.Fatalf("raster has %d pixels, want %d", len(raster.Pixels), len(bitmap))
	}
	for i := range bitmap {
		if raster.Pixels[i] != bitmap[i]&3 {
			t.Fatalf("pixel %d=%d, want %d", i, raster.Pixels[i], bitmap[i]&3)
		}
	}
	if !raster.Used[2] {
		t.Fatal("outline colour index 2 was not decoded")
	}
}

func TestDemoSPUAnimationAndMissingStop(t *testing.T) {
	bitmap := makeDemoBitmap(56, 16, false)
	rect := Rect{X: 430, Y: 370, Width: 56, Height: 16}
	raw, err := buildDemoSPU(bitmap, rect, 2000, true, false)
	if err != nil {
		t.Fatal(err)
	}
	spu, err := DecodeSPU(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !spu.Animation {
		t.Fatal("animated SPU not detected")
	}
	if !spu.HasStop {
		t.Fatal("animated SPU lost stop command")
	}

	raw, err = buildDemoSPU(bitmap, rect, 2000, false, true)
	if err != nil {
		t.Fatal(err)
	}
	spu, err = DecodeSPU(raw)
	if err != nil {
		t.Fatal(err)
	}
	if spu.HasStop {
		t.Fatal("missing stop command was invented")
	}
}

func TestConvertDemoEndToEnd(t *testing.T) {
	root := t.TempDir()
	idxPath, subPath, err := WriteDemoPairs(filepath.Join(root, "source"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	result, err := Convert(ConvertOptions{
		IndexPath: idxPath, SubPath: subPath, OutDir: out, Rounds: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := result.Review.Summary
	if s.Total != 4 || s.Migrated != 2 || s.NotMigrated != 2 || s.Animation != 1 || s.Outline < 2 {
		t.Fatalf("unexpected summary: %+v", s)
	}
	if result.Verification == nil || !result.Verification.OK {
		t.Fatalf("verification failed: %+v", result.Verification)
	}
	if result.Verification.Rounds != 3 || result.Verification.Checks != 48 {
		t.Fatalf("verification rounds/checks: %+v", result.Verification)
	}
	ass, err := ParseASS(filepath.Join(out, "subtitles.ass"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ass.Events) != 2 {
		t.Fatalf("ASS has %d events, want 2", len(ass.Events))
	}
	if ass.Events[0].X != 100 || ass.Events[0].Y != 390 || ass.Events[0].Color != "#FFFFFF" {
		t.Fatalf("unexpected first event: %+v", ass.Events[0])
	}

	reviewData, err := os.ReadFile(filepath.Join(out, "review.json"))
	if err != nil {
		t.Fatal(err)
	}
	var review Review
	if err := json.Unmarshal(reviewData, &review); err != nil {
		t.Fatal(err)
	}
	for _, cue := range review.Cues {
		if cue.Original == "" {
			t.Fatalf("cue %04d has no original image", cue.Index)
		}
		info, err := os.Stat(filepath.Join(out, filepath.FromSlash(cue.Original)))
		if err != nil || info.Size() == 0 {
			t.Fatalf("cue %04d original image: %v", cue.Index, err)
		}
		if cue.RawSPU == "" {
			t.Fatalf("cue %04d has no raw SPU", cue.Index)
		}
	}
	if len(review.Cues[2].States) != 4 {
		t.Fatalf("animated cue has %d state images, want 4", len(review.Cues[2].States))
	}
	for _, item := range review.Cues[:2] {
		if item.Verification == nil || !item.Verification.PixelMatch {
			t.Fatalf("cue %04d composite verification missing", item.Index)
		}
		for _, path := range []string{item.Verification.InImage, item.Verification.LastImage, item.Verification.OutImage} {
			if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(path))); err != nil {
				t.Fatalf("missing composite %s: %v", path, err)
			}
		}
	}
}

func TestVerificationRejectsTamperedOutPoint(t *testing.T) {
	root := t.TempDir()
	idxPath, subPath, err := WriteDemoPairs(filepath.Join(root, "source"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	result, err := Convert(ConvertOptions{
		IndexPath: idxPath, SubPath: subPath, OutDir: out, Rounds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	assPath := filepath.Join(out, "subtitles.ass")
	data, err := os.ReadFile(assPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.ReplaceAll(string(data), "0:00:02.50", "0:00:02.60")
	if tampered == string(data) {
		t.Fatal("test did not find out point to tamper")
	}
	if err := os.WriteFile(assPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	verification, err := VerifyASS(out, result.Index.Frame, result.Cues, 1)
	if err != nil {
		t.Fatal(err)
	}
	if verification.OK {
		t.Fatal("tampered out point passed verification")
	}
}

func TestParseIndexRejectsMissingPalette(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.idx")
	text := "size: 720x480\ntimestamp: 00:00:00:000, filepos: 000000000\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseIndex(path); err == nil {
		t.Fatal("missing palette was accepted")
	}
}

func TestParseMSTimecode(t *testing.T) {
	ms, err := ParseMS("01:02:03:456")
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(3_723_456); ms != want {
		t.Fatalf("got %d, want %d", ms, want)
	}
}
