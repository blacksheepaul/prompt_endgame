又一个“LLM 网关“，包含请求 → 排队 → 合并 → streaming → 限流 → 熔断这些 LLM 网关会需要的能力。

- 可拔插的 LLM Provider（vLLM、外部 API、Mock）
- 使用 Grafana+Prometheus+Loki+OpenTelemetry 支撑可观测性，所有关键行为都可观测：排队、熔断、限流、首 token、tokens/s

---

#### 前端（for 测试）

https://github.com/blacksheepaul/prompt_endgame_fe

##### 典型场景

和多个 AI NPC 进行交流，类似群面，但是多个面试官拷打同一个面试者(已经不在这个context了...)

> 你是面试的 candidate，多个 agent 会针对你简历上的内容（其实是我简历上的内容）进行轮番拷打，每个 agent 都有各自的 persona

##### 用户故事

- 断线重连期间的事件生产与回放（已补充见 `docs/user_story.md`）

##### 核心玩法设计

类似象棋残局，你会面对一个固定的上下文，在此基础上有两种玩法模式：

1. 你需要存活尽可能多的对话轮次（从深度拷打中活下来）
2. 你需要在最少轮次内获得最高认可度

##### for example

TODO: 此处应有一张图片或 高清 gif

### Todo List

#### Stage Zero：文档建设

- [x] `docs/architecture.md` 描述架构设计，包括数据模型、数据流转

#### Stage A：网关骨架完善

- [x] 语义与状态机梳理：room/turn 状态转移、允许/拒绝条件、错误语义
- [x] 幂等与错误语义统一：SubmitAnswer/Cancel 的幂等边界与 HTTP 语义
- [x] SSE 回放一致性：fromOffset/Last-Event-ID 语义与历史+实时拼接
- [x] 断线重连一致性：不重不漏校验与测试
- [x] cancel 可用性闭环：HTTP/WS cancel 立即停止 streaming，事件完整
- [x] 最小演示闭环：create/answer/stream/cancel 路径可演示说明

#### Stage B：可观测性与 Baseline 压测

- [x] 最小可观测性：埋点（active turns、goroutine 数、处理延迟）、暴露 `/metrics`
- [ ] 分级压测建立 Baseline：10/50/100 并发测试，收集内存、延迟、goroutine 曲线
- [ ] 关键指标输出：p95/p99、TTFT、tokens/s、pprof 截图

##### Baseline 压测执行（profile 驱动）

- profile 放在 `benchmarks/profiles/stageb_v1/`
- 压测脚本只读取 profile，并校验 mockllm `/admin/config` 与 profile 的 `expected_config` 一致
- 不再由压测脚本动态 PATCH mockllm 配置

执行方式：

```bash
# 单个 profile
go run ./scripts/baseline_loadtest.go \
  --base-url http://localhost:10180 \
  --profile benchmarks/profiles/stageb_v1/50_fast.json

# 跑 Stage B baseline 矩阵
./scripts/run_stage_b.sh
```

profile schema 与校验规则见：`docs/baseline_profile.md`

#### Stage C：高并发与流式 I/O（基于 Baseline 优化）

- [ ] 连接管理：SSE/WS 心跳、超时、最大连接数限制
- [ ] 背压：worker pool + 队列上限 + reject/degrade 策略（基于压测数据调参）
- [ ] 速率限制：按 room / IP / token 维度的限流策略
- [ ] 幂等/重试：请求去重与重放安全
- [ ] 优化效果验证：对比优化前后的 metrics 数据

#### Stage D：可观测性增强

- [ ] OTEL tracing：HTTP → App → Provider 全链路 span
- [ ] 结构化日志：room/turn 维度，错误与取消事件可追踪
- [ ] 精确 Tokenizer：基于 tiktoken/sentencepiece 的 token 计算（用于计费、统计、指标等）

#### Stage E：调度与 Provider 体系

- [x] Provider 抽象：mock + external API + vLLM 预留
- [ ] Multi-Provider
- [ ] 路由策略：local-prefer / cost-aware / quality-aware
- [ ] 会话策略：sticky session + provider 选择可追踪
- [ ] 熔断与 failover：按 provider 维度
- [ ] 流量治理: 限流(全局/分层)

#### Stage F：工程质量

- [ ] 单测：room/turn/event sink/handler 关键路径
- [ ] 集成测试：create/answer/stream/cancel 端到端
- [ ] 负载测试：并发下的稳定性回归

---

#### Legacy（原规划中移除的条目）

以下条目来自早期路线图，当前主线不再追踪，保留供参考。

<details>
<summary>展开查看</summary>

**原 Stage 1 — 游戏玩法闭环**

- **scenery & persona**
    - 最小 schema（Go struct + validate）
    - scenery#1：群面拷打（3 interviewers）
    - persona 要有：archetype / system prompt / 禁忌与目标

- **orchestrator v0**
    - 三个 agent 串行轮询发言
    - 用户回答 → 推进下一轮
    - 支持策略：串行（v0 默认）/ 随机（feature flag）
    - 预留接口：小模型调度

- **计分模型 + 结算**
    - hp / 认可度 / round
    - 失败原因必须 event 化
    - 回合结束自动结算

- **Demo 打磨**
    - 固定演示脚本：正常流 / cancel / 背压触发 / 断线重连
    - 录屏 / gif
    - README 演示说明

**原 Stage 2 — 稳定性与用户可见功能**

- **故障注入**（稳定性工程子项）
    - 延迟 / 错误率 / 限流注入
    - Demo 开关

- **可观测升级**（完整版，超出 Stage C 范围的部分）
    - metrics → Prometheus
    - dashboard → Grafana
    - logs → Loki
    - tracing → OTEL 完整版（baggage / span link / error semantic）

- **用户可见功能**
    - 排行榜
    - event log replay（UI / CLI 均可）
    - 自动报告 v0（失误点标注，rule-based / heuristic）

**原 Stage 3 — 产品化**

- **checkpoint 系统**
    - 保存 room state + event offset
    - 从 checkpoint 派生新 scenery（难度递增 / 分支剧情）

- **多语种**
    - 中英双语 persona
    - 模拟中文思考 → 英文表达
    - 评估信息损失

- **语音交互**
    - ASR / TTS
    - latency 统计

- **教学系统**
    - 更精细评分（多维度 / 权重 / 证据 event）
    - 追问树
    - 更优答案建议（rule + LLM hybrid）

</details>
