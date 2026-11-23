package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// Server 暴露协作剪辑的 HTTP API：
//
//	GET  /projects/{id}/state?since=N   取回（版本 + 物化状态 + 增量操作）
//	GET  /projects/{id}/replay?from=&to= 重放指定区间的操作日志
//	GET  /projects/{id}/version          版本确认
//	POST /projects/{id}/ops              提交操作（携带 baseVersion 与 requestId）
//	POST /projects/{id}/undo             撤销指定序号的操作
type Server struct {
	store *Store
	mux   *http.ServeMux
}

func NewServer(store *Store) *Server {
	s := &Server{store: store, mux: http.NewServeMux()}
	s.mux.HandleFunc("/projects/", s.route)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/projects/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	projectID, action := parts[0], parts[1]
	switch {
	case action == "state" && r.Method == http.MethodGet:
		s.handleFetch(w, r, projectID)
	case action == "replay" && r.Method == http.MethodGet:
		s.handleReplay(w, r, projectID)
	case action == "version" && r.Method == http.MethodGet:
		s.handleVersion(w, r, projectID)
	case action == "ops" && r.Method == http.MethodPost:
		s.handleSubmit(w, r, projectID)
	case action == "undo" && r.Method == http.MethodPost:
		s.handleUndo(w, r, projectID)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

type submitRequest struct {
	BaseVersion int64 `json:"baseVersion"`
	Op          Op    `json:"op"`
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request, projectID string) {
	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	res, rej, err := s.store.Submit(projectID, req.BaseVersion, &req.Op)
	if rej != nil {
		writeJSON(w, http.StatusConflict, rej)
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type undoRequest struct {
	BaseVersion int64  `json:"baseVersion"`
	TargetSeq   int64  `json:"targetSeq"`
	RequestID   string `json:"requestId"`
	Author      string `json:"author,omitempty"`
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request, projectID string) {
	var req undoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	res, rej, err := s.store.Undo(projectID, req.RequestID, req.Author, req.BaseVersion, req.TargetSeq)
	if rej != nil {
		writeJSON(w, http.StatusConflict, rej)
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request, projectID string) {
	since := queryInt(r, "since", -1)
	snap, err := s.store.Fetch(projectID, since)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request, projectID string) {
	from := queryInt(r, "from", 1)
	to := queryInt(r, "to", 0)
	ops, err := s.store.Replay(projectID, from, to)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projectId": projectID, "ops": ops})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request, projectID string) {
	v, err := s.store.Version(projectID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projectId": projectID, "version": v})
}

func queryInt(r *http.Request, key string, def int64) int64 {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEmptyRequestID):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrOpNotFound), errors.Is(err, ErrAlreadyUndone):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrUnknownOpKind):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
