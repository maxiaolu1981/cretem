## iam-apiserver

IAM API 服务器

### 概述

IAM API 服务器用于验证和配置 API 对象的数据，这些对象包括用户、策略、密钥等。API 服务器通过处理 REST 操作来管理这些 API 对象。
如需了解更多关于 iam-apiserver 的信息，请访问：
https://github.com/marmotedu/iam/blob/master/docs/guide/en-US/cmd/iam-apiserver.md

```
iam-apiserver [flags]
```

### 选项

```
                 --alsologtostderr                      同时将日志输出到标准错误和文件  
  -c, --config FILE                          从指定 FILE 读取配置，支持 JSON、TOML、YAML、HCL 或 Java 属性格式  
      --feature.enable-metrics               在 apiserver 的 /metrics 路径上启用指标（默认 true）  
      --feature.profiling                    通过 web 界面 host:port/debug/pprof/ 启用性能分析（默认 true）  
      --grpc.bind-address string             用于监听 --grpc.bind-port 的 IP 地址（设为 0.0.0.0 表示所有 IPv4 接口，:: 表示所有 IPv6 接口 ，默认 "0.0.0.0"）  
      --grpc.bind-port int                   用于提供未加密、未认证的 gRPC 访问端口。默认防火墙规则确保仅部署机器可访问，且 IAM 公网 443 端口会代理至此（默认由 nginx 实现），设 0 禁用（默认 8081）  
      --grpc.max-msg-size int                gRPC 最大消息大小（默认 4194304）  
  -h, --help                                 iam-apiserver 的帮助信息  
      --insecure.bind-address string         用于监听 --insecure.bind-port 的 IP 地址（设为 0.0.0.0 表示所有 IPv4 接口，:: 表示所有 IPv6 接口 ，默认 "127.0.0.1"）  
      --insecure.bind-port int               用于提供未加密、未认证访问的端口。默认防火墙规则确保仅部署机器可访问，且 IAM 公网 443 端口会代理至此（默认由 nginx 实现），设 0 禁用（默认 8080）  
      --jwt.key string                       用于签署 JWT 令牌的私钥  
      --jwt.max-refresh duration             客户端可刷新令牌的最长时间（默认 1h0m0s）  
      --jwt.realm string                     显示给用户的领域名称（默认 "iam jwt"）  
      --jwt.timeout duration                 JWT 令牌的有效期（默认 1h0m0s）  
      --log-backtrace-at traceLocation       当日志命中 line file:N 时，输出堆栈跟踪（默认 :0）  
      --log-dir string                       若不为空，日志文件将写入该目录  
      --log.development                      开发模式下，日志器会改变 DPanicLevel 行为，更频繁记录堆栈跟踪  
      --log.disable-caller                   禁用日志中输出调用者信息  
      --log.disable-stacktrace               禁用在 panic 级别及以上日志中记录堆栈跟踪  
      --log.enable-color                     在纯文本格式日志中启用 ANSI 颜色输出  
      --log.error-output-paths strings       日志的错误输出路径（默认 [stderr]）  
      --log.format FORMAT                    日志输出格式，支持 plain 或 json 格式（默认 "console"）  
      --log.level LEVEL                      最小日志输出级别（默认 "info"）  
      --log.name string                      日志器的名称  
      --log.output-paths strings             日志的输出路径（默认 [stdout]）  
      --logtostderr                          仅将日志输出到标准错误，而非文件  
      --mysql.database string                服务器使用的数据库名称  
      --mysql.host string                    MySQL 服务的主机地址。若留空，相关 MySQL 选项将被忽略（默认 "127.0.0.1:3306"）  
      --mysql.log-mode int                   指定 gorm 日志级别（默认 1）  
      --mysql.max-connection-life-time duration  允许连接到 MySQL 的最大连接生命周期（默认 10s）  
      --mysql.max-idle-connections int       允许连接到 MySQL 的最大空闲连接数（默认 100）  
      --mysql.max-open-connections int       允许连接到 MySQL 的最大打开连接数（默认 100）  
      --mysql.password string                访问 MySQL 的密码，需与用户名配合使用  
      --mysql.username string                访问 MySQL 服务的用户名  
      --redis.addrs strings                  Redis 地址集合（格式：127.0.0.1:6379）  
      --redis.database int                   默认数据库为 0。Redis 集群不支持设置数据库，若 --redis.enable-cluster=true，该值应省略或显式设为 0  
      --redis.enable-cluster                 若使用 Redis 集群，启用此选项以开启插槽模式  
      --redis.host string                    Redis 服务器的主机名（默认 "127.0.0.1"）  
      --redis.master-name string             Redis 主实例的名称  
      --redis.optimisation-max-active int    为避免过度提交 Redis 服务器连接，可限制 Redis 最大活动连接数。生产环境建议设为约 4000（默认 4000）  
      --redis.optimisation-max-idle int      配置空闲（无流量）时连接池保持的连接数。需将 --redis.optimisation-max-active 设为较大值，高可用部署通常设为 2000 左右（默认 2000）  
      --redis.password string                Redis 数据库的可选认证密码  
      --redis.port int                       Redis 服务器监听的端口（默认 6379）  
      --redis.ssl-insecure-skip-verify       连接加密 Redis 数据库时，允许使用自签名证书  
      --redis.timeout int                    连接 Redis 服务的超时时间（秒）  
      --redis.use-ssl                        若设置，IAM 将假设与 Redis 的连接已加密（用于支持传输中加密的 Redis 提供商）  
      --redis.username string                访问 Redis 服务的用户名  
      --secure.bind-address string           用于监听 --secure.bind-port 端口的 IP 地址。相关接口需可被引擎其他部分及 CLI/web 客户端访问，若为空则使用所有接口（0.0.0.0 表示所有 IPv4 接口，:: 表示所有 IPv6 接口 ，默认 "0.0.0.0"）  
      --secure.bind-port int                 用于提供带认证和授权的 HTTPS 服务的端口，不能通过设 0 禁用（默认 8443）  
      --secure.tls.cert-dir string           TLS 证书所在目录。若提供 --secure.tls.cert-key.cert-file 和 --secure.tls.cert-key.private-key-file，此标志将被忽略（默认 "/var/run/iam"）  
      --secure.tls.cert-key.cert-file string 用于 HTTPS 的默认 x509 证书文件（若有 CA 证书，需跟在服务器证书后）  
      --secure.tls.cert-key.private-key-file string 与 --secure.tls.cert-key.cert-file 匹配的默认 x509 私钥文件  
      --secure.tls.pair-name string          与 --secure.tls.cert-dir 配合使用的证书和密钥文件名前缀，最终文件为 <cert-dir>/<pair-name>.crt 和 <cert-dir>/<pair-name>.key（默认 "iam"）  
      --server.healthz                       添加自身就绪检查并安装 /healthz 路由（默认 true）  
      --server.middlewares strings           服务器允许的中间件列表，用逗号分隔。若为空，将使用默认中间件  
      --server.mode string                   以指定模式启动服务器，支持模式：debug、test、release（默认 "release"）  
      --stderrthreshold severity             此阈值及以上级别的日志将输出到 stderr（默认 2）  
  -v, --v Level                              V 日志的级别  
      --version version[=true]               打印版本信息并退出  
      --vmodule moduleSpec                   用于文件过滤日志的 pattern=N 设置的逗号分隔列表  
```
