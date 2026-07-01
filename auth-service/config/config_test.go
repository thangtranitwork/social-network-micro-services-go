package config

import "testing"

func TestLoadConfigPanicsWhenProductionSecretsUseFallback(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_TOKEN_KEY", "")
	t.Setenv("JWT_REFRESH_TOKEN_KEY", "real-refresh-secret")

	defer func() {
		if recover() == nil {
			t.Fatal("expected LoadConfig to panic when production access token secret is missing")
		}
	}()

	_ = LoadConfig()
}

func TestLoadConfigAllowsExplicitProductionSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_TOKEN_KEY", "real-access-secret")
	t.Setenv("JWT_REFRESH_TOKEN_KEY", "real-refresh-secret")

	cfg := LoadConfig()
	if cfg.JWTSecret != "real-access-secret" {
		t.Fatalf("unexpected access secret: %q", cfg.JWTSecret)
	}
	if cfg.JWTRefreshSecret != "real-refresh-secret" {
		t.Fatalf("unexpected refresh secret: %q", cfg.JWTRefreshSecret)
	}
}
