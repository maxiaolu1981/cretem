# 日志规范（logging.md）

目的：统一仓库内日志级别使用准则，减少高吞吐场景下的日志噪声，便于运维与问题定位。

总体原则：
- Error：致命错误或需要立即人工干预的场景（进程异常、资源不可用、严重数据丢失等）。
- Warn：可恢复或需关注的问题（外部依赖暂时不可用、配置不当、重试次数用尽等）。
- Info：重要的生命周期事件与运维关注点（程序启动/停止、重要配置打印、升级/迁移事件、外部系统健康恢复）。
- Debug：高频、详细、或仅用于开发/排查的日志（每条消息的成功、循环性心跳、内部重试详细信息等）。

建议用法（针对 `internal/pkg/server`）：
- Kafka 消费/生产路径：
  - 每条消息成功处理（create/update/delete）应使用 Debug（避免高吞吐时日志膨胀）。
  - 消费者/生产者启动和停止保持 Info（运维需要知道实例数量/拓扑），但工作循环的准备/完成、单条消息成功应为 Debug。
  - 偏移提交失败、发送失败等保留 Warn/Error 并增加计数指标。
- Redis / DB：
  - 连接成功/失败的关键事件用 Info / Error。
  - 周期性健康探测的正常通过用 Debug，失败用 Warn 或 Error（取决于严重度）。
- HTTP / 路由：
  - 路由初始化和服务器可用性信息用 Info。
  - 请求级别的业务成功通常由访问日志或 APM 采集，不在代码中大量打印 Info；若项目没有访问日志，考虑把请求成功改为 Debug。

本次变更计划（scope: `internal/pkg/server`，低风险、分批执行）：
1. 已完成：
   - `consumer.go`：将每条消息成功的 Info 降为 Debug（已应用）。
   - `producer.go`：将死信队列发送成功的 Warn -> Info（已应用）。
   - `retry_consumer.go`：将启动状态打印降级为 Debug（部分已应用）。
2. 本批次（将立即应用，低风险）：
   - `retry_consumer.go`：把“等待所有worker完成...”“所有worker已完成...”由 Info -> Debug。

后续批次（待确认后执行）：
- Redis/health 心跳相关的 Info -> Debug（若不影响运维即时感知）。
- `genericapiserver.go` 中非关键的周期性 Info -> Debug（例如重复打印的健康检查成功，保留 Info 的关键里程碑）。

如何回滚/验证：
- 每次批量修改后运行 `go list ./...` 与 `go vet`。
- 在测试环境以 debug 模式运行服务，验证 Debug 日志显示，生产环境保持 Info/Warn/Error 输出。

如果你同意，我会继续按上述本批次变更执行并提交修改。
