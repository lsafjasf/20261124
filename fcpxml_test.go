package fcpxml

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTimeAndFrames(t *testing.T) {
	ntsc, err := ParseTime("1001/30000s")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := FrameAt(ntsc, ntsc, RoundNearest)
	if err != nil || frame != 1 {
		t.Fatalf("NTSC frame = %d, %v; want 1", frame, err)
	}
	oneAndHalf, err := ParseTime("1.5s")
	if err != nil {
		t.Fatal(err)
	}
	frame, err = FrameAt(oneAndHalf, Time{Num: 1, Den: 30}, RoundNearest)
	if err != nil || frame != 45 {
		t.Fatalf("1.5s at 30fps = %d, %v; want 45", frame, err)
	}
	negative, err := ParseTime("-1/30s")
	if err != nil {
		t.Fatal(err)
	}
	frame, err = FrameAt(negative, Time{Num: 1, Den: 30}, RoundFloor)
	if err != nil || frame != -1 {
		t.Fatalf("-1/30s floor = %d, %v; want -1", frame, err)
	}
}

func TestNestedTimelineMapsToTopFrames(t *testing.T) {
	document := loadFixture(t, "testdata/nested.fcpxml")
	timeline, err := document.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got, want := len(timeline.PictureEvents), 3; got != want {
		t.Fatalf("picture events = %d, want %d: %+v", got, want, timeline.PictureEvents)
	}
	events := make(map[string]PictureEvent, len(timeline.PictureEvents))
	for _, event := range timeline.PictureEvents {
		events[event.Name] = event
	}

	assertSpan(t, events["opening"], 0, 60)
	assertSpan(t, events["negative"], 60, 120)
	assertSpan(t, events["tail"], 180, 240)
	if got := events["negative"].RawStart; got.String() != "-1s" {
		t.Fatalf("negative raw start = %s, want -1s", got)
	}

	if got, want := len(timeline.Transitions), 1; got != want {
		t.Fatalf("transitions = %d, want %d", got, want)
	}
	transition := timeline.Transitions[0]
	if transition.StartFrame != 45 || transition.CenterFrame != 60 ||
		transition.EndFrame != 75 || transition.CutFrame != 60 {
		t.Fatalf("transition frames = %d/%d/%d cut %d, want 45/60/75 cut 60",
			transition.StartFrame, transition.CenterFrame, transition.EndFrame,
			transition.CutFrame)
	}

	transitionCut := findCut(timeline.Cuts, "transition-cut")
	if transitionCut == nil || transitionCut.Frame != 60 || transitionCut.Time.String() != "2s" {
		t.Fatalf("transition cut = %+v, want frame 60 at 2s", transitionCut)
	}
	for _, frame := range []int64{0, 60, 120, 180, 240} {
		if findCutFrame(timeline.Cuts, frame) == nil {
			t.Errorf("missing key frame %d in %+v", frame, timeline.Cuts)
		}
	}
}

func TestSupportedSubsetRoundTripPreservesCuts(t *testing.T) {
	document := loadFixture(t, "testdata/nested.fcpxml")
	result, err := document.VerifyCutRoundTrip("sqTop")
	if err != nil {
		t.Fatalf("VerifyCutRoundTrip: %v", err)
	}
	if len(result.BeforeSignature) != len(result.AfterSignature) {
		t.Fatalf("signature length changed: %d -> %d",
			len(result.BeforeSignature), len(result.AfterSignature))
	}
	var encoded bytes.Buffer
	if err := document.ExportSupported(&encoded); err != nil {
		t.Fatalf("ExportSupported: %v", err)
	}
	reimported, err := Parse(encoded.Bytes())
	if err != nil {
		t.Fatalf("Parse exported XML: %v\n%s", err, encoded.String())
	}
	if _, err := reimported.AnalyzeSequence("sqTop"); err != nil {
		t.Fatalf("Analyze re-imported XML: %v\n%s", err, encoded.String())
	}
}

func TestMissingReferenceIsReported(t *testing.T) {
	data, err := os.ReadFile("testdata/nested.fcpxml")
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(data), `ref="r12"`, `ref="missing-asset"`, 1)
	document, err := Parse([]byte(broken))
	if err != nil {
		t.Fatal(err)
	}
	report, err := document.Validate()
	if err == nil {
		t.Fatal("Validate succeeded with a missing reference")
	}
	if len(report.MissingRefs) != 1 {
		t.Fatalf("missing refs = %+v, want exactly one", report.MissingRefs)
	}
	missing := report.MissingRefs[0]
	if missing.ID != "missing-asset" || missing.Attribute != "ref" {
		t.Fatalf("unexpected missing reference: %+v", missing)
	}
}

