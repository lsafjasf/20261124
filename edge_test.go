package offlinelog

import (
	"errors"
	"strings"
	"testing"
)

func TestMalformedEvents(t *testing.T) {
	eng := newTestEngine()
	r := eng.ApplyLogs([]Event{
		{Seq: 1, Op: OpCreate, FragmentID: "X", TargetPath: "/x"}, // 无 ID
		{ID: "c1", Seq: 2, Op: OpCreate, FragmentID: "X"},         // 缺路径
		{ID: "d1", Seq: 3, Op: OpDelete},                          // 缺片段 ID
		{ID: "m1", Seq: 4, Op: OpMove, FragmentID: "F",
			ExpectedVersion: 1}, // 缺目标路径
		{ID: "u1", Seq: 5, Op: OpUndo},                            // 缺撤销目标
		{ID: "z1", Seq: 6, Op: "freeze", FragmentID: "F"},         // 未知操作
	})
	if r.OK() || len(r.Rejections) != 6 {
		t.Fatalf("got %+v, want 6 rejections", r.Rejections)
	}
	for _, rej := range r.Rejections {
		if rej.Reason != ReasonMalformed || rej.Error() == "" {
			t.Fatalf("unexpected rejection: %+v", rej)
		}
	}
}

func TestUndoCreateRemovesFragment(t *testing.T) {
	eng := newTestEngine()
	r := eng.ApplyLogs([]Event{
		{ID: "c1", Seq: 1, Op: OpCreate, FragmentID: "N", TargetPath: "/n"},
		{ID: "u1", Seq: 2, Op: OpUndo, TargetID: "c1"},
	})
	if !r.OK() {
		t.Fatalf("rejected: %+v", r.Rejections)
	}
	if _, ok := eng.Fragment("N"); ok {
		t.Fatal("undo of create must remove the fragment")
	}
	if _, ok := eng.FragmentAt("/n"); ok {
		t.Fatal("undo of create must release the path")
	}
}

func TestDecodeCorruptLog(t *testing.T) {
	if _, err := DecodeLog([]byte("{not-json")); err == nil {
		t.Fatal("corrupt JSON must return an error")
	}
	events, err := DecodeLog([]byte("   "))
	if err != nil || len(events) != 0 {
		t.Fatalf("blank input = %v, err=%v", events, err)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("disk full")
}

func TestWriteLogPropagatesError(t *testing.T) {
	if err := WriteLog(failWriter{}, []Event{{ID: "x"}}); err == nil {
		t.Fatal("write error must be propagated")
	}
}

func TestValidationErrorMessage(t *testing.T) {
	eng := newTestEngine()
	_, err := eng.ApplyAfterValidation([]Event{
		{ID: "m1", Seq: 1, Op: OpMove, FragmentID: "F",
			TargetPath: "/b", ExpectedVersion: 99},
	})
	if err == nil || !strings.Contains(err.Error(), "1 rejection(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
}
