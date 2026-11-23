package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(store))
	t.Cleanup(func() {
		srv.Close()
		store.Close()
	})
	return srv
}

func postJSON(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	buf, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}

func TestHTTPFlow(t *testing.T) {
	srv := newTestServer(t)
	base := srv.URL + "/projects/demo"

	// 提交
	status, body := postJSON(t, base+"/ops", map[string]any{
		"baseVersion": 0,
		"op": map[string]any{
			"requestId": "h1", "kind": "move_clip", "clipId": "c1", "start": 12,
		},
	})
	if status != http.StatusOK {
		t.Fatalf("submit status=%d body=%v", status, body)
	}
	if body["version"].(float64) != 1 {
		t.Fatalf("version = %v", body)
	}

	// 过期版本被拒绝，且返回可重放的 missingOps
	status, body = postJSON(t, base+"/ops", map[string]any{
		"baseVersion": 0,
		"op": map[string]any{
			"requestId": "h2", "kind": "move_clip", "clipId": "c2", "start": 1,
		},
	})
	if status != http.StatusConflict {
		t.Fatalf("stale submit status=%d", status)
	}
	if body["reason"] != "stale_version" || len(body["missingOps"].([]any)) != 1 {
		t.Fatalf("rejection body = %v", body)
	}

	// 断网重试同一 requestId：不产生新序号
	status, body = postJSON(t, base+"/ops", map[string]any{
		"baseVersion": 0,
		"op": map[string]any{
			"requestId": "h1", "kind": "move_clip", "clipId": "c1", "start": 12,
		},
	})
	if status != http.StatusOK || body["duplicate"] != true {
		t.Fatalf("retry status=%d body=%v", status, body)
	}

	// 版本确认
	status, body = getJSON(t, base+"/version")
	if status != http.StatusOK || body["version"].(float64) != 1 {
		t.Fatalf("version confirm = %v", body)
	}

	// 撤销
	status, body = postJSON(t, base+"/undo", map[string]any{
		"baseVersion": 1, "targetSeq": 1, "requestId": "u1",
	})
	if status != http.StatusOK {
		t.Fatalf("undo status=%d body=%v", status, body)
	}

	// 取回：状态应回到撤销前
	status, body = getJSON(t, base+"/state")
	if status != http.StatusOK {
		t.Fatalf("fetch status=%d", status)
	}
	state := body["state"].(map[string]any)
	clip := state["clips"].(map[string]any)["c1"].(map[string]any)
	if clip["start"].(float64) != 0 {
		t.Fatalf("start after undo = %v", clip["start"])
	}

	// 重放
	status, body = getJSON(t, base+"/replay?from=1")
	if status != http.StatusOK || len(body["ops"].([]any)) != 2 {
		t.Fatalf("replay = %v", body)
	}
}

func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}
