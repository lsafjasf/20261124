# VobSub 保守迁移

将 VobSub `.idx/.sub` 中的**时间、位置、颜色**迁入 ASS 文本字幕，不做 OCR，也不从画面猜台词。

ASS 中每条可迁移事件只显示类似 `[VobSub 0001]` 的编号标记；它不是台词，也不冒充台词。每条原始图形字幕都会导出 PNG，并在 `review.md` / `review.json` 中逐条列出。人工确认文字后再替换标记即可。

## 运行

```bash
go run ./cmd/vobsubtext \
  -idx /path/to/movie.idx \
  -sub /path/to/movie.sub \
  -out out/movie \
  -rounds 3
```

`-sub` 可省略，默认使用与 `.idx` 同名的 `.sub`。输出目录：

```text
out/movie/
├── subtitles.ass          # 只含编号标记、入出点、位置和主色
├── review.json            # 逐条机器可读审查清单
├── review.md              # 逐条附原图的人工审查页
├── verification.json      # 多轮 ASS 回读与边界合成结果
├── original/              # 透明裁图、整帧原图、动画各状态图
├── composite/             # 每条事件的入点/末帧/出点合成图
└── raw/                   # 原 SPU 数据，便于追溯
```

内置夹具演示：

```bash
go run ./cmd/vobsubtext -demo -out dist/demo -rounds 3
```

当前仓库没有真实 VobSub 素材，`dist/demo` 是内置夹具的完整转换/验证产物；换成自己的 `.idx/.sub` 即可处理实际字幕。

退出码：`0` 成功且验证通过；`2` 输入/参数/解析失败；`3` 入出点或合成验证失败。

## 迁移边界

- 时间：从 `.idx` 的 `timestamp` 与 SPU 控制序列的 `STA_DSP` / `STP_DSP` 取得。没有停止命令时不推断出点；短于 ASS 厘秒精度的区间也不写入 ASS。两类都留在审查清单。
- 位置：取 `SET_DAREA` 的显示矩形和左上角；ASS 使用 `\an7\pos(x,y)`。
- 颜色：把 SPU 的 4 项 palette 映射、contrast/alpha 与 `.idx` 的 16 色 palette 合成，换算为逐色 RGBA。ASS 事件取面积最大的可见色作为主色，完整色表保留在审查清单。
- 描边/阴影/抗锯齿：同一图形里出现多个可见颜色即标记 `outline_or_shadow_or_antialias`，原图和整帧合成图都会附上。不会凭空生成 ASS 描边样式。
- 动画：同一可见区间内显示矩形、palette、alpha 或像素数据发生变化时标记 `animation`，导出每个不同状态的 PNG，默认不写 ASS。
- 文字：一律标记 `text_not_recognized_no_ocr`。工具不会识别、补全或猜测字符。
- 其他：未知控制命令、截断 SPU、越界矩形、无效 palette、缺失出点都会逐条说明并保守处理。

ASS 的时间精度只有厘秒。写入时使用最近 10 ms，并在 `review.json` 中记录相对源时间的 `start_delta_ms` / `end_delta_ms`；源值仍以毫秒完整保留。

## 入出点验证

`-rounds N` 会重新打开生成后的 `subtitles.ass` 做 N 轮独立回读，而不是复用内存中的事件：

1. 核对事件数、编号、入点、出点、位置、主色和透明度。
2. 检查每条事件在入点为激活、出点前 1 ms 仍激活、出点即失效。
3. 分别合成入点、末帧、出点三张整帧 PNG，并与按源矩形摆放的参考图逐像素比较。
4. 后续轮次重新解析并重新合成，比较逻辑图像 SHA-256，确认结果稳定。

任一检查失败都会写入 `verification.json`，命令退出码为 `3`。

## Go 接口

```go
result, err := vobsub.Convert(vobsub.ConvertOptions{
    IndexPath: "movie.idx",
    SubPath:   "movie.sub",
    OutDir:    "out/movie",
    Rounds:    3,
})
if err != nil {
    return err
}
if !result.Verification.OK {
    // result.Verification.Failures 逐条列出边界/合成错误
}
```

仅使用 Go 标准库；`go.mod` 没有第三方依赖。运行时不调用 FFmpeg、OCR、libass 或其他字幕包。

## 测试

```bash
go test ./...
go test -race ./...
go vet ./...
```

测试现场生成 `.idx/.sub` 夹具，覆盖静态项、描边项、动画项、缺失出点项、RLE 三种码宽、ASS 回读、重复合成，以及“篡改 ASS 出点必须导致验证失败”。

## 范围

支持 DVD VobSub `.idx/.sub`（MPEG-PS + DVD SPU）。不解析 PGS/SUP、蓝光字幕或已经 OCR 的文本字幕。生成的 ASS 是**待人工转录的文本容器**，不是自动识别后的最终台词稿。

## 格式依据

- DVD Sub-Pictures: <https://dvd.sourceforge.net/dvdinfo/spu.html>
- FFmpeg `dvdsubdec` 参考: <https://ffmpeg.org/doxygen/trunk/dvdsubdec_8c_source.html>
