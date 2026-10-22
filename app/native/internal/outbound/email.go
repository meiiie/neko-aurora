package outbound

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// Mailer is the email adapter: it delivers APPROVED email cards over SMTP. Configured only from the
// server environment (AURORA_SMTP_HOST, AURORA_SMTP_PORT, AURORA_SMTP_USER,
// AURORA_SMTP_PASS, AURORA_SMTP_FROM); credentials never reach the client.
type Mailer struct {
	host, port, user, pass, From string
}

func MailerFromEnv() *Mailer {
	env := func(k string) string { return os.Getenv("AURORA_SMTP_" + k) }
	m := &Mailer{host: env("HOST"), port: env("PORT"), user: env("USER"), pass: env("PASS"), From: env("FROM")}
	if m.port == "" {
		m.port = "587"
	}
	if m.From == "" {
		m.From = m.user
	}
	return m
}

func (m *Mailer) Configured() bool { return m != nil && m.host != "" && m.From != "" }

func (m *Mailer) Kind() string   { return "email" }
func (m *Mailer) Sender() string { return m.From }

func (m *Mailer) Validate(r Request) error {
	if _, ok := ValidEmail(r.To); !ok {
		return errors.New("invalid_recipient")
	}
	return nil
}

func (m *Mailer) Deliver(_ context.Context, r Request) error {
	subj := r.Subject
	if subj == "" {
		subj = r.Title
	}
	return m.Send(r.To, subj, r.Body)
}

func ValidEmail(a string) (string, bool) {
	p, err := mail.ParseAddress(strings.TrimSpace(a))
	if err != nil || strings.ContainsAny(p.Address, "\r\n") {
		return "", false
	}
	return p.Address, true
}

func buildMessage(from, to, subject, body string) []byte {
	h := []string{
		"From: " + from, "To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", strings.ReplaceAll(subject, "\n", " ")),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: 8bit",
		"X-Mailer: Neko Aurora",
	}
	return []byte(strings.Join(h, "\r\n") + "\r\n\r\n" + strings.ReplaceAll(body, "\n", "\r\n") + "\r\n")
}

func (m *Mailer) Send(to, subject, body string) error {
	if !m.Configured() {
		return errors.New("email_channel_not_configured")
	}
	addr := net.JoinHostPort(m.host, m.port)
	var c *smtp.Client
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	if m.port == "465" {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: m.host})
		if err != nil {
			return fmt.Errorf("smtp_connect: %w", err)
		}
		if c, err = smtp.NewClient(conn, m.host); err != nil {
			return err
		}
	} else {
		conn, err := dialer.Dial("tcp", addr)
		if err != nil {
			return fmt.Errorf("smtp_connect: %w", err)
		}
		if c, err = smtp.NewClient(conn, m.host); err != nil {
			return err
		}
		if ok, _ := c.Extension("STARTTLS"); !ok {
			c.Close()
			return errors.New("smtp_starttls_required")
		}
		if err = c.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
			c.Close()
			return err
		}
	}
	defer c.Close()
	if m.user != "" {
		if err := c.Auth(smtp.PlainAuth("", m.user, m.pass, m.host)); err != nil {
			return errors.New("smtp_auth_failed")
		}
	}
	if err := c.Mail(m.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(buildMessage(m.From, to, subject, body)); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
