package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/net/idna"
	"ucenter/internal/config"
	"ucenter/internal/store"
)

type Message struct {
	TenantID primitive.ObjectID
	To       string
	Subject  string
	Text     string
	HTML     string
	Kind     string
}

type Mailer interface {
	Send(ctx context.Context, message Message) error
}

type Mock struct {
	Store *store.Store
}

func (m *Mock) Send(ctx context.Context, message Message) error {
	_, err := m.Store.DB.Collection("mail_outbox").InsertOne(ctx, bson.M{
		"tenant_id":  message.TenantID,
		"to":         message.To,
		"subject":    message.Subject,
		"text":       message.Text,
		"html":       message.HTML,
		"kind":       message.Kind,
		"created_at": time.Now(),
	})
	return err
}

type SMTP struct {
	cfg config.Config
}

func New(cfg config.Config, st *store.Store) Mailer {
	if strings.EqualFold(cfg.MailDriver, "smtp") {
		return &SMTP{cfg: cfg}
	}
	return &Mock{Store: st}
}

func (s *SMTP) Send(ctx context.Context, message Message) error {
	addr := net.JoinHostPort(s.cfg.SMTPHost, s.cfg.SMTPPort)
	from := s.cfg.SMTPFrom
	to := message.To
	headers := []string{
		"From: " + s.cfg.SMTPFromName + " <" + from + ">",
		"To: " + to,
		"Subject: " + message.Subject,
		"MIME-Version: 1.0",
	}
	body := message.Text
	contentType := "text/plain; charset=UTF-8"
	if message.HTML != "" {
		body = message.HTML
		contentType = "text/html; charset=UTF-8"
	}
	headers = append(headers, "Content-Type: "+contentType)
	raw := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)

	var client *smtp.Client
	var err error
	if s.cfg.SMTPTLS {
		conn, dialErr := tls.Dial("tcp", addr, &tls.Config{ServerName: s.cfg.SMTPHost, MinVersion: tls.VersionTLS12})
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(conn, s.cfg.SMTPHost)
	} else {
		client, err = smtp.Dial(addr)
	}
	if err != nil {
		return err
	}
	defer client.Close()
	if !s.cfg.SMTPTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.cfg.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
	}
	if s.cfg.SMTPUser != "" {
		auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email")
	}
	_, err := idna.Lookup.ToASCII(email)
	return err
}
