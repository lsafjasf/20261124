package videoedit

import "testing"

// failingResolver returns a resolver error while conflicts are being handled,
// proving the project stays untouched even when the failure happens before
// any project write.
type failingResolver struct{}

func (failingResolver) Resolve(Conflict) (Choice, error) {
	return ChoiceAbort, errResolverBlewUp
}

var errResolverBlewUp = &AbortError{Reason: "simulated resolver failure"}

func TestResolverFailureLeavesProjectUntouched(t *testing.T) {
	project := sampleProject()
	baseline := project.DeepClone()
	order := &ChangeOrder{ID: "r1", Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(-800)},
	}}
	_, err := Apply(project, order, ApplyOptions{Resolver: failingResolver{}})
	if err == nil {
		t.Fatal("expected resolver failure")
	}
	if !project.Equal(baseline) {
		t.Fatal("project changed although resolver failed")
	}
}

func TestOverlapRejectedAndProjectUntouched(t *testing.T) {
	project := sampleProject()
	baseline := project.DeepClone()
	// Extending c1 into c2's slot makes the track overlap; planning must
	// reject it before writing anything.
	order := &ChangeOrder{ID: "r2", Operations: []Operation{
		{ClipID: "c1", Action: ActionTrimEnd, Delta: ms(500)},
	}}
	_, err := Apply(project, order, ApplyOptions{Resolver: AbortResolver{}})
	if err == nil {
		t.Fatal("expected overlap rejection")
	}
	if !project.Equal(baseline) {
		t.Fatal("project changed although order was rejected")
	}
}

func TestDeleteAndTrimOnAlreadyMissingClipIsNoHardApply(t *testing.T) {
	project := sampleProject()
	baseline := project.DeepClone()
	order := &ChangeOrder{ID: "r3", Operations: []Operation{
		{ClipID: "gone", Action: ActionTrimStart, Delta: ms(100)},
		{ClipID: "gone", Action: ActionDelete},
	}}
	_, err := Apply(project, order, ApplyOptions{Resolver: AbortResolver{}})
	if err == nil {
		t.Fatal("expected unknown-clip rejection instead of hard apply")
	}
	if !project.Equal(baseline) {
		t.Fatal("project changed although target clip does not exist")
	}
}
