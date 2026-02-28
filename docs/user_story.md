# User Story

## SSE 断线重连期间的事件生产

**场景**

- 用户在 streaming 中刷新页面或短暂离线，SSE 连接中断
- 当前 turn 已经开始生成 token，LLM 继续输出并写入 EventSink
- 客户端短时间后重连，期望回放离线期间的历史事件，并无缝接上实时事件

**约束**

- 已开始的 turn 可以继续生成
- 未开始的 turn 不触发生成

**验收标准**

- 断线重连后事件不丢失、不重复
- fromOffset/Last-Event-ID 语义一致
- 历史回放与实时订阅之间无空窗
