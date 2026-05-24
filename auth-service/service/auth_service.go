package service

import (
	"context"
	"fmt"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"social-network-go/auth-service/config"
	"social-network-go/auth-service/db"
	"social-network-go/auth-service/model"
	red "social-network-go/auth-service/redis"
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/pb"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxLoginAttempts = 5
	loginLockTTL     = 15 * time.Minute
	loginAttemptTTL  = 15 * time.Minute
)

type AuthService struct {
	Cfg         *config.Config
	KafkaWriter *kafka.Writer
	UserClient  pb.UserServiceClient
}

type TokenClaims struct {
	UserId   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Scope    string `json:"scope"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func NewAuthService(cfg *config.Config, userClient pb.UserServiceClient) *AuthService {
	writer := &kafka.Writer{
		Addr:     kafka.TCP(cfg.KafkaAddr),
		Topic:    "user-events",
		Balancer: &kafka.LeastBytes{},
	}
	return &AuthService{
		Cfg:         cfg,
		KafkaWriter: writer,
		UserClient:  userClient,
	}
}

// ==============================
// Common helpers
// ==============================

func (s *AuthService) sendHTMLEmail(to, subject, htmlBody string) {
	if s.Cfg.Mail == "" || s.Cfg.MailPassword == "" {
		logger.Warn("Mail configuration missing. Cannot send real email.")
		return
	}

	auth := smtp.PlainAuth("", s.Cfg.Mail, s.Cfg.MailPassword, "smtp.gmail.com")
	msg := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-version: 1.0;\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\";\r\n" +
		"\r\n" +
		htmlBody + "\r\n")

	if err := smtp.SendMail("smtp.gmail.com:587", auth, s.Cfg.Mail, []string{to}, msg); err != nil {
		logger.Field("to", to).Field("error", err).Error("Failed to send email")
		return
	}
	logger.Field("to", to).Info("Successfully sent email")
}

func getTemplateContent(fileName string) ([]byte, error) {
	// 1. Try env variable first
	if envDir := os.Getenv("EMAIL_TEMPLATES_DIR"); envDir != "" {
		path := filepath.Join(envDir, fileName)
		if content, err := os.ReadFile(path); err == nil {
			return content, nil
		}
	}
	// 2. Try relative to root
	path1 := filepath.Join("auth-service", "templates", fileName)
	if content, err := os.ReadFile(path1); err == nil {
		return content, nil
	}
	// 3. Try relative to active folder
	path2 := filepath.Join("templates", fileName)
	if content, err := os.ReadFile(path2); err == nil {
		return content, nil
	}
	return nil, fmt.Errorf("template not found: %s", fileName)
}

func (s *AuthService) getRegisterEmailHTML(verificationLink string) string {
	content, err := getTemplateContent("register-verify-email.html")
	if err == nil {
		html := string(content)
		html = strings.ReplaceAll(html, `th:text="${title}">Confirm Your Email`, `>Confirm Your Email`)
		html = strings.ReplaceAll(html, `th:href="${verificationLink}"`, `href="`+verificationLink+`"`)
		return html
	}
	return "<h1>Confirm Email</h1><p>Click here: <a href=\"" + verificationLink + "\">Verify</a></p>"
}

func (s *AuthService) getResetPasswordEmailHTML(resetLink string) string {
	content, err := getTemplateContent("update-password-email.html")
	if err == nil {
		html := string(content)
		html = strings.ReplaceAll(html, `th:href="${resetLink}"`, `href="`+resetLink+`"`)
		return html
	}
	return "<h1>Reset Password</h1><p>Click here: <a href=\"" + resetLink + "\">Reset</a></p>"
}

func (s *AuthService) publishKafkaEvent(key, payload string) {
	if s.KafkaWriter == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := s.KafkaWriter.WriteMessages(ctx, kafka.Message{
			Key:   []byte(key),
			Value: []byte(payload),
		}); err != nil {
			logger.Field("key", key).Field("error", err).Warn("Kafka failed to publish event")
		}
	}()
}

// ==============================
// Redis login-attempt helpers
// Java equivalent: LoginAttemptRepository
// ==============================

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

func (s *AuthService) validateLogin(ctx context.Context, account model.Account) error {
	if !account.IsVerified {
		return exception.NewAppException(exception.AccountNotVerified)
	}

	if red.RedisClient == nil {
		return nil
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

// ==============================
// Register service
// Java equivalent: RegisterServiceImpl
// ==============================

func (s *AuthService) Register(email, password, givenName, familyName, birthdate string) (*model.VerifyCode, error) {
	var existing model.Account
	if err := db.DB.Where("email = ?", email).First(&existing).Error; err == nil {
		if existing.IsVerified {
			return nil, exception.NewAppException(exception.AccountAlreadyExists)
		}
		return nil, exception.NewAppException(exception.EmailNotVerified)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	accountID := uuid.New()
	verifyCode := model.VerifyCode{
		Code:       uuid.New(),
		AccountID:  accountID,
		Verified:   false,
		ExpiryTime: time.Now().Add(s.Cfg.VerifyEmailDuration),
	}
	account := model.Account{
		ID:         accountID,
		Email:      email,
		Password:   string(hashedPassword),
		Role:       "USER",
		IsVerified: false,
		CreatedAt:  time.Now(),
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	if err := tx.Create(&account).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Create(&verifyCode).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	verificationLink := fmt.Sprintf("%s/register?email=%s&code=%s", s.Cfg.FrontendURL, email, verifyCode.Code.String())
	go s.sendHTMLEmail(email, "Email Verification", s.getRegisterEmailHTML(verificationLink))

	eventPayload := fmt.Sprintf(
		`{"event":"AccountCreated","account_id":"%s","email":"%s","given_name":"%s","family_name":"%s","birthdate":"%s"}`,
		accountID, email, givenName, familyName, birthdate,
	)
	s.publishKafkaEvent(accountID.String(), eventPayload)

	return &verifyCode, nil
}

func (s *AuthService) Verify(email string, code uuid.UUID) error {
	var account model.Account
	if err := db.DB.Where("email = ?", email).First(&account).Error; err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}
	if account.IsVerified {
		return exception.NewAppException(exception.AccountVerified)
	}

	var verifyCode model.VerifyCode
	if err := db.DB.Where("account_id = ? AND code = ?", account.ID, code).First(&verifyCode).Error; err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}
	if verifyCode.Code != code { // defensive; normally Code == code is enough
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}
	if time.Now().After(verifyCode.ExpiryTime) {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Model(&model.Account{}).Where("id = ?", account.ID).Update("is_verified", true).Error; err != nil {
		tx.Rollback()
		return err
	}
	// Java removes verify code after successful register verify.
	if err := tx.Where("account_id = ?", account.ID).Delete(&model.VerifyCode{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	eventPayload := fmt.Sprintf(`{"event":"AccountVerified","account_id":"%s"}`, account.ID)
	s.publishKafkaEvent(account.ID.String(), eventPayload)

	return nil
}

func (s *AuthService) ResendEmail(email string) (*model.VerifyCode, error) {
	var account model.Account
	if err := db.DB.Where("email = ?", email).First(&account).Error; err != nil {
		return nil, exception.NewAppException(exception.AccountNotFound)
	}
	if account.IsVerified {
		return nil, exception.NewAppException(exception.AccountVerified)
	}

	var verifyCode model.VerifyCode
	if err := db.DB.Where("account_id = ? AND verified = ?", account.ID, false).
		Order("expiry_time DESC").
		First(&verifyCode).Error; err != nil {
		return nil, exception.NewAppException(exception.VerificationCodeNotFound)
	}

	verifyCode.ExpiryTime = time.Now().Add(s.Cfg.VerifyEmailDuration)
	if err := db.DB.Save(&verifyCode).Error; err != nil {
		return nil, err
	}

	verificationLink := fmt.Sprintf("%s/register?email=%s&code=%s", s.Cfg.FrontendURL, email, verifyCode.Code.String())
	go s.sendHTMLEmail(email, "Email Verification", s.getRegisterEmailHTML(verificationLink))

	return &verifyCode, nil
}

// ==============================
// Authentication service
// Java equivalent: AuthenticationServiceImpl
// ==============================

func (s *AuthService) Login(email, password string, isAdmin bool) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	role := "USER"
	if isAdmin {
		role = "ADMIN"
	}

	var account model.Account
	if err := db.DB.Where("email = ? AND role = ?", email, role).First(&account).Error; err != nil {
		return "", "", exception.NewAppException(exception.AccountNotFound)
	}

	if err := s.validateLogin(ctx, account); err != nil {
		return "", "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(password)); err != nil {
		return "", "", s.processLoginFailed(ctx, account.ID)
	}

	s.processLoginSucceeded(ctx, account.ID)

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

	var account model.Account
	if err := db.DB.Where("id = ?", claims.UserId).First(&account).Error; err != nil {
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

func (s *AuthService) ValidateToken(tokenStr string) (*TokenClaims, error) {
	claims := &TokenClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(s.Cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, exception.NewAppException(exception.InvalidToken)
	}
	return claims, nil
}

func (s *AuthService) GenerateToken(userId, email, role string, secret string, duration time.Duration) (string, error) {
	username := ""
	cacheKey := fmt.Sprintf("user_info:%s", userId)

	if red.RedisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		cachedUsername, err := red.RedisClient.Get(ctx, cacheKey).Result()
		cancel()
		if err == nil && cachedUsername != "" {
			username = cachedUsername
		}
	}

	if username == "" && s.UserClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{UserId: userId})
		if err == nil && resp != nil && resp.Username != "" {
			username = resp.Username
			if red.RedisClient != nil {
				_ = red.RedisClient.Set(context.Background(), cacheKey, username, 24*time.Hour).Err()
			}
		} else {
			logger.Field("userId", userId).Field("error", err).Error("Failed to get username for user")
		}
	}

	claims := TokenClaims{
		UserId:   userId,
		Email:    email,
		Role:     role,
		Scope:    role,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Subject:   userId,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ==============================
// Update password service
// Java equivalent: UpdatePasswordServiceImpl
// Flow: send -> verify -> updatePassword
// ==============================

func (s *AuthService) ForgotPassword(email string) error {
	return s.ForgotPasswordWithContinueURL(email, s.Cfg.FrontendURL+"/reset-password")
}

func (s *AuthService) ForgotPasswordWithContinueURL(email, continueURL string) error {
	var account model.Account
	if err := db.DB.Where("email = ?", email).First(&account).Error; err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	var resetToken model.PasswordResetToken
	err := db.DB.Where("account_id = ? AND used = ?", account.ID, false).
		Order("expiry_time DESC").
		First(&resetToken).Error

	if err != nil {
		resetToken = model.PasswordResetToken{
			Code:       uuid.New(),
			AccountID:  account.ID,
			Used:       false,
			ExpiryTime: time.Now().Add(s.Cfg.PasswordResetDuration),
		}
		if err := db.DB.Create(&resetToken).Error; err != nil {
			return err
		}
	} else {
		resetToken.ExpiryTime = time.Now().Add(s.Cfg.PasswordResetDuration)
		if err := db.DB.Save(&resetToken).Error; err != nil {
			return err
		}
	}

	resetLink := fmt.Sprintf("%s?email=%s&code=%s", continueURL, email, resetToken.Code.String())
	go s.sendHTMLEmail(email, "Update Password Verification", s.getResetPasswordEmailHTML(resetLink))

	return nil
}

func (s *AuthService) VerifyResetPassword(email, codeStr string) error {
	code, err := uuid.Parse(codeStr)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}

	var account model.Account
	if err := db.DB.Where("email = ?", email).First(&account).Error; err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	var resetToken model.PasswordResetToken
	if err := db.DB.Where("account_id = ? AND code = ?", account.ID, code).First(&resetToken).Error; err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}
	if resetToken.Used || time.Now().After(resetToken.ExpiryTime) {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	if red.RedisClient != nil {
		ttl := time.Until(resetToken.ExpiryTime)
		if ttl > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return red.RedisClient.Set(ctx, resetVerifiedKey(email, code), "1", ttl).Err()
		}
	}

	return nil
}

func (s *AuthService) UpdatePassword(email, codeStr, newPassword string) error {
	code, err := uuid.Parse(codeStr)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}

	var account model.Account
	if err := db.DB.Where("email = ?", email).First(&account).Error; err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	var resetToken model.PasswordResetToken
	if err := db.DB.Where("account_id = ? AND code = ?", account.ID, code).First(&resetToken).Error; err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}
	if resetToken.Used || time.Now().After(resetToken.ExpiryTime) {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	if red.RedisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		exists, err := red.RedisClient.Exists(ctx, resetVerifiedKey(email, code)).Result()
		if err != nil {
			return err
		}
		if exists == 0 {
			return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
		}
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Model(&model.Account{}).Where("id = ?", account.ID).Update("password", string(hashedPassword)).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Model(&model.PasswordResetToken{}).Where("code = ?", code).Update("used", true).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	if red.RedisClient != nil {
		_ = red.RedisClient.Del(context.Background(), resetVerifiedKey(email, code)).Err()
	}

	return nil
}

// Backward-compatible old API: reset directly by code.
// Prefer Java-like flow: ForgotPassword -> VerifyResetPassword -> UpdatePassword.
func (s *AuthService) ResetPassword(codeStr, newPassword string) error {
	code, err := uuid.Parse(codeStr)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}

	var resetToken model.PasswordResetToken
	if err := db.DB.Where("code = ?", code).First(&resetToken).Error; err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}
	if resetToken.Used {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}
	if time.Now().After(resetToken.ExpiryTime) {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Model(&model.Account{}).Where("id = ?", resetToken.AccountID).Update("password", string(hashedPassword)).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Model(&model.PasswordResetToken{}).Where("code = ?", code).Update("used", true).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func (s *AuthService) ChangePassword(accountID uuid.UUID, oldPassword, newPassword string) error {
	var account model.Account
	if err := db.DB.Where("id = ?", accountID).First(&account).Error; err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(oldPassword)); err != nil {
		return exception.NewAppException(exception.InvalidPassword)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := db.DB.Model(&model.Account{}).Where("id = ?", accountID).Update("password", string(hashedPassword)).Error; err != nil {
		return err
	}

	return nil
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

