package service

import (
	"context"
	"fmt"
	"github.com/pquerna/otp/totp"
	"strconv"
	"strings"
	"time"

	"social-network-go/auth-service/model"
	red "social-network-go/auth-service/redis"
	"social-network-go/exception"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxLoginAttempts = 5
	loginLockTTL     = 15 * time.Minute
	loginAttemptTTL  = 15 * time.Minute
)

// Redis login-attempt helpers
// Java equivalent: LoginAttemptRepository
func loginAttemptKey(accountID uuid.UUID) string {
	return fmt.Sprintf("auth:login_attempt:%s", accountID.String())
}

func loginLockKey(accountID uuid.UUID) string {
	return fmt.Sprintf("auth:lock:%s", accountID.String())
}

func resetVerifiedKey(email string, code uuid.UUID) string {
	return fmt.Sprintf("auth:reset_verified:%s:%s", email, code.String())
}

func (s *AuthService) getLockoutTime(ctx context.Context, accountID uuid.UUID) time.Duration {
	if red.RedisClient == nil {
		return 0
	}

	ttl, err := red.RedisClient.TTL(ctx, loginLockKey(accountID)).Result()
	if err != nil || ttl < 0 {
		return 0
	}
	return ttl
}

func (s *AuthService) validateLogin(ctx context.Context, account *model.Account) error {
	if !account.IsVerified {
		return exception.NewAppException(exception.AccountNotVerified)
	}

	if red.RedisClient == nil {
		return nil
	}

	// Check if user is suspended in Redis
	suspendedKey := fmt.Sprintf("auth:suspended:user:%s", account.ID.String())
	existsSuspended, err := red.RedisClient.Exists(ctx, suspendedKey).Result()
	if err == nil && existsSuspended > 0 {
		return fmt.Errorf("USER_SUSPENDED")
	}

	exists, err := red.RedisClient.Exists(ctx, loginLockKey(account.ID)).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return fmt.Errorf("ACCOUNT_LOCKED:time=%s", s.getLockoutTime(ctx, account.ID).String())
	}

	return nil
}

func (s *AuthService) processLoginFailed(ctx context.Context, accountID uuid.UUID) error {
	if red.RedisClient == nil {
		return exception.NewAppException(exception.AuthenticationFailed)
	}

	key := loginAttemptKey(accountID)
	failedAttempts, err := red.RedisClient.Incr(ctx, key).Result()
	if err != nil {
		return err
	}

	// Ensure the counter is temporary, same idea as Java lock window.
	if failedAttempts == 1 {
		_ = red.RedisClient.Expire(ctx, key, loginAttemptTTL).Err()
	}

	if failedAttempts >= maxLoginAttempts {
		if err := red.RedisClient.Set(ctx, loginLockKey(accountID), "1", loginLockTTL).Err(); err != nil {
			return err
		}
		_ = red.RedisClient.Del(ctx, key).Err()
		return fmt.Errorf("ACCOUNT_LOCKED:time=%s", loginLockTTL.String())
	}

	remainingAttempts := maxLoginAttempts - int(failedAttempts)
	return fmt.Errorf("AUTHENTICATION_FAILED:remainingAttempts=%d", remainingAttempts)
}

func (s *AuthService) processLoginSucceeded(ctx context.Context, accountID uuid.UUID) {
	if red.RedisClient == nil {
		return
	}
	_ = red.RedisClient.Del(ctx, loginAttemptKey(accountID), loginLockKey(accountID)).Err()
}

// Authentication service
// Java equivalent: AuthenticationServiceImpl
func (s *AuthService) Login(email, password, twoFactorCode string, isAdmin bool) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	role := "USER"
	if isAdmin {
		role = "ADMIN"
	}

	account, err := s.AccountRepo.FindByEmailAndRole(ctx, email, role)
	if err != nil {
		return "", "", exception.NewAppException(exception.AccountNotFound)
	}

	if err := s.validateLogin(ctx, account); err != nil {
		return "", "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(password)); err != nil {
		return "", "", s.processLoginFailed(ctx, account.ID)
	}

	s.processLoginSucceeded(ctx, account.ID)

	if account.IsTwoFactorEnabled {
		if twoFactorCode == "" {
			return "", "", exception.NewAppException(exception.NewAppError(401, "2FA_REQUIRED", "Two-factor authentication is required"))
		}
		valid := totp.Validate(twoFactorCode, account.TwoFactorSecret)
		if !valid {
			return "", "", exception.NewAppException(exception.NewAppError(401, "INVALID_2FA_CODE", "Invalid 2FA code"))
		}
	}

	accessToken, err := s.GenerateToken(account.ID.String(), account.Email, account.Role, s.Cfg.JWTSecret, s.Cfg.AccessTokenDuration)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := s.GenerateToken(account.ID.String(), account.Email, account.Role, s.Cfg.JWTRefreshSecret, s.Cfg.RefreshTokenDuration)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func (s *AuthService) RefreshToken(tokenStr string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if red.RedisClient != nil {
		blacklisted, _ := red.RedisClient.Exists(ctx, tokenStr).Result()
		if blacklisted > 0 {
			return "", exception.NewAppException(exception.Unauthorized)
		}
	}

	claims := &TokenClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(s.Cfg.JWTRefreshSecret), nil
	})
	if err != nil || !token.Valid {
		return "", exception.NewAppException(exception.InvalidOrExpiredRefreshToken)
	}

	accountID, err := uuid.Parse(claims.UserId)
	if err != nil {
		return "", exception.NewAppException(exception.AccountNotFound)
	}
	account, err := s.AccountRepo.FindByID(ctx, accountID)
	if err != nil {
		return "", exception.NewAppException(exception.AccountNotFound)
	}

	return s.GenerateToken(account.ID.String(), account.Email, account.Role, s.Cfg.JWTSecret, s.Cfg.AccessTokenDuration)
}

func (s *AuthService) Logout(refreshToken string) error {
	if refreshToken == "" || red.RedisClient == nil {
		return nil
	}

	claims := &TokenClaims{}
	token, _, _ := new(jwt.Parser).ParseUnverified(refreshToken, claims)
	if token == nil || claims.ExpiresAt == nil {
		return nil
	}

	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return red.RedisClient.Set(ctx, refreshToken, "blacklisted", ttl).Err()
}

// ParseAuthError is optional: use this in controller if you want to convert error string
// into JSON fields like Java: { remainingAttempts: n } or { time: "15m0s" }.
func ParseAuthError(err error) (code string, meta map[string]any) {
	if err == nil {
		return "", nil
	}

	msg := err.Error()
	meta = map[string]any{}

	if strings.HasPrefix(msg, "AUTHENTICATION_FAILED:remainingAttempts=") {
		value := strings.TrimPrefix(msg, "AUTHENTICATION_FAILED:remainingAttempts=")
		remaining, _ := strconv.Atoi(value)
		meta["remainingAttempts"] = remaining
		return "AUTHENTICATION_FAILED", meta
	}

	if strings.HasPrefix(msg, "ACCOUNT_LOCKED:time=") {
		meta["time"] = strings.TrimPrefix(msg, "ACCOUNT_LOCKED:time=")
		return "ACCOUNT_LOCKED", meta
	}

	return msg, meta
}
