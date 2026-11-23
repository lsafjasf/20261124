package videoedit

import (
	"testing"
	"time"
)

func ms(v int64) Duration { return Duration{Duration: time.Duration(v) * time.Millisecond} }

func sampleProject() *Project {
	return &Project{
		ID: "p1",
		Tracks: []Track{
			{
				ID: "video",
				Clips: []Clip{
					{ID: "c1", Start: ms(0), Length: ms(2000), SourceIn: ms(0)},
					{ID: "c2", Start: ms(2000), Length: ms(3000), SourceIn: ms(0)},
				},
			},
			{
				ID:       "subs",
				ParentID: "video",
				Clips: []Clip{
					{ID: "s1", Start: ms(300), Length: ms(1000), SourceIn: ms(0)},
					{ID: "s2", Start: ms(2200), Length: ms(2000), SourceIn: ms(0)},
				},
			},
		},
	}
}

func TestDeleteThenTrimMergesToDelete(t *testing.T) {
	project := sampleProject()
	order := &ChangeOrder{ID: "o1", Operations: []Operation{
		{ClipID: "c2", Action: ActionTrimStart, Delta: ms(500)},
		{ClipID: "c2", Action: ActionDelete},
		{ClipID: "c2", Action: ActionTrimEnd, Delta: ms(-300)},
	}}
	result, err := Apply(project, order, ApplyOptions{Resolver: FixedResolver{Choice: ChoiceDeleteChildren}})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if _, ok := project.FindClip("c2"); ok {
		t.Fatal("c2 should have been deleted regardless of later trims")
	}
	if len(result.Journal.Entries) != 2 {
		t.Fatalf("unexpected journal: %+v", result.Journal.Entries)
	}
	for _, entry := range result.Journal.Entries {
		if entry.Kind != EntryDeleteClip {
			t.Fatalf("unexpected journal entry kind: %+v", entry)
		}
	}
}

func TestUnknownClipLeavesProjectUntouched(t *testing.T) {
	project := sampleProject()
	baseline := project.DeepClone()
	order := &ChangeOrder{ID: "o2", Operations: []Operation{
		{ClipID: "c1", Action: ActionMove, Delta: ms(100)},
		{ClipID: "ghost", Action: ActionDelete},
	}}
	_, err := Apply(project, order, ApplyOptions{Resolver: AbortResolver{}})
	if err == nil {
		t.Fatal("expected error for unknown clip")
	}
	if !project.Equal(baseline) {
		t.Fatal("project mutated although application failed")
	}
}

func TestShrinkToZeroRollsBack(t *testing.T) {
	project := sampleProject()
	baseline := project.DeepClone()
	order := &ChangeOrder{ID: "o3", Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimStart, Delta: ms(1500)},
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-600)},
	}}
	_, err := Apply(project, order, ApplyOptions{Resolver: AbortResolver{}})
	if err == nil {
		t.Fatal("expected non-positive length error")
	}
	if !project.Equal(baseline) {
		t.Fatalf("project changed after failed apply:\n%+v", project)
	}
}

func TestThreeOrdersConverge(t *testing.T) {
	ops := []Operation{
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-800)},
		{ClipID: "c2", Action: ActionTrimStart, Delta: ms(200)},
		{ClipID: "c2", Action: ActionTrimEnd, Delta: ms(-400)},
	}
	permutations := [][]int{{0, 1, 2}, {2, 0, 1}, {1, 2, 0}}
	snapshots := map[string]bool{}
	for i, perm := range permutations {
		project := sampleProject()
		shuffled := make([]Operation, len(ops))
		for j, idx := range perm {
			shuffled[j] = ops[idx]
		}
		order := &ChangeOrder{ID: "converge", Operations: shuffled}
		result, err := Apply(project, order, ApplyOptions{Resolver: FixedResolver{Choice: ChoiceCascade}})
		if err != nil {
			t.Fatalf("permutation %d failed: %v", i, err)
		}
		c2, ok := project.FindClip("c2")
		if !ok {
			t.Fatalf("permutation %d lost c2", i)
		}
		if c2.Clip.Start != ms(2200) || c2.Clip.Length != ms(2400) || c2.Clip.SourceIn != ms(200) {
			t.Fatalf("permutation %d wrong c2 geometry: %+v", i, c2.Clip)
		}
		snapshots[projectKey(t, project)] = true
		if len(result.Journal.Entries) != 3 {
			t.Fatalf("permutation %d expected 3 journal entries, got %d", i, len(result.Journal.Entries))
		}
	}
	if len(snapshots) != 1 {
		t.Fatalf("orders did not converge: %d distinct snapshots", len(snapshots))
	}
}

func projectKey(t *testing.T, project *Project) string {
	t.Helper()
	key, err := jsonMarshal(project)
	if err != nil {
		t.Fatal(err)
	}
	return string(key)
}
