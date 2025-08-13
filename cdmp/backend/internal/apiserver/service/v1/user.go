/*
一、包摘要
v1 包聚焦用户领域的业务逻辑，通过 UserSrv 接口及 userService 实现，封装了用户资源的全生命周期操作（创建、查询、更新、删除等）。核心解决：
数据交互标准化：基于 metav1 元数据规范（如 CreateOptions、ListOptions），统一用户操作的入参 / 出参格式；
存储层解耦：通过 store.Factory 对接存储层（数据库操作），实现业务逻辑与存储细节分离；
性能优化实践：对比实现了并行查询优化（List 方法）与串行低效示例（ListWithBadPerformance），验证不同查询策略对性能的影响。
二、核心流程
代码围绕 UserSrv 接口展开，覆盖用户资源的 CURD + 复杂查询，关键流程拆解：
1. 接口定义（UserSrv）
定义用户领域的核心操作契约，要求实现类必须覆盖：
基础操作：Create（创建）、Update（更新）、Delete（删除单条 / 批量）、Get（查询单条）；
查询场景：List（高性能查询）、ListWithBadPerformance（串行低效查询）；
业务扩展：ChangePassword（密码修改）。
2. 实现类（userService）
通过依赖注入 store.Factory（存储层工厂），实现 UserSrv 接口，核心逻辑：
存储层代理：调用 u.store.Users()/u.store.Policies() 对接数据库操作；
性能对比：
List：通过 并行查询 + sync.Map + WaitGroup 优化关联数据（用户 - 策略）查询效率；
ListWithBadPerformance：串行查询 作为反面示例，暴露低效查询的问题；
错误处理：通过 errors.WithCode 封装业务错误码（如 code.ErrUserAlreadyExist），统一错误格式。
3. 关键设计细节
并行查询优化（List 方法）：
用 sync.WaitGroup 管理 goroutine 生命周期；
用 sync.Map 存储并发查询结果（规避 map 并发安全问题）；
用 errChan 快速捕获 goroutine 中的错误，提前终止流程。
存储层解耦：所有数据库操作通过 store 接口代理，未来替换存储实现（如从 MySQL 改 MongoDB）时，业务逻辑层无需改动。


*/

package v1

import (
	"context"
	"regexp"
	"sync"

	// 外部依赖：同项目的 api 定义（用户资源的 DTO）
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	// 外部依赖：元数据规范（如 CreateOptions、ListOptions）
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	// 外部依赖：业务错误码封装
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	// 内部依赖：存储层工厂（对接数据库操作）
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store"
	// 内部依赖：错误码枚举
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	// 内部依赖：日志工具
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
)

// UserSrv 定义用户领域的核心操作契约，是用户业务逻辑的抽象接口。
// 所有用户相关的 CURD 及扩展操作（如密码修改）都需实现此接口。
type UserSrv interface {
	// 创建用户，支持传入创建选项（metav1.CreateOptions）
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error
	// 更新用户，支持传入更新选项（metav1.UpdateOptions）
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error
	// 删除单个用户，需指定用户名和删除选项
	Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error
	// 批量删除用户，支持传入用户名列表
	DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error
	// 查询单个用户，需指定用户名和查询选项
	Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error)
	// 高性能查询用户列表（并行优化）
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	// 低效查询用户列表（串行示例，用于性能对比）
	ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	// 修改用户密码（业务扩展操作）
	ChangePassword(ctx context.Context, user *v1.User) error
}

// userService 是 UserSrv 接口的实现类，依赖存储层工厂完成数据库交互。
type userService struct {
	store store.Factory // 存储层工厂，代理数据库操作（解耦存储实现）
}

// 编译时断言：确保 userService 实现了 UserSrv 接口
var _ UserSrv = (*userService)(nil)

// newUsers 初始化 userService，需传入 service 层依赖（此处通过 srv.store 注入）
func newUsers(srv *service) *userService {
	return &userService{store: srv.store}
}

