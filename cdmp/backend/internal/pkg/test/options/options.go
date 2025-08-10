// Copyright (c) 2025 马晓璐
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package main

import (
	"time"

	"gorm.io/gorm"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/options" // 替换为实际 options 包路径
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/db"               // 替换为实际 db 包路径

	"github.com/maxiaolu1981/cretem/nexuscore/log" // 导入自定义日志系统
)

// User 测试用数据模型
type User struct {
	gorm.Model
	Name string
	Age  int
}

func main() {
	// 1. 初始化日志系统（关键步骤：配置并启动自定义日志）
	initLogger()

	// 2. 初始化数据库配置
	mysqlOpts := options.NewMySQLOptions()
	// 根据实际环境修改数据库配置
	mysqlOpts.Host = "127.0.0.1:3306"
	mysqlOpts.Username = "root"
	mysqlOpts.Password = "iam59!z$"
	mysqlOpts.Database = "iam"

	// 3. 校验配置合法性
	if errs := mysqlOpts.Validate(); len(errs) > 0 {
		log.Errorf("数据库配置错误: %v", errs) // 使用自定义日志输出错误
		return
	}

	// 4. 创建数据库连接
	gormDB, err := mysqlOpts.NewClient()
	if err != nil {
		log.Errorf("创建数据库连接失败: %v", err) // 使用自定义日志输出错误
		return
	}

	// 5. 应用 TracePlugin 插件（核心：将 SQL 耗时日志接入自定义日志系统）
	if err := gormDB.Use(&db.TracePlugin{}); err != nil {
		log.Errorf("注册 SQL 追踪插件失败: %v", err)
		return
	}

	// 6. 执行数据库操作，验证日志输出
	log.Infof("开始执行数据库操作测试") // 记录测试开始日志

	// 6.1 创建测试表
	if err := gormDB.AutoMigrate(&User{}); err != nil {
		log.Errorf("创建用户表失败: %v", err)
		return
	}
	log.Info("用户表创建成功")

	// 6.2 插入数据
	user := User{Name: "测试用户", Age: 25}
	if result := gormDB.Create(&user); result.Error != nil {
		log.Errorf("插入用户数据失败: %v", result.Error)
	} else {
		log.Infow("用户数据插入成功", "user_id", user.ID, "name", user.Name) // 带键值对的日志
	}

	// 6.3 查询数据
	var queryUser User
	if result := gormDB.First(&queryUser, user.ID); result.Error != nil {
		log.Errorf("查询用户失败: %v", result.Error)
	} else {
		log.Infof("查询到用户: ID=%d, Name=%s, Age=%d", queryUser.ID, queryUser.Name, queryUser.Age)
	}

	// 6.4 更新数据
	if result := gormDB.Model(&User{}).Where("id = ?", user.ID).Update("Age", 26); result.Error != nil {
		log.Errorf("更新用户年龄失败: %v", result.Error)
	} else {
		log.Infow("用户年龄更新成功", "user_id", user.ID, "new_age", 26)
	}

	// 6.5 删除数据
	if result := gormDB.Delete(&User{}, user.ID); result.Error != nil {
		log.Errorf("删除用户失败: %v", result.Error)
	} else {
		log.Infow("用户删除成功", "user_id", user.ID)
	}

	// 等待日志缓冲刷新
	time.Sleep(100 * time.Millisecond)
	log.Info("所有数据库操作测试完成")

	// 程序退出前刷新日志，确保所有日志写入完成
	log.Flush()
}

// initLogger 初始化自定义日志系统（基于 Zap 封装）
func initLogger() {
	// 创建日志配置选项（可根据需求调整）
	opts := log.NewOptions()
	opts.Level = "info"                                                    // 日志级别：debug/info/warn/error
	opts.Format = "console"                                                // 输出格式：console（控制台）或 json（结构化）
	opts.OutputPaths = []string{"stdout", "/tmp/cdmp-logs/app.log"}        // 普通日志输出到控制台
	opts.ErrorOutputPaths = []string{"stderr", "/tmp/cdmp-logs/error.log"} // 错误日志输出到 stderr

	// 初始化日志系统
	log.Init(opts)
	log.Info("日志系统初始化完成")
}
