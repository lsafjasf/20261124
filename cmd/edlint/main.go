package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lsafjasf/20261124/edl"
)

type options struct {
	fps       string
	mode      string
	json      bool
	out       string
	exportDir string
	frames    bool
	framesOut string
	track     string
	from      int64
	to        int64
}

func main() {
	var opts options
	flag.StringVar(&opts.fps, "fps", "29.97", "fallback frame rate (29.97 -> 30000/1001)")
	flag.StringVar(&opts.mode, "mode", "auto", "timecode mode for colon labels: auto, df, or ndf")
	flag.BoolVar(&opts.json, "json", false, "write the complete batch timeline as JSON to stdout")
	flag.StringVar(&opts.out, "out", "", "write the complete batch timeline JSON to this file")
	flag.StringVar(&opts.exportDir, "export", "", "write cut-only EDL roundtrip files and a JSON report to this directory")
	flag.BoolVar(&opts.frames, "frames", false, "enumerate frame-accurate source mappings")
	flag.StringVar(&opts.framesOut, "frames-out", "-", "frame mapping output file, or - for stdout")
	flag.StringVar(&opts.track, "track", "", "track for -frames (default: first track per file)")
	flag.Int64Var(&opts.from, "from", 0, "first timeline frame for -frames")
	flag.Int64Var(&opts.to, "to", -1, "exclusive last timeline frame for -frames (-1 means file end)")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() == 0 {
		usage()
		os.Exit(2)
	}
	if err := run(opts, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "edlint:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  edlint [flags] FILE.edl [FILE.edl ...]

flags:
  -fps 29.97        fallback rate; 29.97 is normalized to 30000/1001
  -mode auto        auto, df, or ndf for colon-separated labels
  -json             write the batch timeline JSON to stdout
  -out FILE         write the batch timeline JSON to FILE
  -export DIR       export cut-only EDLs, parse them again, and write report
  -frames           enumerate per-frame source mappings
  -frames-out FILE  frame mapping destination (default "-", stdout)
  -track NAME       track to enumerate (default: first track)
  -from N -to N     optional half-open timeline frame range
`)
}

func run(opts options, paths []string) error {
	rate, err := edl.ParseRate(opts.fps)
	if err != nil {
		return err
	}
	defaultMode, err := parseMode(opts.mode)
	if err != nil {
		return err
	}
	var parsed []*edl.EDLFile
	for _, path := range paths {
		file, err := edl.ParseFile(path, edl.ParseOptions{Rate: rate, DefaultMode: defaultMode})
		if err != nil {
			return err
		}
		parsed = append(parsed, file)
	}
	batch, err := edl.BuildBatch(parsed)
	if err != nil {
		return err
	}

	if opts.out != "" {
		if err := writeJSONFile(opts.out, batch); err != nil {
			return err
		}
	}
	if opts.json {
		if err := edl.WriteBatchJSON(os.Stdout, batch); err != nil {
			return err
		}
	} else if !opts.frames {
		if err := edl.WriteTextReport(os.Stdout, batch); err != nil {
			return err
		}
	}
	if opts.frames {
		if err := dumpFrames(opts, batch); err != nil {
			return err
		}
	}
	if opts.exportDir != "" {
		if err := exportRoundtrip(opts, batch); err != nil {
			return err
		}
	}
	return nil
}

func parseMode(value string) (edl.TimecodeMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "", nil
	case "df", "drop", "drop-frame":
		return edl.DF, nil
	case "ndf", "non-drop", "nondrop":
		return edl.NDF, nil
	default:
		return "", fmt.Errorf("unknown -mode %q", value)
	}
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func dumpFrames(opts options, batch *edl.BatchTimeline) error {
	var out *os.File
	var err error
	closeAfter := false
	if opts.framesOut == "" || opts.framesOut == "-" {
		out = os.Stdout
	} else {
		out, err = os.Create(opts.framesOut)
		if err != nil {
			return err
		}
		closeAfter = true
	}
	if closeAfter {
		defer out.Close()
	}
	w := bufio.NewWriter(out)
	defer w.Flush()
	if _, err := fmt.Fprintln(w, "file\ttrack\ttimelineFrame\tevent\treel\tsourceFrame\tsourceTimecode"); err != nil {
		return err
	}
	for _, file := range batch.Files {
		track := opts.track
		if track == "" {
			if len(file.Tracks) == 0 {
				continue
			}
			track = file.Tracks[0].Name
		}
		mappings, err := file.FrameMappings(track, opts.from, opts.to)
		if err != nil {
			return fmt.Errorf("%s track %s: %w", file.Path, track, err)
		}
		for _, mapping := range mappings {
			if _, err := fmt.Fprintf(
				w,
				"%s\t%s\t%d\t%03d\t%s\t%d\t%s\n",
				file.Path, mapping.Track, mapping.TimelineFrame, mapping.Event,
				mapping.Reel, mapping.SourceFrame, mapping.UnwrappedSourceTC,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

type roundtripReport struct {
	Files []roundtripFile `json:"files"`
	OK    bool            `json:"ok"`
}

type roundtripFile struct {
	Source          string               `json:"source"`
	Exported        string               `json:"exportedEdl"`
	Result          edl.RoundtripResult  `json:"result"`
	LostTransitions []edl.LostTransition `json:"lostTransitions,omitempty"`
}

func exportRoundtrip(opts options, batch *edl.BatchTimeline) error {
	if err := os.MkdirAll(opts.exportDir, 0o755); err != nil {
		return err
	}
	rate, err := edl.ParseRate(opts.fps)
	if err != nil {
		return err
	}
	report := roundtripReport{OK: true}
	for _, file := range batch.Files {
		base := strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path))
		if base == "" {
			base = "timeline"
		}
		path := filepath.Join(opts.exportDir, base+".roundtrip.edl")
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		exportErr := edl.ExportEDL(f, &file)
		closeErr := f.Close()
		if exportErr != nil {
			return exportErr
		}
		if closeErr != nil {
			return closeErr
		}
		reparsed, err := edl.ParseFile(path, edl.ParseOptions{Rate: rate})
		if err != nil {
			return fmt.Errorf("reparse %s: %w", path, err)
		}
		roundtrip, err := edl.BuildTimeline(reparsed)
		if err != nil {
			return fmt.Errorf("rebuild %s: %w", path, err)
		}
		result := edl.CompareCutRoundtrip(&file, roundtrip)
		if !result.OK {
			report.OK = false
		}
		report.Files = append(report.Files, roundtripFile{
			Source:          file.Path,
			Exported:        path,
			Result:          result,
			LostTransitions: file.LostTransitions,
		})
	}
	if err := writeJSONFile(filepath.Join(opts.exportDir, "roundtrip-report.json"), report); err != nil {
		return err
	}
	if !report.OK {
		return errors.New("cut-only roundtrip changed a frame boundary; see roundtrip-report.json")
	}
	fmt.Fprintf(os.Stderr, "roundtrip: %d file(s) verified, %d transition(s) downgraded\n",
		len(report.Files), batch.Summary.UnsupportedTransition)
	return nil
}
