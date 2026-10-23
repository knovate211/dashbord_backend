package resolvers

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// userMailer sends a new account its own login details.
//
// Configuration is entirely by environment and shared with every other mailer
// in the gateway (SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS, SMTP_FROM). With
// SMTP_HOST unset the mailer is disabled: the account is still created and the
// generated password is still returned to the admin, but nothing is emailed and
// the fact is logged. So a local database never sends real mail, and a missing
// relay degrades the welcome rather than the account creation.
//
// The message is plain text on purpose. A credentials email that arrives as a
// styled HTML card reads like phishing — exactly the reflex we want a new user
// NOT to have about the one mail that carries their password — and plain text
// also survives the corporate filters that strip HTML wholesale.
type userMailer struct {
	smtpConfig
	loginURL string
	log      *zap.Logger
}

func NewUserMailer(loginURL string, log *zap.Logger) *userMailer {
	m := &userMailer{
		smtpConfig: loadSMTPConfig(),
		loginURL:   strings.TrimRight(loginURL, "/") + "/login",
		log:        log,
	}
	if m.host == "" {
		log.Info("welcome email disabled (SMTP_HOST unset) — new accounts are created but not emailed")
	} else {
		log.Info("welcome email enabled")
	}
	return m
}

func (m *userMailer) enabled() bool { return m.host != "" && m.from != "" }

// notifyNewUser emails the account holder their credentials.
//
// Fire-and-forget on a background goroutine: the account is already committed
// before this runs, so a slow or broken relay must never delay, or fail, the
// admin's create. A delivery error is logged, not surfaced — the admin has the
// password on screen and can pass it on by hand if the mail bounces.
func (m *userMailer) notifyNewUser(name, email, password string) {
	if !m.enabled() {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.log.Error("welcome email panicked", zap.Any("panic", r))
			}
		}()
		if err := m.deliver(name, email, password); err != nil {
			m.log.Error("welcome email failed to send", zap.String("email", email), zap.Error(err))
			return
		}
		m.log.Info("welcome email sent", zap.String("email", email))
	}()
}

func (m *userMailer) deliver(name, email, password string) error {
	if !m.enabled() {
		return fmt.Errorf("email is not configured (SMTP_HOST is unset)")
	}

	greeting := "Hi,"
	if n := strings.TrimSpace(name); n != "" {
		greeting = "Hi " + n + ","
	}

	body := strings.Join([]string{
		greeting,
		"",
		"An account has been created for you on the Knovate learning portal.",
		"Here are your sign-in details:",
		"",
		"  Login page:  " + m.loginURL,
		"  Email:       " + email,
		"  Password:    " + password,
		"",
		"For your security, please change this password after you sign in the",
		"first time. You can set a new one from the \"Forgot password?\" link on",
		"the login page at any time.",
		"",
		"If you were not expecting this email, you can ignore it.",
		"",
		"— The Knovate team",
	}, "\n")

	msg := []byte(strings.Join([]string{
		"From: " + m.from,
		"To: " + email,
		"Subject: Your Knovate account is ready",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n"))

	return m.sendMail([]string{email}, msg)
}