// List 高性能查询用户列表：通过并行查询优化关联数据（用户-策略）的查询效率。
// 核心优化点：并行查询 + sync.Map 处理并发结果 + WaitGroup 管理 goroutine。
func (u *userService) List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	// 1. 从存储层查询用户列表
	users, err := u.store.Users().List(ctx, opts)
	if err != nil {
		log.L(ctx).Errorf("从存储层查询用户列表失败: %s", err.Error())
		// 封装业务错误码（数据库层错误）
		return nil, errors.WithCode(code.ErrDatabase, "%s", err.Error())
	}

	// 2. 初始化并发控制组件
	wg := sync.WaitGroup{}         // 管理 goroutine 等待
	errChan := make(chan error, 1) // 捕获 goroutine 中的错误
	finished := make(chan bool, 1) // 标记所有 goroutine 完成
	var m sync.Map                 // 并发安全的 map，存储查询结果

	// 3. 并行查询每个用户的关联策略（优化点：并行减少总耗时）
	for _, user := range users.Items {
		wg.Add(1) // 新增一个 goroutine 任务

		// 闭包传参：避免循环变量捕获问题
		go func(user *v1.User) {
			defer wg.Done() // 任务结束时标记完成

			// 模拟耗时操作：查询用户关联的策略列表
			policies, err := u.store.Policies().List(ctx, user.Name, metav1.ListOptions{})
			if err != nil {
				errChan <- errors.WithCode(code.ErrDatabase, "%s", err.Error()) // 发送错误到通道
				return
			}

			// 构造带策略信息的用户对象，存入 sync.Map
			m.Store(user.ID, &v1.User{
				ObjectMeta: metav1.ObjectMeta{
					ID:         user.ID,
					InstanceID: user.InstanceID,
					Name:       user.Name,
					Extend:     user.Extend,
					CreatedAt:  user.CreatedAt,
					UpdatedAt:  user.UpdatedAt,
				},
				Nickname:    user.Nickname,
				Email:       user.Email,
				Phone:       user.Phone,
				TotalPolicy: policies.TotalCount, // 补充策略统计信息
				LoginedAt:   user.LoginedAt,
			})
		}(user)
	}

	// 4. 等待所有 goroutine 完成，或捕获错误提前终止
	go func() {
		wg.Wait()       // 等待所有任务完成
		close(finished) // 标记完成
	}()

	// 5. 选择：要么完成，要么捕获错误
	select {
	case <-finished: // 正常完成
	case err := <-errChan: // 捕获到错误
		return nil, err
	}

	// 6. 整理结果：从 sync.Map 转换为切片
	infos := make([]*v1.User, 0, len(users.Items))
	for _, user := range users.Items {
		if info, ok := m.Load(user.ID); ok {
			infos = append(infos, info.(*v1.User))
		}
	}

	log.L(ctx).Debugf("从存储层查询到 %d 条用户数据（并行优化）", len(infos))
	return &v1.UserList{ListMeta: users.ListMeta, Items: infos}, nil
}

// ListWithBadPerformance 低效查询用户列表：串行查询关联数据，作为反面示例。
// 暴露问题：无并发优化时，关联查询会导致总耗时线性增加。
func (u *userService) ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	// 1. 查询用户列表（同 List 方法）
	users, err := u.store.Users().List(ctx, opts)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, "%s", err.Error())
	}

	infos := make([]*v1.User, 0)
	// 2. 串行查询每个用户的关联策略（无并发，总耗时 = 单个查询耗时 * 用户数）
	for _, user := range users.Items {
		policies, err := u.store.Policies().List(ctx, user.Name, metav1.ListOptions{})
		if err != nil {
			return nil, errors.WithCode(code.ErrDatabase, "%s", err.Error())
		}

		// 构造用户对象（补充策略信息）
		infos = append(infos, &v1.User{
			ObjectMeta: metav1.ObjectMeta{
				ID:        user.ID,
				Name:      user.Name,
				CreatedAt: user.CreatedAt,
				UpdatedAt: user.UpdatedAt,
			},
			Nickname:    user.Nickname,
			Email:       user.Email,
			Phone:       user.Phone,
			TotalPolicy: policies.TotalCount,
		})
	}

	return &v1.UserList{ListMeta: users.ListMeta, Items: infos}, nil
}

// Create 创建用户：调用存储层创建方法，处理“用户已存在”等业务错误。
func (u *userService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	err := u.store.Users().Create(ctx, user, opts)
	if err != nil {
		// 正则匹配“唯一键冲突”错误（用户已存在）
		if match, _ := regexp.MatchString("Duplicate entry '.*' for key 'idx_name'", err.Error()); match {
			return errors.WithCode(code.ErrUserAlreadyExist, "%s", err.Error())
		}
		return errors.WithCode(code.ErrDatabase, "%s", err.Error())
	}
	return nil
}

// DeleteCollection 批量删除用户：调用存储层批量删除方法。
func (u *userService) DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error {
	if err := u.store.Users().DeleteCollection(ctx, usernames, opts); err != nil {
		return errors.WithCode(code.ErrDatabase, "%s", err.Error())
	}
	return nil
}

// Delete 删除单个用户：调用存储层删除方法。
func (u *userService) Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error {
	if err := u.store.Users().Delete(ctx, username, opts); err != nil {
		return err
	}
	return nil
}

// Get 查询单个用户：调用存储层查询方法。
func (u *userService) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	user, err := u.store.Users().Get(ctx, username, opts)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// Update 更新用户信息：调用存储层更新方法。
func (u *userService) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error {
	if err := u.store.Users().Update(ctx, user, opts); err != nil {
		return errors.WithCode(code.ErrDatabase, "%s", err.Error())
	}
	return nil
}

// ChangePassword 修改用户密码：本质是调用存储层的更新方法（仅更新密码字段）。
func (u *userService) ChangePassword(ctx context.Context, user *v1.User) error {
	// 调用存储层更新方法，此处使用空更新选项（可根据需求扩展）
	if err := u.store.Users().Update(ctx, user, metav1.UpdateOptions{}); err != nil {
		return errors.WithCode(code.ErrDatabase, "%s", err.Error())
	}
	return nil
}
