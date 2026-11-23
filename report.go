package fcpxml

import (
	"encoding/json"
	"fmt"
	"io"
)

const ReportSchemaVersion = 1

type Summary struct {
	SourceTracks         int    `json:"sourceTracks"`
	CandidateTracks      int    `json:"candidateTracks"`
	SourceItems          int    `json:"sourceItems"`
	CandidateItems       int    `json:"candidateItems"`
	SourceEffects        int    `json:"sourceEffects"`
	CandidateEffects     int    `json:"candidateEffects"`
	SourceTransitions    int    `json:"sourceTransitions"`
	CandidateTransitions int    `json:"candidateTransitions"`
	SourceRetimes        int    `json:"sourceRetimes"`
	CandidateRetimes     int    `json:"candidateRetimes"`
	SourceMediaRefs      int    `json:"sourceMediaReferences"`
	CandidateMediaRefs   int    `json:"candidateMediaReferences"`
	LossCount            int    `json:"lossCount"`
	WarningCount         int    `json:"warningCount"`
	LostTracks           int    `json:"lostTracks"`
	LostItems            int    `json:"lostItems"`
	LostEffects          int    `json:"lostEffects"`
	LostMediaRefs        int    `json:"lostMediaReferences"`
	EarliestSeconds      string `json:"earliestLossSeconds,omitempty"`
	EarliestFrame        string `json:"earliestLossFrame,omitempty"`
	EarliestFrameRound   int64  `json:"earliestLossFrameRounded,omitempty"`
	ConversionSucceeded  bool   `json:"conversionSucceeded"`
	DeletedEffects       int    `json:"deletedEffects"`
}

type Report struct {
	SchemaVersion   int           `json:"schemaVersion"`
	Status          string        `json:"status"`
	SourcePath      string        `json:"sourcePath,omitempty"`
	CandidatePath   string        `json:"candidatePath,omitempty"`
	Summary         Summary       `json:"summary"`
	Source          *Inventory    `json:"source"`
	Candidate       *Inventory    `json:"candidate,omitempty"`
	Losses          []Finding     `json:"losses,omitempty"`
	Warnings        []Finding     `json:"warnings,omitempty"`
	SourceIssues    []Finding     `json:"sourceIssues,omitempty"`
	CandidateIssues []Finding     `json:"candidateIssues,omitempty"`
	EarliestLoss    *LossPoint    `json:"earliestLoss,omitempty"`
	Repair          *RepairReport `json:"repair,omitempty"`
	Policy          string        `json:"policy"`
}

func AuditDocuments(source, candidate *Document) (*Report, error) {
	if source == nil {
		return nil, fmt.Errorf("source document is nil")
	}
	sourceInventory, err := source.Inventory()
	if err != nil {
		return nil, err
	}
	report := &Report{
		SchemaVersion: ReportSchemaVersion,
		SourcePath:    source.Path,
		Source:        sourceInventory,
		Policy:        "effects and transitions are never deleted to make a conversion pass",
	}
	if candidate == nil {
		report.Status = "compatible"
		if hasLoss(sourceInventory.Findings) {
			report.Status = "invalid"
		}
		for _, finding := range sourceInventory.Findings {
			if finding.Severity == SeverityLoss {
				report.SourceIssues = append(report.SourceIssues, finding)
			} else if finding.Severity == SeverityWarning {
				report.Warnings = append(report.Warnings, finding)
			}
		}
		fillInventorySummary(&report.Summary, sourceInventory, nil)
		report.Summary.WarningCount = len(report.Warnings)
		report.Summary.ConversionSucceeded = report.Status == "compatible"
		return report, nil
	}

	candidateInventory, err := candidate.Inventory()
	if err != nil {
		return nil, err
	}
	comparison := CompareInventories(sourceInventory, candidateInventory)
	report.CandidatePath = candidate.Path
	report.Candidate = candidateInventory
	report.Status = comparison.Status
	report.Losses = comparison.Losses
	report.Warnings = combinedWarnings(comparison)
	report.SourceIssues = comparison.SourceIssues
	report.CandidateIssues = comparison.CandidateIssues
	report.EarliestLoss = comparison.EarliestLoss
	fillComparisonSummary(&report.Summary, comparison)
	return report, nil
}

