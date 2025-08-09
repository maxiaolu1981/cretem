package db

import (
	"time"

	"gorm.io/gorm"

	"github.com/maxiaolu1981/cretem/nexuscore/log"
)

// 常量定义：回调函数名称和存储开始时间的键名
const (
	callBackBeforeName = "core:before" // 前置回调的注册名称
	callBackAfterName  = "core:after"  // 后置回调的注册名称
	startTime          = "_start_time" // 用于在GORM上下文中存储SQL执行开始时间的键
)

// TracePlugin 定义了一个GORM插件，用于追踪SQL语句的执行时间
type TracePlugin struct{}

// Name 返回当前插件的名称，实现gorm.Plugin接口的必要方法
func (op *TracePlugin) Name() string {
	return "tracePlugin"
}

// Initialize 初始化插件，注册SQL执行前后的回调函数，实现gorm.Plugin接口的必要方法
func (op *TracePlugin) Initialize(db *gorm.DB) (err error) {
	// 注册前置回调：在各种SQL操作（创建、查询、删除、更新、行查询、原生SQL）执行前触发
	// 绑定到GORM内置回调的前面，确保在SQL执行前记录开始时间
	_ = db.Callback().Create().Before("gorm:before_create").Register(callBackBeforeName, before)
	_ = db.Callback().Query().Before("gorm:query").Register(callBackBeforeName, before)
	_ = db.Callback().Delete().Before("gorm:before_delete").Register(callBackBeforeName, before)
	_ = db.Callback().Update().Before("gorm:setup_reflect_value").Register(callBackBeforeName, before)
	_ = db.Callback().Row().Before("gorm:row").Register(callBackBeforeName, before)
	_ = db.Callback().Raw().Before("gorm:raw").Register(callBackBeforeName, before)

	// 注册后置回调：在各种SQL操作执行后触发
	// 绑定到GORM内置回调的后面，确保在SQL执行完成后计算耗时
	_ = db.Callback().Create().After("gorm:after_create").Register(callBackAfterName, after)
	_ = db.Callback().Query().After("gorm:after_query").Register(callBackAfterName, after)
	_ = db.Callback().Delete().After("gorm:after_delete").Register(callBackAfterName, after)
	_ = db.Callback().Update().After("gorm:after_update").Register(callBackAfterName, after)
	_ = db.Callback().Row().After("gorm:row").Register(callBackAfterName, after)
	_ = db.Callback().Raw().After("gorm:raw").Register(callBackAfterName, after)

	return nil // 初始化成功，无错误
}

// 确保TracePlugin实现了gorm.Plugin接口（编译期检查）
var _ gorm.Plugin = &TracePlugin{}

// before 是前置回调函数，在SQL执行前记录开始时间
// 将时间存储到GORM的上下文中（db.InstanceSet），供后置回调使用
func before(db *gorm.DB) {
	db.InstanceSet(startTime, time.Now()) // 存储当前时间作为SQL执行的开始时间
}

// after 是后置回调函数，在SQL执行后计算并打印执行耗时
func after(db *gorm.DB) {
	// 从GORM上下文中获取前置回调存储的开始时间
	_ts, isExist := db.InstanceGet(startTime)
	if !isExist {
		return // 如果没有开始时间，直接返回（避免异常）
	}

	// 将获取到的时间转换为time.Time类型
	ts, ok := _ts.(time.Time)
	if !ok {
		return // 如果类型转换失败，直接返回
	}

	// 计算SQL执行耗时（当前时间 - 开始时间），并通过日志输出
	// 注释掉的代码用于获取执行的SQL语句和参数，需要时可启用
	// sql := db.Dialector.Explain(db.Statement.SQL.String(), db.Statement.Vars...)
	log.Infof("sql cost time: %fs", time.Since(ts).Seconds()) // 输出耗时（单位：秒）
}
