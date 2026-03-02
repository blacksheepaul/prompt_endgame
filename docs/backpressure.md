场景：

1000个room同时submit answer，每个turn调用3个agent，每个agent流式输出1000个token；

我们要做的就是调度turn对agent的调用。如果不限流，可能会把下游打挂，导致我方被封禁，进而导致业务被迫降级。

和AI头脑风暴了一下，好的坏的方案很多

1. 抢占式（乐观锁/抢锁）
2. 令牌桶
3. 类似GMP那样work stealing(不过调度goroutine和调度LLM，区别很大，本质是成本模型不一样)
4. 优先队列，对用户分级，队列分成不同优先级
5. limited全局work pool，类似1，但不用mutex用channel

LLM流式输出耗时长，CAS失败率高，CPU长时间空转
公平性问题，随机可能导致某些room饿死
没有背压

附录：下游rate limites

- openai：https://developers.openai.com/api/docs/guides/rate-limits
- deepseek(no rate limits, best effort): https://api-docs.deepseek.com/quick_start/rate_limit
- kimi
    - https://platform.moonshot.cn/docs/introduction#%E9%80%9F%E7%8E%87%E9%99%90%E5%88%B6
    - https://platform.moonshot.cn/docs/pricing/limits
