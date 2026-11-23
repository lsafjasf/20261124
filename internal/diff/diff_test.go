package diff

import (
	"strings"
	"testing"

	"ediff/internal/project"
)

func boolp(b bool) *bool { return &b }

func loadProjects(t *testing.T, base, target *project.Project) (*project.Loaded, *project.Loaded) {
	t.Helper()
	bp := t.TempDir() + "/base.json"
	tp := t.TempDir() + "/target.json"
	mustWriteProject(t, bp, base)
	mustWriteProject(t, tp, target)
	b, err := project.LoadFile(bp)
	if err != nil {
		t.Fatalf("load base: %v", err)
	}
	tg, err := project.LoadFile(tp)
	if err != nil {
		t.Fatalf("load target: %v", err)
	}
	return b, tg
}

func findChange(t *testing.T, r *Report, op, id string) Change {
	t.Helper()
	for _, c := range r.Changes {
		if c.Op == op && c.ID == id {
			return c
		}
	}
	t.Fatalf("change %s %s not found; have %+v", op, id, r.Changes)
	return Change{}
}

func hasConflict(r *Report, kind, id string) bool {
	for _, c := range r.Conflicts {
		if c.Kind == kind && c.ID == id {
			return true
		}
	}
	return false
}

func fieldPaths(c Change) []string {
	out := make([]string, 0, len(c.Fields))
	for _, f := range c.Fields {
		out = append(out, f.Path)
	}
	return out
}

func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

// TestSameIDMove 必测 1：同 ID 仅时间线位置变化 => 一次 clip.move，
// 不得拆成 remove+add。
func TestSameIDMove(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	target.Tracks[0].Clips[0].TimelineIn = 100

	r := Compare(mustLoad(t, base), mustLoad(t, target))

	c := findChange(t, r, "clip.move", "c1")
	if len(c.Fields) != 1 || c.Fields[0].Path != "timelineIn" {
		t.Fatalf("move should carry only timelineIn, got %+v", c.Fields)
	}
	for _, c := range r.Changes {
		if (c.Op == "clip.remove" || c.Op == "clip.add") && c.ID == "c1" {
			t.Fatalf("move must not be reported as %s", c.Op)
		}
	}
	if r.Summary.ClipsMoved != 1 {
		t.Fatalf("want 1 move, got %d", r.Summary.ClipsMoved)
	}
}

// TestMoveAcrossNestedTracks 必测 1/4：同 ID 跨嵌套轨道移动。
func TestMoveAcrossNestedTracks(t *testing.T) {
	base := nestedProject()
	target := nestedProject()
	// 把 c1 从 V1/A1 移到 V1/A2
	target.Tracks[0].Children[0].Clips = nil
	target.Tracks[0].Children[1].Clips = append(
		target.Tracks[0].Children[1].Clips, base.Tracks[0].Children[0].Clips[0])

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	c := findChange(t, r, "clip.move", "c1")
	if c.FromTrack != "V1/A1" || c.ToTrack != "V1/A2" {
		t.Fatalf("unexpected move tracks %q -> %q", c.FromTrack, c.ToTrack)
	}
}

// TestDuplicateID 必测 2：单份工程内片段 ID 重复 => 冲突。
func TestDuplicateID(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	target.Tracks[0].Clips = append(target.Tracks[0].Clips, project.Clip{
		ID: "c1", Name: "dup", MediaID: "m1", TimelineIn: 50, Speed: 1,
	})

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	if !hasConflict(r, "DUP_CLIP_ID", "c1") {
		t.Fatalf("expected DUP_CLIP_ID conflict, got %+v", r.Conflicts)
	}
}

// TestDuplicateTrackID 必测 2 延伸：轨道 ID 重复 => 冲突。
func TestDuplicateTrackID(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	target.Tracks = append(target.Tracks, project.Track{ID: "V1", Name: "shadow"})

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	if !hasConflict(r, "DUP_TRACK_ID", "V1") {
		t.Fatalf("expected DUP_TRACK_ID conflict, got %+v", r.Conflicts)
	}
}

// TestSameNameDifferentMedia 必测 3：同名异媒 => 冲突。
func TestSameNameDifferentMedia(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	target.Media = append(target.Media,
		project.Media{ID: "m9", Name: "other.mov", Path: "/data/other.mov", Checksum: "zz"})
	target.Tracks[0].Clips[0].MediaID = "m9" // 名称仍为 "intro"

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	if !hasConflict(r, "NAME_MEDIA_CONFLICT", "intro") {
		t.Fatalf("expected NAME_MEDIA_CONFLICT, got %+v", r.Conflicts)
	}
	// 同时必须作为 mediaId 字段修改出现，而不是 move。
	c := findChange(t, r, "clip.modify", "c1")
	if !containsPath(fieldPaths(c), "mediaId") {
		t.Fatalf("expected mediaId field change, got %+v", c.Fields)
	}
	for _, c := range r.Changes {
		if c.Op == "clip.move" {
			t.Fatalf("re-identification must not be a move: %+v", c)
		}
	}
}

