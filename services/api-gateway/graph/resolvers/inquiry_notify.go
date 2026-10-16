package resolvers

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// mailer sends the "new enquiry" alert.
//
// Response time is most of what decides whether a placement lead converts, and
// before this an enquiry sat in the table until somebody happened to open the
// admin panel.
//
// Configuration is entirely by environment. With SMTP_HOST unset the mailer is
// disabled and enquiries are only logged — so local development never sends
// real email, and a missing config degrades the alert rather than the capture.
type mailer struct {
	host string
	port string
	user string
	pass string
	from string
	to   []string
	log  *zap.Logger
}

func newMailer(log *zap.Logger) *mailer {
	to := []string{}
	for _, addr := range strings.Split(os.Getenv("INQUIRY_NOTIFY_TO"), ",") {
		if a := strings.TrimSpace(addr); a != "" {
			to = append(to, a)
		}
	}

	m := &mailer{
		host: os.Getenv("SMTP_HOST"),
		port: envOr("SMTP_PORT", "587"),
		user: os.Getenv("SMTP_USER"),
		pass: os.Getenv("SMTP_PASS"),
		from: envOr("SMTP_FROM", os.Getenv("SMTP_USER")),
		to:   to,
		log:  log,
	}

	switch {
	case m.host == "":
		log.Info("enquiry email disabled (SMTP_HOST unset) — enquiries are logged only")
	case len(m.to) == 0:
		log.Warn("SMTP configured but INQUIRY_NOTIFY_TO is empty — nobody will be alerted")
	default:
		log.Info("enquiry email enabled", zap.Strings("to", m.to))
	}
	return m
}

func (m *mailer) enabled() bool { return m.host != "" && len(m.to) > 0 }

// notify sends the alert on a background goroutine.
//
// Deliberately fire-and-forget: the visitor's submission must not fail, or even
// wait, because an SMTP server is slow or down. The enquiry is already
// committed before this is called.
func (m *mailer) notify(in inquiryInput, name, email string) {
	if !m.enabled() {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.log.Error("enquiry email panicked", zap.Any("panic", r))
			}
		}()

		subject := fmt.Sprintf("New enquiry: %s (%s)", name, normalizeSource(in.Source))
		body := strings.Join([]string{
			"A new enquiry just came in.",
			"",
			"Name:      " + name,
			"Email:     " + email,
			"Phone:     " + orDash(in.Phone),
			"WhatsApp:  " + orDash(in.WhatsApp),
			"Interest:  " + orDash(in.Interest),
			"Company:   " + orDash(in.Company),
			"Role:      " + orDash(in.JobTitle),
			"Size:      " + orDash(in.CompanySize),
			"Hiring:    " + orDash(in.HiringVolume),
			"Source:    " + normalizeSource(in.Source),
			"Page:      " + orDash(in.PageURL),
			"Received:  " + time.Now().Format(time.RFC1123),
			"",
			"Message:",
			orDash(in.Message),
			"",
			"Open the admin panel → Enquiries to mark it contacted.",
		}, "\n")

		msg := []byte(strings.Join([]string{
			"From: " + m.from,
			"To: " + strings.Join(m.to, ", "),
			// Replying goes straight to the candidate rather than to the
			// sending mailbox, which is what anyone hitting Reply expects.
			"Reply-To: " + email,
			"Subject: " + subject,
			"MIME-Version: 1.0",
			"Content-Type: text/plain; charset=UTF-8",
			"",
			body,
		}, "\r\n"))

		// Only offer credentials when there are credentials to offer. Go's
		// PlainAuth refuses to send a username over an unencrypted connection
		// to anything but localhost, so passing an empty auth turns every
		// no-auth relay — a local mail catcher, an internal smarthost, SES on
		// an allow-listed IP — into "unencrypted connection" instead of a sent
		// email. SendMail accepts a nil auth and simply skips AUTH.
		var auth smtp.Auth
		if m.user != "" {
			auth = smtp.PlainAuth("", m.user, m.pass, m.host)
		}
		addr := m.host + ":" + m.port
		if err := smtp.SendMail(addr, auth, m.from, m.to, msg); err != nil {
			// The enquiry is already saved, so this is an alerting failure, not
			// a lost lead — log loudly and carry on.
			m.log.Error("enquiry email failed to send",
				zap.String("email", email), zap.Error(err))
			return
		}
		m.log.Info("enquiry email sent", zap.String("email", email))
	}()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
