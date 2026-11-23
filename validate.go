package fcpxml

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Severity distinguishes a document error from a construct that is outside
// the analyzer's supported subset but does not by itself invalidate the file.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is a validation finding tied to an XML element.
type Issue struct {
	Severity Severity
	Code     string
	Path     string
	Message  string
}

func (i Issue) String() string {
	return fmt.Sprintf("%s: %s: %s", i.Severity, i.Path, i.Message)
}

// MissingReference identifies an unresolved FCPXML ID reference.
type MissingReference struct {
	Path      string
	Attribute string
	ID        string
}

// Cycle identifies a sequence-nesting cycle.
type Cycle struct {
	IDs   []string
	Paths []string
}

// ValidationReport contains all structural findings for a document.
type ValidationReport struct {
	Issues           []Issue
	MissingRefs      []MissingReference
	Cycles           []Cycle
	TopSequence      *Node
	TopSequenceID    string
	TopFrameDuration Time
}

// HasErrors reports whether the report contains any error-severity issue.
func (r *ValidationReport) HasErrors() bool {
	if r == nil {
		return true
	}
	for _, issue := range r.Issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

// ValidationError is returned when a document cannot be safely analyzed.
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "FCPXML validation failed"
	}
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		if issue.Severity == SeverityError {
			parts = append(parts, issue.String())
		}
	}
	if len(parts) == 0 {
		return "FCPXML validation failed"
	}
	return strings.Join(parts, "; ")
}

// Validate checks IDs, references, sequence cycles, times, formats, and the
// top-level sequence. It returns a report alongside an error when any
// error-severity issue is present.
func (d *Document) Validate() (*ValidationReport, error) {
	return d.validate("")
}

// ValidateSequence validates while selecting a specific sequence as the top.
func (d *Document) ValidateSequence(sequenceID string) (*ValidationReport, error) {
	return d.validate(sequenceID)
}

func (d *Document) validate(sequenceID string) (*ValidationReport, error) {
	report := &ValidationReport{}
	if d == nil || d.Root == nil {
		report.addError("document", "nil FCPXML document")
		return report, &ValidationError{Issues: report.Issues}
	}

	for _, duplicate := range d.Duplicate {
		report.addError(duplicate.Second.Path(), fmt.Sprintf(
			"duplicate id %q (first at %s)", duplicate.Value, duplicate.First.Path()))
	}
	d.validateTimes(report)
	d.validateReferences(report)
	d.validateRequiredReferences(report)
	d.detectCycles(report)

	top, err := d.TopSequence(sequenceID)
	if err != nil {
		report.addError("sequence", err.Error())
	} else {
		report.TopSequence = top
		report.TopSequenceID = top.Attr("id")
		frameDuration, err := d.SequenceFrameDuration(top)
		if err != nil {
			report.addError(top.Path(), err.Error())
		} else {
			report.TopFrameDuration = frameDuration
		}
	}

	if report.HasErrors() {
		return report, &ValidationError{Issues: report.Issues}
	}
	return report, nil
}

func (r *ValidationReport) addError(path, message string) {
	r.Issues = append(r.Issues, Issue{
		Severity: SeverityError, Code: "invalid-document", Path: path, Message: message,
	})
}

func (r *ValidationReport) add(code, path, message string) {
	r.Issues = append(r.Issues, Issue{
		Severity: SeverityError, Code: code, Path: path, Message: message,
	})
}

func (r *ValidationReport) warn(code, path, message string) {
	r.Issues = append(r.Issues, Issue{
		Severity: SeverityWarning, Code: code, Path: path, Message: message,
	})
}

func (d *Document) validateTimes(report *ValidationReport) {
	timeAttrs := map[string]bool{
		"offset": true, "start": true, "duration": true, "tcStart": true,
		"audioStart": true, "audioDuration": true, "frameDuration": true,
	}
	_ = d.Root.Walk(func(node *Node) error {
		for _, attr := range node.Attrs {
			if !timeAttrs[attr.Name.Local] || attr.Value == "" {
				continue
			}
			value, err := ParseTime(attr.Value)
			if err != nil {
				report.add("invalid-time", node.Path(), fmt.Sprintf(
					"%s=%q: %v", attr.Name.Local, attr.Value, err))
				continue
			}
			if (attr.Name.Local == "duration" || attr.Name.Local == "audioDuration" ||
				attr.Name.Local == "frameDuration") && value.Num <= 0 {
				report.add("invalid-duration", node.Path(), fmt.Sprintf(
					"%s=%q must be positive", attr.Name.Local, attr.Value))
			}
		}
		return nil
	})
}

