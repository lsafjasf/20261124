# 丢包恢复对比实验（Opus 风格 FEC / 静音插入 / 独立 PLC）

- 采样率 48000 Hz，帧长 960 样本（20 ms），共 60 帧（1.2 s）
- 纯 Go 标准库实现；接收器无法访问发送端干净源，恢复帧逐样本审计禁止原文回填。
- FEC 为发送端低通+8bit 量化的前一帧冗余，由下一帧分组携带。

状态链图例：`.` 正常 `R` 恢复交叉淡化 `S` 静音 `P` PLC `C` 冷启动PLC `F` FEC修复 `p` FEC回退PLC `s` FEC回退静音

## 孤立丢包（单帧，间隔足够，FEC 可修复）

丢包事件：帧6，帧18，帧34，帧49

| 策略 | 丢包帧 | 恢复样本数 | MAE | RMSE | 峰值误差 | 分段SNR(dB) | 输出 |
|---|---:|---:|---:|---:|---:|---:|---|
| Opus内建FEC | 4 | 3840 | 0.05108 | 0.12344 | 0.77931 | 7.86 | `output/isolated_fec.wav` |
| 静音插入 | 4 | 3840 | 0.25969 | 0.30511 | 0.70736 | 0.00 | `output/isolated_silence.wav` |
| 独立PLC | 4 | 3840 | 0.08442 | 0.11312 | 0.45640 | 8.62 | `output/isolated_plc.wav` |

状态链（每 token 一帧）：

- Opus内建FEC：`. . . . . . F . . . . . . . . . . . F . . . . . . . . . . . . . . . F . . . . . . . . . . . . . . F . . . . . . . . . .`
- 静音插入：`. . . . . . S . . . . . . . . . . . S . . . . . . . . . . . . . . . S . . . . . . . . . . . . . . S . . . . . . . . . .`
- 独立PLC：`. . . . . . P R . . . . . . . . . . P R . . . . . . . . . . . . . . P R . . . . . . . . . . . . . P R . . . . . . . . .`

逐帧状态链明细：

**Opus内建FEC**

| 帧 | 状态 | 丢包 | 恢复样本 | 衰减增益 | 基音滞后 |
|---:|---|:--:|---:|---:|---:|
| 0 | OK | false | 0 | - | - |
| 1 | OK | false | 0 | - | - |
| 2 | OK | false | 0 | - | - |
| 3 | OK | false | 0 | - | - |
| 4 | OK | false | 0 | - | - |
| 5 | OK | false | 0 | - | - |
| 6 | FEC | true | 960 | - | - |
| 7 | OK | false | 0 | - | - |
| 8 | OK | false | 0 | - | - |
| 9 | OK | false | 0 | - | - |
| 10 | OK | false | 0 | - | - |
| 11 | OK | false | 0 | - | - |
| 12 | OK | false | 0 | - | - |
| 13 | OK | false | 0 | - | - |
| 14 | OK | false | 0 | - | - |
| 15 | OK | false | 0 | - | - |
| 16 | OK | false | 0 | - | - |
| 17 | OK | false | 0 | - | - |
| 18 | FEC | true | 960 | - | - |
| 19 | OK | false | 0 | - | - |
| 20 | OK | false | 0 | - | - |
| 21 | OK | false | 0 | - | - |
| 22 | OK | false | 0 | - | - |
| 23 | OK | false | 0 | - | - |
| 24 | OK | false | 0 | - | - |
| 25 | OK | false | 0 | - | - |
| 26 | OK | false | 0 | - | - |
| 27 | OK | false | 0 | - | - |
| 28 | OK | false | 0 | - | - |
| 29 | OK | false | 0 | - | - |
| 30 | OK | false | 0 | - | - |
| 31 | OK | false | 0 | - | - |
| 32 | OK | false | 0 | - | - |
| 33 | OK | false | 0 | - | - |
| 34 | FEC | true | 960 | - | - |
| 35 | OK | false | 0 | - | - |
| 36 | OK | false | 0 | - | - |
| 37 | OK | false | 0 | - | - |
| 38 | OK | false | 0 | - | - |
| 39 | OK | false | 0 | - | - |
| 40 | OK | false | 0 | - | - |
| 41 | OK | false | 0 | - | - |
| 42 | OK | false | 0 | - | - |
| 43 | OK | false | 0 | - | - |
| 44 | OK | false | 0 | - | - |
| 45 | OK | false | 0 | - | - |
| 46 | OK | false | 0 | - | - |
| 47 | OK | false | 0 | - | - |
| 48 | OK | false | 0 | - | - |
| 49 | FEC | true | 960 | - | - |
| 50 | OK | false | 0 | - | - |
| 51 | OK | false | 0 | - | - |
| 52 | OK | false | 0 | - | - |
| 53 | OK | false | 0 | - | - |
| 54 | OK | false | 0 | - | - |
| 55 | OK | false | 0 | - | - |
| 56 | OK | false | 0 | - | - |
| 57 | OK | false | 0 | - | - |
| 58 | OK | false | 0 | - | - |
| 59 | OK | false | 0 | - | - |

