/*
┌─────────────────┐       ┌─────────────────────┐
│   Context       │       │   Strategy          │
│   (上下文)       │       │   (策略接口)        │
├─────────────────┤       ├─────────────────────┤
│ + SetStrategy() │──────>│ + ExecuteAlgorithm()│
│ + Execute()     │       └─────────────────────┘
└─────────────────┘                   △

	            ┌──────┴──────┐
	            │             │
	┌─────────────────────┐ ┌─────────────────────┐
	│ ConcreteStrategyA   │ │ ConcreteStrategyB   │
	│ (具体策略A)          │ │ (具体策略B)          │
	├─────────────────────┤ ├─────────────────────┤
	│ + ExecuteAlgorithm()│ │ + ExecuteAlgorithm()│
	└─────────────────────┘ └─────────────────────┘

这个包是IAM项目的认证策略接口定义模块，提供了认证策略的统一接口和操作器模式。以下是包摘要：

包功能
接口定义: 定义统一的认证策略接口
策略模式: 提供策略切换的操作器实现
解耦设计: 将认证策略与使用代码解耦

核心接口
AuthStrategy
认证策略接口：

go

	type AuthStrategy interface {
	    AuthFunc() gin.HandlerFunc  // 返回Gin认证中间件函数
	}

核心结构体
AuthOperator
认证策略操作器：

go

	type AuthOperator struct {
	    strategy AuthStrategy  // 当前设置的认证策略
	}

主要方法
SetStrategy(strategy AuthStrategy)
设置当前使用的认证策略，支持运行时动态切换

AuthFunc() gin.HandlerFunc
执行资源认证，委托给当前设置的策略

设计特点
策略模式: 允许在运行时选择不同的认证算法

统一接口: 所有认证策略实现相同的AuthStrategy接口

依赖倒置: 高层模块不依赖具体认证实现，只依赖接口

开闭原则: 易于扩展新的认证策略，无需修改现有代码

使用场景
多认证支持: 如BasicAuth、JWT、OAuth等不同认证方式

动态切换: 根据请求特征选择不同的认证策略

测试模拟: 便于在测试时替换为模拟认证策略

实现示例
项目中已有实现：

# BasicStrategy - Basic认证

# JWTStrategy - JWT令牌认证

# AutoStrategy - 自动选择认证策略

这个包作为认证系统的抽象层，为IAM项目提供了灵活、可扩展的认证架构，是策略设计模式的典型应用。
*/
package middleware

import (
	"github.com/gin-gonic/gin"
)

type AuthStrategy interface {
	AuthFunc() gin.HandlerFunc
}

type AuthOperator struct {
	strategy AuthStrategy
}

func (operator *AuthOperator) SetAuthStrategy(authStrategy AuthStrategy) {
	operator.strategy = authStrategy
}

func (operator *AuthOperator) AuthFunc() gin.HandlerFunc {
	return operator.strategy.AuthFunc()
}