func (d *Document) validateReferences(report *ValidationReport) {
	refAttrs := map[string]bool{"ref": true, "format": true, "dataLocator": true}
	_ = d.Root.Walk(func(node *Node) error {
		for _, attr := range node.Attrs {
			if !refAttrs[attr.Name.Local] || attr.Value == "" {
				continue
			}
			if _, exists := d.IDs[attr.Value]; exists {
				continue
			}
			report.MissingRefs = append(report.MissingRefs, MissingReference{
				Path: node.Path(), Attribute: attr.Name.Local, ID: attr.Value,
			})
			report.add("missing-reference", node.Path(), fmt.Sprintf(
				"%s=%q does not resolve to an id", attr.Name.Local, attr.Value))
		}
		return nil
	})
}

func (d *Document) validateRequiredReferences(report *ValidationReport) {
	requiresRef := map[string]bool{
		"asset-clip": true, "ref-clip": true, "video": true, "audio": true,
		"mc-clip": true, "filter-video": true, "filter-audio": true,
	}
	_ = d.Root.Walk(func(node *Node) error {
		if node.Name.Local == "sequence" && node.Attr("format") == "" {
			report.warn("missing-required-reference", node.Path(),
				"sequence requires a format reference")
		}
		if (node.Name.Local == "asset" || node.Name.Local == "media") &&
			node.Attr("format") == "" {
			report.warn("missing-required-reference", node.Path(),
				fmt.Sprintf("<%s> requires a format reference", node.Name.Local))
		}
		if node.Name.Local == "format" && node.Attr("frameDuration") == "" {
			report.warn("missing-required-attribute", node.Path(),
				"format requires a frameDuration")
		}
		if requiresRef[node.Name.Local] && node.Attr("ref") == "" {
			report.warn("missing-required-reference", node.Path(),
				fmt.Sprintf("<%s> requires a ref reference", node.Name.Local))
		}
		if node.Name.Local == "transition" && node.Attr("duration") == "" {
			report.warn("missing-required-reference", node.Path(),
				"transition requires a duration")
		}
		switch node.Name.Local {
		case "timeMap", "conform-rate":
			report.warn("unsupported-retiming", node.Path(),
				fmt.Sprintf("<%s> retiming is outside the 1:1 supported subset", node.Name.Local))
		}
		return nil
	})
}