**静音插入**

| 帧 | 状态 | 丢包 | 恢复样本 | 衰减增益 | 基音滞后 |
|---:|---|:--:|---:|---:|---:|
| 0 | OK | false | 0 | - | - |
| 1 | OK | false | 0 | - | - |
| 2 | OK | false | 0 | - | - |
| 3 | OK | false | 0 | - | - |
| 4 | OK | false | 0 | - | - |
| 5 | OK | false | 0 | - | - |
| 6 | SILENCE | true | 960 | - | - |
| 7 | OK | false | 0 | - | - |
| 8 | OK | false | 0 | - | - |
| 9 | OK | false | 0 | - | - |
| 10 | OK | false | 0 | - | - |
| 11 | OK | false | 0 | - | - |
| 12 | OK | false | 0 | - | - |
| 13 | OK | false | 0 | - | - |
| 14 | OK | false | 0 | - | - |
| 15 | OK | false | 0 | - | - |
| 16 | OK | false | 0 | - | - |
| 17 | OK | false | 0 | - | - |
| 18 | SILENCE | true | 960 | - | - |
| 19 | OK | false | 0 | - | - |
| 20 | OK | false | 0 | - | - |
| 21 | OK | false | 0 | - | - |
| 22 | OK | false | 0 | - | - |
| 23 | OK | false | 0 | - | - |
| 24 | OK | false | 0 | - | - |
| 25 | OK | false | 0 | - | - |
| 26 | OK | false | 0 | - | - |
| 27 | OK | false | 0 | - | - |
| 28 | OK | false | 0 | - | - |
| 29 | OK | false | 0 | - | - |
| 30 | OK | false | 0 | - | - |
| 31 | OK | false | 0 | - | - |
| 32 | OK | false | 0 | - | - |
| 33 | OK | false | 0 | - | - |
| 34 | SILENCE | true | 960 | - | - |
| 35 | OK | false | 0 | - | - |
| 36 | OK | false | 0 | - | - |
| 37 | OK | false | 0 | - | - |
| 38 | OK | false | 0 | - | - |
| 39 | OK | false | 0 | - | - |
| 40 | OK | false | 0 | - | - |
| 41 | OK | false | 0 | - | - |
| 42 | OK | false | 0 | - | - |
| 43 | OK | false | 0 | - | - |
| 44 | OK | false | 0 | - | - |
| 45 | OK | false | 0 | - | - |
| 46 | OK | false | 0 | - | - |
| 47 | OK | false | 0 | - | - |
| 48 | OK | false | 0 | - | - |
| 49 | SILENCE | true | 960 | - | - |
| 50 | OK | false | 0 | - | - |
| 51 | OK | false | 0 | - | - |
| 52 | OK | false | 0 | - | - |
| 53 | OK | false | 0 | - | - |
| 54 | OK | false | 0 | - | - |
| 55 | OK | false | 0 | - | - |
| 56 | OK | false | 0 | - | - |
| 57 | OK | false | 0 | - | - |
| 58 | OK | false | 0 | - | - |
| 59 | OK | false | 0 | - | - |

**独立PLC**

