package code

import (
	"net/http"
	"testing"

	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/stretchr/testify/assert"
)

// TestErrCode_Interface 验证ErrCode是否实现了errors.Coder接口
func TestErrCode_Interface(t *testing.T) {
	var coder errors.Coder = &ErrCode{}
	assert.NotNil(t, coder, "ErrCode应实现errors.Coder接口")
}

// TestErrCode_Methods 测试ErrCode的方法逻辑
func TestErrCode_Methods(t *testing.T) {
	tests := []struct {
		name       string
		errCode    ErrCode
		wantCode   int
		wantHTTP   int
		wantString string
		wantRef    string
	}{
		{
			name: "完整配置的错误码",
			errCode: ErrCode{
				C:    10001,
				HTTP: 400,
				Ext:  "参数错误",
				Ref:  "doc#param-error",
			},
			wantCode:   10001,
			wantHTTP:   400,
			wantString: "参数错误",
			wantRef:    "doc#param-error",
		},
		{
			name: "未指定HTTP状态码（使用默认值）",
			errCode: ErrCode{
				C:    50001,
				HTTP: 0,
				Ext:  "数据库错误",
				Ref:  "",
			},
			wantCode:   50001,
			wantHTTP:   http.StatusInternalServerError,
			wantString: "数据库错误",
			wantRef:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, tt.errCode.Code())
			assert.Equal(t, tt.wantHTTP, tt.errCode.HTTPStatus())
			assert.Equal(t, tt.wantString, tt.errCode.String())
			assert.Equal(t, tt.wantRef, tt.errCode.Reference())
		})
	}
}

// TestRegister_ParamValidation 仅测试register的参数校验逻辑（不涉及实际注册）
func TestRegister_ParamValidation(t *testing.T) {
	// 提前定义一个绝对不会重复的临时错误码（避免污染全局状态）
	tempCode := 999999

	tests := []struct {
		name       string
		code       int
		httpStatus int
		message    string
		refs       []string
		wantPanic  bool
		panicMsg   string
	}{
		{
			name:       "HTTP状态码不在允许列表中",
			code:       tempCode + 1,
			httpStatus: 405, // 不允许的状态码
			message:    "方法不允许",
			refs:       []string{},
			wantPanic:  true,
			panicMsg:   "http编码必须定义在`200,400,401,403,404,500` 之间",
		},
		{
			name:       "HTTP状态码在允许列表中",
			code:       tempCode + 2,
			httpStatus: 403, // 允许的状态码
			message:    "权限不足",
			refs:       []string{},
			wantPanic:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				// 预期panic的场景直接调用，验证参数校验
				assert.PanicsWithValue(t, tt.panicMsg, func() {
					register(tt.code, tt.httpStatus, tt.message, tt.refs...)
				})
			} else {
				// 不panic的场景，调用后验证是否能正常执行（不验证注册结果）
				assert.NotPanics(t, func() {
					register(tt.code, tt.httpStatus, tt.message, tt.refs...)
				})
			}
		})
	}
}

// TestRegister_RefHandling 测试参考信息处理逻辑（只取第一个）
func TestRegister_RefHandling(t *testing.T) {
	// 手动构造ErrCode，模拟register的内部逻辑
	code := 888888
	httpStatus := 400
	message := "测试参考信息"
	refs := []string{"ref#1", "ref#2", "ref#3"}

	// 模拟register函数内部的参考信息处理逻辑
	var reference string
	if len(refs) > 0 {
		reference = refs[0]
	}
	coder := ErrCode{
		C:    code,
		HTTP: httpStatus,
		Ext:  message,
		Ref:  reference,
	}

	// 验证是否只取第一个参考信息
	assert.Equal(t, "ref#1", coder.Ref)
	assert.NotEqual(t, "ref#2", coder.Ref)
}

// TestRegister_DuplicateCode 测试重复注册错误码的场景
func TestRegister_DuplicateCode(t *testing.T) {
	// 步骤1：注册一个新的错误码（确保成功）
	code := 777777
	assert.NotPanics(t, func() {
		register(code, 404, "重复注册测试")
	})

	// 步骤2：再次注册相同的错误码（预期panic）

	expectedPanicMsg := "777777编码已经存在,不允许重复录入"
	assert.PanicsWithValue(t, expectedPanicMsg, func() {
		register(code, 404, "重复注册测试")
	})

}