func TestCircularNestingIsReported(t *testing.T) {
	document := parseString(t, circularDocument)
	report, err := document.Validate()
	if err == nil {
		t.Fatal("Validate succeeded with circular sequence nesting")
	}
	if len(report.Cycles) == 0 {
		t.Fatal("no cycle was reported")
	}
	joined := strings.Join(report.Cycles[0].IDs, " ")
	if !strings.Contains(joined, "sqA") || !strings.Contains(joined, "sqB") {
		t.Fatalf("cycle does not name both sequences: %+v", report.Cycles[0])
	}
}

func TestOffCenterTransitionKeepsEditorialCut(t *testing.T) {
	document := parseString(t, offCenterTransitionDocument)
	timeline, err := document.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	transition := timeline.Transitions[0]
	if transition.CenterFrame != 105 {
		t.Fatalf("transition midpoint frame = %d, want 105", transition.CenterFrame)
	}
	if !transition.HasCut || transition.CutFrame != 120 {
		t.Fatalf("transition cut = %+v, want frame 120", transition)
	}
	cut := findCut(timeline.Cuts, "transition-cut")
	if cut == nil || cut.Frame != 120 {
		t.Fatalf("transition cut point = %+v, want frame 120", cut)
	}
	if findCutFrame(timeline.Cuts, 105) != nil {
		t.Fatal("transition midpoint must not be reported as an editorial cut")
	}
}

func TestConnectedStorylineUsesParentClock(t *testing.T) {
	document := parseString(t, connectedStorylineDocument)
	timeline, err := document.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(timeline.PictureEvents) != 2 {
		t.Fatalf("picture events = %+v, want base and overlay", timeline.PictureEvents)
	}
	events := make(map[string]PictureEvent)
	for _, event := range timeline.PictureEvents {
		events[event.Name] = event
	}
	assertSpan(t, events["base"], 0, 240)
	// Parent starts at source 5s; storyline offset 7s is 2s into the clip.
	assertSpan(t, events["overlay"], 60, 120)
}

func assertSpan(t *testing.T, event PictureEvent, start, end int64) {
	t.Helper()
	if event.StartFrame != start || event.EndFrame != end {
		t.Fatalf("%s frame span = [%d,%d), want [%d,%d)",
			event.Name, event.StartFrame, event.EndFrame, start, end)
	}
}

func findCut(cuts []Cut, kind string) *Cut {
	for index := range cuts {
		if cuts[index].Kind == kind {
			return &cuts[index]
		}
	}
	return nil
}

func findCutFrame(cuts []Cut, frame int64) *Cut {
	for index := range cuts {
		if cuts[index].Frame == frame {
			return &cuts[index]
		}
	}
	return nil
}

func loadFixture(t *testing.T, path string) *Document {
	t.Helper()
	document, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	return document
}

func parseString(t *testing.T, value string) *Document {
	t.Helper()
	document, err := Parse([]byte(value))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return document
}

const circularDocument = `<?xml version="1.0" encoding="UTF-8"?>
<fcpxml version="1.10">
  <resources>
    <format id="f" frameDuration="1/30s"/>
    <sequence id="sqA" format="f" duration="2s">
      <spine><ref-clip ref="sqB" offset="0s" duration="1s"/></spine>
    </sequence>
    <sequence id="sqB" format="f" duration="2s">
      <spine><ref-clip ref="sqA" offset="0s" duration="1s"/></spine>
    </sequence>
  </resources>
  <library><event><project name="cycle">
    <sequence id="sqTop" format="f" duration="2s">
      <spine><ref-clip ref="sqA" offset="0s" duration="2s"/></spine>
    </sequence>
  </project></event></library>
</fcpxml>`

const offCenterTransitionDocument = `<?xml version="1.0" encoding="UTF-8"?>
<fcpxml version="1.10">
  <resources>
    <format id="f" frameDuration="1/30s"/>
    <asset id="a" format="f" duration="10s"/>
  </resources>
  <library><event><project name="off-center">
    <sequence id="top" format="f" duration="8s">
      <spine>
        <asset-clip ref="a" name="outgoing" offset="0s" start="0s" duration="4s"/>
        <transition name="dissolve" offset="3s" duration="1s"/>
        <asset-clip ref="a" name="incoming" offset="4s" start="4s" duration="4s"/>
      </spine>
    </sequence>
  </project></event></library>
</fcpxml>`

const connectedStorylineDocument = `<?xml version="1.0" encoding="UTF-8"?>
<fcpxml version="1.10">
  <resources>
    <format id="f" frameDuration="1/30s"/>
    <asset id="base-asset" format="f" duration="20s"/>
    <asset id="overlay-asset" format="f" duration="20s"/>
  </resources>
  <library><event><project name="connected">
    <sequence id="top" format="f" duration="10s">
      <spine>
        <asset-clip ref="base-asset" name="base" offset="0s" start="5s" duration="8s">
          <spine lane="1" offset="7s">
            <asset-clip ref="overlay-asset" name="overlay" offset="0s" start="0s" duration="2s"/>
          </spine>
        </asset-clip>
      </spine>
    </sequence>
  </project></event></library>
</fcpxml>`