| 帧 | 状态 | 丢包 | 恢复样本 | 衰减增益 | 基音滞后 |
|---:|---|:--:|---:|---:|---:|
| 0 | OK | false | 0 | - | - |
| 1 | OK | false | 0 | - | - |
| 2 | OK | false | 0 | - | - |
| 3 | OK | false | 0 | - | - |
| 4 | OK | false | 0 | - | - |
| 5 | OK | false | 0 | - | - |
| 6 | PLC | true | 960 | 0.794 | 662 |
| 7 | FADE_IN | false | 0 | - | - |
| 8 | OK | false | 0 | - | - |
| 9 | OK | false | 0 | - | - |
| 10 | OK | false | 0 | - | - |
| 11 | OK | false | 0 | - | - |
| 12 | OK | false | 0 | - | - |
| 13 | OK | false | 0 | - | - |
| 14 | OK | false | 0 | - | - |
| 15 | OK | false | 0 | - | - |
| 16 | OK | false | 0 | - | - |
| 17 | OK | false | 0 | - | - |
| 18 | PLC | true | 960 | 0.794 | 948 |
| 19 | FADE_IN | false | 0 | - | - |
| 20 | OK | false | 0 | - | - |
| 21 | OK | false | 0 | - | - |
| 22 | OK | false | 0 | - | - |
| 23 | OK | false | 0 | - | - |
| 24 | OK | false | 0 | - | - |
| 25 | OK | false | 0 | - | - |
| 26 | OK | false | 0 | - | - |
| 27 | OK | false | 0 | - | - |
| 28 | OK | false | 0 | - | - |
| 29 | OK | false | 0 | - | - |
| 30 | OK | false | 0 | - | - |
| 31 | OK | false | 0 | - | - |
| 32 | OK | false | 0 | - | - |
| 33 | OK | false | 0 | - | - |
| 34 | PLC | true | 960 | 0.794 | 697 |
| 35 | FADE_IN | false | 0 | - | - |
| 36 | OK | false | 0 | - | - |
| 37 | OK | false | 0 | - | - |
| 38 | OK | false | 0 | - | - |
| 39 | OK | false | 0 | - | - |
| 40 | OK | false | 0 | - | - |
| 41 | OK | false | 0 | - | - |
| 42 | OK | false | 0 | - | - |
| 43 | OK | false | 0 | - | - |
| 44 | OK | false | 0 | - | - |
| 45 | OK | false | 0 | - | - |
| 46 | OK | false | 0 | - | - |
| 47 | OK | false | 0 | - | - |
| 48 | OK | false | 0 | - | - |
| 49 | PLC | true | 960 | 0.794 | 831 |
| 50 | FADE_IN | false | 0 | - | - |
| 51 | OK | false | 0 | - | - |
| 52 | OK | false | 0 | - | - |
| 53 | OK | false | 0 | - | - |
| 54 | OK | false | 0 | - | - |
| 55 | OK | false | 0 | - | - |
| 56 | OK | false | 0 | - | - |
| 57 | OK | false | 0 | - | - |
| 58 | OK | false | 0 | - | - |
| 59 | OK | false | 0 | - | - |

## 连续丢包（两段 4 帧突发，共 8 帧）

丢包事件：帧10-13（4帧），帧40-43（4帧）

| 策略 | 丢包帧 | 恢复样本数 | MAE | RMSE | 峰值误差 | 分段SNR(dB) | 输出 |
|---|---:|---:|---:|---:|---:|---:|---|
| Opus内建FEC | 8 | 7680 | 0.12384 | 0.18544 | 0.74522 | 0.98 | `output/burst_fec.wav` |
| 静音插入 | 8 | 7680 | 0.17522 | 0.20748 | 0.53392 | 0.00 | `output/burst_silence.wav` |
| 独立PLC | 8 | 7680 | 0.15371 | 0.20204 | 0.74522 | 0.23 | `output/burst_plc.wav` |

状态链（每 token 一帧）：

- Opus内建FEC：`. . . . . . . . . . p p p F . . . . . . . . . . . . . . . . . . . . . . . . . . p p p F . . . . . . . . . . . . . . . .`
- 静音插入：`. . . . . . . . . . S S S S . . . . . . . . . . . . . . . . . . . . . . . . . . S S S S . . . . . . . . . . . . . . . .`
- 独立PLC：`. . . . . . . . . . P P P P R . . . . . . . . . . . . . . . . . . . . . . . . . P P P P R . . . . . . . . . . . . . . .`

逐帧状态链明细：

**Opus内建FEC**

