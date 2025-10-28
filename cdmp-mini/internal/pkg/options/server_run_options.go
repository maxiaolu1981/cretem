package options

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/util/sets"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation/field"
	"github.com/spf13/pflag"
	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultContactLookupTimeout           = 2 * time.Second
	DefaultContactRefreshTimeout          = 3 * time.Second
	DefaultContactPreflightMaxConcurrency = 64
	// MinUserPendingCreateTTL guarantees pending markers survive slow consumer restarts and large Kafka backlogs.
	MinUserPendingCreateTTL = 10 * time.Minute
)

type ServerRunOptions struct {
	Mode                           string        `json:"mode"        mapstructure:"mode"`
	Healthz                        bool          `json:"healthz"     mapstructure:"healthz"`
	Middlewares                    []string      `json:"middlewares" mapstructure:"middlewares"`
	EnableProfiling                bool          `json:"enableProfiling" mapstructure:"enableProfiling"`
	EnableMetrics                  bool          `json:"enableMetrics" mapstructure:"enableMetrics"`
	FastDebugStartup               bool          `json:"fastDebugStartup" mapstructure:"fastDebugStartup"`
	EnableContactWarmup            bool          `json:"enableContactWarmup" mapstructure:"enableContactWarmup"`
	EnableUserTraceLogging         bool          `json:"enableUserTraceLogging" mapstructure:"enableUserTraceLogging"`
	UserTraceLogSampleRate         float64       `json:"userTraceLogSampleRate" mapstructure:"userTraceLogSampleRate"`
	UserTraceForceLogErrors        bool          `json:"userTraceForceLogErrors" mapstructure:"userTraceForceLogErrors"`
	UserTraceDisableLogging        bool          `json:"userTraceDisableLogging" mapstructure:"userTraceDisableLogging"`
	ContactLookupTimeout           time.Duration `json:"contactLookupTimeout" mapstructure:"contactLookupTimeout"`
	ContactRefreshTimeout          time.Duration `json:"contactRefreshTimeout" mapstructure:"contactRefreshTimeout"`
	ContactPreflightMaxConcurrency int           `json:"contactPreflightMaxConcurrency" mapstructure:"contactPreflightMaxConcurrency"`
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
	// 新增：Kafka 生产者失败消息的降级目录
	ProducerFallbackDir  string        `json:"producer-fallback-dir" mapstructure:"producer-fallback-dir"`
	PasswordHashCost     int           `json:"password-hash-cost" mapstructure:"password-hash-cost"`
	UserPendingCreateTTL time.Duration `json:"userPendingCreateTTL" mapstructure:"userPendingCreateTTL"`
}

