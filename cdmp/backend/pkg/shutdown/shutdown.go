/*
Package shutdown Providing shutdown callbacks for graceful app shutdown

# Installation

To install run:

	go get github.com/marmotedu/iam/pkg/shutdown

# Example - posix signals

Graceful shutdown will listen for posix SIGINT and SIGTERM signals.
When they are received it will run all callbacks in separate go routines.
When callbacks return, the application will exit with os.Exit(0)

	package main

	import (
		"fmt"
		"time"

		"github.com/marmotedu/iam/pkg/shutdown"
		"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/posixsignal"
	)

	func main() {
		// initialize shutdown
		gs := shutdown.New()

		// add posix shutdown manager
		gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

		// add your tasks that implement ShutdownCallback
		gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
			fmt.Println("Shutdown callback start")
			time.Sleep(time.Second)
			fmt.Println("Shutdown callback finished")
			return nil
		}))

		// start shutdown managers
		if err := gs.Start(); err != nil {
			fmt.Println("Start:", err)
			return
		}

		// do other stuff
		time.Sleep(time.Hour)
	}

# Example - posix signals with error handler

The same as above, except now we set an ErrorHandler that prints the
error returned from ShutdownCallback.

	package main

	import (
		"fmt"
		"time"
		"errors"

		"github.com/marmotedu/iam/pkg/shutdown"
		"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/posixsignal"
	)

	func main() {
		// initialize shutdown
		gs := shutdown.New()

		// add posix shutdown manager
		gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

		// set error handler
		gs.SetErrorHandler(shutdown.ErrorFunc(func(err error) {
			fmt.Println("Error:", err)
		}))

		// add your tasks that implement ShutdownCallback
		gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
			fmt.Println("Shutdown callback start")
			time.Sleep(time.Second)
			fmt.Println("Shutdown callback finished")
			return errors.New("my-error")
		}))

		// start shutdown managers
		if err := gs.Start(); err != nil {
			fmt.Println("Start:", err)
			return
		}

		// do other stuff
		time.Sleep(time.Hour)
	}

# Example - aws

Graceful shutdown will listen for SQS messages on "example-sqs-queue".
If a termination message has current EC2 instance id,
it will run all callbacks in separate go routines.
While callbacks are running it will call aws api
RecordLifecycleActionHeartbeatInput autoscaler every 15 minutes.
When callbacks return, the application will call aws api CompleteLifecycleAction.
The callback will delay only if shutdown was initiated by awsmanager.
If the message does not have current instance id, it will forward the
message to correct instance via http on port 7999.

	package main

	import (
		"fmt"
		"time"

		"github.com/marmotedu/iam/pkg/shutdown"
		"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/awsmanager"
		"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/posixsignal"
	)

	func main() {
		// initialize shutdown with ping time
		gs := shutdown.New()

		// add posix shutdown manager
		gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

		// set error handler
		gs.SetErrorHandler(shutdown.ErrorFunc(func(err error) {
			fmt.Println("Error:", err)
		}))

		// add aws shutdown manager
		gs.AddShutdownManager(awsmanager.NewAwsManager(&awsmanager.AwsManagerConfig{
			SqsQueueName:      "example-sqs-queue",
			LifecycleHookName: "example-lifecycle-hook",
			Port:              7999,
		}))

		// add your tasks that implement ShutdownCallback
		gs.AddShutdownCallback(shutdown.ShutdownFunc(func(shutdownManager string) error {
			fmt.Println("Shutdown callback start")
			if shutdownManager == awsmanager.Name {
				time.Sleep(time.Hour)
			}
			fmt.Println("Shutdown callback finished")
			return nil
		}))

		// start shutdown managers
		if err := gs.Start(); err != nil {
			fmt.Println("Start:", err)
			return
		}

		// do other stuff
		time.Sleep(time.Hour * 2)
	}
	shutdown 包提供了应用程序优雅关闭的机制，通过管理关闭管理器（ShutdownManager） 和关闭回调（ShutdownCallback），实现对关闭信号（如 POSIX 信号、AWS 通知等）的监听与处理。核心功能包括：
支持多种关闭触发源（如系统信号、云服务通知）；
在关闭时按序执行自定义回调函数（如资源释放、连接关闭）；
提供错误处理机制，捕获关闭过程中的异常。
核心流程
初始化：通过 shutdown.New() 创建 GracefulShutdown 实例，作为优雅关闭的核心控制器。
配置组件：
添加关闭管理器（AddShutdownManager）：如监听 POSIX 信号的 posixsignal 管理器，负责检测关闭触发事件。
添加关闭回调（AddShutdownCallback）：注册需要在关闭时执行的逻辑（如释放数据库连接、停止服务）。
设置错误处理器（SetErrorHandler）：定义关闭过程中异常的处理方式。
启动监听：调用 GracefulShutdown.Start() 启动所有管理器，开始监听关闭信号。
触发关闭：当管理器检测到关闭信号（如 SIGINT），执行以下步骤：
调用管理器的 ShutdownStart()：预处理（如日志记录）。
并发执行所有注册的关闭回调（OnShutdown）：释放资源。
等待所有回调完成后，调用管理器的 ShutdownFinish()：收尾工作（如退出程序）。
*/
/*
Package shutdown 提供应用程序优雅关闭的回调机制

# 安装

执行以下命令安装：

	go get github.com/marmotedu/iam/pkg/shutdown

# 示例 - POSIX信号

优雅关闭会监听POSIX的SIGINT和SIGTERM信号。
当收到信号时，所有回调会在独立的goroutine中执行。
回调完成后，应用会以os.Exit(0)退出。

	package main

	import (
		"fmt"
		"time"

		"github.com/marmotedu/iam/pkg/shutdown"
		"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/posixsignal"
	)

	func main() {
		// 初始化关闭管理器
		gs := shutdown.New()

		// 添加POSIX信号管理器
		gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

		// 添加自定义关闭回调
		gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
			fmt.Println("关闭回调开始")
			time.Sleep(time.Second)
			fmt.Println("关闭回调完成")
			return nil
		}))

		// 启动关闭管理器
		if err := gs.Start(); err != nil {
			fmt.Println("启动失败:", err)
			return
		}

		// 执行其他业务逻辑
		time.Sleep(time.Hour)
	}

# 示例 - 带错误处理的POSIX信号

与上述示例类似，但增加了错误处理器，用于打印回调返回的错误。

	package main

	import (
		"fmt"
		"time"
		"errors"

		"github.com/marmotedu/iam/pkg/shutdown"
		"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/posixsignal"
	)

	func main() {
		// 初始化关闭管理器
		gs := shutdown.New()

		// 添加POSIX信号管理器
		gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

		// 设置错误处理器
		gs.SetErrorHandler(shutdown.ErrorFunc(func(err error) {
			fmt.Println("错误:", err)
		}))

		// 添加自定义关闭回调
		gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
			fmt.Println("关闭回调开始")
			time.Sleep(time.Second)
			fmt.Println("关闭回调完成")
			return errors.New("自定义错误")
		}))

		// 启动关闭管理器
		if err := gs.Start(); err != nil {
			fmt.Println("启动失败:", err)
			return
		}

		// 执行其他业务逻辑
		time.Sleep(time.Hour)
	}

# 示例 - AWS集成

优雅关闭会监听"example-sqs-queue"的SQS消息。
如果终止消息包含当前EC2实例ID，会在独立goroutine中执行所有回调。
回调执行期间，每15分钟会调用AWS API发送生命周期心跳。
回调完成后，会调用AWS API完成生命周期动作。
仅当关闭由awsmanager触发时，回调才会延迟。
如果消息不匹配当前实例ID，会通过7999端口的HTTP转发给目标实例。

	package main

	import (
		"fmt"
		"time"

		"github.com/marmotedu/iam/pkg/shutdown"
		"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/awsmanager"
		"github.com/marmotedu/iam/pkg/shutdown/shutdownmanagers/posixsignal"
	)

	func main() {
		// 初始化关闭管理器
		gs := shutdown.New()

		// 添加POSIX信号管理器
		gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

		// 设置错误处理器
		gs.SetErrorHandler(shutdown.ErrorFunc(func(err error) {
			fmt.Println("错误:", err)
		}))

		// 添加AWS关闭管理器
		gs.AddShutdownManager(awsmanager.NewAwsManager(&awsmanager.AwsManagerConfig{
			SqsQueueName:      "example-sqs-queue",
			LifecycleHookName: "example-lifecycle-hook",
			Port:              7999,
		}))

		// 添加自定义关闭回调
		gs.AddShutdownCallback(shutdown.ShutdownFunc(func(shutdownManager string) error {
			fmt.Println("关闭回调开始")
			if shutdownManager == awsmanager.Name {
				time.Sleep(time.Hour)
			}
			fmt.Println("关闭回调完成")
			return nil
		}))

		// 启动关闭管理器
		if err := gs.Start(); err != nil {
			fmt.Println("启动失败:", err)
			return
		}

		// 执行其他业务逻辑
		time.Sleep(time.Hour * 2)
	}
*/
package shutdown

