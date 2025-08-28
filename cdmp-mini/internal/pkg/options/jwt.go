/*
这是一个 Go 语言代码包，主要用于 JWT (JSON Web Token) 配置选项的定义、解析和验证。

包摘要
这个 options 包提供了：

1. 核心结构体 JwtOptions
包含了 JWT 相关的配置项：

Realm: 显示给用户的领域名称

Key: 用于签署 JWT 令牌的私钥

Timeout: JWT token 超时时间

MaxRefresh: 令牌可刷新的最长窗口期

2. 主要功能
构造函数

NewJwtOptions(): 创建带有默认值的 JwtOptions 实例

命令行集成

AddFlags(): 将 JWT 配置绑定到命令行标志，支持通过命令行参数配置

配置验证

Validate(): 验证配置的合法性，包括：

领域名称非空检查

超时时间必须大于 0

密钥长度限制（6-32 字符）

刷新窗口期必须大于 0 且不能小于超时时间

3. 特性
使用 pflag 包支持命令行参数解析

使用 govalidator 进行字符串长度验证

提供完整的配置验证和错误信息

与服务器配置默认值集成

4. 使用场景
用于微服务或应用程序中 JWT 认证模块的配置管理，支持通过配置文件和命令行参数两种方式灵活配置 JWT 参数。

*/

package options
