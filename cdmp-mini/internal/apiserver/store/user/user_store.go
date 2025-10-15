package user

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

type Users struct {
	db          *gorm.DB
	policyStore interfaces.PolicyStore
}

var ensureIndexesOnce sync.Once

func ensureUserCoveringIndexes(db *gorm.DB) {
	ensureIndexesOnce.Do(func() {
		if db == nil {
			return
		}
		databaseName := db.Migrator().CurrentDatabase()
		if databaseName == "" {
			log.Warn("无法获取当前数据库名称，跳过覆盖索引检查")
			return
		}
		indexSpecs := []struct {
			name    string
			columns string
		}{
			{name: "idx_user_email_name", columns: "email, name"},
			{name: "idx_user_phone_name", columns: "phone, name"},
		}
		for _, spec := range indexSpecs {
			var exists int64
			query := `SELECT COUNT(1) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 'user' AND INDEX_NAME = ?`
			if err := db.Raw(query, databaseName, spec.name).Scan(&exists).Error; err != nil {
				log.Warnf("检查用户索引失败: index=%s err=%v", spec.name, err)
				continue
			}
			if exists > 0 {
				continue
			}
			createSQL := fmt.Sprintf(`CREATE INDEX %s ON user (%s)`, spec.name, spec.columns)
			if err := db.Exec(createSQL).Error; err != nil {
				log.Warnf("创建用户覆盖索引失败: index=%s err=%v", spec.name, err)
				continue
			}
			log.Infof("创建用户覆盖索引成功: index=%s columns=%s", spec.name, spec.columns)
		}
	})
}

func NewUsers(db *gorm.DB, policyStore interfaces.PolicyStore) *Users {
	ensureUserCoveringIndexes(db)
	return &Users{
		db:          db,
		policyStore: policyStore,
	}
}

// executeSingleGet 执行单次查询
func (u *Users) executeSingleGet(ctx context.Context, username string) (*v1.User, error) {
	//start := time.Now()
	user := &v1.User{}
	// 先查询用户是否存在（不管状态）
	err := u.db.WithContext(ctx).
		Where("name = ?", username).
		First(user).Error

	// 记录数据库查询指标
	//metrics.RecordDatabaseQuery("get", "user", float64(duration), nil)

	if err != nil {
		// 检查是否是 gorm.ErrRecordNotFound
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err := errors.WithCode(code.ErrUserNotFound, "用户没有发现")
			return nil, err
		}
		return nil, err
	}
	// 检查用户状态
	if user.Status == 0 { // status = 0 表示失效
		return nil, errors.WithCode(code.ErrUserDisabled, "用户已失效")
	}
	return user, nil
}

// handleGetError 处理查询错误
func (u *Users) handleGetError(err error) error {
	// 使用错误码框架解析错误
	coder := errors.ParseCoderByErr(err)
	if coder != nil {
		// 如果是已知错误码，直接返回
		return err
	}

	// 处理GORM原始错误
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.WithCode(code.ErrDatabaseTimeout, "查询用户超时")

	case errors.Is(err, gorm.ErrRecordNotFound):
		return errors.WithCode(code.ErrUserNotFound, "用户不存在")

	case u.isMySQLDeadlockError(err):
		return errors.WithCode(code.ErrDatabaseDeadlock, "系统繁忙，请稍后重试")

	default:
		return errors.WithCode(code.ErrDatabase, "数据库查询失败: %v", err)
	}
}

// isMySQLDeadlockError 检查MySQL死锁错误
func (u *Users) isMySQLDeadlockError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1213 // ER_LOCK_DEADLOCK
	}
	return false
}

// isRetryableError 判断错误是否可重试 - 生产级实现
func (u *Users) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 使用错误码框架解析错误
	coder := errors.ParseCoderByErr(err)
	if coder != nil {
		errorCode := coder.Code()

		// 1. 首先排除明确不可重试的业务错误
		if u.isNonRetryableBusinessError(errorCode) {
			return false
		}

		// 2. 检查可重试的错误
		if u.isRetryableErrorCode(errorCode) {
			return true
		}
	}

	// 3. 对于未明确分类的错误，检查错误消息中的模式
	return u.isRetryableByErrorMessage(err.Error())
}

