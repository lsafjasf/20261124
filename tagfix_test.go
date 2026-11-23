package tagfix

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSynchronizesContainerAndSPS(t *testing.T) {
	input := makeMP4(t, fixtureOptions{
		sps: spsOptions{
			HasVUI: true, HasSignal: true, HasDescription: true,
			Primaries: 1, Transfer: 1, Matrix: 1, FullRange: false,
		},
		container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
	})
	want := ColorTags{Primaries: 9, Transfer: 16, Matrix: 9, FullRange: false}
	output, report, err := Build(input, want)
	if err != nil {
		t.Fatalf("Build: %v\nreport: %+v", err, report)
	}
	if report.Status != "validated" {
		t.Fatalf("status = %q", report.Status)
	}
	if len(report.Changes) != 8 {
		t.Fatalf("changes = %d, want 8", len(report.Changes))
	}
	for _, change := range report.Changes {
		if !change.ReadbackOK {
			t.Errorf("%s readback %v, want %v", change.Target, change.Readback, change.New)
		}
		if change.ByteOffset < 0 || change.NewByteOffset < 0 {
			t.Errorf("%s missing byte offset: old=%d new=%d",
				change.Target, change.ByteOffset, change.NewByteOffset)
		}
	}
	if err := verifyTagsFromBytes(t, output, want); err != nil {
		t.Fatal(err)
	}
	assertPayloadUnchanged(t, input, output)
}

func TestBuildInsertsMissingSPSColorDescription(t *testing.T) {
	input := makeMP4(t, fixtureOptions{
		sps: spsOptions{
			HasVUI: true, HasSignal: true, HasDescription: false,
			Primaries: 1, Transfer: 1, Matrix: 1,
		},
		container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
	})
	want := ColorTags{Primaries: 12, Transfer: 13, Matrix: 1, FullRange: true}
	output, report, err := Build(input, want)
	if err != nil {
		t.Fatalf("Build: %v\nreport: %+v", err, report)
	}
	if err := verifyTagsFromBytes(t, output, want); err != nil {
		t.Fatal(err)
	}
	assertPayloadUnchanged(t, input, output)
}

func TestBuildInsertsMissingContainerAndVUI(t *testing.T) {
	input := makeMP4(t, fixtureOptions{
		sps:       spsOptions{HasVUI: false},
		noColr:    true,
		container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
	})
	want := ColorTags{Primaries: 9, Transfer: 16, Matrix: 9, FullRange: true}
	output, report, err := Build(input, want)
	if err != nil {
		t.Fatalf("Build: %v\nreport: %+v", err, report)
	}
	if err := verifyTagsFromBytes(t, output, want); err != nil {
		t.Fatal(err)
	}
	assertPayloadUnchanged(t, input, output)
	for _, change := range report.Changes {
		if !change.ReadbackOK {
			t.Errorf("%s readback failed: %v", change.Target, change.Readback)
		}
	}
}

