package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"strings"
	"time"
	"unicode/utf8"
)

type Message struct{ To, Subject, Text string }
type Mailer interface {
	Send(context.Context, Message) error
}

var ErrTLSRequired = errors.New("SMTP requires TLS")

type SMTP struct {
	address, host, from, username, password string
	allowPlaintext                          bool
}

func NewSMTP(environment, address, from, username, password string) (*SMTP, error) {
	if environment != "development" && environment != "test" && environment != "production" {
		return nil, errors.New("invalid mail environment")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("invalid SMTP address")
	}
	sender, err := stdmail.ParseAddress(from)
	if err != nil || strings.ContainsAny(from, "\r\n") {
		return nil, errors.New("invalid mail sender")
	}
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
	}
	return &SMTP{address: address, host: host, from: sender.Address, username: username, password: password, allowPlaintext: environment != "production" && loopback}, nil
}
func (s *SMTP) Send(ctx context.Context, message Message) error {
	to, err := stdmail.ParseAddress(message.To)
	if err != nil || strings.ContainsAny(message.To+message.Subject, "\r\n") || !utf8.ValidString(message.Subject+message.Text) || utf8.RuneCountInString(message.Subject) > 200 || len(message.Text) > 100000 {
		return errors.New("invalid mail message")
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	} else if !s.allowPlaintext {
		return ErrTLSRequired
	}
	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return err
		}
	}
	if err := client.Mail(s.from); err != nil {
		return err
	}
	if err := client.Rcpt(to.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	text := strings.ReplaceAll(strings.ReplaceAll(message.Text, "\r\n", "\n"), "\n", "\r\n")
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", s.from, to.String(), mime.QEncoding.Encode("UTF-8", message.Subject), text)
	if err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
