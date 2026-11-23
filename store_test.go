package main

import (
	"os"
	"path/filepath"
	"testing"
)

func openTempStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func mustSubmit(t *testing.T, s *Store, proj, reqID string, base int64, op Op) *SubmitResult {
	t.Helper()
	res, rej, err := s.Submit(proj, base, &op)
	if err != nil {
		t.Fatalf("submit %s: %v", reqID, err)
	}
	if rej != nil {
		t.Fatalf("submit %s: unexpected rejection: %v", reqID, rej)
	}
	return res
}

func TestSubmitAssignsSequentialSeqs(t *testing.T) {
	s, _ := openTempStore(t)
	r1 := mustSubmit(t, s, "p1", "r1", 0, Op{RequestID: "r1", Kind: OpMoveClip, ClipID: "c1", Start: 10})
	r2 := mustSubmit(t, s, "p1", "r2", 1, Op{RequestID: "r2", Kind: OpSetKeyframe, ClipID: "c1", Property: "opacity", Time: 2, Value: 0.5})
	if r1.Op.Seq != 1 || r2.Op.Seq != 2 || r2.Version != 2 {
		t.Fatalf("bad seqs: %d %d version=%d", r1.Op.Seq, r2.Op.Seq, r2.Version)
	}
	v, err := s.Version("p1")
	if err != nil || v != 2 {
		t.Fatalf("version confirm = %d, %v; want 2", v, err)
	}
	snap, err := s.Fetch("p1", -1)
	if err != nil {
		t.Fatal(err)
	}
	clip := snap.State.Clips["c1"]
	if clip.Start != 10 {
		t.Fatalf("clip start = %v, want 10", clip.Start)
	}
	kf := clip.Keyframes["opacity"]
	if len(kf) != 1 || kf[0].Value != 0.5 {
		t.Fatalf("keyframes = %+v", kf)
	}
}

func TestDuplicateSubmitKeepsSingleSeq(t *testing.T) {
	s, _ := openTempStore(t)
	op := Op{RequestID: "retry-1", Kind: OpMoveClip, ClipID: "c1", Start: 5}
	first := mustSubmit(t, s, "p1", "retry-1", 0, op)
	// 断网重试：同一 requestId 再次提交
	second := mustSubmit(t, s, "p1", "retry-1", 0, op)
	if !second.Duplicate {
		t.Fatal("expected duplicate flag")
	}
	if second.Op.Seq != first.Op.Seq {
		t.Fatalf("duplicate got seq %d, want %d", second.Op.Seq, first.Op.Seq)
	}
	if v, _ := s.Version("p1"); v != 1 {
		t.Fatalf("version = %d, want 1 (no extra seq allocated)", v)
	}
}

func TestStaleVersionRejectionIsReplayable(t *testing.T) {
	s, _ := openTempStore(t)
	mustSubmit(t, s, "p1", "a1", 0, Op{RequestID: "a1", Kind: OpMoveClip, ClipID: "c1", Start: 10})
	mustSubmit(t, s, "p1", "a2", 1, Op{RequestID: "a2", Kind: OpSetKeyframe, ClipID: "c1", Property: "scale", Time: 1, Value: 2})

	// 客户端 B 基于旧版本 0 提交，必须被拒绝并拿到可重放的缺失操作
	_, rej, err := s.Submit("p1", 0, &Op{RequestID: "b1", Kind: OpMoveClip, ClipID: "c2", Start: 3})
	if err != nil {
		t.Fatal(err)
	}
	if rej == nil {
		t.Fatal("expected rejection")
	}
	if rej.Reason != "stale_version" || rej.CurrentVersion != 2 || len(rej.MissingOps) != 2 {
		t.Fatalf("bad rejection: %+v", rej)
	}

	// 客户端在本地重放缺失操作，把 baseVersion 推进到 2 后重新提交
	local := newProjectState()
	for _, op := range rej.MissingOps {
		cp := *op
		if err := local.apply(&cp); err != nil {
			t.Fatalf("replay missing op: %v", err)
		}
	}
	if local.Clips["c1"].Start != 10 {
		t.Fatalf("replayed state wrong: %+v", local.Clips["c1"])
	}
	res := mustSubmit(t, s, "p1", "b1", rej.CurrentVersion, Op{RequestID: "b1", Kind: OpMoveClip, ClipID: "c2", Start: 3})
	if res.Op.Seq != 3 {
		t.Fatalf("rebased submit seq = %d, want 3", res.Op.Seq)
	}
}