func (d *Document) detectCycles(report *ValidationReport) {
	sequences := d.AllSequences()
	graph := make(map[*Node][]*Node, len(sequences))
	for _, sequence := range sequences {
		graph[sequence] = nil
	}
	for _, sequence := range sequences {
		directChildren := sequence.ChildrenNamed("spine")
		for _, spine := range directChildren {
			var addNested func(*Node)
			addNested = func(node *Node) {
				for _, child := range node.Children {
					if child.Name.Local == "sequence" {
						graph[sequence] = appendUniqueNode(graph[sequence], child)
						continue
					}
					addNested(child)
				}
			}
			addNested(spine)
		}
		_ = sequence.Walk(func(node *Node) error {
			if node == sequence {
				return nil
			}
			ref := node.Attr("ref")
			if ref == "" {
				return nil
			}
			target := d.IDs[ref]
			if target == nil {
				return nil
			}
			for _, targetSequence := range sequencesForTarget(target) {
				if owner := owningSequence(node); owner != nil {
					graph[owner] = appendUniqueNode(graph[owner], targetSequence)
				}
			}
			return nil
		})
	}

	state := make(map[*Node]int, len(sequences))
	stack := make([]*Node, 0, 8)
	var visit func(*Node)
	visit = func(node *Node) {
		state[node] = 1
		stack = append(stack, node)
		for _, next := range graph[node] {
			switch state[next] {
			case 0:
				visit(next)
			case 1:
				index := 0
				for index < len(stack) && stack[index] != next {
					index++
				}
				cycleNodes := append([]*Node(nil), stack[index:]...)
				cycleNodes = append(cycleNodes, next)
				cycle := Cycle{}
				for _, cycleNode := range cycleNodes {
					cycle.Paths = append(cycle.Paths, cycleNode.Path())
					if id := cycleNode.Attr("id"); id != "" {
						cycle.IDs = append(cycle.IDs, id)
					} else {
						cycle.IDs = append(cycle.IDs, "<anonymous>")
					}
				}
				report.Cycles = append(report.Cycles, cycle)
				report.add("circular-nesting", next.Path(), "circular sequence nesting: "+
					strings.Join(cycle.IDs, " -> "))
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = 2
	}
	for _, sequence := range sequences {
		if state[sequence] == 0 {
			visit(sequence)
		}
	}
}

func appendUniqueNode(nodes []*Node, node *Node) []*Node {
	for _, existing := range nodes {
		if existing == node {
			return nodes
		}
	}
	return append(nodes, node)
}

func sequencesForTarget(target *Node) []*Node {
	if target == nil {
		return nil
	}
	if target.Name.Local == "sequence" {
		return []*Node{target}
	}
	result := make([]*Node, 0, 2)
	_ = target.Walk(func(node *Node) error {
		if node.Name.Local == "sequence" {
			result = append(result, node)
		}
		return nil
	})
	return result
}

func owningSequence(node *Node) *Node {
	for current := node.Parent(); current != nil; current = current.Parent() {
		if current.Name.Local == "sequence" {
			return current
		}
	}
	return nil
}

// AllSequences returns sequences in document order.
func (d *Document) AllSequences() []*Node {
	if d == nil || d.Root == nil {
		return nil
	}
	result := make([]*Node, 0, 8)
	_ = d.Root.Walk(func(node *Node) error {
		if node.Name.Local == "sequence" {
			result = append(result, node)
		}
		return nil
	})
	return result
}

// TopSequence selects the project sequence, or one explicitly named by ID.
func (d *Document) TopSequence(sequenceID string) (*Node, error) {
	if d == nil || d.Root == nil {
		return nil, fmt.Errorf("nil FCPXML document")
	}
	if sequenceID != "" {
		node := d.IDs[sequenceID]
		if node == nil {
			return nil, fmt.Errorf("top sequence %q does not resolve", sequenceID)
		}
		if node.Name.Local != "sequence" {
			return nil, fmt.Errorf("top sequence %q is <%s>, not <sequence>", sequenceID, node.Name.Local)
		}
		return node, nil
	}

	var projects []*Node
	_ = d.Root.Walk(func(node *Node) error {
		if node.Name.Local == "project" {
			projects = append(projects, node)
		}
		return nil
	})
	candidates := make([]*Node, 0, len(projects))
	for _, project := range projects {
		for _, child := range project.Children {
			if child.Name.Local == "sequence" {
				candidates = append(candidates, child)
			}
		}
	}
	if len(candidates) == 0 {
		allSequences := d.AllSequences()
		for _, sequence := range allSequences {
			if !isInsideName(sequence, "resources") && owningSequence(sequence) == nil {
				candidates = append(candidates, sequence)
			}
		}
		if len(candidates) == 0 && len(allSequences) == 1 {
			candidates = append(candidates, allSequences[0])
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no top-level project sequence found")
	}
	if len(candidates) > 1 {
		ids := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.Attr("id"))
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("multiple top-level sequences (%s); select one by id",
			strings.Join(ids, ", "))
	}
	return candidates[0], nil
}

func isInsideName(node *Node, name string) bool {
	for current := node; current != nil; current = current.Parent() {
		if current.Name.Local == name {
			return true
		}
	}
	return false
}

// SequenceFrameDuration resolves a sequence's format resource.
func (d *Document) SequenceFrameDuration(sequence *Node) (Time, error) {
	if sequence == nil {
		return Time{}, errors.New("nil sequence")
	}
	formatID := sequence.Attr("format")
	if formatID == "" {
		return Time{}, fmt.Errorf("%s has no format reference", sequence.Path())
	}
	format := d.IDs[formatID]
	if format == nil {
		return Time{}, fmt.Errorf("%s format %q is missing", sequence.Path(), formatID)
	}
	if format.Name.Local != "format" {
		return Time{}, fmt.Errorf("%s format %q is <%s>, not <format>",
			sequence.Path(), formatID, format.Name.Local)
	}
	value := format.Attr("frameDuration")
	if value == "" {
		return Time{}, fmt.Errorf("%s has no frameDuration", format.Path())
	}
	duration, err := ParseTime(value)
	if err != nil {
		return Time{}, fmt.Errorf("%s frameDuration: %w", format.Path(), err)
	}
	if duration.Num <= 0 {
		return Time{}, fmt.Errorf("%s frameDuration must be positive", format.Path())
	}
	return duration, nil
}

func parseNodeTime(node *Node, attr string, fallback Time) (Time, error) {
	value := node.Attr(attr)
	if value == "" {
		return fallback, nil
	}
	parsed, err := ParseTime(value)
	if err != nil {
		return Time{}, fmt.Errorf("%s %s=%q: %w", node.Path(), attr, value, err)
	}
	return parsed, nil
}

func parseLane(node *Node) int {
	value := node.Attr("lane")
	if value == "" {
		return 0
	}
	lane, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return lane
}
