# ADM 编辑器（PCM 不可变）

纯 Go 标准库实现的 ADM（Annotation Data Model）事务编辑器。工程语言为 Go，
**外部依赖为零**，只用标准库。

## 设计原则

- ADM 编辑页只允许修改三类数据：对象名称、组关系（对象所属组、组的父组）、
  位置点。对象/组集合、`pcmRef`、schema 版本、PCM 路径与哈希等均为不可变元数据。
- 底层 PCM 必须**逐字节不变**。编辑器永远以只读方式打开 PCM，从不创建写句柄；
  事务开始前和提交前各做一次 SHA-256 比对。
- 整个编辑过程是**单事务**：任一步失败即整体回滚，ADM 与 PCM 文件保持事务前状态。
- 提交成功后**重新从磁盘解析** ADM，逐项核对旧值没有残留，并再次哈希 PCM。

## 目录结构

```
pcm/            PCM 二进制容器：解析、构造、PTS 载荷、SHA-256
adm/            ADM 文档模型、白名单编辑、事务提交/回滚/残留核对
cmd/adm/        零依赖命令行入口（含端到端测试）
```

## PCM 二进制格式

小端序，容器自带校验和：

```
"PCM1" | version(u16) | flags(u16) | chunkCount(u32)
重复 chunk: type(u32) | reserved(u32) | length(u64) | data[length]
sha256[32]  —— 对之前所有字节求 SHA-256
```

- `type=1` PTS：位置/时间样本，记录几何引用 id（ADM 对象的 `pcmRef` 必须存在）
- `type=2` RAW：不透明载荷
- 魔数错误、版本/标志位非法、chunk 类型未知、截断、尾部冗余字节、哈希失配均拒绝解析

## ADM 文档格式

JSON 文档（`schemaVersion=1`），包含组森林和对象列表；对象的位置点时间戳为
Unix 纳秒，必须**严格递增、禁止重复**。

## 事务提交流程

1. 只读打开 PCM 并校验文档中记录的 SHA-256，索引 PTS 中的几何引用 id
2. 在深拷贝工作副本上累积白名单操作
3. 提交时先校验不可变字段与原文档完全一致
4. 全量校验：时间点顺序、对象/组/PCM 引用存在性、组层级无环
5. 预检输出目标可写（只读文件/目录直接失败，不触碰任何文件）
6. 再次从磁盘哈希 PCM，确认无漂移
7. 写同目录临时文件 → `fsync` → 原子 `rename`
8. 重新解析落盘文档，核对每个预期变更已生效且旧值无残留，并再哈希 PCM

失败路径：工作副本丢弃、临时文件清理、事务关闭；ADM 与 PCM 均保持原样。

## 回滚触发条件

| 条件 | 错误哨兵 |
| --- | --- |
| 位置点时间戳乱序或重复 | `adm.ErrPointsOrder` / `adm.ErrDuplicatePoint` |
| 对象、组或 PCM 几何引用不存在 | `adm.ErrUnknownObject` / `ErrUnknownGroup` / `ErrUnknownPCMRef` |
| 组父子关系成环 | `adm.ErrGroupCycle` |
| 输出文件或目录只读 | `adm.ErrReadOnlyOutput` |
| PCM 字节发生任何变化 | `adm.ErrPCMChanged` |
| 修改白名单外字段/增删对象或组 | `adm.ErrImmutableField` |

## 命令行用法

```sh
go build ./...
go test ./...

adm init <pcmPath> <admPath> <objID> <pcmRef>
adm rename        <admPath> <objID> <newName>
adm set-group     <admPath> <objID> <groupID|->
adm set-parent    <admPath> <groupID> <parentID|->
adm set-points    <admPath> <objID> <time:x:y:z,...>
adm add-point     <admPath> <objID> <time> <x> <y> <z>
adm delete-point  <admPath> <objID> <time>
adm verify        <admPath>
```

每个编辑子命令对应一次完整事务；`-` 表示清空组归属或父组。

## 测试

- `go test ./...`：单元 + 回滚 + 端到端测试
- `go test -race ./...`：竞态检测
- 覆盖：PCM 损坏/截断/哈希失配、时间乱序、悬空引用、组环、只读输出、
  PCM 漂移、不可变字段篡改，以及成功提交后的旧值残留核对