func TestUndoRestoresStateAndIsLogged(t *testing.T) {
	s, _ := openTempStore(t)
	mustSubmit(t, s, "p1", "u1", 0, Op{RequestID: "u1", Kind: OpMoveClip, ClipID: "c1", Start: 42})
	res, rej, err := s.Undo("p1", "undo-1", "bob", 1, 1)
	if err != nil || rej != nil {
		t.Fatalf("undo: rej=%v err=%v", rej, err)
	}
	if res.Op.UndoOf != 1 || res.Op.Seq != 2 {
		t.Fatalf("undo op = %+v", res.Op)
	}
	snap, _ := s.Fetch("p1", -1)
	if snap.State.Clips["c1"].Start != 0 {
		t.Fatalf("start after undo = %v, want 0", snap.State.Clips["c1"].Start)
	}
	// 重复撤销同一序号被拒绝
	if _, _, err := s.Undo("p1", "undo-2", "bob", 2, 1); err != ErrAlreadyUndone {
		t.Fatalf("second undo err = %v, want ErrAlreadyUndone", err)
	}
	// 撤销的幂等重试返回首次结果
	dup, rej, err := s.Undo("p1", "undo-1", "bob", 1, 1)
	if err != nil || rej != nil || !dup.Duplicate || dup.Op.Seq != 2 {
		t.Fatalf("undo retry: dup=%+v rej=%v err=%v", dup, rej, err)
	}
}

func TestUndoKeyframe(t *testing.T) {
	s, _ := openTempStore(t)
	mustSubmit(t, s, "p1", "k1", 0, Op{RequestID: "k1", Kind: OpSetKeyframe, ClipID: "c1", Property: "opacity", Time: 1, Value: 0.8})
	mustSubmit(t, s, "p1", "k2", 1, Op{RequestID: "k2", Kind: OpSetKeyframe, ClipID: "c1", Property: "opacity", Time: 1, Value: 0.3})
	if _, rej, err := s.Undo("p1", "uk2", "a", 2, 2); err != nil || rej != nil {
		t.Fatalf("undo: %v %v", rej, err)
	}
	snap, _ := s.Fetch("p1", -1)
	kf := snap.State.Clips["c1"].Keyframes["opacity"]
	if len(kf) != 1 || kf[0].Value != 0.8 {
		t.Fatalf("keyframe after undo = %+v, want 0.8", kf)
	}
}

func TestRecoveryAfterCrash(t *testing.T) {
	s, dir := openTempStore(t)
	mustSubmit(t, s, "p1", "c1", 0, Op{RequestID: "c1", Kind: OpMoveClip, ClipID: "c1", Start: 7})
	mustSubmit(t, s, "p1", "c2", 1, Op{RequestID: "c2", Kind: OpSetKeyframe, ClipID: "c1", Property: "vol", Time: 0, Value: 1})
	s.Close()

	// 模拟宕机造成的撕裂尾部：半条 JSON 记录
	journal := filepath.Join(dir, "p1.journal")
	f, err := os.OpenFile(journal, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"seq":3,"requestId":"c3","kind":"move_cl`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if v, _ := s2.Version("p1"); v != 2 {
		t.Fatalf("version after recovery = %d, want 2", v)
	}
	snap, _ := s2.Fetch("p1", -1)
	if snap.State.Clips["c1"].Start != 7 {
		t.Fatalf("state after recovery = %+v", snap.State.Clips["c1"])
	}
	// 恢复后仍可继续写，且幂等索引完整
	dup := mustSubmit(t, s2, "p1", "c2", 2, Op{RequestID: "c2", Kind: OpSetKeyframe, ClipID: "c1", Property: "vol", Time: 0, Value: 1})
	if !dup.Duplicate || dup.Op.Seq != 2 {
		t.Fatalf("dedup lost after recovery: %+v", dup)
	}
	next := mustSubmit(t, s2, "p1", "c4", 2, Op{RequestID: "c4", Kind: OpMoveClip, ClipID: "c2", Start: 1})
	if next.Op.Seq != 3 {
		t.Fatalf("seq after recovery = %d, want 3", next.Op.Seq)
	}
}

func TestUndoSurvivesReopen(t *testing.T) {
	s, dir := openTempStore(t)
	mustSubmit(t, s, "p1", "m1", 0, Op{RequestID: "m1", Kind: OpMoveClip, ClipID: "c1", Start: 9})
	if _, rej, err := s.Undo("p1", "um1", "a", 1, 1); err != nil || rej != nil {
		t.Fatalf("undo: %v %v", rej, err)
	}
	s.Close()

	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	snap, _ := s2.Fetch("p1", -1)
	if snap.State.Clips["c1"].Start != 0 {
		t.Fatalf("undo lost after reopen: start=%v", snap.State.Clips["c1"].Start)
	}
	if _, _, err := s2.Undo("p1", "um2", "a", 2, 1); err != ErrAlreadyUndone {
		t.Fatalf("undone flag lost after reopen: %v", err)
	}
}

func TestReplayRange(t *testing.T) {
	s, _ := openTempStore(t)
	for i, id := range []string{"x1", "x2", "x3"} {
		mustSubmit(t, s, "p1", id, int64(i), Op{RequestID: id, Kind: OpMoveClip, ClipID: "c", Start: float64(i)})
	}
	ops, err := s.Replay("p1", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[0].Seq != 2 || ops[1].Seq != 3 {
		t.Fatalf("replay = %+v", ops)
	}
}
