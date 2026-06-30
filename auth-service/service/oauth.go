package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"social-network-go/auth-service/model"
	"social-network-go/logger"
	"social-network-go/pb"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

func (s *AuthService) getGoogleConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.Cfg.GoogleClientID,
		ClientSecret: s.Cfg.GoogleClientSecret,
		RedirectURL:  s.Cfg.GoogleRedirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}
}

func (s *AuthService) GetGoogleAuthURL() string {
	return s.getGoogleConfig().AuthCodeURL("state")
}

func (s *AuthService) GetFrontendURL() string {
	return s.Cfg.FrontendURL
}

func (s *AuthService) GoogleCallback(ctx context.Context, code string) (string, string, string, string, error) {
	config := s.getGoogleConfig()
	token, err := config.Exchange(ctx, code)
	if err != nil {
		logger.Field("error", err).Error("Failed to exchange auth code with Google")
		return "", "", "", "", err
	}

	// Fetch user info from Google
	client := config.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		logger.Field("error", err).Error("Failed to get userinfo from Google")
		return "", "", "", "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", "", err
	}

	var googleUser GoogleUserInfo
	if err := json.Unmarshal(data, &googleUser); err != nil {
		logger.Field("error", err).Error("Failed to unmarshal Google user info")
		return "", "", "", "", err
	}

	if googleUser.Email == "" {
		return "", "", "", "", fmt.Errorf("google response did not contain email")
	}

	// Find or create account
	var account *model.Account
	account, err = s.AccountRepo.FindByEmail(ctx, googleUser.Email)
	var username string

	if err != nil {
		// Register new user
		logger.Field("email", googleUser.Email).Info("Registering new user via Google Sign-In")
		accountID := uuid.New()
		account = &model.Account{
			ID:         accountID,
			Email:      googleUser.Email,
			Password:   "", // OAuth users don't have passwords
			Role:       "USER",
			IsVerified: true,
			CreatedAt:  time.Now(),
		}

		tx := s.DB.Begin()
		if tx.Error != nil {
			return "", "", "", "", tx.Error
		}

		txAccountRepo := s.AccountRepo.WithTx(tx)
		if err := txAccountRepo.Create(ctx, account); err != nil {
			tx.Rollback()
			return "", "", "", "", err
		}

		if err := tx.Commit().Error; err != nil {
			return "", "", "", "", err
		}

		givenName := googleUser.GivenName
		if givenName == "" {
			givenName = "User"
		}
		familyName := googleUser.FamilyName
		if familyName == "" {
			familyName = "Google"
		}

		// Call User Service via gRPC to create User Node
		resp, err := s.UserClient.CreateUser(ctx, &pb.CreateUserRequest{
			UserId:     accountID.String(),
			Email:      googleUser.Email,
			GivenName:  givenName,
			FamilyName: familyName,
			Birthdate:  "2000-01-01",
		})
		if err != nil {
			logger.Field("account_id", accountID).Field("error", err).Warn("Failed to create User node in user-service")
			username = strings.Split(googleUser.Email, "@")[0]
		} else {
			username = resp.Username
		}
	} else {
		// Existing account, get username from user-service
		logger.Field("email", googleUser.Email).Info("User already registered. Fetching user info from user-service")
		userInfo, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{UserId: account.ID.String()})
		if err == nil {
			username = userInfo.Username
		} else {
			logger.Field("account_id", account.ID).Field("error", err).Warn("Failed to fetch username from user-service")
			username = strings.Split(account.Email, "@")[0]
		}
	}

	// Generate access and refresh tokens
	accessToken, err := s.GenerateToken(account.ID.String(), account.Email, account.Role, s.Cfg.JWTSecret, s.Cfg.AccessTokenDuration)
	if err != nil {
		return "", "", "", "", err
	}
	refreshToken, err := s.GenerateToken(account.ID.String(), account.Email, account.Role, s.Cfg.JWTRefreshSecret, s.Cfg.RefreshTokenDuration)
	if err != nil {
		return "", "", "", "", err
	}

	return accessToken, refreshToken, account.ID.String(), username, nil
}
