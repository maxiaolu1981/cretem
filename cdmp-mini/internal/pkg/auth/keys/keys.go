package keys

import (
	"fmt"
	"strings"
)

const (
	// GenericPrefix is the fixed prefix applied to all authentication-related keys to avoid collisions.
	GenericPrefix = "genericapiserver:"

	refreshTokenPrefix = "auth:refresh_token:"
	userSessionsPrefix = "auth:user_sessions:"
	loginFailPrefix    = "auth:login_fail:"
	hashTagTemplate    = "{auth:user:%s}"
	defaultBlacklist   = "auth:blacklist:"
)

func normalizeUserID(userID string) string {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return "anonymous"
	}
	return trimmed
}

// UserHashTag returns the hash tag portion that should be embedded in every key tied to the provided user ID.
func UserHashTag(userID string) string {
	return fmt.Sprintf(hashTagTemplate, normalizeUserID(userID))
}

// RefreshTokenPrefix builds the common prefix for all refresh-token keys bound to the specified user.
// The returned value always ends with a trailing colon so callers can append the raw refresh token directly.
func RefreshTokenPrefix(userID string) string {
	return GenericPrefix + refreshTokenPrefix + UserHashTag(userID) + ":"
}

// RefreshTokenKey returns the fully-qualified Redis key for the refresh token of the given user.
func RefreshTokenKey(userID, refreshToken string) string {
	return RefreshTokenPrefix(userID) + refreshToken
}

// UserSessionsKey returns the Redis key for the set containing all refresh tokens that belong to the user.
func UserSessionsKey(userID string) string {
	return GenericPrefix + userSessionsPrefix + UserHashTag(userID)
}

// LoginFailKey builds the Redis key that stores login failure counters for the provided username.
func LoginFailKey(username string) string {
	trimmed := strings.TrimSpace(username)
	if trimmed == "" {
		trimmed = "anonymous"
	}
	return GenericPrefix + loginFailPrefix + trimmed
}

func blacklistBase(prefix string) string {
	base := strings.TrimSpace(prefix)
	if base == "" {
		base = defaultBlacklist
	}
	// ensure a trailing colon for further concatenation
	if !strings.HasSuffix(base, ":") {
		base += ":"
	}
	if !strings.HasPrefix(base, GenericPrefix) {
		base = GenericPrefix + base
	}
	return base
}

// BlacklistPrefix returns the prefix (ending with a colon) for blacklist entries tied to a user.
func BlacklistPrefix(prefix string, userID string) string {
	return blacklistBase(prefix) + UserHashTag(userID) + ":"
}

// BlacklistKey builds the Redis key used to store blacklist entries by token ID (jti).
func BlacklistKey(prefix string, userID, jti string) string {
	return BlacklistPrefix(prefix, userID) + jti
}
