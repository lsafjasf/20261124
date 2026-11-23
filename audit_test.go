package fcpxml

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadDoc(t *testing.T, path string) *Document {
	t.Helper()
	doc, err := ParseDocumentFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestInventoryTracksEffectsReferencesAndFrames(t *testing.T) {
	doc := loadDoc(t, "testdata/source.fcpxml")
	inventory, err := doc.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	if hasLoss(inventory.Findings) {
		t.Fatalf("source unexpectedly invalid: %#v", inventory.Findings)
	}
	if got, want := len(inventory.Tracks), 2; got != want {
		t.Fatalf("tracks = %d, want %d", got, want)
	}
	if got, want := len(inventory.Items), 5; got != want {
		t.Fatalf("items = %d, want %d", got, want)
	}
	if got, want := len(inventory.Effects), 4; got != want {
		t.Fatalf("effects = %d, want %d", got, want)
	}

	var transition, timeMap *EffectInfo
	for _, effect := range inventory.Effects {
		switch effect.Kind {
		case "transition":
			transition = effect
		case "time-map":
			timeMap = effect
		}
	}
	if transition == nil {
		t.Fatal("transition was not inventoried")
	}
	if transition.Offset == nil || transition.Offset.FrameExact != "96" {
		t.Fatalf("transition frame = %#v, want 96", transition.Offset)
	}
	if !transition.Resolved {
		t.Fatal("transition effect reference did not resolve")
	}
	if timeMap == nil {
		t.Fatal("timeMap was not inventoried")
	}
	if got, want := timeMap.Speed, "2/5"; got != want {
		t.Fatalf("timeMap speed = %q, want %q", got, want)
	}
	if timeMap.Offset == nil || timeMap.Offset.FrameExact != "184" {
		t.Fatalf("timeMap frame = %#v, want 184", timeMap.Offset)
	}

	present := 0
	for _, ref := range inventory.MediaRefs {
		if ref.FileState == "present" {
			present++
		}
		if !ref.Resolved && ref.Kind == "item-ref" {
			t.Errorf("unresolved item ref: %#v", ref)
		}
	}
	if present < 3 {
		t.Fatalf("present media files = %d, want at least 3", present)
	}
}

func TestCompareFindsTransitionBeforeDroppedRetime(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	candidate := loadDoc(t, "testdata/lossy.fcpxml")
	comparison, err := CompareDocuments(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Compatible {
		t.Fatal("lossy candidate was accepted as compatible")
	}
	if comparison.Status != "lossy" {
		t.Fatalf("status = %q, want lossy", comparison.Status)
	}
	if comparison.EarliestLoss == nil || comparison.EarliestLoss.Position == nil {
		t.Fatal("earliest loss was not reported")
	}
	if got, want := comparison.EarliestLoss.Code, "TRANSITION_FLATTENED"; got != want {
		t.Fatalf("earliest loss = %s, want %s", got, want)
	}
	if got, want := comparison.EarliestLoss.Position.FrameExact, "96"; got != want {
		t.Fatalf("earliest frame = %s, want %s", got, want)
	}
	if got, want := comparison.LostEffectCount, 3; got != want {
		t.Fatalf("lost effects = %d, want %d", got, want)
	}
	assertLossCode(t, comparison.Losses, "TRANSITION_FLATTENED")
	assertLossCode(t, comparison.Losses, "RETIME_EFFECT_MISSING")
}

func TestRepairUsesSourceInsteadOfPassingLossyCandidate(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	candidate := loadDoc(t, "testdata/lossy.fcpxml")
	report, repair, err := AuditAndRepair(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.ConversionSucceeded {
		t.Fatal("lossy conversion reported success")
	}
	if report.Status != "lossy" {
		t.Fatalf("report status = %q, want lossy", report.Status)
	}
	if repair.Report.Strategy != RepairSourceFallback {
		t.Fatalf("repair strategy = %q, want %q", repair.Report.Strategy, RepairSourceFallback)
	}
	if !repair.Report.Reimportable || !repair.Report.PreservesEffects {
		t.Fatalf("repair flags = %#v", repair.Report)
	}
	if !bytes.Equal(repair.Data, source.Raw) {
		t.Fatal("source fallback did not preserve source bytes")
	}
	repaired, err := ParseDocumentBytes(repair.Data)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := repaired.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	if hasLoss(inventory.Findings) {
		t.Fatalf("repaired output is invalid: %#v", inventory.Findings)
	}
	if got := countEffectKind(inventory, "transition"); got != 1 {
		t.Fatalf("repaired transitions = %d, want 1", got)
	}
	if got := countEffectKind(inventory, "time-map"); got != 1 {
		t.Fatalf("repaired retimes = %d, want 1", got)
	}
	if got, want := len(inventory.Effects), 4; got != want {
		t.Fatalf("repaired effects = %d, want %d", got, want)
	}
}

func TestCompatibleCandidateKeepsItsOwnOutput(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	candidate := loadDoc(t, "testdata/source.fcpxml")
	report, repair, err := AuditAndRepair(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Summary.ConversionSucceeded || report.Summary.LossCount != 0 {
		t.Fatalf("compatible audit = %#v", report.Summary)
	}
	if repair.Report.Strategy != RepairCandidatePreserved {
		t.Fatalf("repair strategy = %q, want candidate_preserved", repair.Report.Strategy)
	}
	if repair.Report.DeletedEffectCount != 0 {
		t.Fatalf("deleted effects = %d, want 0", repair.Report.DeletedEffectCount)
	}
}

func TestRenumberedResourceIDsRemainCompatible(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	replacer := strings.NewReplacer(
		`id="r1"`, `id="x1"`, `format="r1"`, `format="x1"`,
		`id="r2"`, `id="x2"`, `ref="r2"`, `ref="x2"`,
		`id="r3"`, `id="x3"`, `ref="r3"`, `ref="x3"`,
		`id="r4"`, `id="x4"`, `ref="r4"`, `ref="x4"`,
		`id="r5"`, `id="x5"`, `ref="r5"`, `ref="x5"`,
		`id="r6"`, `id="x6"`, `ref="r6"`, `ref="x6"`,
	)
	candidate, err := ParseDocumentBytes([]byte(replacer.Replace(string(source.Raw))))
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := CompareDocuments(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !comparison.Compatible {
		t.Fatalf("resource renumbering was treated as loss: %#v", comparison.Losses)
	}
}

func TestChangedEffectResourceIsALoss(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	raw := strings.Replace(string(source.Raw), `uid="ColorCorrect"`, `uid="DifferentEffect"`, 1)
	candidate, err := ParseDocumentBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := CompareDocuments(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	assertLossCode(t, comparison.Losses, "EFFECT_REFERENCE_CHANGED")
}

func TestChangedFilterParameterIsALoss(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	raw := strings.Replace(string(source.Raw), `value="0.5"`, `value="0.7"`, 1)
	candidate, err := ParseDocumentBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := CompareDocuments(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	assertLossCode(t, comparison.Losses, "EFFECT_PARAMETER_CHANGED")
	if comparison.EarliestLoss == nil || comparison.EarliestLoss.Position.FrameExact != "0" {
		t.Fatalf("earliest parameter loss = %#v, want frame 0", comparison.EarliestLoss)
	}
}

func TestMissingConnectedTrackIsALossAtItsFirstFrame(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	raw := string(source.Raw)
	start := strings.Index(raw, `            <clip ref="r4"`)
	if start < 0 {
		t.Fatal("music track fixture not found")
	}
	end := strings.Index(raw[start:], "</clip>")
	if end < 0 {
		t.Fatal("music track end not found")
	}
	end = start + end + len("</clip>")
	candidate, err := ParseDocumentBytes([]byte(raw[:start] + raw[end:]))
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := CompareDocuments(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.LostTrackCount != 1 {
		t.Fatalf("lost tracks = %d, want 1", comparison.LostTrackCount)
	}
	assertLossCode(t, comparison.Losses, "TRACK_MISSING")
	if comparison.EarliestLoss == nil || comparison.EarliestLoss.Position.FrameExact != "0" {
		t.Fatalf("earliest track loss = %#v, want frame 0", comparison.EarliestLoss)
	}
}

func TestFrameAndMediaReferenceChangesAreLosses(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	shiftedRaw := strings.Replace(string(source.Raw), `name="A" offset="0s"`, `name="A" offset="1/25s"`, 1)
	shifted, err := ParseDocumentBytes([]byte(shiftedRaw))
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := CompareDocuments(source, shifted)
	if err != nil {
		t.Fatal(err)
	}
	assertLossCode(t, comparison.Losses, "FRAME_POSITION_CHANGED")

	changedRaw := strings.Replace(string(source.Raw), `ref="r2" name="B"`, `ref="r3" name="B"`, 1)
	changed, err := ParseDocumentBytes([]byte(changedRaw))
	if err != nil {
		t.Fatal(err)
	}
	comparison, err = CompareDocuments(source, changed)
	if err != nil {
		t.Fatal(err)
	}
	assertLossCode(t, comparison.Losses, "MEDIA_REFERENCE_CHANGED")

	componentRaw := strings.Replace(string(source.Raw), `<audio ref="r4"`, `<audio ref="r3"`, 1)
	component, err := ParseDocumentBytes([]byte(componentRaw))
	if err != nil {
		t.Fatal(err)
	}
	comparison, err = CompareDocuments(source, component)
	if err != nil {
		t.Fatal(err)
	}
	assertLossCode(t, comparison.Losses, "MEDIA_REFERENCE_MISSING")

	rateRaw := strings.Replace(string(source.Raw), `frameDuration="1/25s"`, `frameDuration="1/30s"`, 1)
	rateChanged, err := ParseDocumentBytes([]byte(rateRaw))
	if err != nil {
		t.Fatal(err)
	}
	comparison, err = CompareDocuments(source, rateChanged)
	if err != nil {
		t.Fatal(err)
	}
	assertLossCode(t, comparison.Losses, "SEQUENCE_FRAME_RATE_CHANGED")
	if comparison.EarliestLoss == nil || comparison.EarliestLoss.Position.FrameExact != "0" {
		t.Fatalf("earliest frame-rate loss = %#v, want frame 0", comparison.EarliestLoss)
	}
}

func TestDanglingCandidateRefFallsBackButDanglingSourceBlocks(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	raw := strings.Replace(string(source.Raw), `ref="r2"`, `ref="r99"`, 1)
	broken, err := ParseDocumentBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, repair, err := AuditAndRepair(source, broken)
	if err != nil {
		t.Fatal(err)
	}
	if repair.Report.Strategy != RepairSourceFallback || len(repair.Data) == 0 {
		t.Fatalf("dangling candidate was not repaired: %#v", repair.Report)
	}
	report, blocked, err := AuditAndRepair(broken, broken)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Report.Strategy != RepairBlocked || len(blocked.Data) != 0 {
		t.Fatalf("dangling source did not block: %#v", blocked.Report)
	}
	if report.Status != "invalid" || len(blocked.Report.NonExpressible) == 0 {
		t.Fatalf("missing explicit non-expressible list: %#v", report)
	}
}

func TestOfflineMediaIsWarnedButRemainsStructurallyCompatible(t *testing.T) {
	source := loadDoc(t, "testdata/source.fcpxml")
	offlinePath := filepath.Join(t.TempDir(), "offline.fcpxml")
	if err := os.WriteFile(offlinePath, source.Raw, 0o644); err != nil {
		t.Fatal(err)
	}
	offline, err := ParseDocumentFile(offlinePath)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := CompareDocuments(source, offline)
	if err != nil {
		t.Fatal(err)
	}
	if !comparison.Compatible {
		t.Fatalf("offline media was treated as loss: %#v", comparison.Losses)
	}
	found := false
	for _, issue := range comparison.CandidateIssues {
		if issue.Code == "MEDIA_FILE_MISSING" {
			found = true
		}
	}
	if !found {
		t.Fatal("offline media warning was not reported")
	}
}

func TestXMLRoundTripPreservesEffectsAndUnknownElements(t *testing.T) {
	doc := loadDoc(t, "testdata/source.fcpxml")
	raw := strings.Replace(string(doc.Raw), "</fcpxml>",
		"  <!-- retained comment -->\n  <note kind=\"audit\">keep</note>\n</fcpxml>", 1)
	doc, err := ParseDocumentBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"<?xml", "transition", "timeMap", "Cross Dissolve", "retained comment", "note", `"audit"`, "keep"} {
		if !strings.Contains(text, want) {
			t.Errorf("re-encoded XML lost %q", want)
		}
	}
	reparsed, err := ParseDocumentBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := reparsed.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	if hasLoss(inventory.Findings) {
		t.Fatalf("re-encoded XML has errors: %#v", inventory.Findings)
	}
	if got, want := len(inventory.Effects), 4; got != want {
		t.Fatalf("re-encoded effects = %d, want %d", got, want)
	}
}

func assertLossCode(t *testing.T, losses []Finding, code string) {
	t.Helper()
	for _, loss := range losses {
		if loss.Code == code {
			return
		}
	}
	t.Fatalf("loss code %s not found in %#v", code, losses)
}
