# 20261124

## admprobe — ADM BWF 只读解析与时间轴探针

影院版本交付前核对 ADM BWF（ITU-R BS.2076）的静态检查工具，Go 标准库实现，
只读、不渲染、不解码音频样本。

### 用法

```sh
go run ./cmd/admprobe <file.bwf>   # 有 ERROR 时退出码为 1
```

### 检查内容

- RIFF 块清单（fmt / axml / chna / data）与声道数、时长
- chna 表：重复 UID、重复轨号、轨数与 fmt 声道数不符、指向越界轨
- axml 内部引用：programme→content→object→pack/channel/stream/track/UID 缺引用
- chna ↔ axml 双向核对：chna 悬空 UID、XML 定义了 UID 但未挂物理轨
- 对象挂接：XML 有 audioObject 但无 audioTrackUIDRef，或其 UID 无 chna 行（音频未挂接）
- 时间轴探针：对象起止、物理轨解析、每通道 audioBlockFormat 序列与位置坐标，
  检测 block 间隙（BLK-GAP）与重叠（BLK-OVERLAP）

输出仅为元数据分析，不代表对象已被渲染或试听。

### 样例与测试

```sh
go run ./cmd/mkminbwf   # 重新生成 testdata/minimal.bwf（2 轨：bed + 运动对象）
go test ./...
```
