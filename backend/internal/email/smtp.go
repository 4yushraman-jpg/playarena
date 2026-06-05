package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

type smtpProvider struct {
	host        string
	port        int
	username    string
	password    string
	useTLS      bool
	fromAddress string
	fromName    string
}

// newSMTPProviderImpl constructs an SMTP provider. When useTLS is true the
// connection is opened over TLS (implicit TLS / port 465). When false,
// smtp.SendMail is used which negotiates STARTTLS when the server offers it.
func newSMTPProviderImpl(host string, port int, username, password string, useTLS bool, fromAddr, fromName string) Provider {
	return &smtpProvider{
		host:        host,
		port:        port,
		username:    username,
		password:    password,
		useTLS:      useTLS,
		fromAddress: fromAddr,
		fromName:    fromName,
	}
}

// Send delivers msg via SMTP. The context deadline is honoured: if ctx is
// cancelled or its deadline expires before the SMTP exchange completes, Send
// returns ctx.Err(). The SMTP goroutine may linger briefly after context
// cancellation; it terminates within the OS TCP timeout (~2 min) or sooner
// if the server responds to the abandoned connection.
func (p *smtpProvider) Send(ctx context.Context, msg Message) error {
	addr := fmt.Sprintf("%s:%d", p.host, p.port)
	body := p.buildMessage(msg)

	type result struct{ err error }
	ch := make(chan result, 1)
	go func() {
		var err error
		if p.useTLS {
			err = p.sendViaTLS(addr, msg.To, body)
		} else {
			err = p.sendViaSendMail(addr, msg.To, body)
		}
		if err != nil {
			ch <- result{fmt.Errorf("email: SMTP send: %w", err)}
		} else {
			ch <- result{}
		}
	}()

	select {
	case r := <-ch:
		return r.err
	case <-ctx.Done():
		return fmt.Errorf("email: SMTP send: %w", ctx.Err())
	}
}

func (p *smtpProvider) sendViaSendMail(addr, to string, body []byte) error {
	var auth smtp.Auth
	if p.username != "" {
		auth = smtp.PlainAuth("", p.username, p.password, p.host)
	}
	return smtp.SendMail(addr, auth, p.fromAddress, []string{to}, body)
}

func (p *smtpProvider) sendViaTLS(addr, to string, body []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: p.host})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, p.host)
	if err != nil {
		return err
	}
	defer c.Quit() //nolint:errcheck

	if p.username != "" {
		if err := c.Auth(smtp.PlainAuth("", p.username, p.password, p.host)); err != nil {
			return err
		}
	}
	if err := c.Mail(p.fromAddress); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	defer w.Close() //nolint:errcheck
	_, err = w.Write(body)
	return err
}

// buildMessage constructs a MIME RFC 5322 message. When both TextBody and
// HTMLBody are present a multipart/alternative structure is used so that mail
// clients without HTML support can fall back to plain text. When only one body
// type is supplied a single-part message is emitted instead.
func (p *smtpProvider) buildMessage(msg Message) []byte {
	from := p.fromAddress
	if p.fromName != "" {
		from = fmt.Sprintf("%s <%s>", p.fromName, p.fromAddress)
	}

	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + msg.To + "\r\n")
	sb.WriteString("Subject: " + msg.Subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")

	if msg.TextBody != "" && msg.HTMLBody != "" {
		boundary := fmt.Sprintf("mime_%d", time.Now().UnixNano())
		sb.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n", boundary))
		sb.WriteString("\r\n")

		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(msg.TextBody)
		sb.WriteString("\r\n")

		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(msg.HTMLBody)
		sb.WriteString("\r\n")

		sb.WriteString("--" + boundary + "--\r\n")
	} else if msg.HTMLBody != "" {
		sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(msg.HTMLBody)
	} else {
		sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(msg.TextBody)
	}

	return []byte(sb.String())
}
