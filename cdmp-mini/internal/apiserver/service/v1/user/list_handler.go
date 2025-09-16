package user

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) List(ctx context.Context, opts metav1.ListOptions, opt *options.Options) (*v1.UserList, error) {
	startTime := time.Now()
	logger := log.L(ctx).WithValues(
		"operation", "ListUsers",
		"timeout", opts.TimeoutSeconds,
		"limit", opts.Limit,
		"offset", opts.Offset,
	)

	// 步骤1：查询原始用户列表
	users, err := u.Store.Users().List(ctx, opts, u.Options)
	if err != nil {
		logger.Errorf("List users from storage failed: %v", err)
		return nil, errors.WithCode(code.ErrDatabase, "query raw users failed: %v", err)
	}

	if len(users.Items) == 0 {
		logger.Debug("No users found in storage")
		return &v1.UserList{ListMeta: users.ListMeta, Items: []*v1.User{}}, nil
	}

	logger.Debugf("Found %d users, starting parallel processing", len(users.Items))

	// 步骤2：初始化并行处理组件
	const (
		maxGoroutines    = 20 // 根据存储层QPS调整
		maxQueueSize     = 100
		timeoutThreshold = 5 * time.Second // 单个请求超时阈值
	)

	var (
		wg                sync.WaitGroup
		mu                sync.Mutex
		hasError          int32
		cancelCtx, cancel = context.WithCancel(ctx)
	)
	defer cancel()

	// 预分配结果切片和索引映射 - 修复：使用 uint64 作为键类型
	resultItems := make([]*v1.User, len(users.Items))
	idToIndex := make(map[uint64]int, len(users.Items)) // 改为 uint64
	for i, user := range users.Items {
		idToIndex[user.ID] = i      // 现在类型匹配了
		resultItems[i] = &v1.User{} // 预分配内存，避免nil
	}

	// 工作池：限制并发数
	userChan := make(chan *v1.User, maxQueueSize)
	errChan := make(chan error, 1) // 只需要一个错误信号

	// 启动worker池
	for i := 0; i < maxGoroutines; i++ {
		wg.Add(1)
		go u.processUserWorker(cancelCtx, i, userChan, errChan, &wg, &mu, idToIndex, resultItems, &hasError)
	}

	// 步骤3：发送任务到工作池
	go func() {
		defer close(userChan)
		for _, user := range users.Items { // 移除了未使用的 i 变量
			if atomic.LoadInt32(&hasError) == 1 {
				break
			}

			select {
			case userChan <- user:
				// 任务发送成功
			case <-cancelCtx.Done():
				return
			default:
				// 队列满时等待或跳过（根据业务需求调整）
				select {
				case userChan <- user:
				case <-cancelCtx.Done():
					return
				case <-time.After(100 * time.Millisecond):
					logger.Warnf("User channel full, skipping user %d", user.ID) // 改为 %d
				}
			}
		}
	}()

	// 步骤4：等待完成并处理结果
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// 监听完成信号
	select {
	case <-cancelCtx.Done():
		err := errors.WithCode(code.ErrContextCanceled, "list users canceled: %v", ctx.Err())
		logger.Errorf("Operation canceled: %v", err)
		return nil, err

	case err, ok := <-errChan:
		if ok && err != nil {
			logger.Errorf("Parallel processing failed: %v", err)
			return nil, err
		}
	}

	// 统计处理时间
	processingTime := time.Since(startTime)
	logger.Infof("Successfully processed %d users in %v", len(resultItems), processingTime)

	return &v1.UserList{
		ListMeta: users.ListMeta,
		Items:    resultItems,
	}, nil
}

// processUserWorker 处理单个用户的worker
func (u *UserService) processUserWorker(
	ctx context.Context,
	workerID int,
	userChan <-chan *v1.User,
	errChan chan<- error,
	wg *sync.WaitGroup,
	mu *sync.Mutex,
	idToIndex map[uint64]int, // 改为 uint64
	resultItems []*v1.User,
	hasError *int32,
) {
	defer wg.Done()
	logger := log.L(ctx).WithValues("worker", workerID)

	for user := range userChan {
		// 快速检查错误状态
		if atomic.LoadInt32(hasError) == 1 {
			return
		}

		// 检查上下文取消
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 处理单个用户（带超时控制）
		processedUser, err := u.processSingleUserWithTimeout(ctx, user)
		if err != nil {
			if atomic.CompareAndSwapInt32(hasError, 0, 1) {
				select {
				case errChan <- err:
				default: // 避免阻塞
				}
			}
			return
		}

		// 安全写入结果
		mu.Lock()
		if idx, exists := idToIndex[user.ID]; exists { // 现在类型匹配了
			resultItems[idx] = processedUser
		} else {
			logger.Warnf("User ID %d not found in index map", user.ID)
		}
		mu.Unlock()
	}
}

// processSingleUserWithTimeout 带超时控制的单个用户处理
func (u *UserService) processSingleUserWithTimeout(ctx context.Context, user *v1.User) (*v1.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 查询用户策略 - 取消注释并根据实际情况实现
	//policies, err := u.store.Policies().List(ctx, user.Name, metav1.ListOptions{})
	//if err != nil {
	//	return nil, errors.WithCode(code.ErrDatabase, "query policies for user %s failed: %v", user.Name, err)
	//}

	// 组装用户数据
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			ID:         user.ID,
			InstanceID: user.InstanceID,
			Name:       user.Name,
			Extend:     user.Extend,
			CreatedAt:  user.CreatedAt,
			UpdatedAt:  user.UpdatedAt,
		},
		Nickname: user.Nickname,
		Email:    user.Email,
		Phone:    user.Phone,
		//TotalPolicy: policies.TotalCount, // 取消注释
		LoginedAt: user.LoginedAt,
	}, nil
}
