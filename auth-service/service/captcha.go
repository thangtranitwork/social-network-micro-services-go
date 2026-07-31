package service

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"social-network-go/logger"
)

type TurnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ErrorCodes  []string `json:"error-codes"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
}

func VerifyCaptchaToken(token, clientIP string) bool {
	enabledStr := os.Getenv("CAPTCHA_ENABLED")
	if enabledStr == "false" || enabledStr == "0" {
		return true
	}

	secretKey := strings.TrimSpace(os.Getenv("CAPTCHA_SECRET_KEY"))
	if secretKey == "" {
		// If secret key is not set in dev/test, bypass
		if os.Getenv("APP_ENV") != "production" {
			return true
		}
		logger.Error("CAPTCHA_SECRET_KEY is empty in production environment")
		return false
	}

	if strings.TrimSpace(token) == "" {
		return false
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	formData := url.Values{
		"secret":   {secretKey},
		"response": {token},
	}
	if clientIP != "" {
		formData.Set("remoteip", clientIP)
	}

	resp, err := httpClient.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", formData)
	if err != nil {
		logger.Err(err).Error("Turnstile siteverify API request failed")
		return false
	}
	defer resp.Body.Close()

	var verifyRes TurnstileVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyRes); err != nil {
		logger.Err(err).Error("Failed to decode Turnstile response")
		return false
	}

	if !verifyRes.Success {
		logger.Warn("Turnstile verification failed for IP %s with error codes: %v", clientIP, verifyRes.ErrorCodes)
	}

	return verifyRes.Success
}
