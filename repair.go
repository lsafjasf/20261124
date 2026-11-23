package fcpxml

import (
	"bytes"
	"fmt"
	"os"
)

const (
	RepairCandidatePreserved = "candidate_preserved"
	RepairSourceFallback     = "source_fallback"
	RepairBlocked            = "blocked"
)

type RepairReport struct {
	Status             string    `json:"status"`
	Strategy           string    `json:"strategy"`
	Output             string    `json:"output,omitempty"`
	Reimportable       bool      `json:"reimportable"`
	PreservesEffects   bool      `json:"preservesEffects"`
	DeletedEffectCount int       `json:"deletedEffectCount"`
	CandidateLossCount int       `json:"candidateLossCount"`
	UsedSourceBytes    bool      `json:"usedSourceBytes"`
	Notes              []string  `json:"notes,omitempty"`
	NonExpressible     []Finding `json:"nonExpressible,omitempty"`
}

type RepairResult struct {
	Report *RepairReport
	Data   []byte
}

func RepairDocuments(source, candidate *Document, comparison *Comparison) (*RepairResult, error) {
	if source == nil {
		return nil, fmt.Errorf("source document is nil")
	}
	if candidate == nil {
		candidate = source
	}
	if comparison == nil {
		var err error
		comparison, err = CompareDocuments(source, candidate)
		if err != nil {
			return nil, err
		}
	}
	sourceInventory := comparison.Source
	if sourceInventory == nil {
		var err error
		sourceInventory, err = source.Inventory()
		if err != nil {
			return nil, err
		}
	}

	result := &RepairResult{Report: &RepairReport{
		CandidateLossCount: len(comparison.Losses),
	}}
	var data []byte
	var strategy string
	var usedSource bool

	switch {
	case comparison.Compatible:
		data = documentBytes(candidate)
		strategy = RepairCandidatePreserved
		result.Report.Notes = append(result.Report.Notes,
			"candidate is semantically complete; its bytes are preserved without dropping effects")
	case !hasLoss(sourceInventory.Findings):
		data = documentBytes(source)
		strategy = RepairSourceFallback
		usedSource = true
		result.Report.Notes = append(result.Report.Notes,
			"candidate lost timeline semantics; conservative repair restores the source timeline",
			"the candidate was not allowed to pass by deleting transitions, retimes, or filters")
	default:
		strategy = RepairBlocked
		for _, finding := range sourceInventory.Findings {
			if finding.Severity == SeverityLoss {
				result.Report.NonExpressible = append(result.Report.NonExpressible, finding)
			}
		}
		result.Report.NonExpressible = append(result.Report.NonExpressible, comparison.Losses...)
		result.Report.Notes = append(result.Report.Notes,
			"source has structural errors, so no reimportable repair can be produced safely")
	}

	if strategy != RepairBlocked {
		if err := verifyRepairOutput(data, sourceInventory); err != nil {
			strategy = RepairBlocked
			data = nil
			result.Report.NonExpressible = append(result.Report.NonExpressible, Finding{
				Code:     "REPAIR_VERIFICATION_FAILED",
				Severity: SeverityLoss,
				Category: CategoryRepair,
				Message:  err.Error(),
			})
			result.Report.Notes = append(result.Report.Notes,
				"repair output failed reparse/coverage verification and was not emitted")
		}
	}

	result.Report.Strategy = strategy
	result.Report.UsedSourceBytes = usedSource
	if strategy == RepairBlocked {
		result.Report.Status = "blocked"
		result.Report.Reimportable = false
		result.Report.PreservesEffects = false
		return result, nil
	}
	result.Data = data
	result.Report.Status = "ready"
	result.Report.Reimportable = true
	result.Report.PreservesEffects = true
	result.Report.DeletedEffectCount = deletedEffectCount(sourceInventory, comparison.Candidate)
	return result, nil
}

func verifyRepairOutput(data []byte, source *Inventory) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("repair output is empty")
	}
	doc, err := ParseDocumentBytes(data)
	if err != nil {
		return fmt.Errorf("parse repair output: %w", err)
	}
	output, err := doc.Inventory()
	if err != nil {
		return fmt.Errorf("inventory repair output: %w", err)
	}
	if hasLoss(output.Findings) {
		return fmt.Errorf("repair output still has structural reference/time errors")
	}
	verification := CompareInventories(source, output)
	if !verification.Compatible {
		return fmt.Errorf("repair output does not cover the source timeline: %s", firstLossMessage(verification.Losses))
	}
	return nil
}

func firstLossMessage(losses []Finding) string {
	if len(losses) == 0 {
		return "unknown loss"
	}
	first := losses[0]
	return fmt.Sprintf("%s at %s: %s", first.Code, positionSummary(first.Position), first.Message)
}

func deletedEffectCount(source, candidate *Inventory) int {
	if source == nil || candidate == nil {
		return 0
	}
	return len(source.Effects) - matchedEffectCount(source, candidate)
}

func matchedEffectCount(source, candidate *Inventory) int {
	if len(candidate.Effects) == 0 {
		return 0
	}
	used := make(map[int]bool)
	matched := 0
	for _, effect := range source.Effects {
		match, ok := bestEffectMatch(effect, candidate.Effects, used)
		if ok {
			used[match.index] = true
			matched++
		}
	}
	return matched
}

func documentBytes(doc *Document) []byte {
	if len(doc.Raw) > 0 {
		return append([]byte(nil), doc.Raw...)
	}
	data, err := doc.Bytes()
	if err != nil {
		return nil
	}
	return data
}

func SaveRepair(result *RepairResult, path string) error {
	if result == nil || result.Report == nil {
		return fmt.Errorf("nil repair result")
	}
	if result.Report.Status != "ready" || len(result.Data) == 0 {
		return fmt.Errorf("repair is %s; no output written", result.Report.Strategy)
	}
	if err := os.WriteFile(path, result.Data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	result.Report.Output = path
	return nil
}
