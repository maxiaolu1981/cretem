/*
这是一个 Go 语言编写的服务器运行选项配置包，主要功能是管理服务器的启动配置。以下是该包的详细分析：

核心结构
ServerRunOptions 结构体
定义了服务器运行的三个核心配置项：

Mode - 服务器运行模式（debug/test/release）

# Healthz - 是否启用健康检查端点

# Middlewares - 允许使用的中间件列表

主要方法
1. AddFlags()
将配置选项绑定到命令行标志，支持通过命令行参数配置：

--server.mode - 设置服务器模式

--server.healthz - 启用/禁用健康检查

--server.middlewares - 指定中间件列表

2. Validate()
验证配置参数的合法性：

模式验证：确保 mode 只能是 debug、test、release 之一

中间件验证：检查每个中间件名称格式（字母数字、下划线、连字符）

3. NewServerRunOptions()
创建默认配置选项，从 server.Config 获取默认值

4. ApplyTo()
将选项应用到服务器配置对象，实现配置传递

设计特点
松耦合设计：通过 ApplyTo 方法将选项与具体实现分离

双重配置源：支持代码配置和命令行参数配置

强验证机制：对输入参数进行严格格式校验

默认值管理：从服务器配置获取合理的默认值

使用场景
该包通常用于：

命令行服务器应用的配置管理

多环境配置（开发、测试、生产）

中间件的动态启用/禁用

健康检查功能的开关控制

代码质量
良好的错误处理和多错误返回

使用正则表达式确保输入安全

清晰的注释和文档

符合 Go 语言的惯用法

这个包体现了 Go 语言中常见的配置管理模式，结合了命令行标志绑定和配置验证的良好实践。
*/
package options

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/util/sets"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation/field"
	"github.com/spf13/pflag"
)

type ServerRunOptions struct {
	Mode             string   `json:"mode"        mapstructure:"mode"`
	Healthz          bool     `json:"healthz"     mapstructure:"healthz"`
	Middlewares      []string `json:"middlewares" mapstructure:"middlewares"`
	EnableProfiling  bool     `json:"enableProfiling" mapstructure:"enableProfiling"`
	EnableMetrics    bool     `json:"enableMetrics" mapstructure:"enableMetrics"`
	FastDebugStartup bool     `json:"fastDebugStartup" mapstructure:"fastDebugStartup"`
	// 新增：Cookie相关配置
	CookieDomain             string        `json:"cookieDomain"    mapstructure:"cookieDomain"`
	CookieSecure             bool          `json:"cookieSecure"    mapstructure:"cookieSecure"`
	CtxTimeout               time.Duration `json:"ctxtimeout"    mapstructure:"ctxtimeout"`
	Env                      string        `json:"env"    mapstructure:"env"`
	LoginRateLimit           int           `json:"loginlimit"   mapstructure:"loginlimit"`
	LoginWindow              time.Duration `json:"loginwindow"   mapstructure:"loginwindow"`
	MaxLoginFailures         int           `json:"maxLoginFailures" mapstructure:"maxLoginFailures"`
	LoginFailReset           time.Duration `json:"loginFailReset"   mapstructure:"loginFailReset"`
	LoginFastFailThreshold   int           `json:"loginFastFailThreshold" mapstructure:"loginFastFailThreshold"`
	LoginFastFailMessage     string        `json:"loginFastFailMessage" mapstructure:"loginFastFailMessage"`
	LoginUpdateBuffer        int           `json:"loginUpdateBuffer" mapstructure:"loginUpdateBuffer"`
	LoginUpdateBatchSize     int           `json:"loginUpdateBatchSize" mapstructure:"loginUpdateBatchSize"`
	LoginUpdateFlushInterval time.Duration `json:"loginUpdateFlushInterval" mapstructure:"loginUpdateFlushInterval"`
	LoginUpdateTimeout       time.Duration `json:"loginUpdateTimeout" mapstructure:"loginUpdateTimeout"`
	LoginCredentialCacheTTL  time.Duration `json:"loginCredentialCacheTTL" mapstructure:"loginCredentialCacheTTL"`
	LoginCredentialCacheSize int           `json:"loginCredentialCacheSize" mapstructure:"loginCredentialCacheSize"`
	// WriteRateLimit: 默认的写操作限流阈值（当 Redis 未配置 override 时使用）
	WriteRateLimit int `json:"writeRateLimit"   mapstructure:"writeRateLimit"`
	// AdminToken: 简单的管理API访问令牌（如果为空，只允许本地或 debug 访问）
	AdminToken string `json:"adminToken" mapstructure:"adminToken"`
	// 新增：生产端限流器开关
	EnableRateLimiter bool `json:"enableRateLimiter" mapstructure:"enableRateLimiter"`
	// 并发处理配置
	MaxGoroutines    int           `json:"max-goroutines" mapstructure:"max-goroutines"`
	MaxQueueSize     int           `json:"max-queue-size" mapstructure:"max-queue-size"`
	TimeoutThreshold time.Duration `json:"timeout-threshold" mapstructure:"timeout-threshold"`
}

