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

section Stage 1 (MVP)
核心玩法验证          :a1, 2026-01-13, 7d

section Stage 2 (demo)
核心功能实现        :b1, 2026-01-20, 8d

section Stage 3
产品化                               :c1, after b1, 21d

```

```mermaid
gantt
    title Stage 1 - Playable MVP
    dateFormat  MM-DD

    section 数据结构
    scenery & persona schema       :a1, 2026-01-13, 1d
    制作 scenery#1                     :a2, 2026-01-13, 1d

    section 核心引擎
    room/turn 状态机                  :b1, after a2, 1d
    event log (transcript)             :b2, after a2, 1d

    section API & 流式
    API(create/answer/stream)     :c1, after a2, 1d
    SSE 单路流式输出                   :c2, after a2, 1d
    cancel 中断 (HTTP/WS)              :c3, after c2, 1d

    section Orchestrator
    串行调度(agents + 用户推进)       :d1, after c2, 2d
    调度模型实验(小范围对比)           :d2, after c3, 1d

    section 玩法系统
    计分模型+结算     :e1, after d2, 1d

    section 可观测(最小)
    基础日志 + 少量指标                 :f1, after d2, 1d

    section Demo 打磨（关键）
    压测/pprof/连接稳定性              :g1, after f1, 1d
    Demo 脚本&彩排&修 bug              :g2, after g1, 1d

```

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
    抽象 provider 接口                   :a1, 2026-01-20, 1d
    mock / vLLM / external             :a2, after a1, 2d

    section 路由
    路由策略(prefer local/cost aware)   :b1, after a2, 1d
    sticky session                     :b2, after a2, 1d

    section 稳定性工程
    health check                      :c1, after b2, 1d
    熔断                               :c2, after b2, 1d
    failover                          :c3, after c2, 1d
    故障注入                           :c4, after c2, 1d

    section 可观测升级
    log(loki)                        :d1,after c4, 1d
    tracing(otel)                    :d2,after c4,1d
    metrics(prometheus)              :d3,after c4,2d
    grafana                          :d4,after c4,2d

    section 用户功能
    排行榜                              :e1, after d4, 1d
    replay                             :e2, after e1, 2d
    自动报告(基础版)                     :e3, after e2, 1d


```