| 帧 | 状态 | 丢包 | 恢复样本 | 衰减增益 | 基音滞后 |
|---:|---|:--:|---:|---:|---:|
| 0 | OK | false | 0 | - | - |
| 1 | OK | false | 0 | - | - |
| 2 | OK | false | 0 | - | - |
| 3 | OK | false | 0 | - | - |
| 4 | OK | false | 0 | - | - |
| 5 | OK | false | 0 | - | - |
| 6 | OK | false | 0 | - | - |
| 7 | OK | false | 0 | - | - |
| 8 | OK | false | 0 | - | - |
| 9 | OK | false | 0 | - | - |
| 10 | FEC_FALLBACK_PLC | true | 960 | 0.794 | 675 |
| 11 | FEC_FALLBACK_PLC | true | 960 | 0.631 | 675 |
| 12 | FEC_FALLBACK_PLC | true | 960 | 0.501 | 675 |
| 13 | FEC | true | 960 | - | - |
| 14 | OK | false | 0 | - | - |
| 15 | OK | false | 0 | - | - |
| 16 | OK | false | 0 | - | - |
| 17 | OK | false | 0 | - | - |
| 18 | OK | false | 0 | - | - |
| 19 | OK | false | 0 | - | - |
| 20 | OK | false | 0 | - | - |
| 21 | OK | false | 0 | - | - |
| 22 | OK | false | 0 | - | - |
| 23 | OK | false | 0 | - | - |
| 24 | OK | false | 0 | - | - |
| 25 | OK | false | 0 | - | - |
| 26 | OK | false | 0 | - | - |
| 27 | OK | false | 0 | - | - |
| 28 | OK | false | 0 | - | - |
| 29 | OK | false | 0 | - | - |
| 30 | OK | false | 0 | - | - |
| 31 | OK | false | 0 | - | - |
| 32 | OK | false | 0 | - | - |
| 33 | OK | false | 0 | - | - |
| 34 | OK | false | 0 | - | - |
| 35 | OK | false | 0 | - | - |
| 36 | OK | false | 0 | - | - |
| 37 | OK | false | 0 | - | - |
| 38 | OK | false | 0 | - | - |
| 39 | OK | false | 0 | - | - |
| 40 | FEC_FALLBACK_PLC | true | 960 | 0.794 | 743 |
| 41 | FEC_FALLBACK_PLC | true | 960 | 0.631 | 743 |
| 42 | FEC_FALLBACK_PLC | true | 960 | 0.501 | 743 |
| 43 | FEC | true | 960 | - | - |
| 44 | OK | false | 0 | - | - |
| 45 | OK | false | 0 | - | - |
| 46 | OK | false | 0 | - | - |
| 47 | OK | false | 0 | - | - |
| 48 | OK | false | 0 | - | - |
| 49 | OK | false | 0 | - | - |
| 50 | OK | false | 0 | - | - |
| 51 | OK | false | 0 | - | - |
| 52 | OK | false | 0 | - | - |
| 53 | OK | false | 0 | - | - |
| 54 | OK | false | 0 | - | - |
| 55 | OK | false | 0 | - | - |
| 56 | OK | false | 0 | - | - |
| 57 | OK | false | 0 | - | - |
| 58 | OK | false | 0 | - | - |
| 59 | OK | false | 0 | - | - |

**静音插入**

| 帧 | 状态 | 丢包 | 恢复样本 | 衰减增益 | 基音滞后 |
|---:|---|:--:|---:|---:|---:|
| 0 | OK | false | 0 | - | - |
| 1 | OK | false | 0 | - | - |
| 2 | OK | false | 0 | - | - |
| 3 | OK | false | 0 | - | - |
| 4 | OK | false | 0 | - | - |
| 5 | OK | false | 0 | - | - |
| 6 | OK | false | 0 | - | - |
| 7 | OK | false | 0 | - | - |
| 8 | OK | false | 0 | - | - |
| 9 | OK | false | 0 | - | - |
| 10 | SILENCE | true | 960 | - | - |
| 11 | SILENCE | true | 960 | - | - |
| 12 | SILENCE | true | 960 | - | - |
| 13 | SILENCE | true | 960 | - | - |
| 14 | OK | false | 0 | - | - |
| 15 | OK | false | 0 | - | - |
| 16 | OK | false | 0 | - | - |
| 17 | OK | false | 0 | - | - |
| 18 | OK | false | 0 | - | - |
| 19 | OK | false | 0 | - | - |
| 20 | OK | false | 0 | - | - |
| 21 | OK | false | 0 | - | - |
| 22 | OK | false | 0 | - | - |
| 23 | OK | false | 0 | - | - |
| 24 | OK | false | 0 | - | - |
| 25 | OK | false | 0 | - | - |
| 26 | OK | false | 0 | - | - |
| 27 | OK | false | 0 | - | - |
| 28 | OK | false | 0 | - | - |
| 29 | OK | false | 0 | - | - |
| 30 | OK | false | 0 | - | - |
| 31 | OK | false | 0 | - | - |
| 32 | OK | false | 0 | - | - |
| 33 | OK | false | 0 | - | - |
| 34 | OK | false | 0 | - | - |
| 35 | OK | false | 0 | - | - |
| 36 | OK | false | 0 | - | - |
| 37 | OK | false | 0 | - | - |
| 38 | OK | false | 0 | - | - |
| 39 | OK | false | 0 | - | - |
| 40 | SILENCE | true | 960 | - | - |
| 41 | SILENCE | true | 960 | - | - |
| 42 | SILENCE | true | 960 | - | - |
| 43 | SILENCE | true | 960 | - | - |
| 44 | OK | false | 0 | - | - |
| 45 | OK | false | 0 | - | - |
| 46 | OK | false | 0 | - | - |
| 47 | OK | false | 0 | - | - |
| 48 | OK | false | 0 | - | - |
| 49 | OK | false | 0 | - | - |
| 50 | OK | false | 0 | - | - |
| 51 | OK | false | 0 | - | - |
| 52 | OK | false | 0 | - | - |
| 53 | OK | false | 0 | - | - |
| 54 | OK | false | 0 | - | - |
| 55 | OK | false | 0 | - | - |
| 56 | OK | false | 0 | - | - |
| 57 | OK | false | 0 | - | - |
| 58 | OK | false | 0 | - | - |
| 59 | OK | false | 0 | - | - |