func NewServerRunOptions() *ServerRunOptions {
	return &ServerRunOptions{
		Mode:                     gin.ReleaseMode,
		Healthz:                  true,
		Middlewares:              []string{},
		EnableProfiling:          true,
		EnableMetrics:            true,
		FastDebugStartup:         false,
		CookieDomain:             "",
		CookieSecure:             false,
		CtxTimeout:               50 * time.Second,
		Env:                      "development",
		LoginRateLimit:           500000, // 5万/分钟
		WriteRateLimit:           500000, // 写操作默认限流（每 window）
		LoginWindow:              2 * time.Minute,
		MaxLoginFailures:         5,
		LoginFailReset:           15 * time.Minute,
		LoginFastFailThreshold:   0,
		LoginFastFailMessage:     "系统繁忙，请稍后再试",
		LoginUpdateBuffer:        1024,
		LoginUpdateBatchSize:     64,
		LoginUpdateFlushInterval: 200 * time.Millisecond,
		LoginUpdateTimeout:       2 * time.Second,
		LoginCredentialCacheTTL:  30 * time.Second, //凭证缓存有效期
		LoginCredentialCacheSize: 1024,             //凭证缓存最大条目数
		AdminToken:               "",
		EnableRateLimiter:        true,              // 默认启用生产端限流器
		MaxGoroutines:            100,               // 默认最大并发处理数
		MaxQueueSize:             100,               // 默认任务队列大小
		TimeoutThreshold:         100 * time.Second, // 默认单个请求超时阈值
	}
}