import (
	"sync" // 标准库：提供同步原语（如WaitGroup）
)

// ShutdownCallback 是需要实现的回调接口。
// 当关闭请求触发时，OnShutdown会被调用。参数为触发关闭的ShutdownManager名称。
type ShutdownCallback interface {
	OnShutdown(string) error
}

// ShutdownFunc 是辅助类型，用于将匿名函数作为ShutdownCallback。
type ShutdownFunc func(string) error

// OnShutdown 定义关闭触发时需要执行的动作。
func (f ShutdownFunc) OnShutdown(shutdownManager string) error {
	return f(shutdownManager)
}

// ShutdownManager 是关闭管理器实现的接口。
// GetName返回管理器名称；Start用于启动监听关闭请求；
// 当调用GSInterface的StartShutdown时，先执行ShutdownStart()，
// 再执行所有ShutdownCallback，最后执行ShutdownFinish()。
type ShutdownManager interface {
	GetName() string
	Start(gs GSInterface) error
	ShutdownStart() error
	ShutdownFinish() error
}

// ErrorHandler 是可通过SetErrorHandler设置的接口，用于处理异步错误。
type ErrorHandler interface {
	OnError(err error)
}

// ErrorFunc 是辅助类型，用于将匿名函数作为ErrorHandler。
type ErrorFunc func(err error)

