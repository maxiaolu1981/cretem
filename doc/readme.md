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
服务端这一层的限流基准都来自 ServerRunOptions 里的配置，核心几个参数是：

LoginRateLimit：登录接口每个时间窗口允许的最大请求数（和 /login 中间件直接挂钩）。
LoginWindow：登录限流的时间窗口长度，两个参数一起决定 /login 在 Redis/IP 上的限速。
WriteRateLimit：用户创建、更新、删除等写操作的限流阈值（/v1/users 这类接口用它）。
EnableRateLimiter：全局开关，决定是否启用生产端限流控制。
CLI 或配置文件里更新 server.loginlimit、server.loginwindow、server.writeRateLimit、server.enable-rate-limiter 就能改这些阈值，管理员接口 /admin/ratelimit/login 也会把值写入 loginLimit 调整生效。

curl "<http://localhost:8088/admin/ratelimit/login>"
{"code":100001,"message":"操作成功","data":{"effective":true,"value":500000,"window":"2m0s"}}login$