// TestFullReplacementDisguisedAsMove 必测 1 反例：
// 同 ID 改了素材/入出点/速度，即便位置也变了，也只能是 modify，不是 move。
func TestFullReplacementDisguisedAsMove(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	tc := &target.Tracks[0].Clips[0]
	tc.MediaID = "m2"
	tc.SourceIn = 4
	tc.SourceOut = 8
	tc.Speed = 2
	tc.TimelineIn = 99

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	for _, c := range r.Changes {
		if c.Op == "clip.move" {
			t.Fatalf("content replacement reported as move: %+v", c)
		}
	}
	c := findChange(t, r, "clip.modify", "c1")
	for _, want := range []string{"mediaId", "sourceIn", "sourceOut", "speed", "timelineIn"} {
		if !containsPath(fieldPaths(c), want) {
			t.Fatalf("missing field %s in %v", want, fieldPaths(c))
		}
	}
	if !strings.Contains(c.Detail, "not a move") {
		t.Fatalf("detail should explain not-a-move, got %q", c.Detail)
	}
}

// TestNestedTrackFlatten 必测 4：嵌套轨道上的片段被正确展开并定位。
func TestNestedTrackFlatten(t *testing.T) {
	p := nestedProject()
	l := mustLoad(t, p)
	paths := map[string]string{}
	for _, fc := range l.Clips {
		paths[fc.Clip.ID] = pathString(fc.TrackPath)
	}
	if paths["c1"] != "V1/A1" || paths["c2"] != "V1/A2" {
		t.Fatalf("unexpected nested clip paths: %+v", paths)
	}
}

// TestTransitionAndEffectParams 转场与效果参数的最小差异。
func TestTransitionAndEffectParams(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	tc := &target.Tracks[0].Clips[0]
	tc.Transition.Duration = 1.5
	tc.Transition.Params["softness"] = 0.8
	tc.Effects[0].Params["amount"] = 30

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	c := findChange(t, r, "clip.modify", "c1")
	paths := fieldPaths(c)
	for _, want := range []string{"transition.duration", "transition.params.softness", "effects[fx1].params.amount"} {
		if !containsPath(paths, want) {
			t.Fatalf("missing %s in %v", want, paths)
		}
	}
}

// TestEffectAddRemove 效果新增与删除。
func TestEffectAddRemove(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	target.Tracks[0].Clips[0].Effects = nil

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	c := findChange(t, r, "clip.modify", "c1")
	if !containsPath(fieldPaths(c), "effects[fx1]") {
		t.Fatalf("expected effects[fx1] removal, got %v", fieldPaths(c))
	}
}

// TestTrackAndMediaLifecycle 轨道/素材/片段增删与轨道字段修改。
func TestTrackAndMediaLifecycle(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	target.Tracks = append(target.Tracks, project.Track{ID: "V2", Name: "new"})
	target.Tracks[0].Muted = boolp(true)
	target.Tracks[0].Clips = target.Tracks[0].Clips[:1] // 删除 c2

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	findChange(t, r, "track.add", "V2")
	findChange(t, r, "track.modify", "V1")
	findChange(t, r, "clip.remove", "c2")
}

// TestUnknownMediaConflict 引用不存在素材 => 冲突。
func TestUnknownMediaConflict(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	target.Tracks[0].Clips[0].MediaID = "ghost"

	r := Compare(mustLoad(t, base), mustLoad(t, target))
	if !hasConflict(r, "CLIP_UNKNOWN_MEDIA", "c1") {
		t.Fatalf("expected CLIP_UNKNOWN_MEDIA, got %+v", r.Conflicts)
	}
}

// TestDeterministicOutput 报告排序确定，可重复核查。
func TestDeterministicOutput(t *testing.T) {
	base := sampleProject()
	target := sampleProject()
	target.Tracks = append(target.Tracks, project.Track{ID: "V9"}, project.Track{ID: "V8"})

	r1 := Compare(mustLoad(t, base), mustLoad(t, target))
	r2 := Compare(mustLoad(t, base), mustLoad(t, target))
	if r1.Changes[0].ID > r1.Changes[1].ID {
		t.Fatalf("changes not sorted: %+v", r1.Changes)
	}
	if len(r1.Changes) != len(r2.Changes) {
		t.Fatalf("non-deterministic change count")
	}
}
