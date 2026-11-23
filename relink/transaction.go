package relink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type Options struct {
	Roots                  []string
	Probe                  Probe
	Approver               Approver
	MissingProxy           MissingProxyPolicy
	DurationToleranceMS    int64
	FrameRateTolerance     float64
	AudioOffsetToleranceMS int64
	Store                  Store
}

type HighConfidenceApprover struct{}

func (HighConfidenceApprover) ApproveBatch(context.Context, []Proposal) (bool, error) {
	return true, nil
}

func (HighConfidenceApprover) ApproveOne(context.Context, Proposal) (Approval, error) {
	return Approval{Accept: false}, nil
}

type AcceptAll struct{}

func (AcceptAll) ApproveBatch(context.Context, []Proposal) (bool, error) {
	return true, nil
}

func (AcceptAll) ApproveOne(context.Context, Proposal) (Approval, error) {
	return Approval{Accept: true}, nil
}

type config struct {
	roots    []string
	probe    Probe
	approver Approver
	missing  MissingProxyPolicy
	tol      tolerances
	store    Store
}

func resolveOptions(options Options) (config, error) {
	if len(options.Roots) == 0 {
		return config{}, fmt.Errorf("at least one scan root is required")
	}
	cfg := config{
		roots:    append([]string(nil), options.Roots...),
		probe:    options.Probe,
		approver: options.Approver,
		missing:  options.MissingProxy,
		store:    options.Store,
		tol: tolerances{
			durationMS: 2,
			frameRate:  0.001,
			audioMS:    1,
		},
	}
	if cfg.probe == nil {
		cfg.probe = FilesystemProbe{}
	}
	if cfg.approver == nil {
		cfg.approver = HighConfidenceApprover{}
	}
	if cfg.missing == "" {
		cfg.missing = MissingProxyFail
	}
	if cfg.missing != MissingProxyFail && cfg.missing != MissingProxyKeep {
		return config{}, fmt.Errorf("invalid missing proxy policy %q", cfg.missing)
	}
	if cfg.store == nil {
		cfg.store = FileStore{}
	}
	if options.DurationToleranceMS > 0 {
		cfg.tol.durationMS = options.DurationToleranceMS
	}
	if options.FrameRateTolerance != 0 {
		cfg.tol.frameRate = options.FrameRateTolerance
	}
	if options.AudioOffsetToleranceMS > 0 {
		cfg.tol.audioMS = options.AudioOffsetToleranceMS
	}
	return cfg, nil
}

// Apply relinks a project as one transaction. Nothing is written until every
// high-confidence batch and every low-confidence item has been accepted and all
// selected media pass validation. The committed project is loaded and checked
// again; any failure restores the original bytes.
func Apply(ctx context.Context, projectPath string, options Options) (*Report, error) {
	cfg, err := resolveOptions(options)
	if err != nil {
		return nil, err
	}
	originalBytes, err := cfg.store.Load(projectPath)
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	original, err := ParseProject(originalBytes)
	if err != nil {
		return nil, fmt.Errorf("parse project: %w", err)
	}
	report := &Report{ProjectPath: projectPath}

	files, err := discoverFiles(cfg.roots)
	if err != nil {
		return report, err
	}
	proposals, err := collectProposals(original, files, cfg, report)
	if err != nil {
		return report, err
	}
	if len(proposals) == 0 {
		return report, nil
	}
	approved, err := obtainApproval(ctx, proposals, cfg.approver, report)
	if err != nil {
		return report, err
	}
	next := cloneProject(original)
	links := applyProposals(next, approved)
	if err := ensureSkeletonEqual(original, next); err != nil {
		return report, err
	}
	if _, err := validateLinks(next, links, cfg); err != nil {
		return report, fmt.Errorf("pre-commit validation failed: %w", err)
	}

	nextBytes, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return report, fmt.Errorf("serialize relinked project: %w", err)
	}
	nextBytes = append(nextBytes, '\n')
	if err := cfg.store.Save(projectPath, nextBytes); err != nil {
		rollbackErr := restoreProject(cfg.store, projectPath, originalBytes)
		return report, joinRollback(fmt.Errorf("save relinked project: %w", err), rollbackErr)
	}

	committedBytes, err := cfg.store.Load(projectPath)
	if err != nil {
		rollbackErr := restoreProject(cfg.store, projectPath, originalBytes)
		return report, joinRollback(fmt.Errorf("reload committed project: %w", err), rollbackErr)
	}
	committed, err := ParseProject(committedBytes)
	if err != nil {
		rollbackErr := restoreProject(cfg.store, projectPath, originalBytes)
		return report, joinRollback(fmt.Errorf("parse committed project: %w", err), rollbackErr)
	}
	if err := verifyCommitted(original, next, committed); err != nil {
		rollbackErr := restoreProject(cfg.store, projectPath, originalBytes)
		return report, joinRollback(err, rollbackErr)
	}
	checks, err := validateLinks(committed, links, cfg)
	if err != nil {
		rollbackErr := restoreProject(cfg.store, projectPath, originalBytes)
		return report, joinRollback(fmt.Errorf("post-commit validation failed: %w", err), rollbackErr)
	}

	report.Links = links
	report.Checks = checks
	return report, nil
}

