package middleware

import (
	"testing"
	"time"
)

func TestParseBearerToken(t *testing.T) {
	token, err := ParseBearerToken("Bearer test-token")
	if err != nil {
		t.Fatalf("parse bearer token: %v", err)
	}
	if token != "test-token" {
		t.Fatalf("expected test-token, got %q", token)
	}
}

func TestParseBearerTokenRejectsInvalidFormat(t *testing.T) {
	if _, err := ParseBearerToken("Basic token"); err == nil {
		t.Fatal("expected parse to fail for invalid format")
	}
}

func TestValidateToken(t *testing.T) {
	j := initJwt("secret")

	token, err := j.CreateToken("alice@example.com", "alice")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.Email != "alice@example.com" {
		t.Fatalf("expected alice@example.com, got %q", claims.Email)
	}
	if claims.Login != "alice" {
		t.Fatalf("expected alice, got %q", claims.Login)
	}
}

func TestCreateTokenExpiresInOneWeek(t *testing.T) {
	j := initJwt("secret")
	before := time.Now().Add(SessionDuration - time.Minute)

	token, err := j.CreateToken("alice@example.com", "alice")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected token expiration")
	}

	after := time.Now().Add(SessionDuration + time.Minute)
	if claims.ExpiresAt.Time.Before(before) || claims.ExpiresAt.Time.After(after) {
		t.Fatalf("expected expiration near one week, got %s", claims.ExpiresAt.Time)
	}
}
