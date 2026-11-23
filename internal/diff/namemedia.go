package diff

import (
	"fmt"
	"sort"

	"ediff/internal/project"
)

// detectNameMediaConflicts 检测“同名异媒”：
// 合并两份工程后，若同一个片段名映射到多个不同 mediaId，则必须人工裁决。
// 同时覆盖单份工程内部与跨工程两种情况。
func (e *Engine) detectNameMediaConflicts() {
	type slot struct {
		mediaID string
		side    string
		clipID  string
	}
	byName := make(map[string]map[string]slot)
	// 去重键同时带上 side/clipID/mediaID，避免重复片段产生重复描述。

	add := func(side string, l *project.Loaded) {
		for _, fc := range l.Clips {
			c := fc.Clip
			if c.Name == "" || c.MediaID == "" {
				continue
			}
			if byName[c.Name] == nil {
				byName[c.Name] = make(map[string]slot)
			}
			key := side + ":" + c.MediaID
			if _, seen := byName[c.Name][key]; !seen {
				byName[c.Name][key] = slot{mediaID: c.MediaID, side: side, clipID: c.ID}
			}
		}
	}
	add("base", e.base)
	add("target", e.target)

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		mediaSet := make(map[string]bool)
		var slots []slot
		for _, s := range byName[name] {
			mediaSet[s.mediaID] = true
			slots = append(slots, s)
		}
		if len(mediaSet) <= 1 {
			continue
		}
		sort.Slice(slots, func(i, j int) bool {
			if slots[i].side != slots[j].side {
				return slots[i].side < slots[j].side
			}
			return slots[i].mediaID < slots[j].mediaID
		})
		ids := ""
		for i, s := range slots {
			if i > 0 {
				ids += "; "
			}
			ids += fmt.Sprintf("%s/%s->%s", s.side, s.clipID, s.mediaID)
		}
		e.report.Conflicts = append(e.report.Conflicts, Conflict{
			Kind:   "NAME_MEDIA_CONFLICT",
			ID:     name,
			Side:   "both",
			Detail: "same clip name bound to different media: " + ids,
		})
	}
}
