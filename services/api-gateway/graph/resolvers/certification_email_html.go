package resolvers

import (
	"fmt"
	"html/template"
	"strings"
)

// certificationLinkHTML is the HTML half of the exam-link email.
//
// Table layout and inline styles, like the scholarship templates: mail clients
// remain the one place where 2005 HTML is still the correct answer. Every value
// is escaped — the name comes from a public form.
func certificationLinkHTML(name, examTitle, examURL, expiresOn string) string {
	esc := template.HTMLEscapeString
	return fmt.Sprintf(`<!doctype html>
<html><body style="margin:0;padding:24px;background:#f7f3ec;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#2b2620;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;margin:0 auto;background:#ffffff;border-radius:12px;">
    <tr><td style="padding:32px 32px 8px;">
      <p style="margin:0 0 4px;font-size:12px;letter-spacing:.12em;text-transform:uppercase;color:#b5701f;font-weight:700;">Knovate Certification</p>
      <h1 style="margin:0;font-size:24px;line-height:1.25;color:#2b2620;">Your exam is ready</h1>
    </td></tr>
    <tr><td style="padding:12px 32px 0;font-size:15px;line-height:1.6;color:#4a443c;">
      <p style="margin:0 0 14px;">Hi %s,</p>
      <p style="margin:0 0 14px;">Your payment is confirmed and your <strong>%s</strong> exam is ready to sit.</p>
    </td></tr>
    <tr><td style="padding:20px 32px;">
      <a href="%s" style="display:inline-block;background:#c98a3a;color:#ffffff;text-decoration:none;font-weight:600;padding:13px 26px;border-radius:8px;">Start the exam</a>
      <p style="margin:12px 0 0;font-size:13px;color:#6c6455;">Valid until %s. The link is personal to you — please do not share it.</p>
    </td></tr>
    <tr><td style="padding:0 32px 8px;">
      <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f7f3ec;border-radius:10px;">
        <tr><td style="padding:16px 18px;font-size:14px;line-height:1.6;color:#4a443c;">
          <strong style="color:#2b2620;">Before you begin</strong>
          <ul style="margin:8px 0 0;padding-left:18px;">
            <li>Set aside the full duration in one sitting — the timer does not pause.</li>
            <li>Use a laptop or desktop with a working camera and a stable connection.</li>
            <li>The exam runs in fullscreen and is proctored; leaving the tab is recorded.</li>
          </ul>
        </td></tr>
      </table>
    </td></tr>
    <tr><td style="padding:16px 32px 32px;font-size:14px;line-height:1.6;color:#4a443c;">
      <p style="margin:0 0 14px;">Pass and your certificate is issued straight away, with a link an employer can verify.</p>
      <p style="margin:0;color:#6c6455;font-size:13px;">If the button does not work, paste this into your browser:<br>
        <span style="word-break:break-all;color:#b5701f;">%s</span></p>
    </td></tr>
  </table>
  <p style="max-width:560px;margin:16px auto 0;font-size:12px;color:#8a8175;text-align:center;">You are receiving this because you registered for a Knovate certification exam.</p>
</body></html>`,
		esc(name), esc(examTitle), esc(examURL), esc(expiresOn), esc(examURL))
}

// certificationResendText is the plain-text body used when staff resend a link.
func certificationResendText(name, examTitle, examURL, expiresOn string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\n", name)
	fmt.Fprintf(&b, "Here is your link for the %s exam again.\n\n", examTitle)
	fmt.Fprintf(&b, "Start the exam: %s\n\n", examURL)
	fmt.Fprintf(&b, "It is valid until %s. The link is personal to you — please do not share it.\n\nKnovate", expiresOn)
	return b.String()
}
