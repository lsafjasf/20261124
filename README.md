# id3chap — ID3v2 章节树校验与无损重写工具

纯 Go（仅标准库）实现的 ID3v2.3 / ID3v2.4 CHAP/CTOC 章节工具。CHAP、CTOC、
TIT2 帧按元素 ID 关联，出现顺序无关。

## 功能

- 解析 ID3v2.3/2.4：synchsafe 长度、标签/帧级 unsynchronisation、扩展头、
  分组标识、帧标志；压缩/加密帧按不透明字节保留。
- 建立章节树：CHAP 为叶子，CTOC 的有序子元素构成边，支持任意嵌套。
- 校验并分类报告：
  - `cycle` — CTOC 父子引用形成循环（DFS 三色标记）。
  - `duplicate_chapter` — 重复元素 ID（后续帧忽略）。
  - `duplicate_title` — 多个 CHAP 共用相同 TIT2。
  - `out_of_audio_range` — 起止毫秒超出音频时长（含 TOC 推导 span）。
  - `dangling_reference` — CTOC 引用不存在的元素 ID。
  - `invalid_timing` — CHAP start > end。
  - `nested_order` — 有序子节点重叠/乱序，或子节点超出父 TOC span。
  - `multiple_roots` — 存在多个无父节点的根。
- 未知帧（TXXX、私有帧等）逐字节保留，并在报告中列出。
- 音频时长来源：Xing/Info 帧计数 → MPEG 帧头扫描 → TLEN（`--tlen`）
  → `--duration-ms` 手动覆盖。

## 无损重写保证

`retag` 只重新序列化 tag 区块：解析时把 tag 之后的全部字节切分为 `Audio`，
写出时原样拼接，并在写前/写后各计算一次 SHA-256 断言一致；音频帧不会被
解码或重编码。未知帧保留其原始 wire payload（含标志位与前缀）。

## 用法

```sh
go build ./cmd/id3chap

# 输出 JSON 报告
id3chap inspect [--duration-ms N] [--tlen] file.mp3

# 只重写 tag 区块；可删除指定帧、加 padding
id3chap retag --out new.mp3 [--drop TXXX]... [--padding 256] file.mp3
```

## 包结构

- `internal/id3` — tag/frame 解析、文本编码、CHAP/CTOC 解码、tag 序列化。
- `internal/mp3` — MPEG-1/2 Layer III 帧头扫描与时长估算。
- `internal/chapters` — 章节图构建、循环检测、校验与 JSON 报告。
- `cmd/id3chap` — `inspect` / `retag` 命令行入口。

运行测试：`go test ./...`
