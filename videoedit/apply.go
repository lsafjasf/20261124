package videoedit

import "fmt"

// Skip records a planned action that was intentionally not written, together
// with the reason (for example the user chose to keep a deleted parent).
type Skip struct {
	ClipID string
	Reason string
}

// Result summarizes one application run.
type Result struct {
	Applied   []PlannedAction
	Skipped   []Skip
	Conflicts []Conflict
	Journal   *Journal
}

// ApplyOptions configures an application run.
type ApplyOptions struct {
	Resolver Resolver
}

type applier struct {
	project    *Project
	baseline   *Project
	order      *ChangeOrder
	resolver   Resolver
	journal    *Journal
	sourceOpOf map[string]int

	// decisions gathered before any write, so conflict handling never mutates
	// the project and ordering of resolver calls stays deterministic.
	actions   map[string]PlannedAction
	skipped   map[string]string
	detached  map[string]bool
	conflicts []Conflict
}

// Apply runs a change order against the project. The project is mutated in
// place; on any failure or abort it is restored to its exact input state and
// the returned error explains why.
func Apply(project *Project, order *ChangeOrder, options ApplyOptions) (*Result, error) {
	if options.Resolver == nil {
		options.Resolver = AbortResolver{}
	}
	baseline := project.DeepClone()
	planned, err := Plan(project, order)
	if err != nil {
		return nil, err
	}

	app := &applier{
		project:    project,
		baseline:   baseline,
		order:      order,
		resolver:   options.Resolver,
		journal:    &Journal{OrderID: order.ID},
		sourceOpOf: firstSourceIndex(order),
		actions:    map[string]PlannedAction{},
		skipped:    map[string]string{},
		detached:   map[string]bool{},
	}
	for _, action := range planned {
		app.actions[action.ClipID] = action
	}

	conflicts := detectConflicts(project, planned)
	app.conflicts = conflicts
	for _, conflict := range conflicts {
		if err := app.resolveOne(conflict); err != nil {
			app.rollbackOrPanic(err)
			return nil, &ConflictError{Err: err}
		}
	}

	for _, action := range app.orderedActions(planned) {
		if _, skip := app.skipped[action.ClipID]; skip {
			continue
		}
		effective := action
		if err := app.writeOne(effective); err != nil {
			app.rollbackOrPanic(err)
			return nil, &ConflictError{Err: err}
		}
	}
	if err := project.Validate(); err != nil {
		app.rollbackOrPanic(err)
		return nil, &ConflictError{Err: fmt.Errorf("resulting project is invalid: %w", err)}
	}

	return app.buildResult(), nil
}

func firstSourceIndex(order *ChangeOrder) map[string]int {
	out := map[string]int{}
	for i, op := range order.Operations {
		if _, ok := out[op.ClipID]; !ok {
			out[op.ClipID] = i
		}
	}
	return out
}

func (a *applier) resolveOne(conflict Conflict) error {
	choice, err := a.resolver.Resolve(conflict)
	if err != nil {
		return fmt.Errorf("resolver failure on clip %q: %w", conflict.ChildClipIDs[0], err)
	}
	childID := conflict.ChildClipIDs[0]
	switch choice {
	case ChoiceAbort:
		return &AbortError{Reason: conflict.Message}
	case ChoiceSkipParent:
		if _, ok := a.actions[conflict.ParentClipID]; !ok {
			return &AbortError{Reason: "skipParent is not applicable to " + conflict.ParentClipID}
		}
		a.skipped[conflict.ParentClipID] = "skipped per conflict resolution: " + conflict.Message
		return nil
	case ChoiceDeleteChildren:
		for _, id := range conflict.ChildClipIDs {
			a.actions[id] = PlannedAction{ClipID: id, Delete: true}
		}
		return nil
	case ChoiceDetach:
		for _, id := range conflict.ChildClipIDs {
			a.detached[id] = true
			if _, ok := a.actions[id]; !ok {
				a.actions[id] = PlannedAction{ClipID: id}
			}
		}
		return nil
	case ChoiceCascade:
		if conflict.Kind != ConflictOverflow {
			return &AbortError{Reason: "cascade is only valid for overflow conflicts"}
		}
		parentAction, ok := a.actions[conflict.ParentClipID]
		if !ok {
			return &AbortError{Reason: "cascade requires an action on parent clip " + conflict.ParentClipID}
		}
		child, ok := a.actions[childID]
		if !ok {
			child = PlannedAction{ClipID: childID}
		}
		child.Head += parentAction.Head
		child.Tail += parentAction.Tail
		child.Move += parentAction.Move
		if err := validateBaseline(a.baseline, child); err != nil {
			return &AbortError{Reason: err.Error()}
		}
		a.actions[childID] = child
		return nil
	default:
		return &AbortError{Reason: "unknown resolution choice " + string(choice)}
	}
}