// TODO: 需要和code框架保持一致
// isNonRetryableBusinessError 检查明确不可重试的业务错误码
func (u *Users) isNonRetryableBusinessError(errorCode int) bool {
	nonRetryableCodes := map[int]bool{
		code.ErrSuccess:              true, // 100001 成功
		code.ErrBind:                 true, // 100003 请求格式错误
		code.ErrValidation:           true, // 100004 数据校验失败
		code.ErrUserAlreadyExist:     true, // 110002 用户已存在
		code.ErrUserNotFound:         true, // 110001 用户不存在
		code.ErrInvalidParameter:     true, // 110004 参数无效
		code.ErrPermissionDenied:     true, // 100207 权限不足   true, // 100207 权限不足
		code.ErrResourceConflict:     true, // 110006 资源冲突
		code.ErrReachMaxCount:        true, // 110101 达到上限
		code.ErrSecretNotFound:       true, // 110102 密钥不存在
		code.ErrPolicyNotFound:       true, // 110201 策略不存在
		code.ErrSignatureInvalid:     true, // 100202 签名无效
		code.ErrTokenInvalid:         true, // 100208 令牌无效
		code.ErrExpired:              true, // 100203 令牌过期
		code.ErrInvalidAuthHeader:    true, // 100204 授权头格式无效
		code.ErrMissingHeader:        true, // 100205 缺少授权头
		code.ErrPasswordIncorrect:    true, // 100206 密码不正确
		code.ErrRespCodeRTRevoked:    true, // 100211 令牌已撤销
		code.ErrTokenMismatch:        true, // 100212 令牌不匹配
		code.ErrInvalidJSON:          true, // 100303 JSON格式错误
		code.ErrInvalidYaml:          true, // 100306 YAML格式错误
		code.ErrPageNotFound:         true, // 100005 页面不存在
		code.ErrMethodNotAllowed:     true, // 100006 方法不允许
		code.ErrUnsupportedMediaType: true, // 100007 不支持的Content-Type
		code.ErrNotAdministrator:     true,
		code.ErrUserDisabled:         true,
		code.ErrAccountLocked:        true, // 100213 账户已锁定
	}

	return nonRetryableCodes[errorCode]
}

// isRetryableErrorCode 检查可重试的错误码
func (u *Users) isRetryableErrorCode(errorCode int) bool {
	retryableCodes := map[int]bool{
		code.ErrDatabaseTimeout:  true, // 100102 数据库操作超时
		code.ErrDatabaseDeadlock: true, // 100103 数据库死锁
		code.ErrDatabase:         true, // 100101 数据库操作错误
	}

	if retryableCodes[errorCode] {
		return true
	}

	return false
}

// isRetryableByErrorMessage 根据错误消息判断是否可重试
func (u *Users) isRetryableByErrorMessage(errorMsg string) bool {
	errorMsg = strings.ToLower(errorMsg)

	// 可重试的错误模式
	retryablePatterns := []string{
		"timeout", "deadlock", "lock", "connection",
		"network", "socket", "reset", "refused",
		"busy", "try again", "retry", "temporarily", "wait",
	}

	// 不可重试的错误模式（数据完整性错误）
	nonRetryablePatterns := []string{
		"duplicate", "unique", "foreign key", "not null",
		"constraint", "already exist", "not found",
	}

	// 先检查不可重试的模式
	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errorMsg, pattern) {
			return false
		}
	}

	// 再检查可重试的模式
	for _, pattern := range retryablePatterns {
		if strings.Contains(errorMsg, pattern) {
			return true
		}
	}

	return false
}

// createTimeoutContext 创建超时上下文（考虑重试时间预算）
func (u *Users) createTimeoutContext(ctx context.Context,
	baseTimeout time.Duration, maxAttempts int) (context.Context, context.CancelFunc) {

	// 为整个重试操作分配足够的超时时间
	totalTimeout := time.Duration(maxAttempts+1) * baseTimeout

	// 如果父上下文有更短的超时，使用父上下文的剩余时间
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < totalTimeout {
			return context.WithTimeout(ctx, remaining)
		}
	}

	return context.WithTimeout(ctx, totalTimeout)
}

// 通用的调用者信息获取
func GetCallerInfo(skip int) string {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}

	funcName := runtime.FuncForPC(pc).Name()

	// 简化路径显示
	parts := strings.Split(file, "/")
	if len(parts) > 2 {
		file = strings.Join(parts[len(parts)-2:], "/")
	}

	return fmt.Sprintf("%s@%s:%d", funcName, file, line)
}
