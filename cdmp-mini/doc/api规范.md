get 规范
一、RESTful GET 响应的核心规范（先明确规则）
在写代码前，先对齐 RESTful 对 GET 请求的核心要求，避免踩坑：
场景	HTTP 状态码	响应体结构要求
单资源查询成功	200 OK	包含 单个资源详情（如用户信息），避免冗余字段（如敏感密码）
多资源列表查询成功	200 OK	包含 资源列表 + 分页元数据（总条数、当前页、页大小），支持排序 / 过滤参数
单资源不存在	404 Not Found	携带错误码和提示（如 code:110001, message:"用户不存在"）
参数错误（如非法分页）	400 Bad Request	错误码 + 具体参数错误提示（如 code:100004, message:"page参数必须大于0"）
未授权（无令牌）	401 Unauthorized	错误码 + 授权提示（如 code:100205, message:"缺少Authorization头"）
权限不足	403 Forbidden	错误码 + 权限提示（如 code:100207, message:"无查询用户列表权限"）
服务端错误	500 Internal Server Error	错误码 + 通用提示（如 code:100002, message:"查询用户失败，请稍后重试"）