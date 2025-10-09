# Change Password 测试套件

> 目录位置：`test/iam-apiserver/user/change_passwd`

该目录复制了 `create`、`delete-force` 测试的自动化风格，用于覆盖密码修改接口的功能、异常、安全以及压力测试。核心目标：

- 通过 Go 测试脚本 `change_passwd_test.go` 统一准备测试账号、调用 API、输出结构化结果；
- 自动执行 Python 校验脚本 `tools/check_change_password.py`，对数据库、日志、性能指标进行二次比对；
- 为压力测试提供可复用的指标采集结构(`PerformanceMetrics`)和 `StressConfig` 参数，以便扩展为完整压测方案。

## 目录结构

```text
change_passwd/
├── change_passwd_test.go        # Go 端测试主入口（功能 + 压测）
├── output/                      # 测试结果及校验产物（JSON）
└── tools/
    └── check_change_password.py # Python 校验器
```

运行 `go test ./test/iam-apiserver/user/change_passwd -run TestChangePassword_Functional` 时会自动：

1. 登录管理员账号，批量创建/清理测试用户；
2. 执行功能用例矩阵（正向、边界、负向、安全等）；
3. 将结果写入 `output/change_password_results.json` 和 `output/change_password_perf.json`；
4. 调用 `tools/check_change_password.py` 补充检测数据库哈希、日志是否泄露新密码、性能 SLA
   (平均响应 < 500ms、P95 < 1s、错误率 < 1%)；
5. 总结产出 `output/change_password_summary.json`，用于 CI/报告。

## 快速开始

```bash
cd test/iam-apiserver/user/change_passwd
go test -run TestChangePassword_Functional
```

如需压力测试：

```bash
# 注意：默认运行约 2 分钟，需确保压测环境资源充足
cd test/iam-apiserver/user/change_passwd
go test -run TestChangePassword_Stress -timeout 30m
```

## Python 校验脚本单独运行

```bash
cd test/iam-apiserver/user/change_passwd
python3 tools/check_change_password.py \
  --results output/change_password_results.json \
  --perf output/change_password_perf.json \
  --summary output/change_password_summary.json
```

> 若数据库使用本机端口，可附加 `--db-host localhost --db-fallback-host 127.0.0.1`。

## 平台要求

- Go 1.24+
- Python 3.8+
- MySQL Root 账号（默认密码 `iam59!z$`，可通过参数覆盖）
- 脚本默认尝试 `pymysql` 或 `mysql-connector-python`，至少安装其一；
- IAM 服务日志路径 `/var/log/iam/iam-apiserver.log` 可在脚本参数中调整。

## 结果产物说明

| 文件 | 说明 |
| ---- | ---- |
| `output/change_password_results.json` | 每个用例执行结果、耗时、校验项标记 |
| `output/change_password_perf.json`   | 压测指标（吞吐、P95、错误率、资源等） |
| `output/change_password_summary.json`| Python 校验器总结（DB/日志/SLA 状态） |

## 扩展点

- 在 Go 测试的 `StressConfig` 中增补监控采集端点，输出 Prometheus 抓取结果；
- 在 Python 校验脚本里添加 Redis 缓存命中验证、旧 Token 失效二次检查；
- 与统一基线(`dump_db_usernames.py`)对接，实现压测前后自动 diff。