func (s *ServerRunOptions) Complete() {
	// EnableRateLimiter: 如果为零值，设置默认值 true
	// 注意：bool类型零值为false，只有未配置时才设为true
	// 若希望默认关闭，改为 false
	// 这里默认 true
	// 不做处理即可，除非有特殊需求
	// 如果字段为零值，设置默认值；否则保持配置的值

	// Mode: 如果为空，设置默认值
	if s.Mode == "" {
		s.Mode = gin.ReleaseMode
	} else {
		// 验证Mode是否有效，如果无效则使用默认值
		validModes := []string{gin.DebugMode, gin.ReleaseMode, gin.TestMode}
		isValid := false
		for _, mode := range validModes {
			if s.Mode == mode {
				isValid = true
				break
			}
		}
		if !isValid {
			s.Mode = gin.ReleaseMode
		}
	}

	// Healthz: 如果为零值，设置默认值
	if !s.Healthz {
		s.Healthz = true
	}

	// Middlewares: 如果为nil或空，设置默认空切片
	if s.Middlewares == nil {
		s.Middlewares = []string{}
	}

	// EnableProfiling: 如果为零值，设置默认值
	if !s.EnableProfiling {
		s.EnableProfiling = true
	}

	// EnableMetrics: 如果为零值，设置默认值
	if !s.EnableMetrics {
		s.EnableMetrics = true
	}

	// CookieDomain: 如果为空，设置默认值
	if s.CookieDomain == "" {
		s.CookieDomain = ""
	}

	// CookieSecure: 设置默认值（如果需要）
	// 注意：bool类型的零值是false，所以这里根据业务需求决定
	// 如果希望默认是false，可以不做处理

	// CtxTimeout: 如果为零值，设置默认值
	if s.CtxTimeout <= 0 {
		s.CtxTimeout = 5 * time.Second
	}
	if s.Env == "" {
		s.Env = "Env"
	}

	if s.LoginRateLimit == 0 {
		s.LoginRateLimit = 1000
	}

	if s.WriteRateLimit == 0 {
		s.WriteRateLimit = 1000
	}

	if s.LoginWindow == 0 {
		s.LoginWindow = time.Minute
	}

	if s.MaxLoginFailures <= 0 {
		s.MaxLoginFailures = 5
	}

	if s.LoginFailReset <= 0 {
		s.LoginFailReset = 15 * time.Minute
	}
	if s.LoginFastFailThreshold < 0 {
		s.LoginFastFailThreshold = 0
	}
	if s.LoginFastFailMessage == "" {
		s.LoginFastFailMessage = "系统繁忙，请稍后再试"
	}
	if s.LoginUpdateBuffer <= 0 {
		s.LoginUpdateBuffer = 1024
	}
	if s.LoginUpdateBatchSize <= 0 {
		s.LoginUpdateBatchSize = 64
	}
	if s.LoginUpdateFlushInterval <= 0 {
		s.LoginUpdateFlushInterval = 200 * time.Millisecond
	}
	if s.LoginUpdateTimeout <= 0 {
		s.LoginUpdateTimeout = 2 * time.Second
	}
	if s.LoginCredentialCacheTTL <= 0 {
		s.LoginCredentialCacheTTL = 30 * time.Second
	}
	if s.LoginCredentialCacheSize <= 0 {
		s.LoginCredentialCacheSize = 1024
	}
}

func (s *ServerRunOptions) Validate() []error {
	var errs = field.ErrorList{}
	var path = field.NewPath("server")

	if s.Mode != "" {
		set := sets.NewString(gin.DebugMode, gin.ReleaseMode, gin.TestMode)
		if !set.Has(s.Mode) {
			errs = append(errs, field.Invalid(path.Child("mode"), s.Mode, "无效的mode模式"))
		}
	}
	if s.Env != "" {
		set := sets.NewString("development", "release", "test")
		if !set.Has(s.Env) {
			errs = append(errs, field.Invalid(path.Child("env"), s.Env, "无效的env模式"))
		}
	}

	// 2. 验证CookieDomain
	if s.CookieDomain != "" {
		domainToValidate := s.CookieDomain
		// 处理通配符域名（如 ".example.com"）
		if strings.HasPrefix(domainToValidate, ".") {
			domainToValidate = strings.TrimPrefix(domainToValidate, ".")
			if domainToValidate == "" {
				errs = append(errs, field.Invalid(
					path.Child("cookieDomain"),
					s.CookieDomain,
					"Cookie域名不能仅为点号",
				))
			}
		}

		// 使用标准的DNS验证
		if validationErrs := validation.IsDNS1123Subdomain(domainToValidate); len(validationErrs) > 0 {
			for _, err := range validationErrs {
				errs = append(errs, field.Invalid(
					path.Child("cookieDomain"),
					s.CookieDomain,
					"Cookie域名格式无效: "+err,
				))
			}
		}
	}
	// 3. 验证CookieSecure的合理性
	if s.CookieSecure && s.Mode == gin.DebugMode {
		errs = append(errs, field.Invalid(
			path.Child("cookieSecure"),
			s.CookieSecure,
			"调试模式下不应启用Secure Cookie（建议设置为false）",
		))
	}

	if s.LoginRateLimit < 0 {
		errs = append(errs, field.Invalid(
			path.Child("loginRateLimit"),
			s.LoginRateLimit,
			"限流数不能小于0",
		))
	}

	if s.LoginWindow < 1 {
		errs = append(errs, field.Invalid(
			path.Child("LoginWindow"),
			s.LoginWindow,
			"限流时间不能小于1",
		))
	}

	if s.MaxLoginFailures <= 0 {
		errs = append(errs, field.Invalid(
			path.Child("maxLoginFailures"),
			s.MaxLoginFailures,
			"最大登录失败次数必须大于0",
		))
	}

	if s.LoginFailReset <= 0 {
		errs = append(errs, field.Invalid(
			path.Child("loginFailReset"),
			s.LoginFailReset,
			"登录失败计数失效时间必须大于0",
		))
	}

	agg := errs.ToAggregate()
	if agg == nil {
		return nil // 无错误时返回空切片，而非nil
	}
	return agg.Errors()
}

