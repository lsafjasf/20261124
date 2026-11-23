package videoedit

import (
	"bytes"
	"strings"
	"testing"
)

func TestOverflowConflictAbortRestoresProject(t *testing.T) {
	project := sampleProject()
	baseline := project.DeepClone()
	// Moving c1 right pushes its end past child s1? Instead shrink c1 so the
	// contained child s1 (300..1300) no longer fits in c1 (0..900).
	order := &ChangeOrder{ID: "o4", Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-800)},
	}}
	_, err := Apply(project, order, ApplyOptions{Resolver: AbortResolver{}})
	if err == nil {
		t.Fatal("expected abort due to parent/child overflow")
	}
	var abortErr *AbortError
	if !asAbort(err, &abortErr) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}
	if !project.Equal(baseline) {
		t.Fatal("project must stay intact after abort")
	}
}

func TestOverflowConflictCascade(t *testing.T) {
	project := sampleProject()
	order := &ChangeOrder{ID: "o5", Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-800)},
	}}
	result, err := Apply(project, order, ApplyOptions{Resolver: FixedResolver{Choice: ChoiceCascade}})
	if err != nil {
		t.Fatalf("cascade apply failed: %v", err)
	}
	c1, _ := project.FindClip("c1")
	s1, _ := project.FindClip("s1")
	if c1.Clip.Length != ms(1200) {
		t.Fatalf("c1 length = %s, want 1200ms", c1.Clip.Length)
	}
	if s1.Clip.Length != ms(200) || s1.Clip.Start != ms(300) {
		t.Fatalf("s1 should be cascade-trimmed to 200ms, got %+v", s1.Clip)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %d", len(result.Conflicts))
	}
}

func TestOverflowConflictDetach(t *testing.T) {
	project := sampleProject()
	order := &ChangeOrder{ID: "o6", Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-800)},
	}}
	_, err := Apply(project, order, ApplyOptions{Resolver: FixedResolver{Choice: ChoiceDetach}})
	if err != nil {
		t.Fatalf("detach apply failed: %v", err)
	}
	s1, _ := project.FindClip("s1")
	if !s1.Clip.Detached {
		t.Fatal("s1 should be flagged detached")
	}
	c1, _ := project.FindClip("c1")
	if c1.Clip.Length != ms(1200) {
		t.Fatalf("c1 length = %s, want 1200ms", c1.Clip.Length)
	}
}

func TestParentDeleteChoices(t *testing.T) {
	t.Run("delete children", func(t *testing.T) {
		project := sampleProject()
		order := &ChangeOrder{ID: "o7", Operations: []Operation{
			{ClipID: "c1", Action: ActionDelete},
		}}
		_, err := Apply(project, order, ApplyOptions{Resolver: FixedResolver{Choice: ChoiceDeleteChildren}})
		if err != nil {
			t.Fatalf("apply failed: %v", err)
		}
		if _, ok := project.FindClip("c1"); ok {
			t.Fatal("c1 should be deleted")
		}
		if _, ok := project.FindClip("s1"); ok {
			t.Fatal("s1 should be deleted with parent c1")
		}
		if _, ok := project.FindClip("s2"); !ok {
			t.Fatal("s2 belongs to c2 and must remain")
		}
	})

	t.Run("skip parent", func(t *testing.T) {
		project := sampleProject()
		baseline := project.DeepClone()
		order := &ChangeOrder{ID: "o8", Operations: []Operation{
			{ClipID: "c1", Action: ActionDelete},
		}}
		result, err := Apply(project, order, ApplyOptions{Resolver: FixedResolver{Choice: ChoiceSkipParent}})
		if err != nil {
			t.Fatalf("apply failed: %v", err)
		}
		if _, ok := project.FindClip("c1"); !ok {
			t.Fatal("c1 should remain when skipParent is chosen")
		}
		if !project.Equal(baseline) {
			t.Fatal("skipParent should leave the project unchanged")
		}
		if len(result.Skipped) != 1 {
			t.Fatalf("expected one skip, got %d", len(result.Skipped))
		}
	})
}

func TestJournalRoundTripUndo(t *testing.T) {
	project := sampleProject()
	baseline := project.DeepClone()
	order := &ChangeOrder{ID: "o9", Operations: []Operation{
		{ClipID: "c1", Action: ActionMove, Delta: ms(300)},
		{ClipID: "c2", Action: ActionDelete},
	}}
	result, err := Apply(project, order, ApplyOptions{Resolver: FixedResolver{Choice: ChoiceDetach}})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	data, err := result.Journal.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ParseJournal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := replayed.Rollback(project); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if !project.Equal(baseline) {
		t.Fatal("journal rollback did not restore the baseline project")
	}
}

func TestCLIResolverPrompt(t *testing.T) {
	project := sampleProject()
	order := &ChangeOrder{ID: "o10", Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-800)},
	}}
	input := strings.NewReader("3\n")
	var output bytes.Buffer
	_, err := Apply(project, order, ApplyOptions{Resolver: &CLIResolver{In: input, Out: &output}})
	if err != nil {
		t.Fatalf("interactive apply failed: %v", err)
	}
	if !strings.Contains(output.String(), "[冲突]") {
		t.Fatalf("resolver did not print conflict prompt:\n%s", output.String())
	}
	s1, _ := project.FindClip("s1")
	if !s1.Clip.Detached {
		t.Fatal("option 3 (detach) should flag s1 detached")
	}
}

func TestCLIResolverHandlesMultipleConflicts(t *testing.T) {
	project := sampleProject()
	order := &ChangeOrder{ID: "o11", Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-800)},
		{ClipID: "c2", Action: ActionDelete},
	}}
	// First conflict: detach (3). Second conflict (parentDelete): detach is
	// option 3 as well for the parentDelete choice list.
	input := strings.NewReader("3\n3\n")
	var output bytes.Buffer
	_, err := Apply(project, order, ApplyOptions{Resolver: &CLIResolver{In: input, Out: &output}})
	if err != nil {
		t.Fatalf("interactive multi-conflict apply failed: %v", err)
	}
	s1, _ := project.FindClip("s1")
	s2, _ := project.FindClip("s2")
	if !s1.Clip.Detached || !s2.Clip.Detached {
		t.Fatalf("both children should be detached, got s1=%v s2=%v", s1.Clip.Detached, s2.Clip.Detached)
	}
	if strings.Count(output.String(), "[冲突]") != 2 {
		t.Fatalf("expected two prompts, got:\n%s", output.String())
	}
}

func asAbort(err error, target **AbortError) bool {
	for err != nil {
		if abort, ok := err.(*AbortError); ok {
			*target = abort
			return true
		}
		type unwrapper interface{ Unwrap() error }
		next, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = next.Unwrap()
	}
	return false
}
