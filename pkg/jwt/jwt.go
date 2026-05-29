package jwt

import (
	"errors"
	"time"

	golangjwt "github.com/golang-jwt/jwt/v5"
	"gobench/pkg/config"
)

type Claims struct {
	UserID uint `json:"user_id"`
	golangjwt.RegisteredClaims
}

func GenerateToken(userID uint) (string, error) {
	if config.AppConfig == nil {
		return "", errors.New("config not initialized")
	}

	claims := Claims{
		UserID: userID,
		RegisteredClaims: golangjwt.RegisteredClaims{
			ExpiresAt: golangjwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 1 day expiration
			IssuedAt:  golangjwt.NewNumericDate(time.Now()),
		},
	}

	token := golangjwt.NewWithClaims(golangjwt.SigningMethodHS256, claims)
	secret := []byte(config.AppConfig.JWT.Secret)
	return token.SignedString(secret)
}

func ParseToken(tokenString string) (*Claims, error) {
	if config.AppConfig == nil {
		return nil, errors.New("config not initialized")
	}

	secret := []byte(config.AppConfig.JWT.Secret)

	token, err := golangjwt.ParseWithClaims(tokenString, &Claims{}, func(token *golangjwt.Token) (interface{}, error) {
		return secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
