# IAM API Server E2E 测试说明

> 所有测试用例都会自动将执行结果写入对应目录下的 `output/*_cases.json` 与 `output/*_perf.json`，可通过结果对比工具做回归检测。

## 环境准备

- 导出开关：`export IAM_APISERVER_E2E=1`
- 如需覆盖默认环境，设置以下变量：
  - `IAM_APISERVER_BASEURL`（默认 `http://192.168.10.8:8088`）
  - `IAM_APISERVER_ADMIN_USER`（默认 `admin`）
  - `IAM_APISERVER_ADMIN_PASS`（默认 `Admin@2021`）

## 执行方式与结果对比

1. 进入仓库根目录执行：

  ```bash
  go test ./test/iam-apiserver/...
  ```

  可通过 `-run Test<Name>` 单独触发某个功能或性能用例。
2. 每个套件会在自身目录下创建 `output/`，包括：

- `<prefix>_cases.json`：功能用例执行详情
- `<prefix>_perf.json`：性能用例统计指标（若套件定义性能测试）
- `rate_limiter_snapshot.json`：记录本次运行所使用的生产端动态限速配置（启用时生成，含起始/最小/最大速率与调整周期）

3. 使用结果对比工具检查回归：

  ```bash
  go run ./test/iam-apiserver/tools/resultcmp -baseline <旧结果目录> -current <新结果目录> -fail-on-regression
  ```

  工具基于 `tools/framework/comparison.go`，会标注新增、缺失或指标下降的用例。

### 动态限速验证

- 所有测试在执行写操作或生产端调用时，会自动复用与服务端一致的动态限速器参数，并将快照写入 `rate_limiter_snapshot.json`。
- 新增校验工具：

 ```bash
 go run ./test/iam-apiserver/tools/ratelimitcheck
 ```

 该命令会遍历所有套件的快照，验证启用状态与速率区间，并确保不同套件之间的配置保持一致。

- CI 工作流 `.github/workflows/user-e2e-tests.yml` 已在 `go test` 之后自动运行上述校验，若出现配置不一致或参数异常会直接失败，便于及时发现限速配置漂移。

## 套件概览

| 目录 | 目标 | 功能用例 | 性能场景 | 运行命令 |
| --- | --- | --- | --- | --- |
| `user/login` | 登录鉴权 | 正常登录、错误密码、缺失字段 | 并发登录成功率与 QPS | `go test ./test/iam-apiserver/user/login` |
| `user/logout` | 注销会话 | 成功注销、无效/缺失刷新令牌 | 并发登录后注销、验证二次注销被拒 | `go test ./test/iam-apiserver/user/logout` |
| `user/refresh` | 刷新访问令牌 | 成功刷新、无效/缺失刷新令牌、注销后刷新 | 并发刷新令牌返回率 | `go test ./test/iam-apiserver/user/refresh` |
| `user/jwt` | 登录 + 刷新一体流程 | 刷新成功、注销后刷新失败、缺失字段等 | 登录+刷新+注销完整链路 | `go test ./test/iam-apiserver/user/jwt` |
| `user/change_passwd` | 用户改密 | 正常改密、弱密码、重复密码、错误旧密 | 并发改密（锁重试、耗时） | `go test ./test/iam-apiserver/user/change_passwd` |
| `user/create` | 创建用户 | 新建成功、重复创建、无效载荷 | 并发创建吞吐 | `go test ./test/iam-apiserver/user/create` |
| `user/delete-force` | 强制删除 | 单用户强删、批量强删、并发冲突 | 高并发强删耗时与错误率 | `go test ./test/iam-apiserver/user/delete-force` |
| `user/get` | 查询单个用户 | 命中用户、404、权限控制 | 高频查询响应延迟 | `go test ./test/iam-apiserver/user/get` |
| `user/update` | 更新用户属性 | 正常更新、缺失用户、非法状态值 | 多线程更新一致性 | `go test ./test/iam-apiserver/user/update` |
| `user/list` | 用户分页/过滤 | 精确匹配、缺参校验、未找到 | 并发分页检索成功率 | `go test ./test/iam-apiserver/user/list` |

## 套件详情

### `user/login`

- **目的**：验证登录接口在不同输入下的行为，并确保成功登录可获取可用 AT/RT。
- **功能用例**：
  - `valid_credentials`：生成访问/刷新令牌并验证授权访问
  - `wrong_password`：错误密码返回未授权
  - `missing_password`：缺少字段触发参数错误
- **性能场景**：`login_parallel` 并发登录测算成功率与 QPS。
- **输出**：`output/login_cases.json`、`output/login_perf.json`

### `user/logout`

- **目的**：确保注销接口吊销刷新令牌并阻断后续刷新尝试。
- **功能用例**：
  - `logout_success`：注销后立即刷新应失败
  - `logout_with_invalid_refresh`：无效刷新令牌禁止注销
  - `logout_missing_refresh`：缺失字段返回参数错误
- **性能场景**：`logout_parallel` 并发登录-注销，验证二次注销被拒。
- **输出**：`output/logout_cases.json`、`output/logout_perf.json`

### `user/refresh`

- **目的**：验证刷新访问令牌的正确性及失效策略。
- **功能用例**：
  - `refresh_success`：刷新后返回新的 AT/RT
  - `refresh_with_invalid_token`：无效令牌被拒
  - `refresh_missing_token`：缺失字段报参数错误
  - `refresh_after_logout`：注销后旧令牌被拒