func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&s.EnableRateLimiter, "server.enable-rate-limiter", s.EnableRateLimiter, "是否启用生产端限流器（默认启用）")
	fs.StringVarP(&s.Mode, "server.mode", "M", s.Mode, ""+
		"指定服务器运行模式。支持的服务器模式：debug(调试)、test(测试)、release(发布)。")

	fs.BoolVarP(&s.Healthz, "server.healthz", "z", s.Healthz, ""+
		"启用健康检查并安装 /healthz 路由。")

	fs.BoolVar(&s.CookieSecure, "server.cookieSecure", s.CookieSecure, ""+
		"启用cookie安全设置(建议在生成环境下开启。")

	fs.StringVar(&s.CookieDomain, "server.cookieDomain", s.CookieDomain, ""+
		"指定cookie对域的限制.空字符串表示任何域都可以绑定cookie")
	fs.StringSliceVarP(&s.Middlewares, "server.middlewares", "w", s.Middlewares, ""+
		"服务器允许的中间件列表，逗号分隔。如果列表为空，将使用默认中间件。")
	fs.StringVar(&s.Env, "server.env", s.Env, ""+
		"环境模式包括:development,release,test")

	fs.IntVar(&s.LoginRateLimit, "server.Loginlimit", s.LoginRateLimit, ""+
		"指定限流次数")
	fs.DurationVar(&s.LoginWindow, "server.loginwindow", s.LoginWindow, ""+
		"指定限流时间")
	fs.IntVar(&s.MaxLoginFailures, "server.login-max-attempts", s.MaxLoginFailures, ""+
		"同一用户在计数窗口内允许的最大登录失败次数")
	fs.DurationVar(&s.LoginFailReset, "server.login-fail-reset", s.LoginFailReset, ""+
		"登录失败计数的自动重置时间窗口")
	fs.IntVar(&s.LoginFastFailThreshold, "server.login-fastfail-threshold", s.LoginFastFailThreshold, ""+
		"当并发登录请求超过该值时快速返回（0 表示禁用）")
	fs.StringVar(&s.LoginFastFailMessage, "server.login-fastfail-message", s.LoginFastFailMessage, ""+
		"快速降级时返回给客户端的提示信息")
	fs.IntVar(&s.LoginUpdateBuffer, "server.login-update-buffer", s.LoginUpdateBuffer, ""+
		"登录时间异步更新队列缓存大小")
	fs.IntVar(&s.LoginUpdateBatchSize, "server.login-update-batch", s.LoginUpdateBatchSize, ""+
		"登录时间异步更新单次批量写入的最大条数")
	fs.DurationVar(&s.LoginUpdateFlushInterval, "server.login-update-flush-interval", s.LoginUpdateFlushInterval, ""+
		"登录时间异步更新强制刷新间隔")
	fs.DurationVar(&s.LoginUpdateTimeout, "server.login-update-timeout", s.LoginUpdateTimeout, ""+
		"登录时间批量更新的数据库超时时间")
	fs.DurationVar(&s.LoginCredentialCacheTTL, "server.login-credential-cache-ttl", s.LoginCredentialCacheTTL, ""+
		"登录凭证比较结果在本地缓存的有效期")
	fs.IntVar(&s.LoginCredentialCacheSize, "server.login-credential-cache-size", s.LoginCredentialCacheSize, ""+
		"登录凭证比较结果本地缓存的最大条目数")
	fs.StringVar(&s.AdminToken, "server.admin-token", s.AdminToken,
		"管理API的简单访问令牌（默认为空，仅允许本地访问）")
	fs.BoolVar(&s.FastDebugStartup, "server.fast-debug-startup", s.FastDebugStartup, "调试模式下是否跳过耗时的依赖等待，加速本地调试启动")
}
