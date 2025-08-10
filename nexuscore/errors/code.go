/*
errors:code.go
错误码管理包：提供标准化的错误码定义、注册、解析及查询功能。

设计思路：
- 结构体名称首字母小写（如 defaultCoder）：表示类型为包内细节，外部无需依赖具体实现。
- 结构体字段首字母大写（如 defaultCoder.C）：允许外部通过接口访问字段值，平衡封装与可用性。

HTTP 状态码约定：
- 200：请求成功执行
- 400：客户端请求错误（如参数无效）
- 401：认证失败（如令牌无效）
- 403：授权失败（如权限不足）
- 404：资源不存在（如 URL 或 RESTful 资源未找到）
- 500：服务器内部错误
*/
package errors

import (
	"fmt"
	"sync"
)

// ------------------------------ 全局变量与常量 ------------------------------
const (
	// _iseMsg 内部服务器错误的默认提示信息,用于defaultCoder的Ext
	_iseMsg = `发生了内部服务器错误,请参阅http://github.com/maxiaolu1981/errors/README.md`
)

var (
	// _codes 存储所有注册的错误码（key=错误编码，value=Coder实例）
	_codes = map[int]Coder{}
	// _codeMutex 互斥锁，保证并发环境下对 _codes 的操作安全
	_codeMutex = &sync.Mutex{}
	// _unknownCode[1,500,_iseMsg,"http://github.com/maxiaolu1981/errors/README.md"] 提供未知错误的默认编码（兜底方案）
	_unknownCode = defaultCoder{
		C:    1,
		Http: 500,
		Ext:  _iseMsg,
		Ref:  `http://github.com/maxiaolu1981/errors/README.md`,
	}
)

// ------------------------------ 核心接口定义

// Coder 定义错误码的标准行为，是错误码对外暴露的核心接口
type Coder interface {
	// 返回错误的唯一业务编码（如10001）
	Code() int
	// 返回错误对应的HTTP状态码（如400、500）
	HTTPStatus() int
	// 返回用户可见的错误描述（如"参数无效"）
	String() string
	// 返回错误的参考文档地址
	Reference() string
}

// ------------------------------ -----------------------接口实现结构体

// defaultCoder[C Http Ext Ref] 是 Coder 接口的默认实现，封装错误码的核心信息
type defaultCoder struct {
	C    int
	Http int
	Ext  string
	Ref  string
}

// ------------------------------ defaultCoder 方法实现
//------------------------------

// Code 返回业务错误编码
func (d defaultCoder) Code() int {
	return d.C
}

// HTTPStatus 返回对应的HTTP状态码，默认返回500
func (d defaultCoder) HTTPStatus() int {
	if d.Http == 0 {
		return 500
	}
	return d.Http
}

// String 返回用户可见的错误描述
func (d defaultCoder) String() string {
	return d.Ext
}

// Reference 返回错误的参考文档地址
func (d defaultCoder) Reference() string {
	return d.Ref
}

// ------------------------------ 错误码注册函数
//------------------------------

// Register 注册自定义错误码，若编码已存在则覆盖旧值
// 禁止注册编码=0（预留为未知错误标识），否则会panic
func Register(coder Coder) {
	if coder.Code() == 0 {
		panic(_iseMsg)
	}
	withLock(func() {
		_codes[coder.Code()] = coder
	})
}

// MustRegister 注册自定义错误码，若编码已存在则panic（严格模式）
// 禁止注册编码=0（预留为未知错误标识），否则会panic
func MustRegister(coder Coder) {
	if coder.Code() == 0 {
		panic(_iseMsg)
	}
	withLock(func() {
		if _, ok := _codes[coder.Code()]; ok {
			panic(fmt.Sprintf("%d编码已经存在,不允许重复录入", coder.Code()))
		}
		_codes[coder.Code()] = coder
	})
}

// ------------------------------ 错误码解析与检查函数 ------------------------------

// ParseCoderByErr 将输入的错误解析为对应的Coder实例
// 若err为nil，返回nil；若err不是*withCode类型或编码未注册，返回_unknownCode
func ParseCoderByErr(err error) Coder {
	if err == nil {
		return nil
	}

	if v, ok := err.(*withCode); ok {
		if coder, ok := _codes[v.code]; ok {
			return coder
		}
	}

	return _unknownCode
}

func ParseCoderByCode(code int) Coder {
	if coder, ok := _codes[code]; ok {
		return coder
	}
	return _unknownCode
}

// IsCode 检查错误链中是否包含指定的错误编码
// 支持嵌套错误（通过cause追溯底层错误）
func IsCode(err error, code int) bool {
	e, ok := err.(*withCode)
	if ok {
		if e.code == code {
			return true
		}
		if e.cause != nil {
			return IsCode(e.cause, code)
		}
	}
	return false
}

// ------------------------------ 错误码查询与辅助函数 ------------------------------

// ListAllCodes 格式化打印所有已注册的错误码信息，包含编码、HTTP状态、描述及参考文档
func ListAllCodes() {
	withLock(func() {
		if len(_codes) == 0 {
			fmt.Println("当前无注册的错误码")
			return
		}

		// 打印表头
		fmt.Printf("%-8s %-8s %-20s %s\n", "错误码", "HTTP状态", "描述信息", "参考文档")
		fmt.Println("-------------------------------------------------------------------------------")

		// 遍历并格式化输出
		for code, coder := range _codes {
			fmt.Printf(
				"%-8d %-8d %-20s %s\n",
				code,
				coder.HTTPStatus(),
				truncateString(coder.String(), 38), // 截断过长描述
				coder.Reference(),
			)
		}
	})
}

// truncateString 辅助函数：当字符串长度超过max时截断并添加省略号
func truncateString(s string, max int) string {
	byte := []rune(s)
	if len(byte) <= max {
		return s
	}
	return string(byte[:max]) + "..."
}

// 辅助函数withLock 减少重复代码
func withLock(fn func()) {
	_codeMutex.Lock()
	defer _codeMutex.Unlock()
	fn()
}

// ------------------------------ 包初始化 ------------------------------

// init 初始化包时注册默认的未知错误码
func init() {
	Register(_unknownCode)
}

// ------------------------------ 测试用 ------------------------------
func NewDefaultCoder(code int, http int, msg string, ref string) defaultCoder {
	return defaultCoder{
		C:    code,
		Http: http,
		Ext:  msg,
		Ref:  ref,
	}
}
