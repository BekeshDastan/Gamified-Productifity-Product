package auth

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/golang-jwt/jwt/v5"
)

func TestSessionConfigurationFailsClosed(t *testing.T) {
	service := NewService(nil, NewPasswordHasher(), platform.RealClock{})
	for _, config := range []SessionConfig{
		{Origin: "https://ascend.example", Environment: "production"},
		{Origin: "http://localhost:3000", Environment: "production", InsecureLocalCookies: true, SigningKey: make([]byte, 32)},
		{Origin: "http://other.example", Environment: "development", InsecureLocalCookies: true, SigningKey: make([]byte, 32)},
		{Origin: "http://localhost:3000", Environment: "development", SigningKey: make([]byte, 32)},
		{Origin: "https://ascend.example/path", Environment: "production", SigningKey: make([]byte, 32)},
	} {
		if _, err := NewSessions(context.Background(), service, config); err == nil {
			t.Fatal("unsafe auth configuration accepted")
		}
	}
}

func TestRejectJWTBeforeDatabaseForInvalidClaimsOrAlgorithm(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	clock := platform.FixedClock{Time: now}
	sessions := &Sessions{service: NewService(nil, NewPasswordHasher(), clock), config: SessionConfig{Origin: "https://ascend.example", SigningKey: bytes.Repeat([]byte{5}, 32)}}
	base := accessClaims{SessionID: "819da328-8d82-4d4d-8c22-bc12bf065c02", RegisteredClaims: jwt.RegisteredClaims{Subject: "5302da50-70be-426e-9d6d-f1367625d545", Issuer: sessions.config.Origin, Audience: jwt.ClaimStrings{"ascend-web"}, ID: randomOpaque(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(accessLifetime))}}
	for _, name := range []string{"expired", "no-exp", "no-iat", "future-iat", "issuer", "audience", "no-sid", "no-sub", "no-jti", "long-exp", "wrong-alg", "wrong-key"} {
		t.Run(name, func(t *testing.T) {
			claims := base
			method := jwt.SigningMethodHS256
			key := sessions.config.SigningKey
			switch name {
			case "expired":
				claims.ExpiresAt = jwt.NewNumericDate(now)
			case "no-exp":
				claims.ExpiresAt = nil
			case "no-iat":
				claims.IssuedAt = nil
			case "future-iat":
				claims.IssuedAt = jwt.NewNumericDate(now.Add(time.Minute))
			case "issuer":
				claims.Issuer = "https://other.example"
			case "audience":
				claims.Audience = jwt.ClaimStrings{"other"}
			case "no-sid":
				claims.SessionID = ""
			case "no-sub":
				claims.Subject = ""
			case "no-jti":
				claims.ID = ""
			case "long-exp":
				claims.ExpiresAt = jwt.NewNumericDate(now.Add(time.Hour))
			case "wrong-alg":
				method = jwt.SigningMethodHS512
			case "wrong-key":
				key = bytes.Repeat([]byte{6}, 32)
			}
			token, err := jwt.NewWithClaims(method, claims).SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := sessions.Authenticate(context.Background(), token); err == nil {
				t.Fatal("invalid JWT accepted")
			}
		})
	}
}
