package services

import (
	"fmt"

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

	if err := m.client.DialAndSend(msg); err != nil {
		return err
	}

	return nil
}

func (m *MailService) SendVerificationEmail(to, token string) error {
	link := fmt.Sprintf("https://reflecto.co.in/verify?token=%s", token)

	textBody := fmt.Sprintf(`
Welcome to MyApp!

Verify your email:
%s

If you didn't sign up, ignore this.
`, link)

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; line-height: 1.6;">
	<h2>Welcome to MyApp 🎉</h2>
	<p>Click the button below to verify your email:</p>
	<p>
		<a href="%s" 
		   style="background-color:#4CAF50;color:white;padding:10px 15px;text-decoration:none;border-radius:5px;">
		   Verify Email
		</a>
	</p>
	<p>If the button doesn't work, copy this link:</p>
	<p>%s</p>
</body>
</html>
`, link, link)

	return m.SendMail(to, "Verify your email", textBody, htmlBody)
}

func (m *MailService) SendPasswordResetEmail(to, token string) error {
	link := fmt.Sprintf("https://reflecto.co.in/forgot-password?token=%s", token)

	textBody := fmt.Sprintf(`
Reset your password:

%s

This link expires in 15 minutes.
`, link)

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif;">
	<h2>Password Reset</h2>
	<p>Click below to reset your password:</p>
	<p>
		<a href="%s"
		   style="background-color:#f44336;color:white;padding:10px 15px;text-decoration:none;border-radius:5px;">
		   Reset Password
		</a>
	</p>
	<p>This link expires in 15 minutes.</p>
</body>
</html>
`, link)

	return m.SendMail(to, "Reset your password", textBody, htmlBody)
}
