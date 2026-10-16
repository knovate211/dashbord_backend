package resolvers

import (
	"bytes"
	"html/template"
	"time"
)

// hiringInviteHTML is the candidate's test invitation. It follows the
// scholarship email's rules (see scholarship_email_html.go): tables and inline
// styles only, everything interpolated through html/template, and the link
// repeated as text under the button for clients that strip buttons. The
// company and candidate names are recruiter-typed text, so they are escaped
// like any other untrusted input.
func hiringInviteHTML(name, company, testTitle, testURL string, duration, totalMarks int32, expires time.Time) string {
	var buf bytes.Buffer
	if err := hiringInviteTemplate.Execute(&buf, map[string]any{
		"Name":      name,
		"Company":   company,
		"TestTitle": testTitle,
		"TestURL":   testURL,
		"Duration":  duration,
		"Marks":     totalMarks,
		"Expires":   expires.Format("2 Jan 2006, 15:04 MST"),
		"Ink":       emailInk,
		"Muted":     emailMuted,
		"Gold":      emailGold,
		"Cream":     emailCream,
		"Border":    emailBorder,
	}); err != nil {
		// The plain-text part still carries the link.
		return ""
	}
	return buf.String()
}

var hiringInviteTemplate = template.Must(template.New("hiring").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:{{.Cream}};">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{.Cream}};padding:24px 12px;">
<tr><td align="center">
  <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border:1px solid {{.Border}};border-radius:10px;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;">

    <tr><td style="background:{{.Ink}};padding:26px 32px;">
      <div style="color:#ffffff;font-size:21px;font-weight:700;line-height:1.3;">{{.Company}} — Online Assessment</div>
      <div style="color:#b9b1a4;font-size:13px;padding-top:5px;">Powered by Knovate</div>
    </td></tr>

    <tr><td style="padding:30px 32px 8px;color:{{.Ink}};font-size:15px;line-height:1.6;">
      <p style="margin:0 0 14px;">Hi {{.Name}},</p>
      <p style="margin:0 0 18px;"><strong>{{.Company}}</strong> has invited you to take an online assessment as part of their hiring process.</p>

      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid {{.Border}};border-radius:8px;">
        <tr><td style="padding:12px 16px;border-bottom:1px solid {{.Border}};color:{{.Muted}};font-size:13px;width:40%;">Test</td>
            <td style="padding:12px 16px;border-bottom:1px solid {{.Border}};font-weight:600;">{{.TestTitle}}</td></tr>
        <tr><td style="padding:12px 16px;border-bottom:1px solid {{.Border}};color:{{.Muted}};font-size:13px;">Duration</td>
            <td style="padding:12px 16px;border-bottom:1px solid {{.Border}};">{{.Duration}} minutes</td></tr>
        <tr><td style="padding:12px 16px;color:{{.Muted}};font-size:13px;">Link valid until</td>
            <td style="padding:12px 16px;">{{.Expires}}</td></tr>
      </table>
    </td></tr>

    <tr><td align="center" style="padding:24px 32px 8px;">
      <a href="{{.TestURL}}" style="display:inline-block;background:{{.Gold}};color:#ffffff;text-decoration:none;font-weight:700;font-size:16px;padding:14px 34px;border-radius:8px;">Start your test</a>
    </td></tr>
    <tr><td style="padding:8px 32px 0;color:{{.Muted}};font-size:12px;line-height:1.5;word-break:break-all;">
      If the button does not work, open this link: <a href="{{.TestURL}}" style="color:{{.Gold}};">{{.TestURL}}</a>
    </td></tr>

    <tr><td style="padding:22px 32px 28px;color:{{.Muted}};font-size:13px;line-height:1.6;">
      <strong style="color:{{.Ink}};">Before you begin</strong><br>
      Use a laptop or desktop with a stable connection and set aside the full {{.Duration}} minutes in one sitting.
      The timer runs on our servers once you start, so closing the tab does not stop it.
      This link is personal to you — please do not share it.
    </td></tr>

  </table>
</td></tr>
</table>
</body></html>`))
