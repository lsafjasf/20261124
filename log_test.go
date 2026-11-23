package offlinelog

import (
	"bytes"
	"strings"
	"testing"
)

// TestDuplicateRenameRejected 重复重命名：同一事件 ID 重放必须去重，
// 携带过期版本号的重命名必须按过期事件拒绝。
func TestDuplicateRenameRejected(t *testing.T) {
	eng := newTestEngine()
	first := eng.ApplyLogs([]Event{
		{ID: "rn1", Seq: 1, Op: OpRename, FragmentID: "F",
			TargetPath: "/a2", ExpectedVersion: 1},
	})
	if !first.OK() {
		t.Fatalf("first rename rejected: %+v", first.Rejections)
	}

	// 同一份日志因重连被再次投递：事件 ID 去重，全部拒绝且状态不变。
	retry := eng.ApplyLogs([]Event{
		{ID: "rn1", Seq: 1, Op: OpRename, FragmentID: "F",
			TargetPath: "/a2", ExpectedVersion: 1},
	})
	if retry.OK() || retry.Rejections[0].Reason != ReasonDuplicate {
		t.Fatalf("duplicate rename must be rejected, got %+v", retry.Rejections)
	}

	// 新事件但基于旧版本号：依赖存在而版本过期。
	stale := eng.ApplyLogs([]Event{
		{ID: "rn2", Seq: 2, Op: OpRename, FragmentID: "F",
			TargetPath: "/a3", ExpectedVersion: 1},
	})
	if stale.OK() || stale.Rejections[0].Reason != ReasonStale {
		t.Fatalf("stale rename must be rejected, got %+v", stale.Rejections)
	}
	f, _ := eng.Fragment("F")
	if f.Path != "/a2" || f.Version != 2 {
		t.Fatalf("fragment = %+v, want /a2 v2", f)
	}

	// 基于当前版本的重命名正常生效。
	ok := eng.ApplyLogs([]Event{
		{ID: "rn3", Seq: 3, Op: OpRename, FragmentID: "F",
			TargetPath: "/a3", ExpectedVersion: 2},
	})
	if !ok.OK() {
		t.Fatalf("current rename rejected: %+v", ok.Rejections)
	}
}

// TestEmptyLogs 空日志在合并、校验、重放、编解码各环节都必须安全。
func TestEmptyLogs(t *testing.T) {
	eng := newTestEngine()

	merged := MergeLogs(nil, []Event{}, nil)
	if len(merged) != 0 {
		t.Fatalf("merged len = %d, want 0", len(merged))
	}
	if r := eng.ValidateLogs(merged); !r.OK() || len(r.Applied) != 0 {
		t.Fatalf("empty validation failed: %+v", r)
	}
	if r := eng.ApplyLogs(merged); !r.OK() || len(r.Applied) != 0 {
		t.Fatalf("empty apply failed: %+v", r)
	}
	if r, err := eng.ApplyAfterValidation(merged); err != nil || !r.OK() {
		t.Fatalf("empty reconnect failed: err=%v report=%+v", err, r)
	}
	if _, alive := eng.Fragment("F"); !alive {
		t.Fatal("empty log must not change engine state")
	}

	data, err := EncodeLog(nil)
	if err != nil || string(data) != "[]" {
		t.Fatalf("encode empty = %s, err=%v", data, err)
	}
	decoded, err := DecodeLog(data)
	if err != nil || len(decoded) != 0 {
		t.Fatalf("decode empty = %v, err=%v", decoded, err)
	}
	if decoded2, err := DecodeLog(nil); err != nil || len(decoded2) != 0 {
		t.Fatalf("decode nil bytes failed: %v err=%v", decoded2, err)
	}
}

// TestReconnectValidatesBeforeApply 断线重连必须先校验再继续：
// 预检失败时引擎状态保持原样，修复后可安全重试。
func TestReconnectValidatesBeforeApply(t *testing.T) {
	eng := newTestEngine()
	bad := []Event{
		{ID: "b1", Seq: 1, Op: OpMove, FragmentID: "F",
			TargetPath: "/b", ExpectedVersion: 1},
		{ID: "b2", Seq: 2, Op: OpMove, FragmentID: "F",
			TargetPath: "/c", ExpectedVersion: 1}, // 版本号过期
	}

	report, err := eng.ApplyAfterValidation(bad)
	if err == nil {
		t.Fatal("expected ValidationError")
	}
	verr, ok := err.(*ValidationError)
	if !ok || len(verr.Rejections) != 1 || verr.Rejections[0].Reason != ReasonStale {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if len(eng.AppliedIDs()) != 0 {
		t.Fatal("failed validation must not mutate the real engine")
	}
	if f, _ := eng.Fragment("F"); f.Path != "/a" || f.Version != 1 {
		t.Fatalf("engine mutated after failed validation: %+v", f)
	}

	good := []Event{
		{ID: "b1", Seq: 1, Op: OpMove, FragmentID: "F",
			TargetPath: "/b", ExpectedVersion: 1},
		{ID: "b2", Seq: 2, Op: OpMove, FragmentID: "F",
			TargetPath: "/c", ExpectedVersion: 2},
	}
	report, err = eng.ApplyAfterValidation(good)
	if err != nil || !report.OK() || len(report.Applied) != 2 {
		t.Fatalf("retry failed: err=%v report=%+v", err, report)
	}
	if f, _ := eng.Fragment("F"); f.Path != "/c" || f.Version != 3 {
		t.Fatalf("fragment = %+v, want /c v3", f)
	}
}

// TestMergeDeterministic 多端合并顺序必须稳定且可复现。
func TestMergeDeterministic(t *testing.T) {
	logA := []Event{{ID: "a", Seq: 2}}
	logB := []Event{{ID: "b", Seq: 1}, {ID: "c", Seq: 2}}

	first := MergeLogs(logA, logB)
	second := MergeLogs(logA, logB)
	if len(first) != 3 || first[0].ID != "b" ||
		first[1].ID != "a" || first[2].ID != "c" {
		t.Fatalf("unexpected merge order: %+v", first)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatal("merge is not deterministic across calls")
		}
	}
}

// TestLogJSONRoundtrip 日志经 JSON 落盘再读取后字段完整。
func TestLogJSONRoundtrip(t *testing.T) {
	events := []Event{
		{ID: "e1", Seq: 1, Op: OpCreate, FragmentID: "F", TargetPath: "/a"},
		{ID: "e2", Seq: 2, Op: OpMove, FragmentID: "F",
			TargetPath: "/b", ExpectedVersion: 1},
		{ID: "e3", Seq: 3, Op: OpUndo, TargetID: "e2"},
		{ID: "e4", Seq: 4, Op: OpBarrier},
	}
	var buf bytes.Buffer
	if err := WriteLog(&buf, events); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLog(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(events) {
		t.Fatalf("len = %d, want %d", len(got), len(events))
	}
	for i := range events {
		if got[i] != events[i] {
			t.Fatalf("event %d = %+v, want %+v", i, got[i], events[i])
		}
	}
}
