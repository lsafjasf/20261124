# subdrift

字幕时基漂移的最小复现、逐层定位与修复。技术栈仅使用 Go 标准库；所有时间换算
使用 `math/big.Rat` 精确有理数，只有 SVG 画布坐标使用 `float64`。

## 结论

样例是 `24000/1001`（23.976）fps、500 秒的片尾。旧下载链路把**帧计数按标称
24 fps 重新解释**，丢掉 `1001` 分母：

```text
正确：frame / (24000/1001) = frame * 1001 / 24000 秒
旧值：frame / 24                             秒
误差：-frame / 24000 秒 ≈ -t / 1001
```

回归样例中第 `24` 帧出现第一处**字幕边界**可观察误差（逐帧扫描时第 1 帧已有
约 `-0.0417 ms` 的亚毫秒 PTS 偏差，但它不是字幕事件）：

```text
frame 24 的正确 PTS = 24 * 90000 * 1001 / 24000 = 90090 tick
旧管线 PTS          = 24 * 90000 / 24           = 90000 tick
误差                = -90 tick = -1 ms
```

片尾第 `11988` 帧的精确时间是 `499999.5 ms`，旧输出为 `499500 ms`，正好提前
`500 ms`。误差随绝对时间线性增长，不是某一条字幕的偶然取整，因此**整体加
500 ms 只会把开头 1 ms 早的字幕变成 499 ms 晚**，不能作为修复。

## 快速开始

```sh
# 首个误差事件 + 全事件曲线 + 分层隔离
go run ./cmd/subdrift audit -project testdata/reel_500s.json

# 缺陷输出：片尾 00:08:19,500
go run ./cmd/subdrift convert -legacy -out /tmp/legacy.srt

# 修复输出：片尾 00:08:20,000
go run ./cmd/subdrift convert -out /tmp/fixed.srt

# 重新生成 docs/ERROR_CURVE.md、CSV、SVG、JSON
go run ./cmd/subdrift report

go test ./...
```

`convert` 默认走修复路径；`-legacy` 仅用于复现和回归对比。`audit -json` 输出
机器可读摘要。

## 四个误差层

| 层 | 旧管线 | 影响 | 实测片尾 |
| --- | --- | --- | ---: |
| 帧率 | `24000/1001` 帧按 `24` 帧解释 | 丢掉 `1001` 分母，线性漂移 | `-500 ms` |
| 时基 | 90 kHz PTS 后取整到毫秒 | 单次相位误差 | `-1 ms` |
| 取整 | PTS/ms/ASS 厘秒全部 `floor` | 0–10 ms 抖动 | `-10 ms` |
| 重复换算 | `ms -> cs -> ms -> SRT` | 每层丢掉余数，但绝对时间不累加 | 与上一层叠加 |

`audit` 的 `layer isolation` 表逐项验证：只换成标称 24 fps 就产生 `-500 ms`；
只保留 90 kHz/毫秒/厘秒取整只有 `-1` 到 `-10 ms`。主因因此不是“字幕本身有
半秒偏移”，而是帧率语义在第一条 PTS 转换处就错了。

## 首个误差事件

`frame->pts` 层的首个偏差（第 24 帧）：

```text
层          输入速率  输出时基  本层未取整  正确目标  实际输出  累计误差
frame->pts  24       90000    90000       90090     90000     -1 ms
pts->ms     90000    1000     1000        1001      1000      -1 ms
ms->cs      1000     100      100         100.1     100       -1 ms
cs->ms      100      1000     1000        1001      1000      -1 ms
```

后续层没有制造半秒误差；它们只是把第一层的错误继续带下去。完整逐帧明细见
`docs/ERROR_CURVE.md`、`artifacts/error_curve.csv` 与 `artifacts/error_curve.svg`。

## 修复

1. 帧计数始终以工程精确帧率 `24000/1001` 解释，不使用标称 `24` 回读。
2. 90 kHz PTS 由有理数换算，并只在 mux 边界做一次最近 tick 取整。
3. SRT 交付从精确帧时刻直接生成，不在 `ms -> ASS cs -> ms` 之间反复取整；
   若交付确实需要 ASS，厘秒量化只做一次，并承认其最大 5 ms 的输出分辨率。
4. 最终只在 SRT 毫秒边界做一次最近取整。修复路径在样例片尾输出
   `00:08:20,000`，与精确值 `499999.5 ms` 的误差为 `+0.5 ms`，不会漂移。

修复不是“在片尾补 500 ms”，也不是按总时长整体缩放。正确性约束是逐事件的：

```text
frame -> exact seconds -> PTS boundary -> final SRT milliseconds
```

每个事件的误差都有界，不会把上一事件的余数传给下一事件。

## 回归样例

- `testdata/reel_500s.json`：6 条字幕，`24000/1001` fps，90 kHz，ASS 厘秒中间层。
- `testdata/reel_500s_legacy.srt`：旧路径，最后一条结束于 `00:08:19,500`。
- `testdata/reel_500s_fixed.srt`：修复路径，最后一条结束于 `00:08:20,000`。
- `audit/audit_test.go`：锁定首个误差帧、片尾 `-500 ms`、分层隔离、整体偏移反例。
- `audit/report_test.go`：锁定 CSV/SVG/JSON/Markdown 输出与确定性。

误差曲线公式与检查点：

```text
e(frame) = -frame / 24 ms      （旧管线完整路径，厘秒 floor 另有 ≤10 ms 抖动）
e(t)     ≈ -t / 1001 s

frame    24    240    2400    5994    11988
error    -1    -10    -100    -250    -500 ms
```

## 目录

```text
timebase/       有理数帧率/时基、三种取整、逐层转换
subtitle/       工程模型、SRT 解析与格式化
audit/          缺陷/修复管线、首个误差、误差曲线、报告渲染
cmd/subdrift/   audit / convert / report 命令
testdata/       500 秒片尾样例与 legacy/fixed 金文件
docs/           自动生成的诊断报告
artifacts/      完整 CSV、SVG、JSON
```
