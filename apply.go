package offlinelog

import "fmt"

func (e *Engine) apply(events []Event) Report {
	report := Report{}
	for _, ev := range events {
		if reason, detail := e.check(ev); reason != "" {
			// 被拒绝的事件 ID 同样登记，防止后续日志里重复混入。
			e.seen[ev.ID] = true
			report.Rejections = append(report.Rejections, Rejection{
				Event: ev, Reason: reason, Detail: detail,
			})
			continue
		}
		e.commit(ev)
		report.Applied = append(report.Applied, ev)
	}
	return report
}

// check 执行依赖检查、过期事件拒绝与撤销屏障规则。
func (e *Engine) check(ev Event) (Reason, string) {
	if ev.ID == "" {
		return ReasonMalformed, "missing event id"
	}
	if e.seen[ev.ID] {
		return ReasonDuplicate, "event id " + ev.ID + " already processed"
	}

	switch ev.Op {
	case OpBarrier:
		return "", ""
	case OpCreate:
		if ev.FragmentID == "" || ev.TargetPath == "" {
			return ReasonMalformed, "create requires fragment_id and target_path"
		}
		if _, exists := e.fragments[ev.FragmentID]; exists {
			return ReasonPathConflict, "fragment " + ev.FragmentID + " already exists"
		}
		if id, taken := e.paths[ev.TargetPath]; taken {
			return ReasonPathConflict, "path " + ev.TargetPath + " occupied by " + id
		}
	case OpDelete, OpMove, OpRename:
		if ev.FragmentID == "" {
			return ReasonMalformed, string(ev.Op) + " requires fragment_id"
		}
		f, ok := e.fragments[ev.FragmentID]
		if !ok {
			return ReasonMissingDep, "fragment " + ev.FragmentID + " does not exist"
		}
		if f.Version != ev.ExpectedVersion {
			return ReasonStale, fmt.Sprintf(
				"fragment %s expected version %d, current %d",
				ev.FragmentID, ev.ExpectedVersion, f.Version)
		}
		if ev.Op == OpMove || ev.Op == OpRename {
			if ev.TargetPath == "" {
				return ReasonMalformed, string(ev.Op) + " requires target_path"
			}
			if id, taken := e.paths[ev.TargetPath]; taken && id != ev.FragmentID {
				return ReasonPathConflict, "path " + ev.TargetPath + " occupied by " + id
			}
		}
	case OpUndo:
		return e.checkUndo(ev)
	default:
		return ReasonMalformed, "unknown op " + string(ev.Op)
	}
	return "", ""
}

// checkUndo 校验撤销目标存在、未被撤销过，且没有跨越撤销屏障。
func (e *Engine) checkUndo(ev Event) (Reason, string) {
	if ev.TargetID == "" {
		return ReasonMalformed, "undo requires target_id"
	}
	target, ok := e.applied[ev.TargetID]
	if !ok {
		return ReasonUndoTarget, "target " + ev.TargetID + " is not an applied event"
	}
	if target.event.Op == OpUndo {
		return ReasonUndoTarget, "undo of undo is not allowed"
	}
	if target.undone {
		return ReasonUndoTarget, "target " + ev.TargetID + " already undone"
	}
	// 目标事件发生在屏障位置（含）之前：删除已被屏障确认，
	// 或显式屏障要求此前的操作不可再撤销。唯一例外是撤销一次
	// 删除——删除本身建立屏障，其撤销必须允许以恢复片段。
	pos := e.position(ev.TargetID)
	if pos < e.barrier && target.event.Op != OpDelete {
		return ReasonBarrier, fmt.Sprintf(
			"target %s at position %d is before barrier %d",
			ev.TargetID, pos, e.barrier)
	}
	return "", ""
}

// commit 执行已通过校验的事件并记录撤销所需快照。
func (e *Engine) commit(ev Event) {
	ae := &appliedEvent{
		event:        ev,
		hadSnapshot:  false,
		barrierAfter: e.barrier,
	}
	if f, ok := e.fragments[ev.FragmentID]; ok && ev.Op != OpCreate {
		snap := *f
		ae.snapshot = &snap
		ae.hadSnapshot = true
	}

	switch ev.Op {
	case OpBarrier:
		e.barrier = int64(len(e.order)) + 1
	case OpCreate:
		f := &Fragment{ID: ev.FragmentID, Path: ev.TargetPath, Version: 1}
		e.fragments[f.ID] = f
		e.paths[f.Path] = f.ID
	case OpDelete:
		f := e.fragments[ev.FragmentID]
		delete(e.paths, f.Path)
		delete(e.fragments, f.ID)
		// 隐式撤销屏障：删除一旦确认，屏障之前的历史操作
		// （例如删除前的旧移动）不得再撤销出已删除的片段。
		e.barrier = int64(len(e.order)) + 1
	case OpMove, OpRename:
		f := e.fragments[ev.FragmentID]
		delete(e.paths, f.Path)
		f.Path = ev.TargetPath
		f.Version++
		e.paths[f.Path] = f.ID
	case OpUndo:
		e.rollback(e.applied[ev.TargetID])
	}

	e.applied[ev.ID] = ae
	e.seen[ev.ID] = true
	e.order = append(e.order, ev.ID)
}

// rollback 恢复目标事件执行前的片段状态。
func (e *Engine) rollback(target *appliedEvent) {
	target.undone = true
	ev := target.event

	// 先移除目标事件产生的当前片段占用。
	if cur, ok := e.fragments[ev.FragmentID]; ok {
		delete(e.paths, cur.Path)
	}

	switch {
	case !target.hadSnapshot:
		// 目标是 create，或删除时片段已不存在：撤销即彻底移除。
		delete(e.fragments, ev.FragmentID)
	case target.snapshot != nil:
		snap := *target.snapshot
		e.fragments[ev.FragmentID] = &snap
		e.paths[snap.Path] = snap.ID
	}

	// 撤销删除会同时拆除该删除建立的隐式屏障。
	if ev.Op == OpDelete {
		e.barrier = target.barrierAfter
	}
}

func (e *Engine) position(eventID string) int64 {
	for i, id := range e.order {
		if id == eventID {
			return int64(i)
		}
	}
	return -1
}
