package mail

import (
	"botDashboard/internal/config"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"strings"
)

type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

func LoadSMTPConfig() SMTPConfig {
	env := config.LoadConfig().Env
	port, _ := strconv.Atoi(strings.TrimSpace(envValue(env, "SMTP_PORT")))
	return SMTPConfig{
		Host: strings.TrimSpace(envValue(env, "SMTP_HOST")),
		Port: port,
		User: strings.TrimSpace(envValue(env, "SMTP_USERNAME", "SMTP_USER")),
		Pass: strings.TrimSpace(envValue(env, "SMTP_PASSWORD", "SMTP_PASS")),
		From: strings.TrimSpace(envValue(env, "SMTP_FROM_EMAIL", "SMTP_FROM")),
	}
}

func Enabled() bool {
	cfg := LoadSMTPConfig()
	return cfg.Host != "" && cfg.Port > 0 && cfg.User != "" && cfg.Pass != "" && cfg.From != ""
}

func Send(toEmail, subject, textBody string) error {
	cfg := LoadSMTPConfig()
	if cfg.Host == "" || cfg.Port <= 0 || cfg.User == "" || cfg.Pass == "" || cfg.From == "" {
		return fmt.Errorf("smtp is not configured")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	auth := smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
	body := strings.Join([]string{
		fmt.Sprintf("From: %s", cfg.From),
		fmt.Sprintf("To: %s", strings.TrimSpace(toEmail)),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		textBody,
	}, "\r\n")

	return smtp.SendMail(addr, auth, cfg.From, []string{strings.TrimSpace(toEmail)}, []byte(body))
}

func envValue(env map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
		if value := strings.TrimSpace(env[key]); value != "" {
			return value
		}
	}
	return ""
}
