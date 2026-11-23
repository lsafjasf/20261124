package relink

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fileEntry struct {
	path     string
	relPath  string
	parts    []string
	basename string
}

type matchFailure struct {
	path string
	err  error
}

func discoverFiles(roots []string) ([]fileEntry, error) {
	seen := make(map[string]struct{})
	var files []fileEntry

	for _, root := range roots {
		root = filepath.Clean(root)
		info, err := os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("scan root %q: %w", root, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("scan root %q is not a directory", root)
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != root && strings.HasPrefix(entry.Name(), ".relink-") {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() || strings.HasSuffix(entry.Name(), MetadataSuffix) {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if _, exists := seen[abs]; exists {
				return nil
			}
			seen[abs] = struct{}{}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relParts := normalizedParts(rel)
			if len(relParts) == 0 {
				return nil
			}
			files = append(files, fileEntry{
				path:     abs,
				relPath:  strings.Join(relParts, "/"),
				parts:    relParts,
				basename: relParts[len(relParts)-1],
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan root %q: %w", root, err)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].path < files[j].path
	})
	return files, nil
}

func normalizedParts(value string) []string {
	value = strings.ReplaceAll(value, "\\", "/")
	raw := strings.Split(value, "/")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		if part == "" || part == "." || part == ".." {
			continue
		}
		if len(part) == 2 && part[1] == ':' {
			continue
		}
		parts = append(parts, strings.ToLower(part))
	}
	return parts
}

func matchAsset(
	clipID string,
	clipName string,
	role Role,
	ref MediaRef,
	files []fileEntry,
	probe Probe,
	cache map[string]probeResult,
	tol tolerances,
) (Proposal, error) {
	expectedParts := normalizedParts(ref.Path)
	if len(expectedParts) == 0 {
		return Proposal{}, fmt.Errorf("clip %q %s has no usable path", clipID, role)
	}
	wantBase := expectedParts[len(expectedParts)-1]

	var failures []matchFailure
	var candidates []Candidate
	for _, file := range files {
		if file.basename != wantBase {
			continue
		}
		result, ok := cache[file.path]
		if !ok {
			actual, err := probe.Probe(file.path)
			result = probeResult{info: actual, err: err}
			cache[file.path] = result
		}
		if result.err != nil {
			failures = append(failures, matchFailure{path: file.path, err: result.err})
			continue
		}
		_, err := compareFingerprint(ref.Fingerprint, result.info, tol)
		if err != nil {
			failures = append(failures, matchFailure{path: file.path, err: err})
			continue
		}
		candidates = append(candidates, Candidate{
			Path:    file.path,
			RelPath: file.relPath,
			Info:    result.info,
			Score:   scoreCandidate(expectedParts, file, ref.Fingerprint, result.info),
		})
	}

	if len(candidates) == 0 {
		return Proposal{}, noCandidateError(clipID, role, wantBase, failures)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Path < candidates[j].Path
	})
	confidence := ConfidenceHigh
	reason := "unique fingerprint and metadata match"
	if len(candidates) > 1 {
		confidence = ConfidenceLow
		reason = fmt.Sprintf("%d matching files are relinkable; choose one explicitly", len(candidates))
	}
	return Proposal{
		ClipID:       clipID,
		ClipName:     clipName,
		Role:         role,
		OldPath:      ref.Path,
		Selected:     candidates[0],
		Alternatives: append([]Candidate(nil), candidates...),
		Confidence:   confidence,
		Reason:       reason,
	}, nil
}

type probeResult struct {
	info Fingerprint
	err  error
}

func scoreCandidate(expectedParts []string, file fileEntry, expected, actual Fingerprint) int {
	score := 100
	if expected.Size > 0 && expected.Size == actual.Size {
		score += 20
	}
	if expected.SHA256 != "" {
		score += 200
	}
	common := commonSuffix(expectedParts[:len(expectedParts)-1], file.parts[:len(file.parts)-1])
	score += common * 10
	return score
}

func commonSuffix(left, right []string) int {
	common := 0
	for i, j := len(left)-1, len(right)-1; i >= 0 && j >= 0; i, j = i-1, j-1 {
		if left[i] != right[j] {
			break
		}
		common++
	}
	return common
}

func noCandidateError(clipID string, role Role, basename string, failures []matchFailure) error {
	detail := fmt.Sprintf("clip %q %s: no valid candidate named %q", clipID, role, basename)
	if len(failures) == 0 {
		return fmt.Errorf("%w: %s", ErrNoCandidates, detail)
	}
	reasons := make([]string, 0, len(failures))
	for _, failure := range failures {
		reasons = append(reasons, fmt.Sprintf("%s: %v", failure.path, failure.err))
	}
	return fmt.Errorf("%w: %w: %s (%s)",
		ErrNoCandidates, ErrCandidateInvalid, detail, strings.Join(reasons, " | "))
}
