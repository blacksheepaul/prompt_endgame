- [ ] p2 events传了太多东西，浪费流量

- [ ] p1 保存了rooms的太多状态，单个room占用的内存按照轮次线性增长，资源受限场景下容易OOM

- [ ] p0 idle room的内存没有被回收，内存只增不减，持续泄漏

  ![image-20260323015958736](/Users/n/Library/Application Support/typora-user-images/image-20260323015958736.png)

  ![image-20260323021051091](/Users/n/Library/Application Support/typora-user-images/image-20260323021051091.png)

  运行了一轮压测，内存无法GC（预期内，我还没做持久化，就是按照常驻写的...）

  ```
  - go_memstats_heap_alloc_bytes ≈ 487MB、process_resident_memory_bytes ≈ 530MB：当前驻留内存很高。
  - go_memstats_heap_objects ≈ 6.47M：活对象数量非常多，不像短时抖动。
  - prompt_endgame_tokens_total ≈ 2,105,880、prompt_endgame_turn_total(done+cancelled) ≈ 15,423：历史事件量极大。
  - prompt_endgame_active_turns = 0：当前没有活跃 turn，但内存仍高，说明是历史数据驻留，不是瞬时并发。
  - process_open_fds = 10：不是 FD 泄漏主导。
  - go_gc_duration_seconds_count=573 且仍高内存：GC在工作，但无法回收“仍被引用”的对象。
  ```

- [ ] p0 当前in-mem store已经不满足需求，没有任何持久化行为，重启后无法恢复会话状态

- [ ] p1 部署开启了swap，苟活但性能雪崩

- [ ] p1 prometheus采集的TPS P95/P99口径反了，我们关注最慢的1%而不是最快

- [ ] p2 benchmark无法回报到上述OOM的情况
