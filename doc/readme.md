**登录流程关联的几个关键参数**

- **`LoginCredentialCacheTTL` / `LoginCredentialCacheSize`**  
  登录时 `authenticate` 会先尝试从本地凭证缓存里拿 `{username + hashedPassword + 明文密码}` 的比较结果。  
  - 命中且未过期 → 直接返回比较结果，省掉一次 bcrypt。  
  - 未命中 → 调用 `user.Compare`，把结果写回缓存并尊重 TTL/容量（超过上限会按最久未访问淘汰）。  
  典型用途：通过延长 TTL/增大容量降低高并发场景下的重复 bcrypt 成本。

- **`LoginFastFailThreshold` / `LoginFastFailMessage`**  
  每次进入 `authenticate` 时都会累加一个内存计数器；超过阈值则立刻返回 `ErrServerBusy`，响应里带上 message。  
  这是整个链路的快速“熔断”层，避免请求排队到后端资源（Redis/MySQL）而导致雪崩。  
  实际值可结合压测结果设定，例如硬件承受 300 并发就用 `300`，调高后配合缓存优化看曲线变化。

- **`LoginUpdateBuffer` / `LoginUpdateBatchSize` / `LoginUpdateFlushInterval` / `LoginUpdateTimeout`**  
  登录成功后 `LoginedAt` 不再同步写库，而是塞进 `loginUpdates` 队列：  
  1. 达到 `LoginUpdateBatchSize` 或 `FlushInterval` 到期会批量刷新；  
  2. 刷新时去重同一账号，只保留最新时间；  
  3. 超过队列容量（`LoginUpdateBuffer`）则直接 fallback 到单次 goroutine 写；  
  4. 每次批量写都有 `LoginUpdateTimeout` 的数据库超时保护。  
  这些参数决定了吞吐 vs. 实时性的权衡。增大队列/批量和延长 flush 间隔能减少 MySQL 压力，但登录时间落库会稍慢。

整体串起来：请求进来先过快速失败阈值 → 缓存命中减少 bcrypt → 认证成功后登录时间异步落库。  
根据监控调整上述参数，就能围绕高并发瓶颈（CPU、数据库、队列）做针对性优化。
