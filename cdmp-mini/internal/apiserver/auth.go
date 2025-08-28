/*
实现了基于 JWT 和 Basic 认证的双重认证策略，提供完整的身份验证和授权功能。
1. 认证策略工厂
//创建JWT认证策略
// 创建Basic认证策略
// 创建自动选择策略（JWT优先，Basic备用）
2.JWT 认证核心组件
1.) 基础配置：初始化核心参数
realm signingAlgorithm  key timeout maxRefresh identityKey
2.) 令牌解析配置：定义令牌的获取位置和格式
决定框架从哪里读取令牌（请求头/查询参数/ Cookie）
tokenLoopup tokenHeadName sendCookie
3.) 时间函数配置：定义时间获取方式（影响令牌有效期计算） timeFunc
4.) 认证核心函数：用户登录验证逻辑authenticatorFunc
这是认证流程的入口，验证用户凭据（用户名/密码）
5.)载荷生成函数：定义JWT中存储的用户信息 payloadFunc
认证成功后，生成令牌时需要的用户身份信息
6.) 身份提取函数：从JWT中解析用户身份identityHandler
用于后续请求中识别用户（如权限验证）
7.) 权限验证函数：验证用户是否有权限访问资源authorizatorFunc
在身份识别后执行，判断用户是否能访问当前接口
8.)响应处理函数：定义各种场景的响应格式
包括登录成功、刷新令牌、注销、认证失败等
loginResponse()      // 登录成功响应
refreshResponse()    // 令牌刷新响应
logoutResponse()     // 注销响应
unauthorizedFunc()   // 认证失败响应

3. 凭据解析器
// 支持多种认证方式
parseWithHeader()    // 从HTTP Header解析Basic认证
parseWithBody()      // 从请求体解析JSON凭据
*/
package apiserver

import (
	_ "github.com/maxiaolu1981/cretem/cdmp-mini/pkg/validator"
)

const (
	// APIServerAudience defines the value of jwt audience field.
	APIServerAudience = "iam.api.marmotedu.com"

	// APIServerIssuer defines the value of jwt issuer field.
	APIServerIssuer = "iam-apiserver"
)
