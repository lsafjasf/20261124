package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"tagfix"
	"tagfix/internal/sample"
)

type conflictCase struct {
	name        string
	description string
	autoFixable bool
	options     sample.Options
	desired     tagfix.ColorTags
}

type conflictResult struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	ExpectedFix  bool           `json:"expected_auto_fix"`
	Report       *tagfix.Report `json:"report"`
	Error        string         `json:"error,omitempty"`
	File         string         `json:"file"`
	OriginalFile string         `json:"original_file"`
	OriginalSHA  string         `json:"original_sha256"`
	FinalSHA     string         `json:"final_sha256"`
	OriginalKept bool           `json:"original_kept_on_failure"`
}

type selfTestReport struct {
	Cases           []conflictResult `json:"cases"`
	AutoFixed       int              `json:"auto_fixed"`
	CannotAutoFix   int              `json:"cannot_auto_fix"`
	AllPayloadsKept bool             `json:"all_media_payloads_unchanged"`
}

func runSelfTest(args []string) error {
	flags := flag.NewFlagSet("selftest", flag.ContinueOnError)
	dir := flags.String("dir", "", "directory for the four sample MP4 files")
	reportPath := flags.String("report", "", "write the aggregate JSON report here")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		temp, err := os.MkdirTemp("", "tagfix-selftest-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		*dir = temp
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		return err
	}
	report := runConflicts(*dir)
	if err := writeReport(report, *reportPath); err != nil {
		return err
	}
	if report.AutoFixed != 2 || report.CannotAutoFix != 2 || !report.AllPayloadsKept {
		return fmt.Errorf("self-test expectations failed: fixed=%d cannot=%d payload=%v",
			report.AutoFixed, report.CannotAutoFix, report.AllPayloadsKept)
	}
	return nil
}

func runConflicts(dir string) selfTestReport {
	cases := []conflictCase{
		{
			name:        "container-bitstream-mismatch",
			description: "nclx and SPS disagree; both color descriptions already exist",
			autoFixable: true,
			options: sample.Options{
				SPS: sample.SPSOptions{
					HasVUI: true, HasSignal: true, HasDescription: true,
					Primaries: 9, Transfer: 16, Matrix: 9,
				},
				Container: sample.Color{Primaries: 1, Transfer: 1, Matrix: 1},
			},
			desired: tagfix.ColorTags{Primaries: 9, Transfer: 16, Matrix: 9},
		},
		{
			name:        "missing-sps-color-description",
			description: "SPS has video_signal_type but colour_description_present_flag=0",
			autoFixable: true,
			options: sample.Options{
				SPS: sample.SPSOptions{
					HasVUI: true, HasSignal: true, HasDescription: false,
				},
				Container: sample.Color{Primaries: 1, Transfer: 1, Matrix: 1},
			},
			desired: tagfix.ColorTags{Primaries: 12, Transfer: 13, Matrix: 1, FullRange: true},
		},
		{
			name:        "avcc-without-sps",
			description: "container tags exist but avcC has no SPS whose VUI can be synchronized",
			autoFixable: false,
			options: sample.Options{
				NoSPS:     true,
				Container: sample.Color{Primaries: 1, Transfer: 1, Matrix: 1},
			},
			desired: tagfix.ColorTags{Primaries: 9, Transfer: 16, Matrix: 9},
		},
		{
			name:        "pq-with-bt709-primaries",
			description: "requested PQ transfer with BT.709 primaries is contradictory",
			autoFixable: false,
			options: sample.Options{
				SPS: sample.SPSOptions{
					HasVUI: true, HasSignal: true, HasDescription: true,
					Primaries: 1, Transfer: 1, Matrix: 1,
				},
				Container: sample.Color{Primaries: 1, Transfer: 1, Matrix: 1},
			},
			desired: tagfix.ColorTags{Primaries: 1, Transfer: 16, Matrix: 1},
		},
	}
	report := selfTestReport{AllPayloadsKept: true}
	for _, testCase := range cases {
		path := filepath.Join(dir, testCase.name+".mp4")
		originalPath := filepath.Join(dir, testCase.name+".original.mp4")
		input := sample.MP4(testCase.options)
		if err := os.WriteFile(originalPath, input, 0o644); err != nil {
			report.Cases = append(report.Cases, conflictResult{
				Name: testCase.name, OriginalFile: originalPath, Error: err.Error(),
			})
			break
		}
		if err := os.WriteFile(path, input, 0o644); err != nil {
			report.Cases = append(report.Cases, conflictResult{
				Name: testCase.name, File: path, OriginalFile: originalPath, Error: err.Error(),
			})
			break
		}
		applyReport, applyErr := tagfix.ApplyFile(path, testCase.desired)
		final, readErr := os.ReadFile(path)
		result := conflictResult{
			Name:         testCase.name,
			Description:  testCase.description,
			ExpectedFix:  testCase.autoFixable,
			Report:       applyReport,
			File:         path,
			OriginalFile: originalPath,
			OriginalSHA:  sum(input),
			FinalSHA:     sum(final),
			OriginalKept: readErr == nil && bytes.Equal(input, final),
		}
		passed := false
		if applyErr == nil {
			report.AutoFixed++
			passed = validateCaseResult(testCase, applyErr, applyReport, result)
		} else {
			result.Error = applyErr.Error()
			if errors.Is(applyErr, tagfix.ErrCannotAutoFix) ||
				errors.Is(applyErr, tagfix.ErrInvalidTags) {
				report.CannotAutoFix++
			}
			passed = validateCaseResult(testCase, applyErr, applyReport, result)
		}
		if !passed || readErr != nil {
			report.AllPayloadsKept = false
		}
		if applyReport != nil {
			for _, payload := range applyReport.Payload {
				if !payload.Unchanged {
					report.AllPayloadsKept = false
				}
			}
		}
		report.Cases = append(report.Cases, result)
	}
	return report
}

func sum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func validateCaseResult(testCase conflictCase, err error, report *tagfix.Report, result conflictResult) bool {
	if testCase.autoFixable {
		return err == nil && report != nil && report.Status == "applied" && !result.OriginalKept
	}
	return err != nil && report != nil && report.Status == "rejected" && result.OriginalKept
}