func NewServerRunOptions() *ServerRunOptions {
	return &ServerRunOptions{
		Mode:                           gin.ReleaseMode,
		Healthz:                        true,
		Middlewares:                    []string{},
		EnableProfiling:                true,
		EnableMetrics:                  true,
		FastDebugStartup:               false,
		CookieDomain:                   "",
		CookieSecure:                   false,
		CtxTimeout:                     50 * time.Second,
		Env:                            "development",
		LoginRateLimit:                 500000, // 5万/分钟
		WriteRateLimit:                 500000, // 写操作默认限流（每 window）
		LoginWindow:                    2 * time.Minute,
		MaxLoginFailures:               5,
		LoginFailReset:                 15 * time.Minute,
		LoginFastFailThreshold:         0,
		LoginFastFailMessage:           "系统繁忙，请稍后再试",
		LoginUpdateBuffer:              1024,
		LoginUpdateBatchSize:           64,
		LoginUpdateFlushInterval:       200 * time.Millisecond,
		LoginUpdateTimeout:             2 * time.Second,
		LoginCredentialCacheTTL:        30 * time.Second, //凭证缓存有效期
		LoginCredentialCacheSize:       1024,             //凭证缓存最大条目数
		AdminToken:                     "",
		EnableRateLimiter:              false,             // 默认启用生产端限流器
		MaxGoroutines:                  100,               // 默认最大并发处理数
		MaxQueueSize:                   100,               // 默认任务队列大小
		TimeoutThreshold:               100 * time.Second, // 默认单个请求超时阈值
		EnableContactWarmup:            false,             //联系人预热
		EnableUserTraceLogging:         true,              //跟踪日志
		UserTraceLogSampleRate:         0.1,
		UserTraceForceLogErrors:        true,
		UserTraceDisableLogging:        false,
		ContactLookupTimeout:           DefaultContactLookupTimeout,
		ContactRefreshTimeout:          DefaultContactRefreshTimeout,
		ContactPreflightMaxConcurrency: DefaultContactPreflightMaxConcurrency,
		ProducerFallbackDir:            "/var/log/iam/producer",
		PasswordHashCost:               6,
		UserPendingCreateTTL:           MinUserPendingCreateTTL,
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

	if s.UserTraceLogSampleRate < 0 {
		s.UserTraceLogSampleRate = 0
	} else if s.UserTraceLogSampleRate > 1 {
		s.UserTraceLogSampleRate = 1
	}

	// Middlewares: 如果为nil或空，设置默认空切片
	if s.Middlewares == nil {
		s.Middlewares = []string{}
	}

	// EnableProfiling 默认为 true，但允许显式关闭
	// EnableMetrics 默认为 true，但允许显式关闭

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

	if s.ContactLookupTimeout <= 0 {
		s.ContactLookupTimeout = DefaultContactLookupTimeout
	}

	if s.ContactRefreshTimeout <= 0 {
		s.ContactRefreshTimeout = DefaultContactRefreshTimeout
	}

	if s.ContactPreflightMaxConcurrency <= 0 {
		s.ContactPreflightMaxConcurrency = DefaultContactPreflightMaxConcurrency
	}

	if s.PasswordHashCost <= 0 {
		s.PasswordHashCost = bcrypt.DefaultCost
	}
	if s.PasswordHashCost < bcrypt.MinCost {
		s.PasswordHashCost = bcrypt.MinCost
	}
	if s.PasswordHashCost > bcrypt.MaxCost {
		s.PasswordHashCost = bcrypt.MaxCost
	}

	if s.UserPendingCreateTTL <= 0 {
		s.UserPendingCreateTTL = MinUserPendingCreateTTL
	}
	if s.UserPendingCreateTTL < MinUserPendingCreateTTL {
		s.UserPendingCreateTTL = MinUserPendingCreateTTL
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

	if s.ContactLookupTimeout <= 0 {
		errs = append(errs, field.Invalid(
			path.Child("contactLookupTimeout"),
			s.ContactLookupTimeout,
			"联系人唯一性查库超时时间必须大于0",
		))
	}

	if s.ContactRefreshTimeout <= 0 {
		errs = append(errs, field.Invalid(
			path.Child("contactRefreshTimeout"),
			s.ContactRefreshTimeout,
			"联系人唯一性负缓存刷新超时时间必须大于0",
		))
	}

	if s.ContactPreflightMaxConcurrency <= 0 {
		errs = append(errs, field.Invalid(
			path.Child("contactPreflightMaxConcurrency"),
			s.ContactPreflightMaxConcurrency,
			"联系人预检最大并发数必须大于0",
		))
	}

	if s.PasswordHashCost < bcrypt.MinCost || s.PasswordHashCost > bcrypt.MaxCost {
		errs = append(errs, field.Invalid(
			path.Child("passwordHashCost"),
			s.PasswordHashCost,
			"bcrypt成本因子超出允许范围",
		))
	}

	if s.UserPendingCreateTTL <= 0 {
		errs = append(errs, field.Invalid(
			path.Child("userPendingCreateTTL"),
			s.UserPendingCreateTTL,
			"pending create 标记的 TTL 必须大于 0",
		))
	}

	if s.UserTraceLogSampleRate < 0 || s.UserTraceLogSampleRate > 1 {
		errs = append(errs, field.Invalid(
			path.Child("userTraceLogSampleRate"),
			s.UserTraceLogSampleRate,
			"用户链路日志采样率必须位于[0,1]区间",
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
	fs.BoolVar(&s.EnableContactWarmup, "server.enable-contact-warmup", s.EnableContactWarmup, "是否在启动后预热邮箱/手机号唯一性缓存（默认关闭）")
	fs.BoolVar(&s.EnableMetrics, "server.enable-metrics", s.EnableMetrics, "是否注册 Prometheus 指标路由")
	fs.BoolVar(&s.EnableProfiling, "server.enable-profiling", s.EnableProfiling, "是否暴露 pprof 调试端点（仅 debug 模式有效）")
	fs.BoolVar(&s.EnableUserTraceLogging, "server.enable-user-trace-logging", s.EnableUserTraceLogging, "是否启用用户API链路追踪日志输出")
	fs.Float64Var(&s.UserTraceLogSampleRate, "server.user-trace-log-sample-rate", s.UserTraceLogSampleRate, "用户API链路日志的采样率（0-1 之间，默认0.1）")
	fs.BoolVar(&s.UserTraceForceLogErrors, "server.user-trace-force-log-errors", s.UserTraceForceLogErrors, "是否在出现错误时强制输出用户链路日志")
	fs.BoolVar(&s.UserTraceDisableLogging, "server.user-trace-disable-logging", s.UserTraceDisableLogging, "是否关闭用户链路日志（仍会在force模式下输出错误日志）")
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
		"登录时间异Async更新强制刷新间隔")
	fs.DurationVar(&s.LoginUpdateTimeout, "server.login-update-timeout", s.LoginUpdateTimeout, ""+
		"登录时间批量更新的数据库超时时间")
	fs.DurationVar(&s.LoginCredentialCacheTTL, "server.login-credential-cache-ttl", s.LoginCredentialCacheTTL, ""+
		"登录凭证比较结果在本地缓存的有效期")
	fs.IntVar(&s.LoginCredentialCacheSize, "server.login-credential-cache-size", s.LoginCredentialCacheSize, ""+
		"登录凭证比较结果本地缓存的最大条目数")
	fs.DurationVar(&s.ContactLookupTimeout, "server.contact-lookup-timeout", s.ContactLookupTimeout, "联系人唯一性查库超时阈值")
	fs.DurationVar(&s.ContactRefreshTimeout, "server.contact-refresh-timeout", s.ContactRefreshTimeout, "联系人唯一性负缓存刷新查库超时阈值")
	fs.IntVar(&s.ContactPreflightMaxConcurrency, "server.contact-preflight-max-concurrency", s.ContactPreflightMaxConcurrency, "预检查询允许的最大并发数，用于保护数据库连接数")
	fs.StringVar(&s.AdminToken, "server.admin-token", s.AdminToken,
		"管理API的简单访问令牌（默认为空，仅允许本地访问）")
	fs.BoolVar(&s.FastDebugStartup, "server.fast-debug-startup", s.FastDebugStartup, "调试模式下是否跳过耗时的依赖等待，加速本地调试启动")
	fs.StringVar(&s.ProducerFallbackDir, "server.producer-fallback-dir", s.ProducerFallbackDir, "Directory to store failed Kafka producer messages as a fallback.")
	fs.IntVar(&s.PasswordHashCost, "server.password-hash-cost", s.PasswordHashCost, "设置bcrypt密码哈希成本（范围 4-31，默认10，压测可适当降低）")
	fs.DurationVar(&s.UserPendingCreateTTL, "server.user-pending-create-ttl", s.UserPendingCreateTTL, "Redis 用户创建幂等标记的过期时间")
}
