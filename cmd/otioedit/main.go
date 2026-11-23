package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	otio "github.com/lsafjasf/20261124/otio"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "inspect":
		err = runInspect(os.Args[2:])
	case "roundtrip":
		err = runRoundTrip(os.Args[2:])
	case "edit":
		err = runEdit(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "otioedit:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  otioedit inspect   -in file [-report file]
  otioedit roundtrip -in file -out file [-report file]
  otioedit edit      -in file -out file [-report file] [edits]

edit flags:
  -trim-clip NAME -trim-start 500ms -trim-end 1s
  -add-track NAME
  -gap-track NAME -gap-name NAME -gap-duration 1s -gap-rate 24
  -marker-track NAME -marker-name NAME -marker-start 1s
    -marker-duration 1s -marker-rate 24 -marker-color RED

Reports name retained unknown fields/objects and missing media. Use -report -
to write the JSON report to stdout.
`)
}

func runInspect(args []string) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	input := flags.String("in", "", "input OTIO JSON file")
	reportPath := flags.String("report", "", "write report JSON to file, or - for stdout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return errors.New("inspect requires -in")
	}
	_, report, err := otio.Load(*input)
	if err != nil {
		return err
	}
	if *reportPath == "" {
		*reportPath = "-"
	}
	return writeReport(*reportPath, report, true)
}

func runRoundTrip(args []string) error {
	flags := flag.NewFlagSet("roundtrip", flag.ContinueOnError)
	input := flags.String("in", "", "input OTIO JSON file")
	output := flags.String("out", "", "output OTIO JSON file")
	reportPath := flags.String("report", "", "write report JSON to file, or - for stdout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return errors.New("roundtrip requires -in and -out")
	}
	timeline, _, err := otio.Load(*input)
	if err != nil {
		return err
	}
	report, err := otio.Save(*output, timeline)
	if err != nil {
		return err
	}
	return writeReport(*reportPath, report, false)
}

func runEdit(args []string) error {
	flags := flag.NewFlagSet("edit", flag.ContinueOnError)
	input := flags.String("in", "", "input OTIO JSON file")
	output := flags.String("out", "", "output OTIO JSON file")
	reportPath := flags.String("report", "", "write report JSON to file, or - for stdout")
	trimClip := flags.String("trim-clip", "", "clip name to trim")
	trimStart := flags.Duration("trim-start", 0, "source in-point adjustment; positive trims inward")
	trimEnd := flags.Duration("trim-end", 0, "source out-point adjustment; positive shortens")
	addTrack := flags.String("add-track", "", "append a new track with this name")
	gapTrack := flags.String("gap-track", "", "track that receives a gap")
	gapName := flags.String("gap-name", "gap", "name of the new gap")
	gapDuration := flags.Duration("gap-duration", 0, "duration of the new gap")
	gapRate := flags.Float64("gap-rate", 24, "frame rate of the new gap")
	markerTrack := flags.String("marker-track", "", "track that receives a marker")
	markerName := flags.String("marker-name", "", "name of the new marker")
	markerStart := flags.Duration("marker-start", 0, "marker start offset")
	markerDuration := flags.Duration("marker-duration", 0, "marker duration")
	markerRate := flags.Float64("marker-rate", 24, "frame rate of the new marker range")
	markerColor := flags.String("marker-color", "RED", "OTIO marker color")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return errors.New("edit requires -in and -out")
	}

	timeline, _, err := otio.Load(*input)
	if err != nil {
		return err
	}
	changed := false
	if *addTrack != "" {
		if err := timeline.AppendTrack(otio.NewTrack(*addTrack)); err != nil {
			return err
		}
		changed = true
	}
	if *gapTrack != "" {
		if *gapDuration <= 0 {
			return errors.New("-gap-duration must be positive")
		}
		track := timeline.TrackByName(*gapTrack)
		if track == nil {
			return fmt.Errorf("track %q not found", *gapTrack)
		}
		duration := otio.NewRationalTime(*gapRate, frames(*gapDuration, *gapRate))
		if err := track.Append(otio.NewGap(*gapName, duration)); err != nil {
			return err
		}
		changed = true
	}
	if *trimClip != "" {
		if *trimStart == 0 && *trimEnd == 0 {
			return errors.New("trim requires -trim-start and/or -trim-end")
		}
		clip := timeline.ClipByName(*trimClip)
		if clip == nil {
			return fmt.Errorf("clip %q not found", *trimClip)
		}
		if *trimStart != 0 {
			if err := clip.TrimSourceStart(*trimStart); err != nil {
				return err
			}
		}
		if *trimEnd != 0 {
			if err := clip.TrimSourceEnd(*trimEnd); err != nil {
				return err
			}
		}
		changed = true
	}
	if *markerTrack != "" || *markerName != "" {
		if *markerTrack == "" || *markerName == "" {
			return errors.New("marker requires both -marker-track and -marker-name")
		}
		if *markerDuration <= 0 {
			return errors.New("-marker-duration must be positive")
		}
		track := timeline.TrackByName(*markerTrack)
		if track == nil {
			return fmt.Errorf("track %q not found", *markerTrack)
		}
		start := otio.NewRationalTime(*markerRate, frames(*markerStart, *markerRate))
		duration := otio.NewRationalTime(*markerRate, frames(*markerDuration, *markerRate))
		track.AddMarker(otio.NewMarker(*markerName, *markerColor, otio.NewTimeRange(start, duration)))
		changed = true
	}
	if !changed {
		return errors.New("edit requires at least one edit flag")
	}

	report, err := otio.Save(*output, timeline)
	if err != nil {
		return err
	}
	return writeReport(*reportPath, report, false)
}

func writeReport(path string, report *otio.Report, forceStdout bool) error {
	if path == "" && !forceStdout {
		fmt.Fprintln(os.Stderr, "otioedit:", report.Summary())
		return nil
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	data = append(data, '\n')
	if path == "" || path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report %s: %w", path, err)
	}
	return nil
}

func frames(duration time.Duration, rate float64) float64 {
	return duration.Seconds() * rate
}
