package resolvers

import (
	"strings"

	"go.uber.org/zap"
)

// resetMailer delivers a reset code to the learner.
//
// Separate from the enquiry mailer because the recipient is different in kind:
// that one alerts a fixed staff list, this one writes to whoever asked. It
// reuses the same SMTP_* environment configuration.
//
// With SMTP_HOST unset the code is written to the service log instead. That
// keeps the whole flow testable before SMTP is wired up — at the cost of
// putting a live reset code in the logs, so it is loud about when it applies
// and must not be how production runs.
type resetMailer struct {
	smtpConfig
	log *zap.Logger
}

func newResetMailer(log *zap.Logger) *resetMailer {
	m := &resetMailer{
		smtpConfig: loadSMTPConfig(),
		log:        log,
	}
	if m.host == "" {
		log.Warn("password reset email disabled (SMTP_HOST unset) — " +
			"reset codes will be written to this log. Do not run production this way.")
	} else {
		log.Info("password reset email enabled", zap.String("smtp_host", m.host))
	}
	return m
}

func (m *resetMailer) enabled() bool { return m.host != "" }

// sendCode delivers the code, or logs it when SMTP is not configured.
//
// Sent inline rather than on a goroutine like the enquiry alert: the learner is
// waiting on this mail to continue, so a delivery failure is worth logging
// against the request that caused it.
func (m *resetMailer) sendCode(email, name, code string) {
	if !m.enabled() {
		m.log.Warn("password reset code generated but SMTP is disabled",
			zap.String("email", email),
			zap.String("code", code),
			zap.String("hint", "set SMTP_HOST to deliver this by email instead"))
		return
	}

	greeting := "Hello,"
	if n := strings.TrimSpace(name); n != "" {
		greeting = "Hello " + n + ","
	}

	body := strings.Join([]string{
		greeting,
		"",
		"Here is the code to reset your Knovate password:",
		"",
		"    " + code,
		"",
		"It expires in 15 minutes and can only be used once.",
		"",
		"If you did not ask to reset your password you can ignore this email — " +
			"your password has not changed.",
	}, "\n")

	msg := []byte(strings.Join([]string{
		"From: " + m.from,
		"To: " + email,
		"Subject: Your Knovate password reset code",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n"))

	if err := m.sendMail([]string{email}, msg); err != nil {
		// The code is already stored, so the learner can retry; this is a
		// delivery failure, not a lost reset.
		m.log.Error("password reset email failed to send",
			zap.String("email", email), zap.Error(err))
		return
	}
	m.log.Info("password reset email sent", zap.String("email", email))
}
