package hub

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Mailer delivers sign-in mail.
type Mailer interface {
	Send(to, subject, body string) error
	Ready() bool
}

// MailerFromEnv: AURORA_SMTP_HOST/PORT/USER/PASS/FROM, or AURORA_DEV_MAIL=1
// which writes each message to a 0600 file under dir (never to logs).
func MailerFromEnv(dir string) Mailer {
	if h := os.Getenv("AURORA_SMTP_HOST"); h != "" {
		return &SMTP{Host: h, Port: envOr("AURORA_SMTP_PORT", "587"), User: os.Getenv("AURORA_SMTP_USER"), Pass: os.Getenv("AURORA_SMTP_PASS"), From: envOr("AURORA_SMTP_FROM", os.Getenv("AURORA_SMTP_USER"))}
	}
	if os.Getenv("AURORA_DEV_MAIL") == "1" {
		return &FileMailer{Dir: dir}
	}
	return nil
}

type SMTP struct{ Host, Port, User, Pass, From string }

func (s *SMTP) Ready() bool { return s.From != "" }

func (s *SMTP) Send(to, subject, body string) error {
	if strings.ContainsAny(to+subject, "\r\n") {
		return errors.New("bad header")
	}
	addr := net.JoinHostPort(s.Host, s.Port)
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if s.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.User, s.Pass, s.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(s.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "From: Neko Aurora <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nDate: %s\r\n\r\n%s", s.From, to, subject, time.Now().Format(time.RFC1123Z), strings.ReplaceAll(body, "\n", "\r\n"))
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

type FileMailer struct{ Dir string }

func (f *FileMailer) Ready() bool { return true }

func (f *FileMailer) Send(to, subject, body string) error {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	name := strings.NewReplacer("@", "_at_", "/", "_").Replace(to) + ".txt"
	return os.WriteFile(filepath.Join(f.Dir, name), []byte("Subject: "+subject+"\n\n"+body), 0o600)
}
