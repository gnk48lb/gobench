package jwt

import (
	"os"
	"testing"
	"time"

	"gobench/pkg/config"
)

// TestMain 在所有测试运行前初始化依赖，这里只需要 JWT secret
func TestMain(m *testing.M) {
	config.AppConfig = &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret-key-for-unit-tests"},
	}
	os.Exit(m.Run())
}

func TestGenerateAndParseToken(t *testing.T) {
	tests := []struct {
		name    string
		userID  uint
		wantErr bool
	}{
		{name: "normal user", userID: 1, wantErr: false},
		{name: "large user id", userID: 99999, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GenerateToken(tt.userID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateToken() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			claims, err := ParseToken(token)
			if err != nil {
				t.Fatalf("ParseToken() error = %v", err)
			}
			if claims.UserID != tt.userID {
				t.Errorf("claims.UserID = %v, want %v", claims.UserID, tt.userID)
			}
		})
	}
}

func TestParseToken_InvalidToken(t *testing.T) {
	_, err := ParseToken("this.is.not.a.valid.token")
	if err == nil {
		t.Error("expected error for invalid token, got nil")
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	// 用正确 secret 生成 token
	token, err := GenerateToken(1)
	if err != nil {
		t.Fatal(err)
	}

	// 换成错误 secret 再解析，应该失败
	config.AppConfig.JWT.Secret = "wrong-secret"
	defer func() {
		config.AppConfig.JWT.Secret = "test-secret-key-for-unit-tests"
	}()

	_, err = ParseToken(token)
	if err == nil {
		t.Error("expected error with wrong secret, got nil")
	}
}

func TestGenerateToken_ExpiryIsInFuture(t *testing.T) {
	token, err := GenerateToken(42)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if !claims.ExpiresAt.After(time.Now()) {
		t.Error("token should expire in the future")
	}
}
