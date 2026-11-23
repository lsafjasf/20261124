package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Rejection 是版本冲突时返回给客户端的可重放拒绝原因：
// 客户端把 MissingOps 依次在本地重放，即可把 baseVersion 推进到
// CurrentVersion，然后变基自己的修改重新提交。
type Rejection struct {
	Reason         string `json:"reason"`
	BaseVersion    int64  `json:"baseVersion"`
	CurrentVersion int64  `json:"currentVersion"`
	MissingOps     []*Op  `json:"missingOps"`
}

func (r *Rejection) Error() string {
	return fmt.Sprintf("%s: base=%d current=%d missing=%d ops",
		r.Reason, r.BaseVersion, r.CurrentVersion, len(r.MissingOps))
}

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrOpNotFound      = errors.New("op not found")
	ErrAlreadyUndone   = errors.New("op already undone")
	ErrEmptyRequestID  = errors.New("requestId is required")
)

// Project 保存单个项目的日志、物化状态与幂等索引。
type Project struct {
	id        string
	version   int64
	state     *ProjectState
	log       []*Op
	byRequest map[string]*Op
	journal   *os.File
}

// Store 管理磁盘上的所有项目。每个项目一个仅追加的 JSONL 日志，
// 每条记录写入后立即 fsync；恢复时截断末尾撕裂的半行，
// 保证不会出现半写状态。
type Store struct {
	dir      string
	mu       sync.Mutex
	projects map[string]*Project
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, projects: map[string]*Project{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".journal") {
			continue
		}
		id := strings.TrimSuffix(name, ".journal")
		if _, err := s.loadProject(id); err != nil {
			return nil, fmt.Errorf("recover project %q: %w", id, err)
		}
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var first error
	for _, p := range s.projects {
		if err := p.journal.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *Store) journalPath(id string) string {
	return filepath.Join(s.dir, id+".journal")
}

// loadProject 重放日志恢复项目，并把末尾不完整（撕裂）的记录截掉。
// 调用方需持有 s.mu。
func (s *Store) loadProject(id string) (*Project, error) {
	if p, ok := s.projects[id]; ok {
		return p, nil
	}
	path := s.journalPath(id)
	p := &Project{
		id:        id,
		state:     newProjectState(),
		byRequest: map[string]*Op{},
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	good := len(data) // 已确认完整的字节数
	if len(data) > 0 {
		good = 0
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
		for sc.Scan() {
			line := sc.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				good += len(line) + 1
				continue
			}
			var op Op
			if err := json.Unmarshal(line, &op); err != nil {
				break // 撕裂的尾部：丢弃
			}
			rec := op
			if err := p.replay(&rec); err != nil {
				break
			}
			good += len(line) + 1
		}
	}
	if good < len(data) {
		if err := os.Truncate(path, int64(good)); err != nil {
			return nil, fmt.Errorf("truncate torn tail: %w", err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	p.journal = f
	s.projects[id] = p
	return p, nil
}

// replay 把日志中已提交的记录恢复到内存状态（不重新分配序号）。
func (p *Project) replay(op *Op) error {
	cp := *op
	if err := p.state.apply(&cp); err != nil {
		return err
	}
	stored := *op
	p.log = append(p.log, &stored)
	if op.UndoOf != 0 {
		if target := p.opBySeq(op.UndoOf); target != nil {
			target.Undone = true
		}
	}
	p.byRequest[op.RequestID] = &stored
	if op.Seq > p.version {
		p.version = op.Seq
	}
	return nil
}

func (p *Project) opBySeq(seq int64) *Op {
	if seq < 1 || seq > int64(len(p.log)) {
		return nil
	}
	return p.log[seq-1]
}

// commit 先在内存中应用以填充 Prev* 撤销信息，再整体落盘
// （写 + fsync）；落盘失败时回滚内存状态，保证已确认的序号
// 绝不会在恢复后丢失或重复。
func (p *Project) commit(op *Op) error {
	op.Seq = p.version + 1
	if err := p.state.apply(op); err != nil {
		return err
	}
	line, err := json.Marshal(op)
	if err != nil {
		p.rollback(op)
		return err
	}
	line = append(line, '\n')
	if _, err := p.journal.Write(line); err != nil {
		p.rollback(op)
		return err
	}
	if err := p.journal.Sync(); err != nil {
		p.rollback(op)
		return err
	}
	p.log = append(p.log, op)
	if op.UndoOf != 0 {
		if target := p.opBySeq(op.UndoOf); target != nil {
			target.Undone = true
		}
	}
	p.byRequest[op.RequestID] = op
	p.version = op.Seq
	return nil
}

// rollback 在落盘失败时用逆操作撤销已应用的内存修改。
func (p *Project) rollback(op *Op) {
	inv, err := invert(op)
	if err != nil {
		return
	}
	inv.Seq = 0
	inv.UndoOf = 0
	_ = p.state.apply(inv)
}

// SubmitResult 是一次成功提交的返回值。Duplicate 为 true 表示
// 这是同一 requestId 的重试，返回的是首次提交的结果，未分配新序号。
type SubmitResult struct {
	Op        *Op   `json:"op"`
	Version   int64 `json:"version"`
	Duplicate bool  `json:"duplicate"`
}

// Submit 校验版本后提交操作。baseVersion 落后于当前版本时返回
// *Rejection（可重放）；requestId 已存在时直接返回首次结果。
func (s *Store) Submit(projectID string, baseVersion int64, op *Op) (*SubmitResult, *Rejection, error) {
	if op.RequestID == "" {
		return nil, nil, ErrEmptyRequestID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.loadProject(projectID)
	if err != nil {
		return nil, nil, err
	}
	if prev, ok := p.byRequest[op.RequestID]; ok {
		return &SubmitResult{Op: prev, Version: p.version, Duplicate: true}, nil, nil
	}
	if rej := p.checkVersion(baseVersion); rej != nil {
		return nil, rej, nil
	}
	stored := *op
	stored.Seq = 0
	stored.Undone = false
	if err := p.commit(&stored); err != nil {
		return nil, nil, err
	}
	return &SubmitResult{Op: &stored, Version: p.version}, nil, nil
}

// Undo 为 targetSeq 对应的操作追加一条逆操作。版本校验与幂等
// 语义与 Submit 一致。
func (s *Store) Undo(projectID, requestID, author string, baseVersion, targetSeq int64) (*SubmitResult, *Rejection, error) {
	if requestID == "" {
		return nil, nil, ErrEmptyRequestID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.loadProject(projectID)
	if err != nil {
		return nil, nil, err
	}
	if prev, ok := p.byRequest[requestID]; ok {
		return &SubmitResult{Op: prev, Version: p.version, Duplicate: true}, nil, nil
	}
	if rej := p.checkVersion(baseVersion); rej != nil {
		return nil, rej, nil
	}
	target := p.opBySeq(targetSeq)
	if target == nil {
		return nil, nil, ErrOpNotFound
	}
	if target.Undone {
		return nil, nil, ErrAlreadyUndone
	}
	inv, err := invert(target)
	if err != nil {
		return nil, nil, err
	}
	inv.RequestID = requestID
	inv.Author = author
	if err := p.commit(inv); err != nil {
		return nil, nil, err
	}
	return &SubmitResult{Op: inv, Version: p.version}, nil, nil
}

// checkVersion 在 baseVersion 落后时构造可重放的拒绝原因。
func (p *Project) checkVersion(baseVersion int64) *Rejection {
	if baseVersion == p.version {
		return nil
	}
	rej := &Rejection{
		Reason:         "stale_version",
		BaseVersion:    baseVersion,
		CurrentVersion: p.version,
	}
	if baseVersion < p.version {
		for _, op := range p.log[baseVersion:] {
			rej.MissingOps = append(rej.MissingOps, op)
		}
	} else {
		rej.Reason = "future_version"
	}
	return rej
}

// Snapshot 是取回（fetch）的返回：当前版本、物化状态，以及
// 自 since 版本以来的操作（客户端增量同步用）。
type Snapshot struct {
	ProjectID string        `json:"projectId"`
	Version   int64         `json:"version"`
	State     *ProjectState `json:"state"`
	Ops       []*Op         `json:"ops,omitempty"`
}

// Fetch 取回项目状态；since >= 0 时附带自该版本以来的操作。
func (s *Store) Fetch(projectID string, since int64) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.loadProject(projectID)
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{ProjectID: projectID, Version: p.version, State: p.state}
	if since >= 0 && since < p.version {
		snap.Ops = append(snap.Ops, p.log[since:]...)
	}
	return snap, nil
}

// Replay 返回 [from, to] 区间内的操作日志，供客户端重放。
func (s *Store) Replay(projectID string, from, to int64) ([]*Op, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.loadProject(projectID)
	if err != nil {
		return nil, err
	}
	if from < 1 {
		from = 1
	}
	if to <= 0 || to > p.version {
		to = p.version
	}
	if from > to {
		return nil, nil
	}
	out := make([]*Op, 0, to-from+1)
	out = append(out, p.log[from-1:to]...)
	return out, nil
}

// Version 返回项目当前版本号（版本确认）。
func (s *Store) Version(projectID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.loadProject(projectID)
	if err != nil {
		return 0, err
	}
	return p.version, nil
}