**独立PLC**

| 帧 | 状态 | 丢包 | 恢复样本 | 衰减增益 | 基音滞后 |
|---:|---|:--:|---:|---:|---:|
| 0 | OK | false | 0 | - | - |
| 1 | OK | false | 0 | - | - |
| 2 | OK | false | 0 | - | - |
| 3 | OK | false | 0 | - | - |
| 4 | OK | false | 0 | - | - |
| 5 | OK | false | 0 | - | - |
| 6 | OK | false | 0 | - | - |
| 7 | OK | false | 0 | - | - |
| 8 | OK | false | 0 | - | - |
| 9 | OK | false | 0 | - | - |
| 10 | PLC | true | 960 | 0.794 | 675 |
| 11 | PLC | true | 960 | 0.631 | 675 |
| 12 | PLC | true | 960 | 0.501 | 675 |
| 13 | PLC | true | 960 | 0.398 | 675 |
| 14 | FADE_IN | false | 0 | - | - |
| 15 | OK | false | 0 | - | - |
| 16 | OK | false | 0 | - | - |
| 17 | OK | false | 0 | - | - |
| 18 | OK | false | 0 | - | - |
| 19 | OK | false | 0 | - | - |
| 20 | OK | false | 0 | - | - |
| 21 | OK | false | 0 | - | - |
| 22 | OK | false | 0 | - | - |
| 23 | OK | false | 0 | - | - |
| 24 | OK | false | 0 | - | - |
| 25 | OK | false | 0 | - | - |
| 26 | OK | false | 0 | - | - |
| 27 | OK | false | 0 | - | - |
| 28 | OK | false | 0 | - | - |
| 29 | OK | false | 0 | - | - |
| 30 | OK | false | 0 | - | - |
| 31 | OK | false | 0 | - | - |
| 32 | OK | false | 0 | - | - |
| 33 | OK | false | 0 | - | - |
| 34 | OK | false | 0 | - | - |
| 35 | OK | false | 0 | - | - |
| 36 | OK | false | 0 | - | - |
| 37 | OK | false | 0 | - | - |
| 38 | OK | false | 0 | - | - |
| 39 | OK | false | 0 | - | - |
| 40 | PLC | true | 960 | 0.794 | 743 |
| 41 | PLC | true | 960 | 0.631 | 743 |
| 42 | PLC | true | 960 | 0.501 | 743 |
| 43 | PLC | true | 960 | 0.398 | 743 |
| 44 | FADE_IN | false | 0 | - | - |
| 45 | OK | false | 0 | - | - |
| 46 | OK | false | 0 | - | - |
| 47 | OK | false | 0 | - | - |
| 48 | OK | false | 0 | - | - |
| 49 | OK | false | 0 | - | - |
| 50 | OK | false | 0 | - | - |
| 51 | OK | false | 0 | - | - |
| 52 | OK | false | 0 | - | - |
| 53 | OK | false | 0 | - | - |
| 54 | OK | false | 0 | - | - |
| 55 | OK | false | 0 | - | - |
| 56 | OK | false | 0 | - | - |
| 57 | OK | false | 0 | - | - |
| 58 | OK | false | 0 | - | - |
| 59 | OK | false | 0 | - | - |

