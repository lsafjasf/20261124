# MicroDVD 帧号转毫秒

使用 Go 标准库解析 MicroDVD 字幕，把 `{起始帧}{结束帧}` 按指定帧率换算为毫秒，并输出逐条 JSON 结果。

## 用法

```bash
go run . -fps 23.976 subtitles.sub
cat subtitles.sub | go run . -fps 25
go run . -fps 25 -pretty=false subtitles.sub
```

参数：

- `-fps RATE`：源字幕帧率，必填且必须是大于零的有限数。
- `-pretty`：缩进 JSON，默认 `true`。
- `-fail-on-issues`：存在任何告警时以状态码 `1` 退出；默认仍输出结果并以 `0` 退出。
- `FILE`：输入文件；省略或传 `-` 时从标准输入读取。

解析或编码错误使用状态码 `2`。

## 解析规则

- 每行格式为 `{start}{end}text`，帧号允许负数，以检测异常时间。
- 开头的连续样式标签会单独解析，例如 `{Y:i}{C:$ff0000}`；样式值不做语义解释。
- `raw_text` 保留时间字段之后的原始 cue 文本，`raw_line` 保留完整原始行。
- `text` 去除开头的样式前缀，并将 MicroDVD 的 `|` 转为换行符，便于直接消费。
- UTF-8 BOM 会被接受；其他非法 UTF-8 会返回错误及字节偏移、行号。
- 毫秒值采用四舍五入到最近的整数毫秒。

## 输出

```json
{
  "fps": 25,
  "cue_count": 1,
  "issue_count": 0,
  "cues": [
    {
      "number": 1,
      "line": 1,
      "start_frame": 0,
      "end_frame": 25,
      "start_ms": 0,
      "end_ms": 1000,
      "duration_ms": 1000,
      "style_prefix": "{Y:i}",
      "style_tags": [
        {"name": "Y", "value": "i", "raw": "{Y:i}"}
      ],
      "raw_text": "{Y:i}Hello|world",
      "text": "Hello\nworld",
      "raw_line": "{0}{25}{Y:i}Hello|world"
    }
  ],
  "issues": []
}
```

`issues` 会同时出现在顶层和对应 cue 中。告警码：

- `invalid_timing`：该行不是合法的 `{start}{end}text`。
- `negative_time`：cue 含负帧号或负毫秒时间。
- `negative_interval`：结束时间早于开始时间。
- `overlap`：该 cue 与更早开始的 cue 时间重叠，`related_cue` 指向对方。
- `time_overflow`：帧号换算后超出 `int64` 毫秒范围。
- `duration_overflow`：时长差值超出 `int64` 范围。

## 验证

```bash
go test ./...
go vet ./...
```
