package videoedit

import "testing"

func TestParseOrderErrors(t *testing.T) {
	if _, err := ParseOrder([]byte("{not json")); err == nil {
		t.Fatal("expected JSON parse error")
	}
	if _, err := ParseOrder([]byte(`{"operations":[{"clipId":"","action":"delete"}]}`)); err == nil {
		t.Fatal("expected empty clipId validation error")
	}
	if _, err := ParseOrder([]byte(`{"operations":[{"clipId":"c1","action":"explode"}]}`)); err == nil {
		t.Fatal("expected unknown action validation error")
	}
	var nilOrder *ChangeOrder
	if err := nilOrder.Validate(); err == nil {
		t.Fatal("expected nil order error")
	}
}

func TestDurationUnmarshalVariants(t *testing.T) {
	var fromMillis Duration
	if err := fromMillis.UnmarshalJSON([]byte("250")); err != nil || fromMillis.Duration != ms(250).Duration {
		t.Fatalf("millis unmarshal failed: %v %v", err, fromMillis)
	}
	var fromText Duration
	if err := fromText.UnmarshalJSON([]byte(`"1.5s"`)); err != nil || fromText.Duration != ms(1500).Duration {
		t.Fatalf("text unmarshal failed: %v %v", err, fromText)
	}
	var bad Duration
	if err := bad.UnmarshalJSON([]byte("true")); err == nil {
		t.Fatal("expected duration unmarshal error")
	}
}

func TestProjectValidateFailures(t *testing.T) {
	cases := map[string]*Project{
		"nil":                 nil,
		"empty track id":      {Tracks: []Track{{Clips: []Clip{{ID: "c1", Length: ms(1)}}}}},
		"duplicate track":     {Tracks: []Track{{ID: "t"}, {ID: "t"}}},
		"unknown parent":      {Tracks: []Track{{ID: "t"}, {ID: "c", ParentID: "ghost"}}},
		"empty clip id":       {Tracks: []Track{{ID: "t", Clips: []Clip{{Length: ms(1)}}}}},
		"duplicate clip":      {Tracks: []Track{{ID: "t", Clips: []Clip{{ID: "x", Length: ms(1)}, {ID: "x", Length: ms(1)}}}}},
		"non-positive length": {Tracks: []Track{{ID: "t", Clips: []Clip{{ID: "x", Length: ms(0)}}}}},
		"negative start":      {Tracks: []Track{{ID: "t", Clips: []Clip{{ID: "x", Start: ms(-1), Length: ms(1)}}}}},
		"overlap": {Tracks: []Track{{ID: "t", Clips: []Clip{
			{ID: "a", Length: ms(10)},
			{ID: "b", Start: ms(5), Length: ms(10)},
		}}}},
	}
	for name, project := range cases {
		t.Run(name, func(t *testing.T) {
			if err := project.Validate(); err == nil {
				t.Fatalf("case %s should fail validation", name)
			}
		})
	}
}

func TestPlanValidationFailures(t *testing.T) {
	project := sampleProject()
	if _, err := Plan(project, &ChangeOrder{Operations: []Operation{
		{ClipID: "ghost", Action: ActionDelete},
	}}); err == nil {
		t.Fatal("expected unknown clip plan error")
	}
	if _, err := Plan(project, &ChangeOrder{Operations: []Operation{
		{ClipID: "c1", Action: ActionMove, Delta: ms(-5000)},
	}}); err == nil {
		t.Fatal("expected negative start plan error")
	}
	if _, err := Plan(project, &ChangeOrder{Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimStart, Delta: ms(5000)},
	}}); err == nil {
		t.Fatal("expected non-positive length plan error")
	}
}

func TestFixedResolverRejectsUnsupportedChoice(t *testing.T) {
	resolver := FixedResolver{Choice: ChoiceCascade}
	choice, err := resolver.Resolve(Conflict{Kind: ConflictParentDelete, Choices: []Choice{ChoiceAbort}})
	if err != nil {
		t.Fatal(err)
	}
	if choice != ChoiceAbort {
		t.Fatalf("unsupported fixed choice must fall back to abort, got %s", choice)
	}
}

func TestCLIResolverInvalidInput(t *testing.T) {
	resolver := &CLIResolver{
		In:  stringsReader("nonsense\n"),
		Out: discardWriter{},
	}
	choice, err := resolver.Resolve(Conflict{Choices: []Choice{ChoiceAbort, ChoiceDetach}})
	if err != nil {
		t.Fatal(err)
	}
	if choice != ChoiceAbort {
		t.Fatalf("invalid answer should abort, got %s", choice)
	}
}

func TestCLIResolverWithoutStreams(t *testing.T) {
	var resolver *CLIResolver
	resolver = &CLIResolver{}
	if _, err := resolver.Resolve(Conflict{}); err == nil {
		t.Fatal("expected error when streams are missing")
	}
}
