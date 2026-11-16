package token

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID int64 `json:"uid"`
	// Ver: users.token_version lúc cấp. Đổi mật khẩu / bị khoá thì tăng, token cũ hết hiệu lực.
	Ver int `json:"ver"`
	jwt.RegisteredClaims
}

// Generate signs an HS256 access token for the given user.
func Generate(secret string, userID int64, ver int, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Ver:    ver,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Parse validates the token signature and expiry and returns its claims.
func Parse(secret, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	if claims.UserID <= 0 {
		return nil, errors.New("token missing user id")
	}
	return claims, nil
}
