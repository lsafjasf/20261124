package adm

import (
	"os"
	"strings"
	"testing"
)

func loadFixture(t *testing.T) *Document {
	t.Helper()
	f, err := os.Open("../testdata/collapse.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := Open(f)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func analyzeFixture(t *testing.T, downmix bool) ([]*ObjectAnalysis, *RenderResult) {
	t.Helper()
	layout, ok := GetLayout("0+5+0")
	if !ok {
		t.Fatal("layout 0+5+0 missing")
	}
	res := Render(loadFixture(t), layout, downmix)
	return Analyze(res), res
}

func findObject(t *testing.T, list []*ObjectAnalysis, name string) *ObjectAnalysis {
	t.Helper()
	for _, oa := range list {
		if oa.Name == name {
			return oa
		}
	}
	t.Fatalf("object %s not found", name)
	return nil
}

func TestParseFixture(t *testing.T) {
	doc := loadFixture(t)
	if len(doc.Objects) != 5 {
		t.Fatalf("want 5 objects, got %d", len(doc.Objects))
	}
	if len(doc.Warnings) == 0 {
		t.Fatal("expected a coordinate-mismatch warning")
	}
	cf := doc.ChannelFormats["AC_0004"]
	if cf.Type != "DirectSpeakers" {
		t.Fatalf("typeLabel 0001 should normalise to DirectSpeakers, got %q", cf.Type)
	}
}

func TestCollapseStages(t *testing.T) {
	analysis, _ := analyzeFixture(t, false)
	cases := map[string]string{
		"Obj_CartMismatch": StageNormalized,
		"Obj_NoPos":        StageNormalized,
		"Obj_Height":       StagePanned,
		"Obj_Lss":          StagePanned,
		"Obj_OK":           "",
	}
	for name, want := range cases {
		if got := findObject(t, analysis, name).CollapseStage; got != want {
			t.Errorf("%s: collapse stage = %q, want %q", name, got, want)
		}
	}
}

func TestUnrenderableAndSuggestions(t *testing.T) {
	analysis, _ := analyzeFixture(t, false)
	lss := findObject(t, analysis, "Obj_Lss")
	if !lss.Unrenderable {
		t.Error("Obj_Lss should be unrenderable in 0+5+0")
	}
	if lss.TotalEnergy != 0 {
		t.Errorf("Obj_Lss total energy = %v, want 0", lss.TotalEnergy)
	}
	for _, name := range []string{"Obj_CartMismatch", "Obj_Height", "Obj_Lss", "Obj_NoPos"} {
		if findObject(t, analysis, name).Suggestion == "" {
			t.Errorf("%s: expected a manual-handling suggestion", name)
		}
	}
	if s := findObject(t, analysis, "Obj_OK").Suggestion; s != "" {
		t.Errorf("Obj_OK should have no suggestion, got %q", s)
	}
}

func TestTimeline(t *testing.T) {
	_, res := analyzeFixture(t, false)
	rows := Timeline(res)
	if len(rows) == 0 {
		t.Fatal("empty timeline")
	}
	var noPos *TimelineRow
	for i := range rows {
		if rows[i].Object == "Obj_NoPos" {
			noPos = &rows[i]
			break
		}
	}
	if noPos == nil {
		t.Fatal("Obj_NoPos missing from timeline")
	}
	if len(noPos.Mapping) == 0 || !strings.HasPrefix(noPos.Mapping[0], "C") {
		t.Errorf("Obj_NoPos should map to C first, got %v", noPos.Mapping)
	}
}

func TestDownmixStage(t *testing.T) {
	analysis, res := analyzeFixture(t, true)
	if !res.Downmix {
		t.Fatal("downmix flag not propagated")
	}
	ok := findObject(t, analysis, "Obj_OK")
	if _, has := ok.StageStatus[StageDownmix]; !has {
		t.Error("downmix stage missing from stage status")
	}
	var lEnergy, cEnergy float64
	for i, s := range res.Layout.Speakers {
		switch s.Short {
		case "L":
			lEnergy = ok.FinalEnergy[i]
		case "C":
			cEnergy = ok.FinalEnergy[i]
		}
	}
	if cEnergy != 0 {
		t.Errorf("after LoRo downmix C energy should be 0, got %v", cEnergy)
	}
	if lEnergy == 0 {
		t.Error("after LoRo downmix L energy should be > 0")
	}
}
