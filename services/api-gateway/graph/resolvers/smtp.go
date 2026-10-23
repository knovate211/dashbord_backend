package resolvers

import (
	"net/smtp"
	"os"

	pkgconfig "github.com/knovate211/pkg/config"
)

// smtpConfig is the SMTP_* settings every mail path in the gateway shares —
// enquiry alerts, password reset codes, scholarship and hiring links, welcome
// emails — and how to send with them. Each mailer embeds it and decides for
// itself when it counts as enabled and what its messages look like.
type smtpConfig struct {
	host string
	port string
	user string
	pass string
	from string
}

func loadSMTPConfig() smtpConfig {
	return smtpConfig{
		host: os.Getenv("SMTP_HOST"),
		port: pkgconfig.Env("SMTP_PORT", "587"),
		user: os.Getenv("SMTP_USER"),
		pass: os.Getenv("SMTP_PASS"),
		from: pkgconfig.Env("SMTP_FROM", os.Getenv("SMTP_USER")),
	}
}

// sendMail delivers one raw message (headers and body) to `to`.
//
// Credentials are only offered when there are credentials to offer. Go's
// PlainAuth refuses to send a username over an unencrypted connection to
// anything but localhost, so passing an empty auth turns every no-auth relay —
// a local mail catcher, an internal smarthost, SES on an allow-listed IP — into
// "unencrypted connection" instead of a sent email. SendMail accepts a nil auth
// and simply skips AUTH.
func (c smtpConfig) sendMail(to []string, msg []byte) error {
	var auth smtp.Auth
	if c.user != "" {
		auth = smtp.PlainAuth("", c.user, c.pass, c.host)
	}
	return smtp.SendMail(c.host+":"+c.port, auth, c.from, to, msg)
}