func TestBuildRejectsMissingSPS(t *testing.T) {
	input := makeMP4(t, fixtureOptions{
		noSPS:     true,
		container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
	})
	output, report, err := Build(input,
		ColorTags{Primaries: 9, Transfer: 16, Matrix: 9})
	if !errors.Is(err, ErrCannotAutoFix) {
		t.Fatalf("error = %v, want ErrCannotAutoFix", err)
	}
	if output != nil {
		t.Fatal("rejected build returned output")
	}
	if report.Status != "rejected" || !report.Rollback.Verified {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestBuildRejectsIllegalCombination(t *testing.T) {
	input := makeMP4(t, fixtureOptions{
		sps: spsOptions{
			HasVUI: true, HasSignal: true, HasDescription: true,
			Primaries: 1, Transfer: 1, Matrix: 1,
		},
		container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
	})
	output, report, err := Build(input,
		ColorTags{Primaries: 1, Transfer: 16, Matrix: 1})
	if !errors.Is(err, ErrInvalidTags) {
		t.Fatalf("error = %v, want ErrInvalidTags", err)
	}
	if output != nil || report.Status != "rejected" {
		t.Fatalf("unexpected result: output=%v report=%+v", output, report)
	}
	for _, change := range report.Changes {
		if change.Changed && change.ReadbackOK {
			t.Errorf("rejected change %s unexpectedly marked readback OK", change.Target)
		}
	}
}

func TestApplyAndInspect(t *testing.T) {
	input := makeMP4(t, fixtureOptions{
		sps: spsOptions{
			HasVUI: true, HasSignal: true, HasDescription: true,
			Primaries: 1, Transfer: 1, Matrix: 1,
		},
		container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
	})
	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	want := ColorTags{Primaries: 9, Transfer: 16, Matrix: 9}
	report, err := ApplyFile(path, want)
	if err != nil {
		t.Fatalf("ApplyFile: %v\nreport: %+v", err, report)
	}
	if report.Status != "applied" || !report.Rollback.Verified {
		t.Fatalf("unexpected report: %+v", report)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyTagsFromBytes(t, got, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
	inspection, err := InspectFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Container.Tags == nil || *inspection.Container.Tags != want {
		t.Fatalf("container inspection = %+v", inspection.Container)
	}
}

func TestApplyWriteFailureLeavesOriginal(t *testing.T) {
	input := makeMP4(t, fixtureOptions{
		sps: spsOptions{
			HasVUI: true, HasSignal: true, HasDescription: true,
			Primaries: 1, Transfer: 1, Matrix: 1,
		},
		container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.mp4")
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	report, err := ApplyFile(path,
		ColorTags{Primaries: 9, Transfer: 16, Matrix: 9})
	if !errors.Is(err, ErrWriteFailed) {
		t.Fatalf("error = %v, want ErrWriteFailed", err)
	}
	if report.Status != "rolled_back" || !report.Rollback.Verified {
		t.Fatalf("unexpected report: %+v", report)
	}
	assertFileEquals(t, path, input)
}

func TestPostReplaceReadbackFailureRestoresOriginal(t *testing.T) {
	input := makeMP4(t, fixtureOptions{
		sps: spsOptions{
			HasVUI: true, HasSignal: true, HasDescription: true,
			Primaries: 1, Transfer: 1, Matrix: 1,
		},
		container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
	})
	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatal(err)
	}
	afterReplaceHook = func() error { return errors.New("injected readback failure") }
	defer func() { afterReplaceHook = nil }()
	want := ColorTags{Primaries: 9, Transfer: 16, Matrix: 9}
	report, err := ApplyFile(path, want)
	if err == nil {
		t.Fatal("ApplyFile unexpectedly succeeded")
	}
	if report.Status != "rolled_back" || !report.Rollback.Performed || !report.Rollback.Verified {
		t.Fatalf("unexpected report: %+v", report)
	}
	assertFileEquals(t, path, input)
	for _, change := range report.Changes {
		if change.Changed && change.ReadbackOK {
			t.Errorf("%s readback marked OK after rollback: %v", change.Target, change.Readback)
		}
		if change.Changed && !valuesEqual(change.Readback, change.Old) {
			t.Errorf("%s rollback readback = %v, want old %v",
				change.Target, change.Readback, change.Old)
		}
	}
}

func TestFourConflictSamples(t *testing.T) {
	cases := []struct {
		name      string
		options   fixtureOptions
		desired   ColorTags
		autoFixed bool
	}{
		{
			name: "container and SPS disagree",
			options: fixtureOptions{
				sps: spsOptions{
					HasVUI: true, HasSignal: true, HasDescription: true,
					Primaries: 9, Transfer: 16, Matrix: 9,
				},
				container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
			},
			desired:   ColorTags{Primaries: 9, Transfer: 16, Matrix: 9},
			autoFixed: true,
		},
		{
			name: "SPS color description absent",
			options: fixtureOptions{
				sps: spsOptions{
					HasVUI: true, HasSignal: true, HasDescription: false,
				},
				container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
			},
			desired:   ColorTags{Primaries: 12, Transfer: 13, Matrix: 1, FullRange: true},
			autoFixed: true,
		},
		{
			name: "avcC has no SPS",
			options: fixtureOptions{
				noSPS:     true,
				container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
			},
			desired: ColorTags{Primaries: 9, Transfer: 16, Matrix: 9},
		},
		{
			name: "PQ combined with BT.709 primaries",
			options: fixtureOptions{
				sps: spsOptions{
					HasVUI: true, HasSignal: true, HasDescription: true,
					Primaries: 1, Transfer: 1, Matrix: 1,
				},
				container: ColorTags{Primaries: 1, Transfer: 1, Matrix: 1},
			},
			desired: ColorTags{Primaries: 1, Transfer: 16, Matrix: 1},
		},
	}

	fixed := 0
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input := makeMP4(t, testCase.options)
			path := filepath.Join(t.TempDir(), "conflict.mp4")
			if err := os.WriteFile(path, input, 0o644); err != nil {
				t.Fatal(err)
			}
			report, err := ApplyFile(path, testCase.desired)
			if testCase.autoFixed {
				fixed++
				if err != nil {
					t.Fatalf("ApplyFile: %v\nreport: %+v", err, report)
				}
				if report.Status != "applied" {
					t.Fatalf("status = %s", report.Status)
				}
				got, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if err := verifyTagsFromBytes(t, got, testCase.desired); err != nil {
					t.Fatal(err)
				}
				assertPayloadUnchanged(t, input, got)
				return
			}
			if err == nil {
				t.Fatal("expected an unrepairable conflict")
			}
			if report.Status != "rejected" || !report.Rollback.Verified {
				t.Fatalf("unexpected report: %+v", report)
			}
			assertFileEquals(t, path, input)
		})
	}
	if fixed != 2 || fixed == len(cases) {
		t.Fatalf("auto-fixed %d of %d cases; need at least one unrepairable", fixed, len(cases))
	}
}

func verifyTagsFromBytes(t *testing.T, data []byte, want ColorTags) error {
	t.Helper()
	inspection, err := Inspect(data)
	if err != nil {
		return err
	}
	if inspection.Container.Tags == nil || *inspection.Container.Tags != want {
		t.Fatalf("container tags = %+v, want %+v", inspection.Container.Tags, want)
	}
	for _, source := range inspection.Bitstream {
		if source.Tags == nil || *source.Tags != want {
			t.Fatalf("%s tags = %+v, want %+v", source.Source, source.Tags, want)
		}
	}
	return nil
}

func assertPayloadUnchanged(t *testing.T, before, after []byte) {
	t.Helper()
	beforeHash := mediaHash(t, before)
	afterHash := mediaHash(t, after)
	if beforeHash != afterHash {
		t.Fatalf("mdat payload changed: %x != %x", beforeHash, afterHash)
	}
}

func mediaHash(t *testing.T, data []byte) [32]byte {
	t.Helper()
	top, err := parseTopLevel(data)
	if err != nil {
		t.Fatal(err)
	}
	var payload []byte
	for _, box := range top {
		if box.typ == "mdat" {
			payload = append(payload, data[box.payload:box.end]...)
		}
	}
	return sha256.Sum256(payload)
}

func assertFileEquals(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file changed: got %d bytes, want %d", len(got), len(want))
	}
}
