/*
Package posixsignal provides a listener for a posix signal. By default
it listens for SIGINT and SIGTERM, but others can be chosen in NewPosixSignalManager.
When ShutdownFinish is called it exits with os.Exit(0)
posixsignal 包实现了一个基于 POSIX 信号的关闭管理器（PosixSignalManager），用于监听系统信号（默认监听 SIGINT 和 SIGTERM），并在收到信号时触发应用程序的优雅关闭流程。其核心作用是作为 shutdown 包的插件，将系统信号与应用的关闭逻辑关联，确保程序在退出前能执行资源释放等收尾工作。
核心流程
初始化：通过 NewPosixSignalManager 创建管理器实例，可指定要监听的信号（默认监听 SIGINT（Ctrl+C）和 SIGTERM（终止信号））。
启动监听：调用 Start 方法后，管理器会启动一个 goroutine 阻塞等待指定的系统信号。
触发关闭：当接收到目标信号时，通过 shutdown.GSInterface 接口调用 StartShutdown，触发全局优雅关闭流程：
执行所有注册的关闭回调（如释放连接、保存状态）。
回调完成后，调用 ShutdownFinish 以 os.Exit(0) 退出程序。
*/
/*
Package posixsignal 提供一个POSIX信号监听器。默认监听SIGINT和SIGTERM信号，
也可通过NewPosixSignalManager自定义监听的信号。当ShutdownFinish被调用时，会以os.Exit(0)退出程序。
*/
package posixsignal

import (
	"os"        // 标准库：提供操作系统相关功能（如信号、退出）
	"os/signal" // 标准库：用于接收和处理系统信号
	"syscall"   // 标准库：提供系统调用相关常量（如信号定义）

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/shutdown" // 自定义库：优雅关闭框架
)

// Name 定义当前关闭管理器的名称。
const Name = "PosixSignalManager"

// PosixSignalManager 实现了ShutdownManager接口，用于添加到GracefulShutdown中。
// 通过NewPosixSignalManager初始化。
type PosixSignalManager struct {
	signals []os.Signal // 需要监听的POSIX信号列表
}

// NewPosixSignalManager 初始化PosixSignalManager实例。
// 可传入要监听的os.Signal参数；若未传入，则默认监听SIGINT（Ctrl+C）和SIGTERM。
func NewPosixSignalManager(sig ...os.Signal) *PosixSignalManager {
	if len(sig) == 0 {
		// 默认信号：os.Interrupt（对应SIGINT）和syscall.SIGTERM
		sig = []os.Signal{os.Interrupt, syscall.SIGTERM}
	}

	return &PosixSignalManager{
		signals: sig,
	}
}

// GetName 返回当前ShutdownManager的名称（固定为Name常量）。
func (posixSignalManager *PosixSignalManager) GetName() string {
	return Name
}

// Start 启动信号监听：创建goroutine阻塞等待指定信号，收到信号后触发优雅关闭。
func (posixSignalManager *PosixSignalManager) Start(gs shutdown.GSInterface) error {
	go func() {
		// 创建信号通道（缓冲大小1，避免阻塞）
		c := make(chan os.Signal, 1)
		// 注册要监听的信号，将信号发送到通道c
		signal.Notify(c, posixSignalManager.signals...)

		// 阻塞等待，直到收到信号
		<-c

		// 收到信号后，调用优雅关闭框架的StartShutdown，触发关闭流程
		gs.StartShutdown(posixSignalManager)
	}()

	return nil
}

// ShutdownStart 关闭开始阶段的回调（本实现中无操作）。
func (posixSignalManager *PosixSignalManager) ShutdownStart() error {
	return nil
}

// ShutdownFinish 关闭完成阶段的回调：以os.Exit(0)退出程序。
func (posixSignalManager *PosixSignalManager) ShutdownFinish() error {
	os.Exit(0) // 正常退出程序

	return nil // 理论上不会执行到这里（Exit会直接终止进程）
}
