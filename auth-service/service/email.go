package service

import (
	"fmt"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"

	"social-network-go/auth-service/config"
	"social-network-go/logger"
)

type EmailSender interface {
	SendHTMLEmail(to, subject, htmlBody string)
	GetRegisterEmailHTML(verificationLink string) string
	GetResetPasswordEmailHTML(resetLink string) string
}

type SMTPEmailSender struct {
	Cfg *config.Config
}

func NewSMTPEmailSender(cfg *config.Config) EmailSender {
	return &SMTPEmailSender{Cfg: cfg}
}

func (s *SMTPEmailSender) SendHTMLEmail(to, subject, htmlBody string) {
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

func (s *SMTPEmailSender) GetRegisterEmailHTML(verificationLink string) string {
	content, err := getTemplateContent("register-verify-email.html")
	if err == nil {
		html := string(content)
		html = strings.ReplaceAll(html, `th:text="${title}">Confirm Your Email`, `>Confirm Your Email`)
		html = strings.ReplaceAll(html, `th:href="${verificationLink}"`, `href="`+verificationLink+`"`)
		return html
	}
	return "<h1>Confirm Email</h1><p>Click here: <a href=\"" + verificationLink + "\">Verify</a></p>"
}

func (s *SMTPEmailSender) GetResetPasswordEmailHTML(resetLink string) string {
	content, err := getTemplateContent("update-password-email.html")
	if err == nil {
		html := string(content)
		html = strings.ReplaceAll(html, `th:href="${resetLink}"`, `href="`+resetLink+`"`)
		return html
	}
	return "<h1>Reset Password</h1><p>Click here: <a href=\"" + resetLink + "\">Reset</a></p>"
}
