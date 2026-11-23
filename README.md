# 事务式媒体重新关联

仅使用 Go 标准库实现。重新关联以整个工程为事务：先完成候选发现、确认和全部校验，
再原子替换工程文件；提交后重新读取工程并复检。任一步失败都保留或恢复原始工程字节。

## 行为

- 高置信项会合并为一次批量确认。
- 低置信项（例如多个内容完全相同的候选）逐条确认，并可指定具体候选。
- 确认完成后才修改内存中的工程克隆，且只允许修改媒体路径字段。
- 提交前后都检查实际媒体的大小、SHA-256、帧率、时长和音频偏移。
  默认容差为帧率 `0.001`、时长 `±2ms`、音频偏移 `±1ms`。
- 比较去掉可重关联路径后的工程骨架及时间线 JSON，禁止时间线内容静默变化。
- 工程 JSON 严格解析；未知字段或尾随内容会被拒绝，避免保存时丢失扩展数据。
- 提交后重新解析落盘工程，并与已验证计划逐字段比较；不一致立即回滚。
- `missing-proxy=fail` 为默认值。`keep` 仅忽略确实不存在的代理，并产生显式警告；
  找到同名代理但媒体校验失败时仍然回滚。

## CLI

```bash
go build ./cmd/relink

./relink \
  -project /path/to/project.json \
  -root /mnt/d/Footage \
  -root /backup/Footage \
  -missing-proxy fail

./relink -project project.json -root ./media -yes -json
```

`-root` 可重复。默认交互模式会一次展示全部高置信项，随后逐个展示低置信项及其候选。
`-yes` 用于无人值守地接受所有候选。

## 工程格式

```json
{
  "version": 1,
  "name": "episode-01",
  "frame_rate": {"numerator": 30000, "denominator": 1001},
  "duration_ms": 4200,
  "audio_offset_ms": 7,
  "clips": [
    {
      "id": "clip-a",
      "name": "cam-a.mp4",
      "primary": {
        "path": "D:\\Footage\\ep1\\cam-a.mp4",
        "fingerprint": {
          "size": 16,
          "sha256": "...",
          "duration_ms": 4200,
          "frame_rate": {"numerator": 30000, "denominator": 1001},
          "audio_offset_ms": 7
        }
      },
      "proxy": {
        "path": "D:\\Proxies\\ep1\\cam-a.proxy.mp4",
        "fingerprint": {
          "size": 15,
          "sha256": "...",
          "duration_ms": 4200,
          "frame_rate": {"numerator": 30000, "denominator": 1001},
          "audio_offset_ms": 7
        }
      }
    }
  ],
  "timeline": [
    {
      "id": "item-1",
      "clip_id": "clip-a",
      "start_frame": 0,
      "end_frame": 120,
      "source_in_frame": 24,
      "audio_offset_ms": 7
    }
  ]
}
```

默认 `FilesystemProbe` 从 `<媒体文件>.media.json` 读取时间信息：

```json
{
  "duration_ms": 4200,
  "frame_rate": {"numerator": 30000, "denominator": 1001},
  "audio_offset_ms": 7
}
```

接入真实媒体容器解析时，实现 `relink.Probe` 即可；事务、匹配和回滚逻辑无需改动。

## 复现与验证

```bash
go test ./...
go vet ./...
```

测试直接构造以下复现场景：

- `TestDriveLetterChangeBatchRelinksAndPreservesTimeline`：Windows 盘符变化后批量重关联。
- `TestDuplicateFilesRequireOneConfirmationPerItem`：重复文件必须逐条选择。
- `TestBatchAndIndividualConfirmationsAreSeparated`：同笔事务中的批量与逐条确认互不混淆。
- `TestMissingProxyRollsBackWholeTransaction`：缺失代理导致整笔事务不落盘。
- `TestInvalidProxyIsNotTreatedAsMissing`：同名但媒体不一致的代理不会被 `keep` 忽略。
- `TestAnyMediaMismatchRestoresOriginalProject`：帧率、时长或音频偏移任一不符即保留原工程。
- `TestPostCommitTimelineChangeTriggersRollback`：提交后时间线漂移会触发字节级恢复。

候选匹配目前要求文件名一致，通过规范化盘符和目录后缀完成跨卷定位；
内容相同但改名的文件不会自动列为候选。
