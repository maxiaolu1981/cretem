// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package options

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"

	"github.com/spf13/pflag"
)

// JwtOptions contains configuration items related to API server features.
type JwtOptions struct {
	Realm      string        `json:"realm"       mapstructure:"realm"`
	Key        string        `json:"key"         mapstructure:"key"`
	Timeout    time.Duration `json:"timeout"     mapstructure:"timeout"`
	MaxRefresh time.Duration `json:"max-refresh" mapstructure:"max-refresh"`
}

func NewJwtOptions() *JwtOptions {
	defaults := server.NewConfig()
	return &JwtOptions{
		Realm:      defaults.Jwt.Realm,
		Key:        defaults.Jwt.Key,
		Timeout:    defaults.Jwt.Timeout,
		MaxRefresh: defaults.Jwt.MaxRefresh,
	}
}

func (j *JwtOptions) AddFlags(fs *pflag.FlagSet) {
	if fs == nil {
		return
	}
	fs.StringVar(&j.Realm, "jwt.realm", j.Realm, "显示给用户的领域名称.")
	fs.StringVar(&j.Key, "jwt.key", j.Key, "用于签署 JWT令牌的私钥。")
	fs.DurationVar(&j.Timeout, "jwt.timeout", j.Timeout, "JWT token超时时间")
	fs.DurationVar(&j.MaxRefresh, "jwt.max-refresh", j.MaxRefresh, "令牌可刷新的最长窗口期")
}

func (j *JwtOptions) Validate() []error {
	const (
		secretKeyMinLen = 6
		secretKeyMaxLen = 32
	)

	var errs []error
	realm := strings.TrimSpace(j.Realm)
	key := strings.TrimSpace(j.Key)
	if realm == "" {
		errs = append(errs, fmt.Errorf("领域名称(realm)不能为空或者包含空格."))
	}

	if j.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("JWT token超时时间必须大于0"))
	}

	if !govalidator.StringLength(key, strconv.Itoa(secretKeyMinLen), strconv.Itoa(secretKeyMaxLen)) {
		errs = append(errs, fmt.Errorf("key最小的长度必须%d-%d之间,当前长度%d", secretKeyMinLen, secretKeyMaxLen, len(key)))
	}

	if j.MaxRefresh <= 0 {
		errs = append(errs, fmt.Errorf("令牌可刷新的最长窗口期必须大于0"))
	}
	if j.MaxRefresh < j.Timeout {
		errs = append(errs, fmt.Errorf(
			"令牌最大刷新期限（%v）不能小于超时时间（%v）",
			j.MaxRefresh, j.Timeout,
		))
	}
	return errs
}
