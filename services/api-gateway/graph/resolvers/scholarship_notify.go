package resolvers

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// scholarshipMailer sends the two emails the funnel needs: the applicant's own
// copy of their test link, and the staff alert.
//
// It is a separate type from the enquiry mailer because it sends to a recipient
// the request supplies, not only to a fixed internal list — which is a
// different risk profile and worth keeping visibly apart. Configuration reuses
// the same SMTP_* environment as every other mail path; with SMTP_HOST unset it
// is disabled and the link is logged instead, so local development never sends
// real email and a missing config degrades the notice rather than the funnel.
//
// The applicant's mail carries a link, never a password. A newly provisioned
// account is created with a random secret nobody is told, and the candidate
// sets their own via the existing password-reset flow — so an intercepted email
// yields at most a time-boxed test link, never a durable credential.
type scholarshipMailer struct {
	host string
	port string
	user string
	pass string
	from string
	// staff receives the "new application" alert; empty means nobody is alerted
	// while applicants are still mailed normally.
	staff []string
	log   *zap.Logger
}

// NewScholarshipMailer exposes the shared applicant mailer to the gateway, so
// the certification funnel sends through the same SMTP configuration and the
// same "log the link when SMTP is unset" degradation.
func NewScholarshipMailer(log *zap.Logger) *scholarshipMailer { return newScholarshipMailer(log) }

func newScholarshipMailer(log *zap.Logger) *scholarshipMailer {
	staff := []string{}
	// Falls back to the enquiry list so a deployment that already alerts on
	// leads keeps alerting once scholarships are switched on.
	raw := os.Getenv("SCHOLARSHIP_NOTIFY_TO")
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv("INQUIRY_NOTIFY_TO")
	}
	for _, addr := range strings.Split(raw, ",") {
		if a := strings.TrimSpace(addr); a != "" {
			staff = append(staff, a)
		}
	}

	m := &scholarshipMailer{
		host:  os.Getenv("SMTP_HOST"),
		port:  envOr("SMTP_PORT", "587"),
		user:  os.Getenv("SMTP_USER"),
		pass:  os.Getenv("SMTP_PASS"),
		from:  envOr("SMTP_FROM", os.Getenv("SMTP_USER")),
		staff: staff,
		log:   log,
	}

	if m.host == "" {
		log.Info("scholarship email disabled (SMTP_HOST unset) — test links are logged only")
	} else {
		log.Info("scholarship email enabled", zap.Strings("staff", m.staff))
	}
	return m
}

func (m *scholarshipMailer) enabled() bool { return m.host != "" && m.from != "" }

// notifyApplicant sends the candidate their own copy of the test link.
//
// The link is the same one the browser was just redirected to, so this is a
// recovery path rather than the primary route: it covers the candidate who
// closes the tab, or who applies on one device and sits the test on another.
func (m *scholarshipMailer) notifyApplicant(name, email, courseName, testTitle, testURL string, durationMinutes, totalMarks int32, isNewAccount bool) {
	if !m.enabled() {
		// Without SMTP the link still has to reach somebody, and in local
		// development that somebody is whoever is reading the logs.
		m.log.Info("scholarship test link (email disabled)",
			zap.String("email", email), zap.String("url", testURL))
		return
	}

	subject, text := applicantSubjectAndText(name, email, courseName, testTitle, testURL, durationMinutes, isNewAccount)
	m.send([]string{email}, "", subject, text,
		applicantHTML(name, courseName, testTitle, testURL, durationMinutes, totalMarks, isNewAccount), email)
}

// applicantSubjectAndText builds the plain-text half, shared by the
// fire-and-forget path and the synchronous one so the two cannot drift.
func applicantSubjectAndText(name, email, courseName, testTitle, testURL string, durationMinutes int32, isNewAccount bool) (string, string) {
	lines := []string{
		"Hi " + name + ",",
		"",
		"Your place in the " + courseName + " scholarship test is confirmed.",
		"",
		"Test:      " + testTitle,
		fmt.Sprintf("Duration:  %d minutes", durationMinutes),
		"Format:    multiple choice and live coding",
		"",
		"Start your test here — the link is personal to you and works for the next 3 days:",
		testURL,
		"",
		"Before you begin: use a laptop or desktop with a stable connection, and set aside",
		"the full duration in one sitting. The timer runs on our servers, so closing the tab",
		"does not stop it.",
	}
	if isNewAccount {
		lines = append(lines,
			"",
			"We have created an account for you under "+email+". You do not need a password to",
			"start the test — use the link above. To sign in later, choose \"Forgot password\"",
			"on the login page to set one.")
	}
	lines = append(lines, "", "Good luck.")
	return "Your " + courseName + " scholarship test is ready", strings.Join(lines, "\n")
}

