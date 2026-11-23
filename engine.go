package offlinelog

import "sort"

// Fragment 是被操作的文件片段的最小状态。
type Fragment struct {
	ID      string
	Path    string
	Version int64
}

// appliedEvent 保存已生效事件及其执行前快照，供撤销使用。
type appliedEvent struct {
	event        Event
	snapshot     *Fragment // 执行前目标片段状态；nil 表示当时不存在
	hadSnapshot  bool
	barrierAfter int64 // 执行前的屏障位置
	undone       bool
}

// Engine 负责离线日志的合并、校验与有序重放。
// 零值不可直接使用，请通过 NewEngine 构造。
type Engine struct {
	fragments map[string]*Fragment
	paths     map[string]string // path -> fragmentID
	applied   map[string]*appliedEvent
	order     []string        // 已接受事件的 ID，按生效顺序排列
	seen      map[string]bool // 全局事件 ID 去重（含被拒绝事件）
	barrier   int64           // 屏障位置：order 中 barrier 之前的事件不可再撤销
}

// NewEngine 创建空引擎，initial 为当前已存在的片段（可为 nil）。
func NewEngine(initial []Fragment) *Engine {
	e := &Engine{
		fragments: map[string]*Fragment{},
		paths:     map[string]string{},
		applied:   map[string]*appliedEvent{},
		seen:      map[string]bool{},
	}
	for i := range initial {
		f := initial[i]
		fp := f
		e.fragments[f.ID] = &fp
		e.paths[f.Path] = f.ID
	}
	return e
}

// MergeLogs 将多份离线日志确定性地拼接为一条全序事件流。
// 排序键为 (Seq, Source)：Seq 相同的事件按来源日志顺序稳定排列，
// 保证多端对同一组日志得到完全一致的重放顺序。
func MergeLogs(logs ...[]Event) []Event {
	type sourced struct {
		event  Event
		source int
	}
	var all []sourced
	for idx, log := range logs {
		for _, ev := range log {
			all = append(all, sourced{event: ev, source: idx})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].event.Seq != all[j].event.Seq {
			return all[i].event.Seq < all[j].event.Seq
		}
		return all[i].source < all[j].source
	})
	merged := make([]Event, len(all))
	for i, s := range all {
		merged[i] = s.event
	}
	return merged
}

// ValidateLogs 在不修改引擎状态的前提下预检整条事件流。
// 任一事件被拒绝即返回完整拒绝报告，断线重连时应先调用本方法，
// 确认无误后再调用 ApplyLogs 继续同步。
func (e *Engine) ValidateLogs(events []Event) Report {
	probe := e.clone()
	return probe.apply(events)
}

// ApplyLogs 按合并后的顺序逐条重放事件，逐条做依赖检查、
// 过期事件拒绝与撤销屏障检查，返回应用与拒绝明细。
func (e *Engine) ApplyLogs(events []Event) Report {
	return e.apply(events)
}

// Reconnect 实现断线重连流程：先整流校验，全部通过才真正落盘执行；
// 校验失败时引擎状态保持不变，调用方修复日志后可安全重试。
func (e *Engine) ApplyAfterValidation(events []Event) (Report, error) {
	report := e.ValidateLogs(events)
	if !report.OK() {
		return report, &ValidationError{Rejections: report.Rejections}
	}
	return e.apply(events), nil
}

// ValidationError 表示重连预检未通过。
type ValidationError struct {
	Rejections []Rejection
}

func (v *ValidationError) Error() string {
	return "log validation failed with " + itoa(len(v.Rejections)) + " rejection(s)"
}

// Fragment 查询当前片段状态，不存在时 ok 为 false。
func (e *Engine) Fragment(id string) (Fragment, bool) {
	f, ok := e.fragments[id]
	if !ok {
		return Fragment{}, false
	}
	return *f, true
}

// FragmentAt 按路径查询当前片段。
func (e *Engine) FragmentAt(path string) (Fragment, bool) {
	id, ok := e.paths[path]
	if !ok {
		return Fragment{}, false
	}
	return *e.fragments[id], true
}

// Applied 返回已生效事件的 ID，按生效顺序排列。
func (e *Engine) AppliedIDs() []string {
	out := make([]string, len(e.order))
	copy(out, e.order)
	return out
}

func (e *Engine) clone() *Engine {
	c := &Engine{
		fragments: map[string]*Fragment{},
		paths:     map[string]string{},
		applied:   map[string]*appliedEvent{},
		seen:      map[string]bool{},
		order:     make([]string, len(e.order)),
		barrier:   e.barrier,
	}
	copy(c.order, e.order)
	for id, f := range e.fragments {
		fp := *f
		c.fragments[id] = &fp
	}
	for p, id := range e.paths {
		c.paths[p] = id
	}
	for id, ae := range e.applied {
		ac := *ae
		if ae.snapshot != nil {
			sp := *ae.snapshot
			ac.snapshot = &sp
		}
		c.applied[id] = &ac
	}
	for id := range e.seen {
		c.seen[id] = true
	}
	return c
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