// OnError 定义错误发生时需要执行的动作。
func (f ErrorFunc) OnError(err error) {
	f(err)
}

// GSInterface 是GracefulShutdown实现的接口，
// 供ShutdownManager调用，以在关闭请求触发时启动关闭流程。
type GSInterface interface {
	StartShutdown(sm ShutdownManager)
	ReportError(err error)
	AddShutdownCallback(shutdownCallback ShutdownCallback)
}

// GracefulShutdown 是处理ShutdownCallbacks和ShutdownManagers的主结构体。
// 通过New函数初始化。
type GracefulShutdown struct {
	callbacks    []ShutdownCallback // 关闭回调列表
	managers     []ShutdownManager  // 关闭管理器列表
	errorHandler ErrorHandler       // 错误处理器
}

// New 初始化GracefulShutdown实例。
func New() *GracefulShutdown {
	return &GracefulShutdown{
		callbacks: make([]ShutdownCallback, 0, 10), // 预分配10个回调容量
		managers:  make([]ShutdownManager, 0, 3),   // 预分配3个管理器容量
	}
}

// Start 调用所有已添加的ShutdownManager的Start方法，启动关闭请求监听。
// 若任何管理器启动失败，返回错误。
func (gs *GracefulShutdown) Start() error {
	for _, manager := range gs.managers {
		if err := manager.Start(gs); err != nil {
			return err
		}
	}

	return nil
}

// AddShutdownManager 添加一个ShutdownManager，用于监听关闭请求。
func (gs *GracefulShutdown) AddShutdownManager(manager ShutdownManager) {
	gs.managers = append(gs.managers, manager)
}

// AddShutdownCallback 添加一个ShutdownCallback，在关闭请求触发时执行。
// 可传入任何实现ShutdownCallback接口的对象，或通过匿名函数：
//
//	AddShutdownCallback(shutdown.ShutdownFunc(func() error {
//	    // 回调逻辑
//	    return nil
//	}))
func (gs *GracefulShutdown) AddShutdownCallback(shutdownCallback ShutdownCallback) {
	gs.callbacks = append(gs.callbacks, shutdownCallback)
}

// SetErrorHandler 设置错误处理器，用于处理ShutdownCallback或ShutdownManager中出现的异常。
// 可传入任何实现ErrorHandler接口的对象，或通过匿名函数：
//
//	SetErrorHandler(shutdown.ErrorFunc(func (err error) {
//	    // 错误处理逻辑
//	}))
func (gs *GracefulShutdown) SetErrorHandler(errorHandler ErrorHandler) {
	gs.errorHandler = errorHandler
}

// StartShutdown 由ShutdownManager调用，用于启动关闭流程：
// 1. 调用管理器的ShutdownStart()
// 2. 并发执行所有ShutdownCallback
// 3. 等待所有回调完成后，调用管理器的ShutdownFinish()
func (gs *GracefulShutdown) StartShutdown(sm ShutdownManager) {
	// 执行关闭前的预处理，捕获错误
	gs.ReportError(sm.ShutdownStart())

	var wg sync.WaitGroup
	// 并发执行所有关闭回调
	for _, shutdownCallback := range gs.callbacks {
		wg.Add(1)
		go func(shutdownCallback ShutdownCallback) {
			defer wg.Done()
			// 执行回调并报告错误
			gs.ReportError(shutdownCallback.OnShutdown(sm.GetName()))
		}(shutdownCallback)
	}

	// 等待所有回调完成
	wg.Wait()

	// 执行关闭后的收尾工作，捕获错误
	gs.ReportError(sm.ShutdownFinish())
}

// ReportError 用于向错误处理器报告错误（若已设置）。
// 供ShutdownManagers调用以处理异常。
func (gs *GracefulShutdown) ReportError(err error) {
	if err != nil && gs.errorHandler != nil {
		gs.errorHandler.OnError(err)
	}
}