// notifyStaff alerts the counselling team that an application landed.
func (m *scholarshipMailer) notifyStaff(name, email, phone, courseName, college string) {
	if !m.enabled() || len(m.staff) == 0 {
		return
	}

	body := strings.Join([]string{
		"A new scholarship application just came in.",
		"",
		"Name:     " + name,
		"Email:    " + email,
		"Phone:    " + orDash(phone),
		"Course:   " + courseName,
		"College:  " + orDash(college),
		"Received: " + time.Now().Format(time.RFC1123),
		"",
		"Open the admin panel → Scholarship to track their test.",
	}, "\n")

	// Reply-To is the candidate, which is what anyone hitting Reply expects.
	m.send(m.staff, email, "New scholarship application: "+name+" ("+courseName+")", body, "", email)
}

// deliverApplicant is notifyApplicant's synchronous twin, for callers who need
// to know whether the message actually left.
func (m *scholarshipMailer) deliverApplicant(name, email, courseName, testTitle, testURL string, durationMinutes, totalMarks int32, isNewAccount bool) error {
	subject, text := applicantSubjectAndText(name, email, courseName, testTitle, testURL, durationMinutes, isNewAccount)
	return m.deliver([]string{email}, "", subject, text,
		applicantHTML(name, courseName, testTitle, testURL, durationMinutes, totalMarks, isNewAccount), email)
}

// notifyAward tells a candidate an admin has confirmed their scholarship.
//
// Deliberately triggered by the decision rather than by the grading pipeline: a
// scholarship is a commitment, and a person should be the one to make it. That
// also means it can never fire twice for the same score, or fire on a score
// that is still being judged.
func (m *scholarshipMailer) notifyAward(name, email, courseName string, awardPercent int) {
	if !m.enabled() {
		m.log.Info("scholarship award (email disabled)",
			zap.String("email", email), zap.Int("award_percent", awardPercent))
		return
	}

	body := strings.Join([]string{
		"Hi " + name + ",",
		"",
		fmt.Sprintf("You have been awarded a %d%% scholarship on %s.", awardPercent, courseName),
		"",
		"That discount is applied to your course fee — there is nothing you need to do to claim it.",
		"One of our counsellors will call you shortly to confirm your batch and take you through",
		"enrolment.",
		"",
		"Congratulations, and welcome.",
	}, "\n")

	m.send([]string{email},
		"", fmt.Sprintf("You've been awarded a %d%% scholarship", awardPercent), body,
		awardHTML(name, courseName, awardPercent), email)
}

// send delivers one message on a background goroutine.
//
// Deliberately fire-and-forget: the application is already committed before any
// of this runs, so a slow or broken SMTP server must never delay, or fail, the
// applicant's submission.
func (m *scholarshipMailer) send(to []string, replyTo, subject, body, html, logEmail string) {
	if len(to) == 0 {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.log.Error("scholarship email panicked", zap.Any("panic", r))
			}
		}()
		_ = m.deliver(to, replyTo, subject, body, html, logEmail)
	}()
}

// deliver sends and reports what happened.
//
// The applicant-facing paths call this through send(), which discards the error
// on purpose: their application is already committed and a broken relay must
// not fail or delay it. An admin pressing "Resend" is in a different position —
// they are waiting, and telling them a link went out when the relay refused it
// is worse than telling them nothing. So that path calls this directly.
func (m *scholarshipMailer) deliver(to []string, replyTo, subject, body, html, logEmail string) error {
	if !m.enabled() {
		return fmt.Errorf("email is not configured (SMTP_HOST is unset)")
	}
	{
		headers := []string{
			"From: " + m.from,
			"To: " + strings.Join(to, ", "),
		}
		if replyTo != "" {
			headers = append(headers, "Reply-To: "+replyTo)
		}
		if html == "" {
			headers = append(headers,
				"Subject: "+subject,
				"MIME-Version: 1.0",
				"Content-Type: text/plain; charset=UTF-8",
				"",
				body,
			)
		} else {
			// multipart/alternative, plain text first. The order matters: a
			// client picks the last part it understands, so text has to come
			// before HTML — and the text part is not a formality. Some
			// corporate filters strip HTML entirely, and a candidate whose
			// mail client shows them an empty message has lost their test.
			const boundary = "knovate-scholarship-boundary"
			headers = append(headers,
				"Subject: "+subject,
				"MIME-Version: 1.0",
				`Content-Type: multipart/alternative; boundary="`+boundary+`"`,
				"",
				"--"+boundary,
				"Content-Type: text/plain; charset=UTF-8",
				"",
				body,
				"",
				"--"+boundary,
				"Content-Type: text/html; charset=UTF-8",
				"",
				html,
				"",
				"--"+boundary+"--",
			)
		}

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
		if err := smtp.SendMail(m.host+":"+m.port, auth, m.from, to, []byte(strings.Join(headers, "\r\n"))); err != nil {
			m.log.Error("scholarship email failed to send",
				zap.String("email", logEmail), zap.Error(err))
			return err
		}
		m.log.Info("scholarship email sent", zap.String("email", logEmail), zap.String("subject", subject))
		return nil
	}
}
