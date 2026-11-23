package offlinelog

import "testing"

func newTestEngine() *Engine {
	return NewEngine([]Fragment{{ID: "F", Path: "/a", Version: 1}})
}

// TestInterleavedDeleteMoveNoResurrection 复现缺陷：两份离线日志直接拼接后，
// 删除之前产生的旧移动操作不得让已删除片段复活。
func TestInterleavedDeleteMoveNoResurrection(t *testing.T) {
	logA := []Event{
		{ID: "a1", Seq: 1, Op: OpDelete, FragmentID: "F", ExpectedVersion: 1},
	}
	logB := []Event{
		// 断联期间基于旧状态产生的移动：F 已被另一端删除。
		{ID: "b1", Seq: 2, Op: OpMove, FragmentID: "F",
			TargetPath: "/b", ExpectedVersion: 1},
	}

	eng := newTestEngine()
	merged := MergeLogs(logA, logB)
	report := eng.ApplyLogs(merged)

	if report.OK() {
		t.Fatal("stale move after delete must be rejected")
	}
	if got := report.Rejections[0].Reason; got != ReasonMissingDep {
		t.Fatalf("reason = %s, want %s", got, ReasonMissingDep)
	}
	if _, alive := eng.Fragment("F"); alive {
		t.Fatal("deleted fragment F must not be resurrected by an old move")
	}
	if _, occupied := eng.FragmentAt("/b"); occupied {
		t.Fatal("path /b must remain empty")
	}
}

// TestInterleavedMoveThenStaleDelete 覆盖另一种交错顺序：
// 移动先生效后，携带旧版本号的删除属于过期事件，必须拒绝。
func TestInterleavedMoveThenStaleDelete(t *testing.T) {
	logB := []Event{
		{ID: "b1", Seq: 1, Op: OpMove, FragmentID: "F",
			TargetPath: "/b", ExpectedVersion: 1},
	}
	logA := []Event{
		{ID: "a1", Seq: 2, Op: OpDelete, FragmentID: "F", ExpectedVersion: 1},
	}

	eng := newTestEngine()
	report := eng.ApplyLogs(MergeLogs(logA, logB))

	if len(report.Applied) != 1 || report.Applied[0].ID != "b1" {
		t.Fatalf("applied = %v, want only b1", report.Applied)
	}
	if report.Rejections[0].Reason != ReasonStale {
		t.Fatalf("reason = %s, want %s", report.Rejections[0].Reason, ReasonStale)
	}
	f, ok := eng.Fragment("F")
	if !ok || f.Path != "/b" || f.Version != 2 {
		t.Fatalf("fragment = %+v, want moved F at /b v2", f)
	}
}

// TestUndoBarrier 验证显式屏障与删除隐式屏障：
// 屏障之前的操作不能被撤销，撤销删除则允许并拆除其隐式屏障。
func TestUndoBarrier(t *testing.T) {
	eng := newTestEngine()
	events := []Event{
		{ID: "e1", Seq: 1, Op: OpRename, FragmentID: "F",
			TargetPath: "/a2", ExpectedVersion: 1},
		{ID: "e2", Seq: 2, Op: OpBarrier},
	}
	if r := eng.ApplyLogs(events); !r.OK() {
		t.Fatalf("setup rejected: %+v", r.Rejections)
	}

	blocked := eng.ApplyLogs([]Event{
		{ID: "u1", Seq: 3, Op: OpUndo, TargetID: "e1"},
	})
	if blocked.OK() || blocked.Rejections[0].Reason != ReasonBarrier {
		t.Fatalf("undo across barrier must fail, got %+v", blocked.Rejections)
	}
	if f, _ := eng.Fragment("F"); f.Path != "/a2" {
		t.Fatalf("path = %s, want /a2 unchanged", f.Path)
	}

	// 删除建立新的隐式屏障；撤销删除合法，并把屏障回退到删除前的位置。
	eng2 := newTestEngine()
	flow := []Event{
		{ID: "r1", Seq: 1, Op: OpRename, FragmentID: "F",
			TargetPath: "/a2", ExpectedVersion: 1},
		{ID: "d1", Seq: 2, Op: OpDelete, FragmentID: "F", ExpectedVersion: 2},
		{ID: "u2", Seq: 3, Op: OpUndo, TargetID: "d1"},
		{ID: "u3", Seq: 4, Op: OpUndo, TargetID: "r1"},
	}
	if r := eng2.ApplyLogs(flow); !r.OK() {
		t.Fatalf("undo flow rejected: %+v", r.Rejections)
	}
	f, ok := eng2.Fragment("F")
	if !ok || f.Path != "/a" || f.Version != 1 {
		t.Fatalf("fragment = %+v ok=%v, want restored F at /a v1", f, ok)
	}
}

// TestUndoGuardrails 覆盖撤销目标的其它非法情形。
func TestUndoGuardrails(t *testing.T) {
	eng := newTestEngine()
	eng.ApplyLogs([]Event{
		{ID: "d1", Seq: 1, Op: OpDelete, FragmentID: "F", ExpectedVersion: 1},
		{ID: "u1", Seq: 2, Op: OpUndo, TargetID: "d1"},
	})

	r := eng.ApplyLogs([]Event{
		{ID: "uX", Seq: 3, Op: OpUndo, TargetID: "ghost"},
		{ID: "u2", Seq: 4, Op: OpUndo, TargetID: "u1"},
		{ID: "u3", Seq: 5, Op: OpUndo, TargetID: "d1"},
	})
	want := []Reason{ReasonUndoTarget, ReasonUndoTarget, ReasonUndoTarget}
	if len(r.Rejections) != len(want) {
		t.Fatalf("got %d rejections, want %d", len(r.Rejections), len(want))
	}
	for i, rej := range r.Rejections {
		if rej.Reason != want[i] {
			t.Fatalf("rejection %d reason = %s, want %s", i, rej.Reason, want[i])
		}
	}
}

// TestDependencyAndConflictChecks 验证依赖缺失与路径/ID 冲突。
func TestDependencyAndConflictChecks(t *testing.T) {
	eng := NewEngine([]Fragment{
		{ID: "F", Path: "/a", Version: 1},
		{ID: "G", Path: "/b", Version: 1},
	})
	r := eng.ApplyLogs([]Event{
		{ID: "c1", Seq: 1, Op: OpCreate, FragmentID: "F", TargetPath: "/c"},
		{ID: "c2", Seq: 2, Op: OpCreate, FragmentID: "H", TargetPath: "/b"},
		{ID: "m1", Seq: 3, Op: OpMove, FragmentID: "X",
			TargetPath: "/x", ExpectedVersion: 1},
		{ID: "rn1", Seq: 4, Op: OpRename, FragmentID: "F",
			TargetPath: "/b", ExpectedVersion: 1},
	})
	want := []Reason{
		ReasonPathConflict, ReasonPathConflict,
		ReasonMissingDep, ReasonPathConflict,
	}
	if len(r.Rejections) != len(want) {
		t.Fatalf("got %d rejections, want %d (%+v)", len(r.Rejections), len(want), r)
	}
	for i, rej := range r.Rejections {
		if rej.Reason != want[i] {
			t.Fatalf("rejection %d = %s, want %s", i, rej.Reason, want[i])
		}
	}
}