func AuditAndRepair(source, candidate *Document) (*Report, *RepairResult, error) {
	if source == nil {
		return nil, nil, fmt.Errorf("source document is nil")
	}
	if candidate == nil {
		candidate = source
	}
	comparison, err := CompareDocuments(source, candidate)
	if err != nil {
		return nil, nil, err
	}
	repair, err := RepairDocuments(source, candidate, comparison)
	if err != nil {
		return nil, nil, err
	}
	report, err := AuditDocuments(source, candidate)
	if err != nil {
		return nil, nil, err
	}
	report.Repair = repair.Report
	report.Summary.DeletedEffects = repair.Report.DeletedEffectCount
	if repair.Report.Status == "blocked" {
		report.Status = "invalid"
		report.Summary.ConversionSucceeded = false
	}
	return report, repair, nil
}

func fillInventorySummary(summary *Summary, source, candidate *Inventory) {
	summary.SourceTracks = len(source.Tracks)
	summary.SourceItems = len(source.Items)
	summary.SourceEffects = len(source.Effects)
	summary.SourceTransitions = countEffectKind(source, "transition")
	summary.SourceRetimes = countEffectKind(source, "time-map")
	summary.SourceMediaRefs = len(source.MediaRefs)
	if candidate != nil {
		summary.CandidateTracks = len(candidate.Tracks)
		summary.CandidateItems = len(candidate.Items)
		summary.CandidateEffects = len(candidate.Effects)
		summary.CandidateTransitions = countEffectKind(candidate, "transition")
		summary.CandidateRetimes = countEffectKind(candidate, "time-map")
		summary.CandidateMediaRefs = len(candidate.MediaRefs)
	}
}

func fillComparisonSummary(summary *Summary, comparison *Comparison) {
	fillInventorySummary(summary, comparison.Source, comparison.Candidate)
	summary.LossCount = len(comparison.Losses)
	summary.WarningCount = len(combinedWarnings(comparison))
	summary.LostTracks = comparison.LostTrackCount
	summary.LostItems = comparison.LostItemCount
	summary.LostEffects = comparison.LostEffectCount
	summary.LostMediaRefs = comparison.LostMediaRefCount
	summary.ConversionSucceeded = comparison.Compatible && !hasLoss(comparison.SourceIssues)
	if comparison.EarliestLoss != nil && comparison.EarliestLoss.Position != nil {
		position := comparison.EarliestLoss.Position
		summary.EarliestSeconds = position.Seconds
		summary.EarliestFrame = position.FrameExact
		summary.EarliestFrameRound = position.FrameRounded
	}
}

func combinedWarnings(comparison *Comparison) []Finding {
	seen := make(map[string]bool)
	var warnings []Finding
	add := func(findings []Finding) {
		for _, finding := range findings {
			if finding.Severity != SeverityWarning {
				continue
			}
			key := finding.Code + "|" + finding.Path + "|" + finding.TargetPath + "|" + finding.Message
			if seen[key] {
				continue
			}
			seen[key] = true
			warnings = append(warnings, finding)
		}
	}
	add(comparison.Warnings)
	add(comparison.SourceIssues)
	add(comparison.CandidateIssues)
	return warnings
}

func countEffectKind(inventory *Inventory, kind string) int {
	count := 0
	for _, effect := range inventory.Effects {
		if effect.Kind == kind {
			count++
		}
	}
	return count
}

func WriteReport(w io.Writer, report *Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode audit report: %w", err)
	}
	return nil
}

func ReportBytes(report *Report) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode audit report: %w", err)
	}
	return append(data, '\n'), nil
}
