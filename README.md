# 20261124

协作剪辑服务：仅使用 Go 标准库实现，不接数据库与网络框架。
每个项目对应一个仅追加的 JSONL 操作日志（写后 fsync），崩溃恢复时
截断撕裂的尾部记录，保证项目不会处于半写状态。

## 运行

```sh
go build -o collabedit . && ./collabedit -addr :8080 -data data
go test ./...
```

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/projects/{id}/state?since=N` | 取回：版本 + 物化状态（+ 自 N 以来的增量操作） |
| GET | `/projects/{id}/replay?from=&to=` | 重放指定区间的操作日志 |
| GET | `/projects/{id}/version` | 版本确认 |
| POST | `/projects/{id}/ops` | 提交操作：`{"baseVersion":N,"op":{...}}` |
| POST | `/projects/{id}/undo` | 撤销：`{"baseVersion":N,"targetSeq":M,"requestId":"..."}` |

操作类型：`move_clip`（`clipId`/`start`）、`set_keyframe` 与
`remove_keyframe`（`clipId`/`property`/`time`/`value`）。

## 一致性约定

- **可重放的拒绝**：`baseVersion` 落后于服务端时返回 409，
  响应体含 `currentVersion` 与 `missingOps`，客户端在本地依次重放
  缺失操作、变基自己的修改后以新版本号重新提交。
- **幂等提交**：每个操作携带唯一 `requestId`，断网重试返回首次
  结果（`duplicate:true`），不会分配第二个序列号；幂等索引随日志
  一起持久化，重启后仍然有效。
- **撤销**：撤销作为一条逆操作（`undoOf` 指向目标序号）追加进日志，
  可重放、可恢复，且同样受版本校验与幂等保护。
