package auth

import (
	"time"

	"github.com/dgrijalva/jwt-go"
	"golang.org/x/crypto/bcrypt"
)

// Encrypt encrypts the plain text with bcrypt.
func Encrypt(source string) (string, error) {
	return EncryptWithCost(source, 0)
}

// EncryptWithCost encrypts the plain text with bcrypt using a configurable cost.
// When cost <= 0 it falls back to the library default, and values outside the
// supported range are clamped to ensure GenerateFromPassword succeeds.
func EncryptWithCost(source string, cost int) (string, error) {
	switch {
	case cost <= 0:
		cost = bcrypt.DefaultCost
	case cost < bcrypt.MinCost:
		cost = bcrypt.MinCost
	case cost > bcrypt.MaxCost:
		cost = bcrypt.MaxCost
	}
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(source), cost)
	return string(hashedBytes), err
}

// Compare compares the encrypted text with the plain text if it's the same.
func Compare(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// Sign issue a jwt token based on secretID, secretKey, iss and aud.
func Sign(secretID string, secretKey string, iss, aud string) string {
	claims := jwt.MapClaims{
		"exp": time.Now().Add(time.Minute).Unix(),
		"iat": time.Now().Unix(),
		"nbf": time.Now().Add(0).Unix(),
		"aud": aud,
		"iss": iss,
	}

	// create a new token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = secretID

	// Sign the token with the specified secret.
	tokenString, _ := token.SignedString([]byte(secretKey))

	return tokenString
}
