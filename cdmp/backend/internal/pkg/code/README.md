## Code 设计规范

Code 代码从 100101 开始，1000 以下为平台保留 code.

错误代码说明：100101
+ 10: 服务
+ 01: 模块
+ 01: 模块下的错误码序号，每个模块可以注册 100 个错误

### 服务和模块说明

|服务|模块|说明|
|----|----|----|
|10|00|通用 - 基本错误|
|10|01|通用 - 数据库类错误|
|10|02|通用 - 认证授权类错误|
|10|03|通用 - 加解码类错误|
|11|00|iam-apiserver服务 - 用户相关(模块)错误|
|11|01|iam-apiserver服务 - 密钥相关(模块)错误|

> **通用** - 所有服务都适用的错误，提高复用性，避免重复造轮子

## 错误描述规范

错误描述包括：对外的错误描述和对内的错误描述两部分。

### 对外的错误描述

- 对外暴露的错误，统一大写开头，结尾不要加`.`
- 对外暴露的错误，要简洁，并能准确说明问题
- 对外暴露的错误说明，应该是 `该怎么做` 而不是 `哪里错了`

### 对内的错误描述

- 告诉用户他们可以做什么，而不是告诉他们不能做什么。
- 当声明一个需求时，用 must 而不是 should。例如，must be greater than 0、must match regex '[a-z]+'。
- 当声明一个格式不对时，用 must not。例如，must not contain。
- 当声明一个动作时用 may not。例如，may not be specified when otherField is empty、only name may be specified。
- 引用文字字符串值时，请在单引号中指示文字。例如，ust not contain '..'。
- 当引用另一个字段名称时，请在反引号中指定该名称。例如，must be greater than request。
- 指定不等时，请使用单词而不是符号。例如，must be less than 256、must be greater than or equal to 0 (不要用 larger than、bigger than、more than、higher than)。
- 指定数字范围时，请尽可能使用包含范围。
- 建议 Go 1.13 以上，error 生成方式为 fmt.Errorf("module xxx: %w", err)。
- 错误描述用小写字母开头，结尾不要加标点符号。

> 错误信息是直接暴露给用户的，不能包含敏感信息

## 错误记录规范

在错误产生的最原始位置调用日志，打印错误信息，其它位置直接返回。

当错误发生时，调用log包打印错误，通过log包的caller功能，可以定位到log语句的位置，也即能够定位到错误发生的位置。当使用这种方式来打印日志时，需要中遵循以下规范：

- 只在错误产生的最初位置打印日志，其它地方直接返回错误，不需要再对错误进行封装。
- 当代码调用第三方包的函数时，第三方包函数出错时，打印错误信息。比如：

```go
if err := os.Chdir("/root"); err != nil {
    log.Errorf("change dir failed: %v", err)
}
```

## 错误码
！！IAM 系统错误码列表，由 `codegen -type=int -doc` 命令生成，不要对此文件做任何更改。

## 功能说明

如果返回结果中存在 `code` 字段，则表示调用 API 接口失败。例如：

```json
{
  "code": 100101,
  "message": "Database error"
}
```

上述返回中 `code` 表示错误码，`message` 表示该错误的具体信息。每个错误同时也对应一个 HTTP 状态码，比如上述错误码对应了 HTTP 状态码 500(Internal Server Error)。

## 错误码列表

平台支持的错误码列表如下：

| Identifier | Code | HTTP Code | Description |
| ---------- | ---- | --------- | ----------- |
| ErrUserNotFound | 110001 | 404 | 用户不存在 |
| ErrUserAlreadyExist | 110002 | 400 | 用户已经存在 |
| ErrReachMaxCount | 110101 | 400 | 密钥数量已达上限 |
| ErrSecretNotFound | 110102 | 404 | 密钥不存在 |
| ErrSuccess | 100001 | 200 | 成功 |
| ErrUnknown | 100002 | 500 | 服务器内部错误 |
| ErrBind | 100003 | 400 |后端接收 HTTP 请求并解析请求体(如 JSON、表单数据等) 发生错误|
| ErrValidation | 100004 | 400 | 验证失败 |
| ErrTokenInvalid | 100005 | 401 | 令牌无效 |
|ErrPageNotFound|100006|404|页面不存在
| ErrDatabase | 100101 | 500 | 数据库错误|
| ErrEncrypt | 100201 | 401 | 加密用户密码时发生错误 |
| ErrSignatureInvalid | 100202 | 401 | 签名无效 |
| ErrExpired | 100203 | 401 | 令牌过期 |
| ErrInvalidAuthHeader | 100204 | 401 | 无效的授权头 |
| ErrMissingHeader | 100205 | 401 |  Authorization头为空|
| ErrorExpired | 100206 | 401 | 令牌过期 |
| ErrPasswordIncorrect | 100207 | 401 | 密码无效 |
| ErrPermissionDenied | 100208 | 403 | 权限拒绝 |
| ErrEncodingFailed | 100301 | 500 | 由于数据问题导致编码失败|
| ErrDecodingFailed | 100302 | 500 | 由于数据问题导致解码失败|
| ErrInvalidJSON | 100303 | 500 | 数据不是有效的 JSON 格式 |
| ErrEncodingJSON | 100304 | 500 | JSON数据不能被编码 |
| ErrDecodingJSON | 100305 | 500 | JSON 数据不能被解码 |
| ErrInvalidYaml | 100306 | 500 | 不是有效的Yaml格式 |
| ErrEncodingYaml | 100307 | 500 | Yaml 编码失败 |
| ErrDecodingYaml | 100308 | 500 | Yaml 数据解码失败 |


