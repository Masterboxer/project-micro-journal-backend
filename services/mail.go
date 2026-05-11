package services

import (
	"fmt"
	"os"
	"strings"

	"github.com/wneessen/go-mail"
)

type MailService struct {
	client *mail.Client
	from   string
}

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

func NewMailService(cfg Config) (*MailService, error) {
	client, err := mail.NewClient(
		cfg.Host,
		mail.WithPort(cfg.Port),
		mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
		mail.WithUsername(cfg.Username),
		mail.WithPassword(cfg.Password),
		mail.WithTLSPolicy(mail.TLSMandatory),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mail client: %w", err)
	}

	return &MailService{
		client: client,
		from:   cfg.From,
	}, nil
}

func (m *MailService) SendMail(to, subject, textBody, htmlBody string) error {
	msg := mail.NewMsg()
	if err := msg.From(m.from); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, textBody)
	msg.AddAlternativeString(mail.TypeTextHTML, htmlBody)

	return m.client.DialAndSend(msg)
}

func (m *MailService) SendPasswordResetEmail(to, token string) error {
	link := fmt.Sprintf("https://reflecto.co.in/reset-password?token=%s", token)

	htmlBytes, err := os.ReadFile("templates/reset-password.html")
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}

	htmlBody := strings.ReplaceAll(string(htmlBytes), "{{RESET_LINK}}", link)
	textBody := fmt.Sprintf("Reset your Reflecto password:\n%s\n\nExpires in 15 minutes.", link)

	return m.SendMail(to, "Reset your password — Reflecto", textBody, htmlBody)
}

func (m *MailService) SendVerificationEmail(to, token string) error {
	link := fmt.Sprintf("https://reflecto.co.in/verify?token=%s", token)

	htmlBytes, err := os.ReadFile("templates/verify-email.html")
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}

	htmlBody := strings.ReplaceAll(string(htmlBytes), "{{VERIFY_LINK}}", link)
	textBody := fmt.Sprintf("Welcome to Reflecto! Verify your email:\n%s", link)

	return m.SendMail(to, "Verify your email — Reflecto", textBody, htmlBody)
}
