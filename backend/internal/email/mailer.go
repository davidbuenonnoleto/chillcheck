package email

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

// Mailer sends transactional email (alerts, invites, password resets). The
// LogMailer needs no setup (it logs); SMTPMailer sends real email. A Twilio/SNS
// sender would be another implementation.
type Mailer interface {
	Send(ctx context.Context, to []string, subject, body string) error
}

// LogMailer writes messages to the log. Used when no SMTP server is configured.
type LogMailer struct{}

func (LogMailer) Send(_ context.Context, to []string, subject, body string) error {
	log.Printf("[EMAIL] (no mailer configured) to=%s | %s | %s",
		strings.Join(to, ","), subject, strings.ReplaceAll(body, "\n", " "))
	return nil
}

// SMTPMailer sends over SMTP with STARTTLS (port 587). Works with Amazon SES
// SMTP, Mailgun, Postmark, or a Gmail app password.
type SMTPMailer struct {
	Host, Port, User, Pass, From string
}

func (m SMTPMailer) Send(_ context.Context, to []string, subject, body string) error {
	if len(to) == 0 {
		return nil
	}
	auth := smtp.PlainAuth("", m.User, m.Pass, m.Host)
	return smtp.SendMail(m.Host+":"+m.Port, auth, m.From, to, buildMessage(m.From, to, subject, body))
}

func buildMessage(from string, to []string, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
