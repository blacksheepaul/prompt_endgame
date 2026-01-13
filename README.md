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
    title Stage 1 - Playable MVP
    dateFormat YYYY-MM-DD

    section 骨架(先跑起来再重构)
    极简链路(create/answer/stream/cancel + event log) :a1, 2026-01-13, 2d
    断线重连(fromOffset) + transcript一致性            :a2, after a1, 1d

    section 可观测
    OTEL tracing v0(http/app/provider spans + room/turn attrs) :b1, 2026-01-15, 1d
    指标v0(TTFT/tokens/s/cancel-lat) + 结构化日志             :b2, after b1, 1d

    section 内容与玩法闭环
    scenery&persona最小schema + scenery#1(群面拷打)           :c1, 2026-01-16, 2d
    room/turn状态机(Idle/Streaming/Done/Cancelled)            :c2, after c1, 1d
    orchestrator v0(3 agents 串行轮询 + 用户推进)             :c3, after c2, 2d
    计分模型v0(hp/认可度/round) + 结算(失败原因event化)        :c4, after c3, 1d

    section 额外演示内容
    worker pool + 队列上限 + reject策略(背压)                 :d1, 2026-01-22, 1d
    压测脚本 + p95/p99 + pprof(goroutine/heap)截图            :d2, after d1, 1d

    section Demo打磨
    Demo脚本&彩排&修bug(含一次“取消/背压/重连”表演)           :e1, 2026-01-24, 2d
```

```mermaid
gantt
    title Stage 2 - External Demo
    dateFormat YYYY-MM-DD

    section Provider(可插拔)
    provider接口抽象(从单体骨架重构切入点)            :a1, 2026-01-27, 2d
    接入mock + external API                            :a2, after a1, 2d
    接入vLLM(本地优先可配)                              :a3, after a2, 2d

    section 路由与会话
    路由策略v0(local-prefer / cost-aware / quality)     :b1, 2026-02-02, 2d
    sticky session + session key(按room/turn维度)       :b2, after b1, 2d

    section 稳定性工程
    health check(readiness/liveness)                    :c1, 2026-02-06, 1d
    熔断(按provider维度, error-rate/timeout)            :c2, after c1, 2d
    failover(熔断/超时触发) + 超时预算                   :c3, after c2, 2d
    故障注入(延迟/错误/限流) + Demo开关                  :c4, after c3, 1d

    section 可观测信号升级
    metrics(prometheus) + dashboard(grafana)            :d1, 2026-02-12, 2d
    logs(loki) + trace完善(otel: baggage/links/events)  :d2, after d1, 2d
    SLO视角展示(p99 TTFT / errors / saturation)         :d3, after d2, 1d

    section 用户可见功能
    event log replay(回放UI/CLI都行)                    :e1, 2026-02-17, 2d
    排行榜(最小版)                                       :e2, after e1, 1d
    自动报告v0(失误点标注: rule-based/heuristic)          :e3, after e2, 3d
```

```mermaid
gantt
    title Stage 3 - Productize
    dateFormat YYYY-MM-DD

    section 内容生产体系
    checkpoint(保存room状态+事件偏移)                   :a1, 2026-02-17, 3d
    基于checkpoint派生新scenery(分支/难度递增)           :a2, after a1, 4d

    section 体验增强
    中英双语(只做两套persona+提示词)                     :b1, 2026-02-24, 6d
    语音输入(ASR) + TTS(可选供应商/本地)                 :b2, after b1, 7d

    section 提升“产品感”
    更精细评分(维度/权重/证据event)                      :c1, 2026-03-09, 5d
    追问树/更优答案建议(半规则半模型)                     :c2, after c1, 5d
```

### 项目框架

入口 cmd/server/main.go
| - 加载配置 internal/config/config.go
| - 手动 DI internal/wiring/wiring.go
