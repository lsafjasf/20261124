package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"subtime"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "map":
		if err := runMap(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "srt":
		if err := runSRT(os.Args[2:]); err != nil {
			fatal(err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  subtime map -from RATE -to RATE [-via RATE,...] (-frame N | -tc TIMECODE) [-round-trips N] [-json]
  subtime srt -from RATE -to RATE [-via RATE,...] -in FILE -out FILE -start POLICY -end POLICY [-ensure-one-frame] [-ms POLICY] [-report FILE]`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "subtime:", err)
	os.Exit(1)
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	return flags
}

func parseRates(text string) ([]subtime.Rate, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	parts := strings.Split(text, ",")
	rates := make([]subtime.Rate, 0, len(parts))
	for _, part := range parts {
		rate, err := subtime.ParseRate(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		rates = append(rates, rate)
	}
	return rates, nil
}

type frameReport struct {
	Frame    int64  `json:"frame"`
	Rate     string `json:"rate"`
	Timecode string `json:"timecode"`
	Seconds  string `json:"seconds"`
}

type targetReport struct {
	Rate         string                  `json:"rate"`
	ExactFrames  string                  `json:"exact_frames"`
	ExactSeconds string                  `json:"exact_seconds"`
	OnFrame      bool                    `json:"on_frame"`
	Frame        *int64                  `json:"frame,omitempty"`
	Timecode     string                  `json:"timecode,omitempty"`
	Candidates   []subtime.SnapCandidate `json:"candidates"`
}

type mapReport struct {
	Source     frameReport  `json:"source"`
	Via        []string     `json:"via,omitempty"`
	Target     targetReport `json:"target"`
	RoundTrips int          `json:"round_trips"`
	RoundTrip  frameReport  `json:"round_trip"`
}

func makeFrameReport(frame subtime.Frame) (frameReport, error) {
	timecode, err := frame.Timecode()
	if err != nil {
		return frameReport{}, err
	}
	seconds, err := frame.Seconds()
	if err != nil {
		return frameReport{}, err
	}
	return frameReport{
		Frame: frame.Number, Rate: frame.Rate.String(), Timecode: timecode, Seconds: seconds.String(),
	}, nil
}

func runMap(args []string) error {
	flags := newFlagSet("map")
	fromText := flags.String("from", "", "source frame rate")
	toText := flags.String("to", "", "target frame rate")
	viaText := flags.String("via", "", "comma-separated intermediate frame rates")
	frameNumber := flags.Int64("frame", -1, "source frame number")
	timecodeText := flags.String("tc", "", "source timecode (HH:MM:SS;FF)")
	roundTrips := flags.Int("round-trips", 3, "exact round-trip cycles to verify")
	jsonOutput := flags.Bool("json", false, "emit JSON instead of text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *fromText == "" || *toText == "" {
		return fmt.Errorf("map requires -from and -to")
	}

	sourceRate, err := subtime.ParseRate(*fromText)
	if err != nil {
		return err
	}
	targetRate, err := subtime.ParseRate(*toText)
	if err != nil {
		return err
	}
	via, err := parseRates(*viaText)
	if err != nil {
		return err
	}

	var source subtime.Frame
	switch {
	case *timecodeText != "" && *frameNumber >= 0:
		return fmt.Errorf("use either -frame or -tc, not both")
	case *timecodeText != "":
		source, err = subtime.ParseTimecode(*timecodeText, sourceRate)
		if err != nil {
			return err
		}
	case *frameNumber >= 0:
		source = subtime.Frame{Number: *frameNumber, Rate: sourceRate}
	default:
		return fmt.Errorf("map requires -frame or -tc")
	}

	path := append(append([]subtime.Rate(nil), via...), targetRate)
	target, err := subtime.ProjectFrame(source, path...)
	if err != nil {
		return err
	}
	candidates, err := target.Candidates()
	if err != nil {
		return err
	}
	recovered, err := subtime.RoundTripN(source, *roundTrips, path...)
	if err != nil {
		return err
	}

	sourceInfo, err := makeFrameReport(source)
	if err != nil {
		return err
	}
	recoveredInfo, err := makeFrameReport(recovered)
	if err != nil {
		return err
	}
	targetSeconds, err := target.Seconds()
	if err != nil {
		return err
	}
	report := mapReport{
		Source: sourceInfo,
		Target: targetReport{
			Rate: target.Rate.String(), ExactFrames: target.Position.String(),
			ExactSeconds: targetSeconds.String(), OnFrame: target.IsFrame(), Candidates: candidates,
		},
		RoundTrips: *roundTrips,
		RoundTrip:  recoveredInfo,
	}
	for _, rate := range via {
		report.Via = append(report.Via, rate.String())
	}
	if exact, ok := target.ExactFrame(); ok {
		report.Target.Frame = &exact.Number
		report.Target.Timecode, _ = exact.Timecode()
	}

	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	fmt.Printf("source   %s frame %d @ %s (%s s)\n", sourceInfo.Timecode, source.Number, source.Rate, sourceInfo.Seconds)
	if len(via) > 0 {
		fmt.Printf("via      %s\n", strings.Join(report.Via, " -> "))
	}
	fmt.Printf("target   exact frame position %s @ %s (%s s)\n", target.Position, target.Rate, targetSeconds)
	if target.IsFrame() {
		fmt.Printf("         on frame %d (%s)\n", *report.Target.Frame, report.Target.Timecode)
	} else {
		fmt.Println("         straddles frames; explicit candidates:")
		for _, candidate := range candidates {
			marker := " "
			if candidate.IsNearest {
				marker = "*"
			}
			fmt.Printf("         %s frame %d %s offset %s frames\n",
				marker, candidate.Frame, candidate.Timecode, candidate.OffsetFrames)
		}
	}
	fmt.Printf("roundtrip %d x -> frame %d (%s)\n", *roundTrips, recovered.Number, recoveredInfo.Timecode)
	return nil
}

func writeJSON(path string, value any) error {
	var writer io.Writer = os.Stdout
	var file *os.File
	if path != "" {
		var err error
		file, err = os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

type srtReport struct {
	SourceRate  string                 `json:"source_rate"`
	TargetRate  string                 `json:"target_rate"`
	Cues        []subtime.MappedSRTCue `json:"cues"`
	Adjusted    int                    `json:"adjusted_out_points"`
	IssueCount  int                    `json:"issue_count"`
	Millisecond string                 `json:"millisecond_snap"`
}

func runSRT(args []string) error {
	flags := newFlagSet("srt")
	fromText := flags.String("from", "", "source frame rate")
	toText := flags.String("to", "", "target frame rate")
	viaText := flags.String("via", "", "comma-separated intermediate frame rates")
	inputPath := flags.String("in", "-", "input SRT path, or - for stdin")
	outputPath := flags.String("out", "-", "output SRT path, or - for stdout")
	startPolicyText := flags.String("start", "", "in-point snap policy: floor, ceil, nearest")
	endPolicyText := flags.String("end", "", "out-point snap policy: floor, ceil, nearest")
	ensureOneFrame := flags.Bool("ensure-one-frame", false, "explicitly advance a collapsed out point by one target frame")
	msPolicyText := flags.String("ms", "nearest", "SRT millisecond policy: floor, ceil, nearest")
	reportPath := flags.String("report", "", "optional JSON mapping report path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *fromText == "" || *toText == "" {
		return fmt.Errorf("srt requires -from and -to")
	}
	if *startPolicyText == "" || *endPolicyText == "" {
		return fmt.Errorf("srt requires explicit -start and -end snap policies")
	}

	sourceRate, err := subtime.ParseRate(*fromText)
	if err != nil {
		return err
	}
	targetRate, err := subtime.ParseRate(*toText)
	if err != nil {
		return err
	}
	via, err := parseRates(*viaText)
	if err != nil {
		return err
	}
	startPolicy, err := subtime.ParseSnapPolicy(*startPolicyText)
	if err != nil {
		return err
	}
	endPolicy, err := subtime.ParseSnapPolicy(*endPolicyText)
	if err != nil {
		return err
	}
	msPolicy, err := subtime.ParseSnapPolicy(*msPolicyText)
	if err != nil {
		return err
	}

	input, closeInput, err := openInput(*inputPath)
	if err != nil {
		return err
	}
	defer closeInput()
	cues, err := subtime.ParseSRT(input)
	if err != nil {
		return err
	}

	policy := subtime.IntervalPolicy{
		Start: startPolicy, End: endPolicy, EnsureOneFrame: *ensureOneFrame,
	}
	path := append(append([]subtime.Rate(nil), via...), targetRate)
	mapped, err := subtime.MapSRTCuesThrough(cues, sourceRate, path, policy)
	if err != nil {
		return err
	}
	outputCues := make([]subtime.SRTCue, len(mapped))
	report := srtReport{
		SourceRate: sourceRate.String(), TargetRate: targetRate.String(),
		Cues: mapped, Millisecond: msPolicy.String(),
	}
	for i, cue := range mapped {
		outputCues[i] = cue.Output
		if cue.Resolved.AdjustedEnd {
			report.Adjusted++
		}
		report.IssueCount += len(cue.Mapping.Issues)
	}

	output, closeOutput, err := openOutput(*outputPath)
	if err != nil {
		return err
	}
	writeErr := subtime.WriteSRT(output, outputCues, subtime.SRTFormatOptions{
		MillisecondSnap: msPolicy, EnsureEndAfterStart: *ensureOneFrame,
	})
	closeErr := closeOutput()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}

	if *reportPath != "" {
		if err := writeJSON(*reportPath, report); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr,
		"subtime: %d cues, %d short-tail adjustments, %d mapping issues, %s -> %s\n",
		len(mapped), report.Adjusted, report.IssueCount, sourceRate, targetRate)
	return nil
}

func openInput(path string) (io.Reader, func() error, error) {
	if path == "-" {
		return os.Stdin, func() error { return nil }, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

func openOutput(path string) (io.Writer, func() error, error) {
	if path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}