func collectProposals(project *Project, files []fileEntry, cfg config, report *Report) ([]Proposal, error) {
	cache := make(map[string]probeResult)
	var proposals []Proposal
	for _, clip := range project.Clips {
		refs := []struct {
			role Role
			ref  *MediaRef
		}{
			{role: RolePrimary, ref: &clip.Primary},
		}
		if clip.Proxy != nil {
			refs = append(refs, struct {
				role Role
				ref  *MediaRef
			}{role: RoleProxy, ref: clip.Proxy})
		}
		for _, item := range refs {
			proposal, err := matchAsset(
				clip.ID, clip.Name, item.role, *item.ref, files, cfg.probe, cache, cfg.tol,
			)
			if err != nil {
				if item.role == RoleProxy && cfg.missing == MissingProxyKeep &&
					errors.Is(err, ErrNoCandidates) && !errors.Is(err, ErrCandidateInvalid) {
					report.Warnings = append(report.Warnings, Warning{
						Code:    "proxy_missing",
						ClipID:  clip.ID,
						Role:    RoleProxy,
						Message: err.Error(),
					})
					continue
				}
				return nil, fmt.Errorf("plan aborted: %w", err)
			}
			proposals = append(proposals, proposal)
		}
	}
	return proposals, nil
}

func obtainApproval(
	ctx context.Context,
	proposals []Proposal,
	approver Approver,
	report *Report,
) ([]Proposal, error) {
	var high, low []Proposal
	for _, proposal := range proposals {
		if proposal.Confidence == ConfidenceHigh {
			high = append(high, proposal)
		} else {
			low = append(low, proposal)
		}
	}

	accepted := make(map[string]Candidate, len(proposals))
	if len(high) > 0 {
		ok, err := approver.ApproveBatch(ctx, high)
		if err != nil {
			return nil, fmt.Errorf("confirm high-confidence batch: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("%w: high-confidence batch", ErrNotConfirmed)
		}
		for _, proposal := range high {
			accepted[proposalKey(proposal.ClipID, proposal.Role)] = proposal.Selected
		}
		report.BatchAccepted = len(high)
	}

	for _, proposal := range low {
		approval, err := approver.ApproveOne(ctx, proposal)
		if err != nil {
			return nil, fmt.Errorf("confirm clip %q %s: %w", proposal.ClipID, proposal.Role, err)
		}
		if !approval.Accept {
			return nil, fmt.Errorf("%w: clip %q %s", ErrNotConfirmed, proposal.ClipID, proposal.Role)
		}
		selected := proposal.Selected
		if approval.CandidatePath != "" {
			found := false
			for _, candidate := range proposal.Alternatives {
				if candidate.Path == approval.CandidatePath {
					selected = candidate
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("confirmed path %q is not a candidate for clip %q",
					approval.CandidatePath, proposal.ClipID)
			}
		}
		accepted[proposalKey(proposal.ClipID, proposal.Role)] = selected
		report.LowAccepted++
	}

	approved := make([]Proposal, 0, len(accepted))
	for _, proposal := range proposals {
		selected, ok := accepted[proposalKey(proposal.ClipID, proposal.Role)]
		if !ok {
			continue
		}
		proposal.Selected = selected
		approved = append(approved, proposal)
	}
	return approved, nil
}

func proposalKey(clipID string, role Role) string {
	return clipID + "\x00" + string(role)
}

func cloneProject(project *Project) *Project {
	clone := *project
	clone.Timeline = append([]TimelineItem(nil), project.Timeline...)
	clone.Clips = append([]Clip(nil), project.Clips...)
	for i := range clone.Clips {
		if project.Clips[i].Proxy != nil {
			proxy := *project.Clips[i].Proxy
			clone.Clips[i].Proxy = &proxy
		}
	}
	return &clone
}

func applyProposals(project *Project, proposals []Proposal) []Link {
	index := make(map[string]int, len(project.Clips))
	for i := range project.Clips {
		index[project.Clips[i].ID] = i
	}
	links := make([]Link, 0, len(proposals))
	for _, proposal := range proposals {
		i := index[proposal.ClipID]
		clip := &project.Clips[i]
		if proposal.Role == RolePrimary {
			clip.Primary.Path = proposal.Selected.Path
		} else {
			clip.Proxy.Path = proposal.Selected.Path
		}
		links = append(links, Link{
			ClipID:     proposal.ClipID,
			Role:       proposal.Role,
			From:       proposal.OldPath,
			To:         proposal.Selected.Path,
			Confidence: proposal.Confidence,
		})
	}
	return links
}

func validateLinks(project *Project, links []Link, cfg config) ([]Check, error) {
	checks := make([]Check, 0, len(links))
	for _, link := range links {
		ref := findMediaRef(project, link.ClipID, link.Role)
		if ref == nil {
			return nil, fmt.Errorf("clip %q %s disappeared from project", link.ClipID, link.Role)
		}
		actual, err := cfg.probe.Probe(ref.Path)
		if err != nil {
			return nil, fmt.Errorf("probe clip %q %s at %q: %w",
				link.ClipID, link.Role, ref.Path, err)
		}
		check, err := compareFingerprint(ref.Fingerprint, actual, cfg.tol)
		check.ClipID = link.ClipID
		check.Role = link.Role
		if err != nil {
			var validationErr *ValidationError
			if errors.As(err, &validationErr) {
				validationErr.ClipID = link.ClipID
				validationErr.Role = link.Role
			}
			return nil, err
		}
		checks = append(checks, check)
	}
	return checks, nil
}

func findMediaRef(project *Project, clipID string, role Role) *MediaRef {
	for i := range project.Clips {
		if project.Clips[i].ID != clipID {
			continue
		}
		if role == RolePrimary {
			return &project.Clips[i].Primary
		}
		return project.Clips[i].Proxy
	}
	return nil
}

func verifyCommitted(original, expected, committed *Project) error {
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	committedJSON, err := json.Marshal(committed)
	if err != nil {
		return err
	}
	if !bytes.Equal(expectedJSON, committedJSON) {
		return fmt.Errorf("committed project differs from validated plan")
	}
	if err := ensureSkeletonEqual(original, committed); err != nil {
		return fmt.Errorf("committed project %w", err)
	}
	return nil
}

func restoreProject(store Store, path string, original []byte) error {
	if err := store.Save(path, original); err != nil {
		return fmt.Errorf("restore original project: %w", err)
	}
	restored, err := store.Load(path)
	if err != nil {
		return fmt.Errorf("verify restored project: %w", err)
	}
	if !bytes.Equal(restored, original) {
		return fmt.Errorf("verify restored project: restored bytes differ from original")
	}
	return nil
}

func joinRollback(cause, rollback error) error {
	if rollback == nil {
		return cause
	}
	return errors.Join(cause, rollback)
}
