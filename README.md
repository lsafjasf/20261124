# subtime

字幕入出点的有理数重映射工具。单 Go module，无第三方依赖。

## 为什么不漂

- `23.976` 固定解析为 `24000/1001`，`29.97DF` 固定解析为 `30000/1001`，小数不会被当作浮点数。
- 所有中间帧位保存为 `math/big.Rat`。`23.976 -> 25 -> 29.97DF -> ...` 的过程不吸附、不取整。
- 跨帧位置返回 floor 和 ceil 两个候选，`SnapNone` 会直接报错，不存在隐藏的 floor。
- `RoundTripN` 用精确位置反算回源帧率，连续多轮往返逐帧恢复原帧号。
- 29.97DF 使用真实帧号与 SMPTE 标签互转，正确处理每分钟丢 00/01（整十分钟除外）以及整点边界。

映射语义是“同一呈现时刻换帧网格”：源帧 `n @ rS` 的时刻仍是 `n/rS` 秒，在目标帧率 `rT` 上的精确帧位是 `n*rT/rS`。它不会自动改播放速度。

## 快速使用

查看单帧的精确目标位置、吸附候选和多轮往返：

```bash
go run ./cmd/subtime map \
  -from 23.976 -via 25 -to 29.97df \
  -frame 1 -round-trips 3
```

跨帧时输出类似：

```text
source   00:00:00:01 frame 1 @ 23.976 (1001/24000 s)
target   exact frame position 5/4 @ 29.97DF (1001/24000 s)
         straddles frames; explicit candidates:
         * frame 1 00:00:00;01 offset -1/4 frames
           frame 2 00:00:00;02 offset 3/4 frames
roundtrip 3 x -> frame 1 (00:00:00:01)
```

加 `-json` 可获得完整的精确有理数和候选 JSON。

转换 SRT。`-start`、`-end` 必须显式给出；短尾需要显式允许补足一帧：

```bash
go run ./cmd/subtime srt \
  -from 23.976 -via 25 -to 29.97df \
  -in movie.srt -out movie-2997df.srt \
  -start floor -end ceil \
  -ensure-one-frame \
  -ms nearest \
  -report mapping.json
```

`-ms` 只控制最终写 SRT 的毫秒落点，默认 `nearest`。`-report` 会记录每个 cue 的精确目标位置、floor/ceil 候选、问题与短尾调整。

## 库 API

```go
source := subtime.Frame{Number: 24000, Rate: subtime.Rate23976}

// 只在最后一跳映射；中间精确位置不会被取整。
point, err := subtime.ProjectFrame(source, subtime.Rate25, subtime.Rate2997DF)
candidates, err := point.Candidates() // floor + ceil，若正好在帧上则只有一个

frame, err := point.Snap(subtime.SnapCeil) // 必须显式选择

recovered, err := subtime.RoundTripN(source, 3,
	subtime.Rate25, subtime.Rate2997DF) // recovered == source
```

入出点：

```go
mapping, err := subtime.MapInterval(interval, subtime.Rate2997DF)
// mapping.Start / mapping.End 是精确目标位置
// mapping.StartCandidates / mapping.EndCandidates 是吸附候选
// mapping.Issues 会报告 tail_under_one_frame、zero_length 等

resolved, err := mapping.Resolve(subtime.IntervalPolicy{
	Start:          subtime.SnapFloor,
	End:            subtime.SnapCeil,
	EnsureOneFrame: true,
})
```

批量映射不去重，重复 ID、重复时间点都按输入顺序保留：

```go
mapped, err := subtime.MapIntervals(intervals, subtime.Rate2997DF)
```

## 测试

```bash
go test ./...
go vet ./...
```

测试固定了 DF 整点、丢号分钟、跨小时入出点、短于一帧的尾点、重复时间点、显式候选以及多轮往返。
