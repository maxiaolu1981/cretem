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
/*
cache 包实现了一个缓存服务（Cache），主要功能是通过存储层（store.Factory）获取并返回密钥（Secret）和策略（Policy）的列表数据。它采用单例模式确保服务实例唯一，提供了统一的接口用于查询所有密钥和策略信息，并处理数据转换与错误封装。
核心流程
初始化缓存服务
通过 GetCacheInsOr 函数，基于存储层工厂（store.Factory）创建缓存服务单例（Cache）：
使用 sync.Once 保证服务只初始化一次（线程安全）；
缓存服务内部持有存储层实例，用于后续数据查询。
查询密钥列表（ListSecrets）
接收客户端请求（ListSecretsRequest），解析分页参数（偏移量 Offset、每页条数 Limit）；
调用存储层的密钥查询接口（store.Secrets().List）获取数据库中的密钥数据；
将数据库模型转换为 API 响应模型（SecretInfo），格式化时间字段；
封装总条数和列表数据，返回响应（ListSecretsResponse），若出错则封装数据库错误码。
查询策略列表（ListPolicies）
流程与 ListSecrets 类似，接收策略查询请求（ListPoliciesRequest）；
调用存储层的策略查询接口（store.Policies().List）获取数据；
转换数据模型为 API 响应模型（PolicyInfo），返回包含总条数和列表的响应（ListPoliciesResponse）
*/
package cache

import (
	"context"
	"fmt"
	"sync"

	pb "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"           // 自定义库：API 协议定义（请求/响应结构）
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1" // 自定义库：元数据（分页选项等）
	"github.com/maxiaolu1981/cretem/nexuscore/errors"                        // 自定义库：错误处理工具

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store" // 自定义库：存储层接口
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"                 // 自定义库：错误码定义
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"                  // 自定义库：日志工具
)

// Cache 定义了一个缓存服务，用于查询所有密钥（secrets）和策略（policies）
type Cache struct {
	store store.Factory // 存储层工厂，用于获取底层数据
	pb.UnimplementedCacheServer
}

var (
	cacheServer *Cache    // 缓存服务单例实例
	once        sync.Once // 确保初始化只执行一次（单例模式）
)

// GetCacheInsOr 基于存储层工厂创建或获取已有的缓存服务实例（单例）
func GetCacheInsOr(store store.Factory) (*Cache, error) {
	if store != nil {
		// 确保缓存服务只初始化一次（线程安全）
		once.Do(func() {
			cacheServer = &Cache{store: store}
		})
	}

	// 若缓存服务未初始化成功，返回错误
	if cacheServer == nil {
		return nil, fmt.Errorf("获取到空的缓存服务实例")
	}

	return cacheServer, nil
}

// ListSecrets 返回所有密钥列表
func (c *Cache) ListSecrets(ctx context.Context, r *pb.ListSecretsRequest) (*pb.ListSecretsResponse, error) {
	log.L(ctx).Info("调用了查询密钥列表的函数")

	// 构建分页查询选项（从请求中提取偏移量和每页条数）
	opts := metav1.ListOptions{
		Offset: r.Offset,
		Limit:  r.Limit,
	}

	// 从存储层查询密钥列表
	secrets, err := c.store.Secrets().List(ctx, "", opts)
	if err != nil {
		// 封装数据库错误（附加错误码）
		return nil, errors.WithCode(code.ErrDatabase, err.Error())
	}

	// 将数据库模型转换为 API 响应模型
	items := make([]*pb.SecretInfo, 0)
	for _, secret := range secrets.Items {
		items = append(items, &pb.SecretInfo{
			SecretId:    secret.SecretID,                                // 密钥ID
			Username:    secret.Username,                                // 用户名
			SecretKey:   secret.SecretKey,                               // 密钥内容
			Expires:     secret.Expires,                                 // 过期时间
			Description: secret.Description,                             // 描述信息
			CreatedAt:   secret.CreatedAt.Format("2006-01-02 15:04:05"), // 创建时间（格式化）
			UpdatedAt:   secret.UpdatedAt.Format("2006-01-02 15:04:05"), // 更新时间（格式化）
		})
	}

	// 返回包含总条数和列表数据的响应
	return &pb.ListSecretsResponse{
		TotalCount: secrets.TotalCount, // 总条数
		Items:      items,              // 密钥列表
	}, nil
}

// ListPolicies 返回所有策略列表
func (c *Cache) ListPolicies(ctx context.Context, r *pb.ListPoliciesRequest) (*pb.ListPoliciesResponse, error) {
	log.L(ctx).Info("调用了查询策略列表的函数")

	// 构建分页查询选项
	opts := metav1.ListOptions{
		Offset: r.Offset,
		Limit:  r.Limit,
	}

	// 从存储层查询策略列表
	policies, err := c.store.Policies().List(ctx, "", opts)
	if err != nil {
		// 封装数据库错误（附加错误码）
		return nil, errors.WithCode(code.ErrDatabase, err.Error())
	}

	// 将数据库模型转换为 API 响应模型
	items := make([]*pb.PolicyInfo, 0)
	for _, pol := range policies.Items {
		items = append(items, &pb.PolicyInfo{
			Name:         pol.Name,                                    // 策略名称
			Username:     pol.Username,                                // 用户名
			PolicyShadow: pol.PolicyShadow,                            // 策略内容
			CreatedAt:    pol.CreatedAt.Format("2006-01-02 15:04:05"), // 创建时间（格式化）
		})
	}

	// 返回包含总条数和列表数据的响应
	return &pb.ListPoliciesResponse{
		TotalCount: policies.TotalCount, // 总条数
		Items:      items,               // 策略列表
	}, nil
}
