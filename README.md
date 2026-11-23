# editchange — 可恢复的剪辑变更单应用器

Go 单模块实现，把一份剪辑变更单（修剪、移动、删除）逐项写入工程，
同时满足四个硬性要求：

1. **删除后再修剪不硬套**：同一片段的多个操作先合并成与输入顺序无关的
   单一动作；删除优先，删除之后的修剪不会被强行套到不存在的片段上。
2. **逐项写入 + 撤销日志**：每次写入都在 `Journal` 里记录逆操作
   （更新前快照 / 删除位置与快照），日志可序列化为 JSON 并反向重放。
3. **父子轨道时间不一致即暂停**：应用前基于最终几何位置预计算冲突，
   通过 `Resolver` 接口让用户选择中止、级联修剪、标记脱离、删除子片段
   或跳过父片段操作。
4. **失败后工程保持原样**：任何失败（校验、冲突中止、解析器错误）都会
   按撤销日志回滚，并断言工程与应用前完全一致；同一变更单按任意顺序
   应用三次都收敛到同一结果。

## 模块结构

```
videoedit/          核心库（无第三方依赖，仅用标准库）
  model.go          工程 / 轨道 / 片段模型，毫秒制 JSON 时间
  order.go          变更单与操作类型
  plan.go           顺序无关的合单、动作校验、确定性排序
  overlap.go        合并后同轨重叠检测
  conflict.go       父子轨道最终几何冲突预计算
  resolver.go       中止 / 固定选择 / 交互式三种决策器
  journal.go        撤销日志与反向重放
  apply.go          应用器主流程与冲突决策
  apply_write.go    逐项写入、回滚、结果汇总
cmd/editchange/     命令行入口（apply / undo / demo）
testdata/           工程与变更单示例
```

## 变更单语义

所有时间量都是毫秒（JSON 数字），也支持 Go duration 字符串。

| action     | delta 含义                                              |
|------------|--------------------------------------------------------|
| trimStart  | 移动片段头部：`>0` 缩短，`<0` 延伸，同时平移 sourceIn |
| trimEnd    | 移动片段尾部：`<0` 缩短，`>0` 延伸                    |
| move       | 整条片段在时间线上平移                                 |
| delete     | 删除片段，忽略 delta                                   |

合单规则（针对同一片段，代数可交换，因此与顺序无关）：

- 任意 `delete` 存在 → 最终动作为删除，其余修剪/移动全部忽略。
- 否则 `trimStart`、`trimEnd`、`move` 的 delta 分别求和。
- 合单后统一校验：长度必须为正、起点不得早于 0、sourceIn 不得为负、
  同轨片段最终不得重叠；任一不满足则整体拒绝，工程不写入任何内容。

## 父子轨道冲突与用户选择

子轨道（`parentId` 指向父轨道）上的片段在基线上必须包含在父轨道的某个
片段内。应用前用「基线 + 合并动作」直接计算每个片段的最终几何位置，
因此冲突集合本身也与操作顺序无关。检测到两类冲突：

- `overflow`：子片段最终区间超出父片段（父片段被修剪/移动，或子片段
  自身被修剪/移动）。可选：`abort`、`cascade`（子片段跟随父片段做相同
  修剪）、`detach`（保留并打 `detached` 标记）、`deleteChildren`。
- `parentDelete`：父片段被删除而子片段仍在。可选：`abort`、
  `deleteChildren`、`detach`、`skipParent`（跳过父片段删除）。

决策在任何写入之前完成；`abort` 或决策器报错都会触发完整回滚。

## 撤销日志

每条写入对应一条 `JournalEntry`：

- `updateClip`：保存写入前完整快照，回滚时原样写回。
- `deleteClip`：保存被删片段与所在轨道下标，回滚时按原位置插回。

日志可落盘为 JSON，用 `undo` 子命令对另一处（或恢复后的）工程重放。
回滚结束后应用器还会用深拷贝基线做一次等值断言，不一致直接 panic，
避免「看似回滚成功、实则残留」的情况。

## 命令行用法

```bash
go build ./cmd/editchange

# 应用变更单并持久化撤销日志；冲突时交互式询问
./editchange apply -project testdata/project.json \
  -order testdata/order-normal.json -journal /tmp/journal.json \
  -mode interactive

# 非交互模式：abort（默认）/ fixed:<choice>
./editchange apply -project p.json -order o.json -mode fixed:detach

# 按日志恢复
./editchange undo -project p.json -journal /tmp/journal.json

# 三种顺序收敛性演示
./editchange demo
```

## 设计保证与验证

- `Plan` 是纯函数：合单、校验、冲突检测都不修改工程，输出确定排序。
- 应用器先深拷贝基线，再逐项写入并记录日志；失败路径只有「回滚 +
  等值断言」，不存在半写入状态外泄。
- 测试覆盖（`go test ./...`，含 `-race`）：
  - 删除与修剪混合、未知片段、修剪到非正长度、同轨重叠等拒绝路径；
  - 三种操作顺序在三种冲突决策下收敛到同一工程状态；
  - 冲突中止 / 级联 / 脱离 / 连带删除 / 跳过父片段；
  - 撤销日志 JSON 往返、反向重放恢复到基线、日志损坏报错；
  - 真实构建二进制的 CLI 端到端 apply → undo 与中止保持原样。
