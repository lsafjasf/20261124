package videoedit

import "testing"

func TestConvergenceWithMovesAndConflictResolution(t *testing.T) {
	baseOps := []Operation{
		{ClipID: "c2", Action: ActionTrimStart, Delta: ms(200)},
		{ClipID: "c2", Action: ActionTrimEnd, Delta: ms(-400)},
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-800)},
		{ClipID: "c1", Action: ActionDelete},
	}
	orders := [][]int{
		{0, 1, 2, 3},
		{3, 2, 1, 0},
		{2, 0, 3, 1},
	}
	snapshots := map[string]string{}
	resolvers := []Resolver{
		FixedResolver{Choice: ChoiceDetach},
		FixedResolver{Choice: ChoiceDeleteChildren},
		FixedResolver{Choice: ChoiceSkipParent},
	}
	for _, resolver := range resolvers {
		for run, perm := range orders {
			project := sampleProject()
			ops := make([]Operation, len(baseOps))
			for i, idx := range perm {
				ops[i] = baseOps[idx]
			}
			order := &ChangeOrder{ID: "conv", Operations: ops}
			result, err := Apply(project, order, ApplyOptions{Resolver: resolver})
			if err != nil {
				t.Fatalf("resolver run failed: %v", err)
			}
			key := projectKey(t, project)
			if first, ok := snapshots[resolverName(resolver)]; ok && first != key {
				t.Fatalf("resolver %s diverged at run %d", resolverName(resolver), run)
			}
			snapshots[resolverName(resolver)] = key
			if len(result.Journal.Entries) == 0 {
				t.Fatal("journal must record every write")
			}
		}
	}
	if len(snapshots) != 3 {
		t.Fatalf("expected three distinct resolution outcomes, got %d", len(snapshots))
	}
}

func resolverName(resolver Resolver) string {
	switch r := resolver.(type) {
	case FixedResolver:
		return string(r.Choice)
	default:
		return "other"
	}
}
