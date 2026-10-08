package notify

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// ErrMailNotConfigured means no server was given, so there is nowhere to send.
var ErrMailNotConfigured = errors.New("no mail server is configured")

// Mailer sends notices by email over SMTP.
//
// SMTP through the standard library rather than a provider's SDK: every provider
// speaks it, so the same code works against Mailpit on a laptop and against a
// real service in a deployment, and the project gains no dependency for it.
type Mailer struct {
	addr string
	from string
	auth smtp.Auth
}

// MailerConfig carries what reaching a mail server needs.
type MailerConfig struct {
	// Addr is host:port. Empty means email is turned off.
	Addr string

	// From is the address the mail comes from.
	From string

	// Username and Password are left empty for a local server that wants no
	// authentication, which is what a development catcher is.
	Username string
	Password string
}

// NewMailer returns a mailer, or nil when no server is configured. A nil mailer
// is how email stays off by default rather than failing at startup.
func NewMailer(cfg MailerConfig) (*Mailer, error) {
	if cfg.Addr == "" {
		return nil, nil
	}

	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("the from address %q is not valid: %w", cfg.From, err)
	}

	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("the mail server address %q is not host:port: %w", cfg.Addr, err)
	}

	mailer := &Mailer{addr: cfg.Addr, from: cfg.From}

	if cfg.Username != "" {
		mailer.auth = smtp.PlainAuth("", cfg.Username, cfg.Password, host)
	}

	return mailer, nil
}

// SendEmail sends one notice to one address.
//
// The context bounds how long this may take: without it a server that accepts
// the connection and then says nothing would hold a worker for ever.
func (m *Mailer) SendEmail(ctx context.Context, address string, notice Notice) error {
	if m == nil {
		return ErrMailNotConfigured
	}

	if _, err := mail.ParseAddress(address); err != nil {
		return fmt.Errorf("%q is not an address to send to: %w", address, err)
	}

	message := m.compose(address, notice)

	// net/smtp has no context, so the call runs on its own goroutine and the
	// context decides whether we keep waiting for it.
	done := make(chan error, 1)

	go func() {
		done <- smtp.SendMail(m.addr, m.auth, m.from, []string{address}, message)
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("sending mail: %w", err)
		}

		return nil
	case <-ctx.Done():
		return fmt.Errorf("sending mail: %w", ctx.Err())
	}
}

// compose builds the message. Headers are written by hand because the whole of
// what is needed is five of them, and a library for that would be a dependency
// for string joining.
func (m *Mailer) compose(address string, notice Notice) []byte {
	var builder strings.Builder

	// CRLF throughout: SMTP ends lines that way, and a bare newline is how a
	// message arrives with its headers folded into the body.
	write := func(format string, args ...any) {
		fmt.Fprintf(&builder, format+"\r\n", args...)
	}

	write("From: %s", m.from)
	write("To: %s", address)
	// Encoded, so a subject with a non-ASCII character does not arrive as
	// mojibake. An event called "Fazıl Say" is the ordinary case here.
	write("Subject: %s", encodeHeader(notice.Subject))
	write("Date: %s", time.Now().Format(time.RFC1123Z))
	write("MIME-Version: 1.0")
	write("Content-Type: text/plain; charset=utf-8")
	write("")
	write("%s", notice.Body)

	return []byte(builder.String())
}
