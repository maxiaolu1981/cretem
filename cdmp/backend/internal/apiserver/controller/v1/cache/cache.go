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
package cache

import (
	"context"
	"fmt"
	"sync"

	pb "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
)

// Cache defines a cache service used to list all secrets and policies.
type Cache struct {
	store store.Factory
}

var (
	cacheServer *Cache
	once        sync.Once
)

// GetCacheInsOr return cache server instance with given factory.
func GetCacheInsOr(store store.Factory) (*Cache, error) {
	if store != nil {
		once.Do(func() {
			cacheServer = &Cache{store}
		})
	}

	if cacheServer == nil {
		return nil, fmt.Errorf("got nil cache server")
	}

	return cacheServer, nil
}

// ListSecrets returns all secrets.
func (c *Cache) ListSecrets(ctx context.Context, r *pb.ListSecretsRequest) (*pb.ListSecretsResponse, error) {
	log.L(ctx).Info("list secrets function called.")
	opts := metav1.ListOptions{
		Offset: r.Offset,
		Limit:  r.Limit,
	}

	secrets, err := c.store.Secrets().List(ctx, "", opts)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, err.Error())
	}

	items := make([]*pb.SecretInfo, 0)
	for _, secret := range secrets.Items {
		items = append(items, &pb.SecretInfo{
			SecretId:    secret.SecretID,
			Username:    secret.Username,
			SecretKey:   secret.SecretKey,
			Expires:     secret.Expires,
			Description: secret.Description,
			CreatedAt:   secret.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:   secret.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return &pb.ListSecretsResponse{
		TotalCount: secrets.TotalCount,
		Items:      items,
	}, nil
}

// ListPolicies returns all policies.
func (c *Cache) ListPolicies(ctx context.Context, r *pb.ListPoliciesRequest) (*pb.ListPoliciesResponse, error) {
	log.L(ctx).Info("list policies function called.")
	opts := metav1.ListOptions{
		Offset: r.Offset,
		Limit:  r.Limit,
	}

	policies, err := c.store.Policies().List(ctx, "", opts)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, err.Error())
	}

	items := make([]*pb.PolicyInfo, 0)
	for _, pol := range policies.Items {
		items = append(items, &pb.PolicyInfo{
			Name:         pol.Name,
			Username:     pol.Username,
			PolicyShadow: pol.PolicyShadow,
			CreatedAt:    pol.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return &pb.ListPoliciesResponse{
		TotalCount: policies.TotalCount,
		Items:      items,
	}, nil
}
