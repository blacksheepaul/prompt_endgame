# Stage B: 可观测性与 Baseline 压测

## 目标

建立完整的可观测性体系，完成分级压测并输出关键指标基线，为后续 Stage C（高并发与流式 I/O 优化）提供数据支撑。

---

## 背景

当前系统已实现基础骨架（Stage A 完成），但使用的 mock provider 过于简单，无法产生有意义的压测数据。Fake LLM（localhost:10181）已就绪，支持：

- OpenAI 兼容的 `/v1/chat/completions`（SSE 流式）
- 故障注入：`/admin/config` 支持延迟、抖动、并发限制、背压模拟
- 统计：`/admin/stats` 提供实时 QPS、并发数、队列深度

---

## 执行计划（细化版）

### Phase 1: OpenAI Provider 基础实现

**目标**: 实现最基本的 SSE 流式连接，能工作即可，不追求完美。

**子任务**:

#### 1.1 创建 Config 结构
- **文件**: `internal/adapter/provider/openai/config.go`
- **内容**: 
  ```go
  type Config struct {
      Endpoint string
      Model    string
      APIKey   string
  }
  ```
- **要求**: 如果 Endpoint 或 Model 为空，直接 panic

#### 1.2 实现基础 Provider 结构
- **文件**: `internal/adapter/provider/openai/provider.go`
- **内容**:
  - `NewProvider(cfg Config) *Provider` - 创建 provider，验证配置
  - `Provider` 结构体持有 config 和 http.Client

#### 1.3 实现 SSE 流式解析（最小可用版本）
- **功能**: 只解析 `data: {...}` 行，提取 `choices[0].delta.content`
- **错误处理**: 网络错误直接返回，不重试
- **TTFT**: 暂不实现精确计时

**验收标准**:
- [ ] 能连接 localhost:10181 并打印收到的 token
- [ ] 手动测试通过：`go run cmd/test_openai/main.go`

---

### Phase 2: 配置与 Wiring 集成

**目标**: 让系统可以通过环境变量切换到 OpenAI provider。

**子任务**:

#### 2.1 扩展 Config 结构
- **文件**: `internal/config/config.go`
- **修改**:
  - `ProviderConfig` 添加 `Type string` 和 `OpenAI OpenAIConfig`
  - `OpenAIConfig` 包含 Endpoint, Model, APIKey
  - 环境变量: `PROVIDER_TYPE`, `PROVIDER_ENDPOINT`, `PROVIDER_MODEL`, `PROVIDER_API_KEY`

#### 2.2 更新 Wiring 逻辑
- **文件**: `internal/wiring/wiring.go`
- **修改**:
  - 根据 `cfg.Provider.Type` 创建对应 provider
  - `openai` -> `openai.NewProvider(cfg.Provider.OpenAI)`
  - `mock` -> `mock.NewProvider(cfg.Provider.Mock.TokenDelay)`
  - 默认使用 mock

**验收标准**:
- [ ] `PROVIDER_TYPE=mock` 启动 mock provider
- [ ] `PROVIDER_TYPE=openai` 启动 openai provider
- [ ] 配置错误时启动失败（panic 或错误日志）

---

### Phase 3: OpenAI Provider 完善

**目标**: 添加重试、错误处理、TTFT 计时。

**子任务**:

#### 3.1 添加重试机制
- **要求**: 每次重试重新创建 http.Request（避免 body 被消费）
- **重试条件**: 网络错误（timeout, connection refused）
- **最大重试**: 3 次

#### 3.2 添加 TTFT 计时
- **定义**: 从 HTTP POST 发送到收到第一个非空 token
- **实现**: 在请求前记录 startTime，第一个 token 到达时计算差值
- **注意**: 暂时只记录，不集成到 metrics

#### 3.3 完善错误处理
- **错误类型**: timeout, connection_refused, 429, parse_error
- **行为**: 
  - 可重试错误 -> 重试
  - 429 -> 退避重试（1s, 2s）
  - 不可重试 -> 立即返回错误

**验收标准**:
- [ ] 网络错误时自动重试
- [ ] 429 时退避重试
- [ ] 能正确计算 TTFT

---

### Phase 4: Metrics 完善

**目标**: 添加缺失的指标。

**子任务**:

#### 4.1 添加 ProviderErrors 指标
- **文件**: `internal/adapter/metrics/metrics.go`
- **内容**:
  ```go
  var ProviderErrors = promauto.NewCounterVec(...)
  ```
