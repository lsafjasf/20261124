# hdr10-remux

转码前置器:把**已确认**的 HDR10 元数据迁移到新的 Matroska 封装,
同时让旧字段彻底消失。纯 Go 标准库实现,**外部依赖为零**。

## 用法

```sh
go build -o hdr10-remux ./cmd/hdr10-remux
./hdr10-remux -i 输入.mkv -o 输出.mkv
```

## 行为保证

- **元数据确认**:仅当源容器同时具备 BT.2020 三要素(Primaries/Transfer/Matrix)、
  PQ 传输曲线、完整的 SMPTE ST 2086 mastering 字段以及 MaxCLL/MaxFALL 时,
  才认定为"已确认"并执行迁移;否则拒绝并回滚。
- **旧字段清除**:输出中不再出现 `ColourSpace`、`Gamma`、`Tags`、`Chapters`、
  `Attachments` 等旧字段;成功后通过结构化扫描复核确认。
- **原子回滚**:输出先写入同目录临时文件,`fsync` 并校验通过后才 `rename`
  替换目标。写权限不足、容器与码流无法同步(结构损坏、时间戳倒退、
  未知轨道引用等)、输出中途失败时,临时文件被清除,输入保持原样,
  无需用户手工修第二遍。
- **成功校验**:落盘后重新解析输出,逐轨道比对音视频载荷的 SHA-256
  与帧数,不一致即判定失败并回滚。

## 支持范围

- 输入:Matroska/WebM,视频 + 音频轨道;Block/SimpleBlock,
  支持 Xiph、定长、EBML 三种 lacing 读取。
- 输出:Matroska,无 lacing 的 SimpleBlock,Segment 未知长度流式写入。

## 测试

```sh
go test ./...
```

覆盖:成功迁移(含三种 lacing)、元数据未确认拒绝、码流损坏回滚、
写权限不足回滚、输入输出同路径拒绝。
