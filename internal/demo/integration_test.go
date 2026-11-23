package demo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lsafjasf/20261124/internal/master"
)

func TestGeneratedDeliveryPreservesIdentityAndQuality(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "source")
	src, err := GenerateSource(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "delivery")
	result, err := master.Process(master.ProcessOptions{
		Source: filepath.Join(srcDir, "manifest.json"), OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Combined.Frames) != len(src.Frames) {
		t.Fatalf("combined frames %d, want %d", len(result.Combined.Frames), len(src.Frames))
	}
	if len(result.SDR.Frames) != len(src.Frames) || len(result.HDR10.Frames) != len(src.Frames) {
		t.Fatalf("set frame counts SDR=%d HDR=%d", len(result.SDR.Frames), len(result.HDR10.Frames))
	}
	for i, sf := range src.Frames {
		got := result.Combined.Frames[i]
		if got.Shot != sf.Shot || got.Frame != sf.Frame {
			t.Fatalf("frame %d identity %s/%d, want %s/%d", i, got.Shot, got.Frame, sf.Shot, sf.Frame)
		}
		if got.Subtitle != sf.Subtitle {
			t.Fatalf("frame %s/%d subtitle changed: %+v != %+v", sf.Shot, sf.Frame, got.Subtitle, sf.Subtitle)
		}
		if result.SDR.Frames[i].Subtitle != sf.Subtitle || result.HDR10.Frames[i].Subtitle != sf.Subtitle {
			t.Fatalf("frame %s/%d set manifest subtitle changed", sf.Shot, sf.Frame)
		}
		if result.SDR.Frames[i].Shot != sf.Shot || result.HDR10.Frames[i].Shot != sf.Shot {
			t.Fatalf("frame %s/%d set manifest shot changed", sf.Shot, sf.Frame)
		}
		if got.SDR == nil || got.HDR10 == nil {
			t.Fatalf("frame %s/%d missing deliverable", sf.Shot, sf.Frame)
		}
		if got.Source.SHA256 == "" || got.SDR.SHA256 == "" || got.HDR10.SHA256 == "" {
			t.Fatalf("frame %s/%d missing source or master hash", sf.Shot, sf.Frame)
		}
		for _, rel := range []string{got.SDR.File, got.HDR10.File} {
			if _, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(rel))); err != nil {
				t.Fatalf("frame %s/%d output %s: %v", sf.Shot, sf.Frame, rel, err)
			}
		}
	}
	if !result.Report.Summary.LegalRangeOK {
		t.Fatal("legal range check failed")
	}
	if result.Report.Summary.MaxMiddleGrayDeltaNits > 0.1 {
		t.Fatalf("middle gray delta %.4f nit", result.Report.Summary.MaxMiddleGrayDeltaNits)
	}
	if !result.Report.Summary.HDRRetainsHighlightDetail {
		t.Fatal("HDR does not retain more highlight steps than SDR")
	}
	if result.Report.Summary.HDRMaxCLLNits < 700 {
		t.Fatalf("HDR MaxCLL %.2f nit is too low for the 800-nit test", result.Report.Summary.HDRMaxCLLNits)
	}
	if result.HDR10.Profile.MaxCLLNits != result.Report.Summary.HDRMaxCLLNits {
		t.Fatalf("profile MaxCLL %.2f != report %.2f",
			result.HDR10.Profile.MaxCLLNits, result.Report.Summary.HDRMaxCLLNits)
	}

	first := result.Combined.Frames[0]
	sdrPath := filepath.Join(outDir, filepath.FromSlash(first.SDR.File))
	hdrPath := filepath.Join(outDir, filepath.FromSlash(first.HDR10.File))
	sdr, err := master.ReadRGB10LE(sdrPath, first.Width, first.Height)
	if err != nil {
		t.Fatal(err)
	}
	hdr, err := master.ReadRGB10LE(hdrPath, first.Width, first.Height)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []*master.Master{sdr, hdr} {
		for _, code := range m.Codes {
			if code < 64 || code > 940 {
				t.Fatalf("code %d outside legal range", code)
			}
		}
	}
}
