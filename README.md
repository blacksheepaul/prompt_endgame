该项目的初衷是为我在 2026Q1 的求职进程中添加 "AI gateway" 的选项可能性，它将会是一个“LLM 网关项目“，包含请求 → 排队 → 合并 → streaming → 限流 → 熔断这些游戏 LLM 网关会需要的场景。

挖掘 AI 的使用场景并且盈利不是我的强项，但如果不考虑盈利，我认为可以玩的点还挺多的。

敬请见证。

---

#### 技术设计原则：

-   使用 SSE 向客户端传输 token 流
-   使用 WS 作为控制面（取消/改参/心跳/查询状态）
-   可拔插的 LLM Provider（vLLM、外部 API、Mock）
-   使用 Grafana+Prometheus+Loki+OpenTelemetry 支撑可观测性，所有关键行为都可观测：排队、熔断、限流、首 token、tokens/s

#### 典型场景

和多个 AI NPC 进行交流，类似群面，但是多个面试官拷打同一个面试者

> 你是面试的 candidate，多个 interview agent 会针对你简历上的内容（其实是我简历上的内容）进行轮番拷打，每个 agent 都有各自的 persona

#### 用户故事(TODO)

#### 核心玩法设计

类似象棋残局，你会面对一个固定的上下文，在此基础上有两种玩法模式：

1. 你需要存活尽可能多的对话轮次（从深度拷打中活下来）
2. 你需要在最少轮次内获得最高认可度（通过你的 SOTA 让 agent 折服于你的艺术！）

##### for example

TODO: 此处应有一张图片或 高清 gif

### Todo List

#### Stage 1, playable MVP

阶段目标：核心玩法验证

1. 定义 scenery&persona 数据结构，制作 scenery#1
2. room/turn 状态机+event log(transcript)
3. SSE 单路流式输出+minimal API(create room / answer / stream / cancel)
4. (hanging)orchestrator: 三个 agent 串行发言+用户回答推进
   考虑中的调度模型：串行调度、随机调度、引入一个更小规模的 LLM 进行调
5. 计分模型(hp/认可度/round)+结算
6. 打断（HTTP cancel / WS）
7. 最小可观测（日志+少量指标）

额外演示内容：

-   并发限流/队列背压（哪怕是简单的 worker pool + 队列长度上限 + reject 策略）

-   压测 + pprof 截图/数据（例如 README 放一张 p95/p99 + goroutine profile ）

#### Stage 2, external demo

阶段目标：可对外展示

1. 抽象 provider (vLLM / external API / mock)
2. 路由策略(prefer local / external / cost aware) + sticky
3. healthy check + 熔断 + 自动 failover + 故障注入(mini 混沌工程)
4. 可观测升级，上 prometheus+grafana+loki+otel
5. 排行榜
6. 基于 event log 的回放
7. 生成基础版报告（失误点标注）

#### Stage 3

阶段目标：高完成度、产品感

1. checkpoint & 派生新的 scenery
2. 多语种（其实只做中英）（用来模拟说中文的“全英”面试）（可能会有信号损失）
3. 语音输入 + TTS
4. 更精细的评分和教学（追问树、更优答案 etc）

#### Gantt

```mermaid
gantt
title Project Roadmap
dateFormat YYYY-MM-DD

section Stage 1 (MVP - runnable core)
骨架跑通(rooms/answer/SSE/cancel/event log) :a1, 2026-01-13, 2d
玩法最小闭环(单场景+基础计分+串行调度)       :a2, after a1, 3d
压测&稳定性修整(pprof/连接/取消一致性)       :a3, after a2, 2d

section Stage 2 (External demo - gateway taste)
provider抽象+多provider接入(mock/vLLM/external) :b1, 2026-01-20, 3d
路由策略+sticky session                         :b2, after b1, 2d
稳定性工程(health/cb/failover/故障注入)         :b3, after b2, 3d

section Stage 3 (Productize)
可观测升级(loki/otel/prom/grafana)              :c1, 2026-01-28, 5d
replay+排行榜+自动报告(基础版)                   :c2, after c1, 6d
打磨&发布(文档/演示/案例场景扩充)                :c3, after c2, 10d

```

```mermaid
gantt
    title Stage 1 - Playable MVP (start 01-13)
    dateFormat  YYYY-MM-DD

    section 主干链路
    极简骨架(rooms/answer/SSE/event stream)      :a1, 2026-01-13, 1d
    cancel中断一致性(HTTP优先; WS可后置)          :a2, after a1, 1d

    section 内容与数据
    scenery/persona 最小schema(Go struct + validate) :b1, 2026-01-14, 1d
    scenery#1(群面拷打) + 文案打磨                   :b2, after b1, 1d

    section 核心引擎
    room/turn 状态机(Idle/Streaming/Done/Cancelled)  :c1, 2026-01-15, 1d
    event log(append + subscribe + fromOffset回放)   :c2, 2026-01-13, 2d

    section Orchestrator(MVP只做串行调度)
    单房间串行调度(用户->agents->用户推进)            :d1, 2026-01-16, 1d
    多agent轮询(最小策略：round-robin)               :d2, after d1, 1d

    section 玩法闭环(能“通关/失败”)
    计分v0(规则少但可解释)                           :e1, 2026-01-17, 1d
    结算/失败原因(event化)                            :e2, after e1, 1d

    section 可观测(最小闭环)
    基础指标(TTFT/tokens/s/cancel-lat) + 日志结构化    :f1, 2026-01-17, 1d

    section Demo 稳定
    压测/pprof/连接稳定性/内存泄漏排查                :g1, 2026-01-18, 1d
    Demo脚本&彩排&修bug                               :g2, after g1, 1d


```

Stage 1 验收标准

-   SSE 能持续输出事件（chunk/event types 正常）
-   cancel 可靠：不再继续吐 chunk，并落 TurnCancelled
-   event log 支持 fromOffset 重连回放
-   单房间串行调度 + 多 agent 轮询能跑完一局
-   计分/结算至少能给出：得分 + 失败原因（可解释）

```mermaid
xychart-beta
    title "Stage 1 Burndown (Workdays, start 2026-01-13)"
    x-axis ["D1","D2","D3","D4","D5","D6","D7"]
    y-axis "Remaining Tasks" 0 --> 14
    line "Ideal"  [14,10,8,6,4,2,0]
    line "Actual" [14,14,14,14,14,14,14]

```

```mermaid
gantt
    title Stage 2 - External Demo
    dateFormat  YYYY-MM-DD

    section Provider
    抽象provider接口(ports/adapters重构切入点)         :a1, 2026-01-20, 1d
    mock + external(OpenAI等)                           :a2, after a1, 1d
    vLLM接入(本地优先/可选fallback)                      :a3, after a2, 1d

    section 路由与会话
    路由策略v0(prefer local / cost aware)                :b1, 2026-01-23, 1d
    sticky session + session迁移策略(最小版)             :b2, after b1, 1d

    section 稳定性工程(minimal, 只做能展示的关键路径)
    health check + readiness                              :c1, 2026-01-25, 1d
    熔断(按provider维度) + fallback                       :c2, after c1, 1d
    failover + 故障注入(最小：延迟/错误率注入)            :c3, after c2, 1d



```

### 项目框架

入口 cmd/server/main.go
| - 加载配置 internal/config/config.go
| - 手动 DI internal/wiring/wiring.go
