package offlinelog

import "fmt"

// OpType 标识离线日志中记录的操作种类。
type OpType string

const (
	OpCreate  OpType = "create"  // 新建片段
	OpDelete  OpType = "delete"  // 删除片段，同时隐式建立撤销屏障
	OpMove    OpType = "move"    // 移动片段到新路径
	OpRename  OpType = "rename"  // 重命名片段（路径末段变更）
	OpUndo    OpType = "undo"    // 撤销 TargetID 指向的历史操作
	OpBarrier OpType = "barrier" // 显式撤销屏障
)

// Event 是离线操作日志中的一条记录。
//
// Seq 由产生端按本地单调递增分配；ExpectedVersion 为乐观并发控制字段，
// 记录该事件产生时目标片段的版本，重放时与当前版本不符即视为过期事件。
type Event struct {
	ID              string `json:"id"`
	Seq             int64  `json:"seq"`
	Op              OpType `json:"op"`
	FragmentID      string `json:"fragment_id,omitempty"`
	TargetPath      string `json:"target_path,omitempty"`
	ExpectedVersion int64  `json:"expected_version,omitempty"`
	TargetID        string `json:"target_id,omitempty"` // 仅 undo 使用
}

// Reason 说明事件在校验阶段被拒绝的原因类别。
type Reason string

const (
	ReasonMalformed    Reason = "malformed_event"      // 事件字段不合法
	ReasonDuplicate    Reason = "duplicate_event"      // 事件 ID 已处理过
	ReasonMissingDep   Reason = "missing_dependency"   // 目标片段不存在
	ReasonPathConflict Reason = "path_conflict"        // 目标路径已被占用
	ReasonStale        Reason = "stale_event"          // 期望版本与当前版本不符
	ReasonUndoTarget   Reason = "invalid_undo_target"  // 撤销目标不可撤销
	ReasonBarrier      Reason = "undo_crosses_barrier" // 撤销跨越了屏障
)

// Rejection 记录一条被拒绝的事件及其原因，便于断线重连后排查。
type Rejection struct {
	Event  Event  `json:"event"`
	Reason Reason `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

func (r Rejection) Error() string {
	return fmt.Sprintf("event %s rejected: %s (%s)", r.Event.ID, r.Reason, r.Detail)
}

// Report 汇总一次日志拼接/重放的结果。
type Report struct {
	Applied    []Event     `json:"applied"`
	Rejections []Rejection `json:"rejections"`
}

// OK 表示没有任何事件被拒绝。
func (r Report) OK() bool { return len(r.Rejections) == 0 }