- **Labels**: type (timeout, connection_refused, 429, parse_error, cancelled)

#### 4.2 添加 QueueWaitTime 指标
- **文件**: `internal/adapter/metrics/metrics.go`
- **内容**:
  ```go
  var QueueWaitTime = promauto.NewHistogram(...)
  ```

#### 4.3 在 turn_runtime.go 中集成
- **位置**: `internal/app/turn_runtime.go`
- **修改**:
  - 在适当位置记录 ProviderErrors
  - 在适当位置记录 QueueWaitTime

**验收标准**:
- [ ] `/metrics` 端点能看到新指标
- [ ] 手动触发错误，观察计数器增加

---

### Phase 5: 压测脚本增强（简化版）

**目标**: 支持场景配置，输出基础报告。

**子任务**:

#### 5.1 添加 Fake LLM 场景配置
- **文件**: `scripts/baseline_loadtest.go`
- **内容**:
  - 定义场景结构体（fast, slow, backpressure）
  - 压测前调用 `/admin/config` 设置 Fake LLM 参数
  - 压测后重置配置

#### 5.2 添加场景参数支持
- **修改**: 支持 `--scenario` 参数
- **行为**: 根据场景名加载对应配置

#### 5.3 输出基础 Markdown 报告
- **内容**:
  - 测试配置（Fake LLM 参数、系统配置）
  - 关键指标（吞吐量、延迟分布）
  - 原始数据路径
- **要求**: 格式正确，能阅读即可，不追求美观

**验收标准**:
- [ ] 支持 `--scenario=fast` 等参数
- [ ] 能自动配置 Fake LLM
- [ ] 生成可读的 Markdown 报告

---

### Phase 6: 自动化脚本

**目标**: 一键执行所有压测。

**子任务**:

#### 6.1 创建 run_stage_b.sh
- **文件**: `scripts/run_stage_b.sh`
- **功能**:
  - 检查 Fake LLM 和 prompt_endgame 服务是否运行
  - 执行测试矩阵（10:fast, 10:slow, 50:fast, 50:backpressure, 100:fast）
  - 生成汇总报告

**验收标准**:
- [ ] `./scripts/run_stage_b.sh` 能完整执行
- [ ] 生成所有测试报告

---

## 依赖与前置条件

### Fake LLM 端点
- 主服务: `http://localhost:10181/v1/chat/completions`
- Admin API: `http://localhost:10181/admin/config`
- Stats API: `http://localhost:10181/admin/stats`

### Prompt Endgame 端点
- API: `http://localhost:10180`
- Metrics: `http://localhost:10180/metrics`
- pprof: `http://localhost:10180/debug/pprof/`

### 工具依赖
- Go 1.25+
- `go tool pprof`（已内置）
- Graphviz（用于生成火焰图 SVG）: `brew install graphviz`

---

## 成功标准

- [ ] OpenAI Provider 能连接 Fake LLM 并完成对话
- [ ] 能通过环境变量切换 provider 类型
- [ ] 所有指标在 `/metrics` 端点可见
- [ ] 压测脚本支持场景配置
- [ ] 生成 Markdown 报告
- [ ] `./scripts/run_stage_b.sh` 能完整执行

---

## 附录

### Fake LLM Admin API 参考

```bash
# 查看当前配置
curl http://localhost:10181/admin/config

# 更新配置
curl -X PATCH http://localhost:10181/admin/config \
  -H "Content-Type: application/json" \
  -d '{
    "max_concurrent": 10,
    "fixed_delay_ms": 100,
    "jitter_ms": 50,
    "slowdown_qps_threshold": 50,
    "slowdown_factor": 0.5
  }'

# 查看统计
curl http://localhost:10181/admin/stats
```

### Prometheus 查询示例

```promql
# TTFT P95
histogram_quantile(0.95, rate(prompt_endgame_ttft_seconds_bucket[5m]))

# Turn Duration P99
histogram_quantile(0.99, rate(prompt_endgame_turn_duration_seconds_bucket[5m]))

# Tokens/s 均值
rate(prompt_endgame_tokens_total[5m]) / rate(prompt_endgame_turn_total[5m])

# 错误率
rate(prompt_endgame_provider_errors_total[5m]) / rate(prompt_endgame_turn_total[5m])
```
