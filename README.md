# MPEG-H scene metadata resolver

纯 Go 标准库实现的音频场景段解析与增量更新器。它把节目、对象、扬声器布局
和工作于一次事务中的更新关联起来，输出每个对象实际落到哪些输出通道，并把
无法解析的引用、绑定和扩展逐项报告出来。

## 核心保证

- 配置段是完整场景快照；更新段只修改明确出现的字段。
- 每次更新都带 `effectiveSequence`。缺序号、重复序号和倒序更新都会失败。
- 每次更新都在副本上执行，成功才提交；失败结果不含旧快照或旧映射。
- 每次提交后重新解析全部引用，不对未出现在新布局下的对象沿用旧布局的通道。
- 对象删除先于 upsert 执行；删除后重新添加空对象不会复活旧通道。
- 布局切换后，只使用对象对当前布局的显式绑定；没有绑定就报告
  `layout_mapping_missing`，不猜测、不回退。
- 未知 JSON 字段报告为 `unknown_field`；显式 `extensions` 原样保留，
  每次报告为 `unknown_extension`。
- 同一 JSON 对象内的重复键、重复实体 ID、重复通道 ID 直接拒绝。

## 运行

```sh
go test ./...
go vet ./...
go build -o /tmp/scenecheck ./cmd/scenecheck
/tmp/scenecheck -in testdata/segments.json -out artifacts/report.json
```

`-in`/`-out` 使用 `-` 表示 stdin/stdout。输入可以是 JSON 数组，也可以是
连续 JSON 段（JSONL 或空格分隔）。输出是 `{"updates":[...]}`。

退出码（指构建出的 `scenecheck` 进程；`go run` 会把任意非零状态折叠为 1）：

- `0`：所有段生效，且没有未解析字段。
- `1`：至少一个更新被拒绝，但报告已写出。
- `2`：参数、输入流或输出错误。
- `3`：所有段都已生效，但存在未解析字段。

## 段格式

配置段：

```json
{
  "type": "config",
  "sequence": 1,
  "config": {
    "sceneId": "demo",
    "layouts": [
      {
        "id": "5.1",
        "channels": [
          {"id": "L"}, {"id": "R"}, {"id": "C"},
          {"id": "LFE"}, {"id": "Ls"}, {"id": "Rs"}
        ]
      },
      {
        "id": "stereo",
        "channels": [{"id": "L"}, {"id": "R"}]
      }
    ],
    "programs": [
      {"id": "main", "layoutId": "5.1", "objectIds": ["voice"]}
    ],
    "objects": [
      {
        "id": "voice",
        "layoutChannels": {
          "5.1": ["C"],
          "stereo": ["L", "R"]
        }
      }
    ]
  }
}
```

也可以省略外层 `config`，把 `layouts`、`programs`、`objects` 直接放在段对象
里。`sequence` 也可写作 `effectiveSequence` 或 `effective_sequence`。

对象通道绑定有两种形式：

- `layoutChannels` 按布局 ID 给出绑定，布局切换时优先使用对应项。
- `channels` 是布局无关的显式绑定；切换后引用的通道仍必须在当前布局中存在。

两种绑定都不会从上一布局继承。对象在 `layoutChannels` 中存在当前布局键但数组
为空时，视为显式空绑定，同样报告 `empty_channel_binding`。

更新段：

```json
{
  "type": "update",
  "sequence": 2,
  "update": {
    "programs": [
      {"id": "main", "layoutId": "stereo"}
    ],
    "objects": [
      {"id": "voice", "layoutChannels": {"stereo": ["L", "R"]}}
    ],
    "deleteObjects": ["old-effect"],
    "deleteLayouts": [],
    "deletePrograms": [],
    "layouts": [],
    "extensions": {"x-vendor/update-token": 44}
  }
}
```

Patch 语义：

- 程序/对象 patch 中未出现的字段保持不变；`objectIds: []` 清空列表。
- `layouts` 是按 `id` 的完整替换/新增，不会与旧通道列表合并。
- `deleteObjects`、`deleteLayouts`、`deletePrograms` 在 upsert 之前执行。
- 同一 ID 同时删除和 upsert 时删除生效，并报告
  `conflicting_delete_and_upsert`。

## 引用解析

解析按当前状态全量进行，输出顺序确定：

1. 每个节目的 `layoutId` 必须存在。
2. 每个 `objectIds` 项必须存在；同一节目重复引用只生成一次映射并报告。
3. 当前布局下选择对象绑定，逐项校验通道是否存在、是否重复。
4. 生成 `objectChannels`，同时给出通道 ID 和从 0 开始的输出索引。

常见未解析原因：

| reason | 含义 |
| --- | --- |
| `unknown_layout` | 节目引用了不存在的布局 |
| `missing_object` | 节目引用了已删除或不存在的对象 |
| `unknown_layout_binding` | 对象绑定了不存在的布局 |
| `layout_mapping_missing` | 新布局没有该对象的绑定 |
| `missing_channel_binding` | 对象没有任何通道绑定 |
| `unknown_output_channel` | 绑定引用了布局中不存在的通道 |
| `duplicate_channel_binding` | 同一对象重复绑定同一通道 |
| `unknown_field` | 段中存在未识别字段 |
| `unknown_extension` | 显式扩展未被语义解析，但已保留 |

## 库调用

```go
engine := scene.NewEngine()
result, err := engine.ApplyJSON(segmentBytes)
if err != nil {
    // 段被拒绝：result.EffectiveSequence 和 result.Unresolved 仍可用于报告。
}
for _, mapping := range result.ObjectChannels {
    fmt.Println(mapping.ObjectID, mapping.LayoutID, mapping.Channels)
}
for _, issue := range result.Unresolved {
    fmt.Println(issue.Path, issue.Reason, issue.Value)
}
```

批量输入使用 `engine.ProcessStream`。单段解析使用 `scene.ParseSegment`；
`scene.ParseConfig`/`scene.ParseUpdate` 也接受裸对象。

## 样例

`testdata/segments.json` 演示六个序号：

1. 5.1 配置，`voice -> C`，`ambience -> Ls/Rs`。
2. 切到立体声，两个对象都重新解析到 `L/R`。
3. 删除 `ambience`，节目中的悬空引用被报告，映射立即消失。
4. 显式重建 `ambience` 并只给出立体声绑定。
5. 切到不存在的 `7.1.4`，所有映射为空，并报告 `unknown_layout`，没有沿用
   立体声映射。
6. 切回 5.1 并显式恢复两个对象绑定。

`artifacts/report.json` 是上述命令的完整输出。每个元素都有
`effectiveSequence`、`revision`、`applied`、`objectChannels` 和
`unresolved`；被拒绝的段不包含 `snapshot`。
