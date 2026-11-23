package relink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func LoadProject(path string) (*Project, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read project: %w", err)
	}
	project, err := ParseProject(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse project %q: %w", path, err)
	}
	return project, data, nil
}

func ParseProject(data []byte) (*Project, error) {
	var project Project
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&project); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("project contains more than one JSON value")
		}
		return nil, fmt.Errorf("project has trailing data: %w", err)
	}
	if err := validateProject(&project); err != nil {
		return nil, err
	}
	return &project, nil
}

func validateProject(project *Project) error {
	if project == nil {
		return fmt.Errorf("project is nil")
	}
	if project.Version != ProjectVersion {
		return fmt.Errorf("unsupported project version %d", project.Version)
	}
	if !project.FrameRate.Valid() {
		return fmt.Errorf("invalid project frame rate %s", project.FrameRate)
	}
	if project.DurationMS < 0 {
		return fmt.Errorf("project duration must not be negative")
	}

	clipIDs := make(map[string]struct{}, len(project.Clips))
	for i := range project.Clips {
		clip := &project.Clips[i]
		if strings.TrimSpace(clip.ID) == "" {
			return fmt.Errorf("clip %d has an empty id", i)
		}
		if _, exists := clipIDs[clip.ID]; exists {
			return fmt.Errorf("duplicate clip id %q", clip.ID)
		}
		clipIDs[clip.ID] = struct{}{}
		if err := validateMediaRef(clip.ID, RolePrimary, &clip.Primary); err != nil {
			return err
		}
		if clip.Proxy != nil {
			if err := validateMediaRef(clip.ID, RoleProxy, clip.Proxy); err != nil {
				return err
			}
		}
	}

	itemIDs := make(map[string]struct{}, len(project.Timeline))
	for i := range project.Timeline {
		item := project.Timeline[i]
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("timeline item %d has an empty id", i)
		}
		if _, exists := itemIDs[item.ID]; exists {
			return fmt.Errorf("duplicate timeline item id %q", item.ID)
		}
		itemIDs[item.ID] = struct{}{}
		if _, exists := clipIDs[item.ClipID]; !exists {
			return fmt.Errorf("timeline item %q references unknown clip %q", item.ID, item.ClipID)
		}
		if item.StartFrame < 0 || item.EndFrame <= item.StartFrame {
			return fmt.Errorf("timeline item %q has an invalid frame range", item.ID)
		}
		if item.SourceInFrame < 0 {
			return fmt.Errorf("timeline item %q has a negative source in-point", item.ID)
		}
	}
	return nil
}

func validateMediaRef(clipID string, role Role, ref *MediaRef) error {
	if ref == nil {
		return fmt.Errorf("clip %q %s is nil", clipID, role)
	}
	if strings.TrimSpace(ref.Path) == "" {
		return fmt.Errorf("clip %q %s has an empty path", clipID, role)
	}
	if !ref.Fingerprint.FrameRate.Valid() {
		return fmt.Errorf("clip %q %s has an invalid frame rate", clipID, role)
	}
	if ref.Fingerprint.DurationMS <= 0 {
		return fmt.Errorf("clip %q %s has a non-positive duration", clipID, role)
	}
	if ref.Fingerprint.Size < 0 {
		return fmt.Errorf("clip %q %s has a negative size", clipID, role)
	}
	return nil
}

// projectSkeleton excludes every path that relinking is allowed to change.
// Comparing skeletons before and after a transaction makes timeline or timing
// edits visible instead of silently accepting them.
func projectSkeleton(project *Project) ([]byte, error) {
	copyProject := *project
	copyProject.Clips = append([]Clip(nil), project.Clips...)
	for i := range copyProject.Clips {
		copyProject.Clips[i].Primary.Path = ""
		if copyProject.Clips[i].Proxy != nil {
			proxy := *copyProject.Clips[i].Proxy
			proxy.Path = ""
			copyProject.Clips[i].Proxy = &proxy
		}
	}
	return json.Marshal(copyProject)
}

func timelineJSON(project *Project) ([]byte, error) {
	return json.Marshal(project.Timeline)
}

func ensureSkeletonEqual(before, after *Project) error {
	beforeSkeleton, err := projectSkeleton(before)
	if err != nil {
		return fmt.Errorf("serialize original project: %w", err)
	}
	afterSkeleton, err := projectSkeleton(after)
	if err != nil {
		return fmt.Errorf("serialize relinked project: %w", err)
	}
	if string(beforeSkeleton) != string(afterSkeleton) {
		return fmt.Errorf("timeline or project content changed during relink")
	}
	beforeTimeline, err := timelineJSON(before)
	if err != nil {
		return fmt.Errorf("serialize original timeline: %w", err)
	}
	afterTimeline, err := timelineJSON(after)
	if err != nil {
		return fmt.Errorf("serialize relinked timeline: %w", err)
	}
	if string(beforeTimeline) != string(afterTimeline) {
		return fmt.Errorf("timeline changed during relink")
	}
	return nil
}
