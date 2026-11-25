# 实验场景

这八个场景是这套内核被真模型真主机跑过之后留下来的，不是为文档编的示例。每个文件是一份完整的实验：靶场要摆成什么样、对产品说哪一句话、什么算答对、什么算糊弄。

场景都在参照靶场上跑过，靶场本身写在 [交接文档第 5 节](../../docs/aiops/03-handover.md)（`5.3` 测试机、`5.4` 容器化验收环境、`5.5` 日志数据集）。`fixture` 里的命令是给那套靶场用的，换环境要按自己的容器名和路径改。

## 一份场景长什么样

```json
{
  "id": "s1-dependency-container-exit",
  "kind": "diagnosis",
  "title": "依赖容器退出，健康检查仍然 200",
  "premise": "为什么这个场景值得跑：矛盾点在哪，取证入口在哪",
  "fixture": ["把靶场摆成故障现场的命令"],
  "cleanup": ["跑完恢复原状的命令"],
  "requires": ["这次运行需要的环境变量，可省略"],
  "task": "原样发给产品的那段话",
  "expect": ["答对必须满足的几条"],
  "traps": ["看起来像答案、其实是糊弄的几种"],
  "recorded": {"flow": 125, "section": "4.96", "verdict": "这一轮真跑的结果"}
}
```

`task` 是给产品看的那段话，可以原样用，也可以改成自己的主机名。`expect` 与 `traps` 是人工判卷的口径——这个仓库不自动判分，因为"结论对不对"要看证据能不能复现，不是字符串匹配。

`recorded.flow` 是参照靶场上的 flow 编号，`recorded.section` 指向交接文档里那次运行的记录。编号只在那套环境里有效，别的地方对不上。

## 怎么跑一个

先用 `fixture` 把靶场摆好，然后用仓库自带的命令行入口跑 `task`：

```bash
python scripts/opsagi_run.py "$(python -c "import json;print(json.load(open('scripts/scenarios/s1-dependency-container-exit.json'))['task'])")"
```

Windows PowerShell 下取字段：

```powershell
$task = (Get-Content scripts/scenarios/s1-dependency-container-exit.json -Raw | ConvertFrom-Json).task
python scripts/opsagi_run.py $task
```

`opsagi_run.py` 需要 `OPSAGI_URL`、`OPSAGI_MAIL`、`OPSAGI_PASSWORD`（或 `--token`），跑完把每条 report 按顺序打出来，flow 到 `finished` 才返回 0。也可以把 `task` 贴进界面新建会话，效果一样。

跑完记得执行 `cleanup`：`s5` 会留一个吃满两个核的容器，`s1` 停掉的数据库不会再自己起来。

## 这八个场景在验什么

| 场景 | 验的是 |
|---|---|
| `s1-dependency-container-exit` | 健康检查绿着、接口 500 时，能不能把根因落到依赖容器的退出上 |
| `s2-slow-query-missing-index` | 性能结论里的毫秒数是不是真的量出来的，没改成 `goal_met=false` 敢不敢填 |
| `s3-row-lock-wait` | 现场等待链与日志里的历史等待能不能分开，阻塞方已消失时会不会硬编一个 pid |
| `s4-log-archive-replay` | 读数是回放还是实时，能不能从时间戳上看出来并改口 |
| `s5-scheduling-latency-trace` | `ops_trace` 的直方图汇总能不能进结论，数字否定报障假设时敢不敢说"假设不成立" |
| `s6-statement-baseline` | `ops_baseline` → `ops_diff` 的闭环，语句段有没有进指纹，自比对会不会被拦下 |
| `l1-hdfs-block-warnings` | 从真实日志原文里读次数、block 数、性质 |
| `l4-spark-healthy-control` | 对照组：日志全 INFO 时，敢不敢顶回"没病" |

八个里两个是控制组（`l4` 全 INFO，`s3` 现场已无阻塞方）。控制组比故障组更容易露馅：故障场景答错只是不准，控制场景编造就是硬伤。

## 加一个新场景

复制一份最接近的改。`id` 用小写加连字符，`kind` 取 `diagnosis` / `optimization`，`task` 写成值班工程师会说的那句话，别在提示词里教它用哪个工具——工具选得对不对本身就是被验的一部分。`expect` 与 `traps` 写你在真实运行里见过的两种结局，写完先自己跑一遍再提交。