- **性能场景**：`refresh_parallel` 并发登录刷新测算成功率。
- **输出**：`output/refresh_cases.json`、`output/refresh_perf.json`

### `user/jwt`

- **目的**：覆盖登录、刷新、注销的组合链路，检查令牌状态迁移。
- **功能用例**：`refresh_success`、`refresh_after_logout`、`refresh_missing_token` 等。
- **性能场景**：
  - `jwt_login_refresh_parallel`：并发请求刷新流量
  - `jwt_logout_parallel`：注销成功率统计
  - `jwt_full_flow`：登录-刷新-注销全流程成功率
- **输出**：`output/jwt_cases.json`、`output/jwt_perf.json`

### `user/change_passwd`

- **目的**：校验改密流程及复杂度/旧密校验逻辑。
- **功能用例**：`happy_path`、`weak_password`、`same_password`、`wrong_old_password`、`missing_new_password`。
- **性能场景**：`change_password_parallel`（高并发改密，评估重试与锁表现）。
- **输出**：`output/change_password_cases.json`、`output/change_password_perf.json`

### `user/create`

- **目的**：验证管理员创建用户的逻辑及校验。
- **功能用例**：`create_success`、`duplicate_user`、`invalid_payload`。
- **性能场景**：`create_parallel` 并发创建吞吐统计。
- **输出**：`output/create_cases.json`、`output/create_perf.json`

### `user/delete-force`

- **目的**：保证强制删除接口可靠处理并发和资源清理。
- **功能用例**：
  - `delete_existing_user`：管理员删除现有用户并验证已清理
  - `delete_nonexistent_user`：不存在的用户返回 404
  - `invalid_username`：非法用户名触发参数错误
- **性能场景**：`delete_force_parallel` 统计高并发强删的耗时和失败率。
- **输出**：`output/delete_force_cases.json`、`output/delete_force_perf.json`

### `user/get`

- **目的**：验证查询单个用户的权限与存在性。
- **功能用例**：
  - `admin_get_existing`：管理员查询现有用户
  - `user_get_self`：普通用户查询自身信息
  - `user_not_found`：查询不存在用户
  - `missing_token`：缺少授权头
  - `invalid_token`：非法 Token
- **性能场景**：`get_user_parallel` 持续查询评估响应时间和错误率。
- **输出**：`output/get_cases.json`、`output/get_perf.json`

### `user/update`

- **目的**：验证用户资料更新、缺失用户及字段校验逻辑。
- **功能用例**：
  - `update_success`：更新昵称、邮箱成功
  - `user_not_found`：目标用户不存在
  - `invalid_status`：非法状态值返回校验错误
- **性能场景**：`update_parallel` 并发更新测试写入一致性。
- **输出**：`output/update_cases.json`、`output/update_perf.json`

### `user/list`

- **目的**：验证用户列表查询在 fieldSelector 下的正确性与鲁棒性。
- **功能用例**：
  - `list_single_user`：fieldSelector 精确匹配返回目标用户
  - `missing_field_selector`：缺参触发参数错误
  - `user_not_found`：不存在的用户返回未找到
- **性能场景**：`list_parallel` 并发分页检索，统计成功率与 QPS。
- **输出**：`output/list_cases.json`、`output/list_perf.json`

## 常见用法

- **只运行功能测试**：`go test ./test/iam-apiserver/user/login -run Functional`
- **只运行性能测试**：`go test ./test/iam-apiserver/user/login -run Performance`
- **比对基线结果**：生成两份 `output/` 后，使用 `resultcmp` 工具生成报告并根据 `--fail-on-regression` 控制回归阻断。

以上信息可帮助快速理解每套用例的目标、覆盖范围与运行方式，方便在 CI/CD 中自动化回归与性能监控。
先梳理 /v1/users/:name/force 的调用链：router.go → userController.ForceDelete → UserService.Delete → checkUserExist，明确哪些环节会命中负缓存并直接返回 ErrUserNotFound。
统计日志证据：在 iam-apiserver.log 中采样同一用户名的 create/get/delete 记录，核对 Redis 命中情况与 Kafka 生产、消费时间点，确认“创建成功但强制删时报 110007”的复现条件。
检查 Redis 空值缓存策略：复习 cacheNullValue、SetNX 过期时间、shouldRefreshNullCache 等逻辑，判断强制删除时是否需要更 aggressive 的回源或刷新锁策略。
验证消费者对删除消息的处理：确认 consumer.go 删除流程是否会在删除前再次写入负缓存，从而在后续删除请求里被误用。
针对 checkUserExist(..., true) 的流程设计实验：
在测试环境临时关闭/缩短空值缓存 TTL，观察是否还会触发 110007。
在强制删除入口增加临时日志，打印 forceRefresh 后的 DB 结果与缓存命中情况。
并行分析创建链路慢的问题：从 router → controller → create_service → ensureContactUnique → producer.SendUserCreateMessage 列出关键耗时节点，对照现有日志确认瓶颈（Kafka enqueue、唯一性校验、DB 访问），必要时增加 step 级别指标。
归纳可能的修复方向：
调整负缓存刷新策略（例如命中空值时强制刷新一次 DB 并更新缓存）。
改写强制删除的返回逻辑——区分“确实不存在”与“缓存暂未同步”的场景。
针对 Kafka 发送超时优化 producer 配置或引入补偿重试指标。
最后输出报告：给出问题根因假设、验证数据、建议修改点及潜在风险，为后续代码改动或配置调整打基础。
