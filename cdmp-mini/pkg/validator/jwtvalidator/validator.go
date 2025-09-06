package jwtvalidator

import (
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/spf13/viper"
)

// CustomClaims 自定义JWT声明结构（与你的业务令牌字段匹配）
type CustomClaims struct {
	UserID               string `json:"user_id"`  // 业务必需的用户ID
	Username             string `json:"username"` // 可选：用户名
	Role                 string `json:"role"`     // 可选：用户角色
	jwt.RegisteredClaims        // JWT标准声明（包含过期时间等）
}

// ValidateToken 校验JWT令牌有效性（完全适配你的withCode错误体系）
// 返回值：
// - 校验通过：返回*CustomClaims和nil
// - 校验失败：返回nil和*errors.withCode类型错误（含业务码和堆栈）
func ValidateToken(tokenString string) (*CustomClaims, error) {
	var jwtSecret = []byte(viper.GetString("jwt.key"))
	// 1. 校验令牌为空场景
	if tokenString == "" {
		log.Errorf("令牌校验失败：缺少Authorization头")
		return nil, errors.WithCode(
			code.ErrMissingHeader, // 100205（与你的Unauthorized逻辑匹配）
			"Authorization header is not present",
		)
	}

	// 2. 处理Bearer前缀并校验格式
	tokenString = trimBearerPrefix(tokenString)
	if tokenString == "" {
		log.Errorf("令牌校验失败：授权头格式错误（无有效令牌内容）")
		return nil, errors.WithCode(
			code.ErrInvalidAuthHeader, // 100204（授权头格式无效）
			"invalid authorization header format",
		)
	}

	// 3. 解析令牌并校验签名算法
	var claims CustomClaims
	token, err := jwt.ParseWithClaims(
		tokenString,
		&claims	// 校验签名算法是否为预期的HMAC类型
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				errMsg := "unsupported signing method: %v"
				log.Errorf(errMsg, token.Header["alg"])
				return nil, errors.WithCode(
					code.ErrTokenInvalid, // 100208（令牌无效）
					errMsg, token.Header["alg"],
				)
			}

			return jwtSecret, nil
		},
	)

	// 4. 处理解析错误（映射为对应的业务码）
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			// 令牌过期 → code.ErrExpired（100203）
			log.Errorf("令牌校验失败：已过期")
			return nil, errors.WithCode(
				code.ErrExpired,
				"token is expired",
			)
		case errors.Is(err, jwt.ErrSignatureInvalid):
			// 签名无效 → code.ErrSignatureInvalid（100202）
			log.Errorf("令牌校验失败：签名无效")
			return nil, errors.WithCode(
				code.ErrSignatureInvalid,
				"signature is invalid",
			)
		case strings.Contains(errors.GetMessage(err), "invalid number of segments"):
			return nil, errors.WithCode(code.ErrTokenInvalid, "令牌格式错误")

		case strings.HasPrefix(err.Error(), "illegal base64 data"):
			// Base64解码失败 → code.ErrTokenInvalid（100208）
			log.Errorf("令牌校验失败：%v", err)
			return nil, errors.WithCode(
				code.ErrTokenInvalid,
				err.Error(),
			)
		default:
			// 其他解析错误 → code.ErrUnauthorized（110003）
			log.Errorf("令牌校验失败：%v", err)
			return nil, errors.WithCode(
				code.ErrUnauthorized,
				"authentication failed: %v", err,
			)
		}
	}

	// 5. 校验令牌有效性并提取声明
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		// 6. 业务规则校验（扩展你的自定义校验逻辑）
		if err := validateCustomClaims(claims); err != nil {
			return nil, err // 已包装为withCode错误
		}
		return claims, nil
	}

	// 7. 令牌无效（未通过基础校验）
	log.Errorf("令牌校验失败：无效的令牌内容")
	return nil, errors.WithCode(
		code.ErrTokenInvalid, // 100208
		"token is invalid",
	)
}

// 辅助函数：移除Bearer前缀（兼容大小写）
func trimBearerPrefix(token string) string {
	if len(token) >= 7 && strings.ToLower(token[:7]) == "bearer " {
		return token[7:]
	}
	return token
}

// 辅助函数：业务规则校验（返回withCode错误）
func validateCustomClaims(claims *CustomClaims) error {
	// 校验用户ID是否存在
	if claims.UserID == "" {
		log.Errorf("令牌校验失败：缺少user_id字段")
		return errors.WithCode(
			code.ErrTokenInvalid, // 100208
			"token missing user_id",
		)
	}

	// 校验角色合法性（根据你的业务角色列表调整）
	validRoles := map[string]bool{"user": true, "admin": true, "moderator": true}
	if !validRoles[claims.Role] {
		log.Errorf("令牌校验失败：无效角色 %s", claims.Role)
		return errors.WithCode(
			code.ErrTokenInvalid, // 100208
			"invalid role in token: %s", claims.Role,
		)
	}

	// 校验令牌签发时间（防止过于老旧的令牌）
	if claims.IssuedAt != nil && claims.IssuedAt.Before(time.Now().Add(-7*24*time.Hour)) {
		log.Errorf("令牌校验失败：签发时间过早（超过7天）")
		return errors.WithCode(
			code.ErrExpired, // 100203（视为过期类错误）
			"token issued too early (more than 7 days ago)",
		)
	}

	return nil
}
