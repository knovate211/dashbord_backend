package resolvers

import "html/template"

// The branded shell of the template-built emails — the hiring invitation and
// the scholarship applicant and award emails: a cream page, a white 600px card
// with a thin border. Each email supplies only the rows inside the card. Its
// data must provide Cream and Border (see the palette constants).
//
// The certification and referral emails use a different, plainer card and are
// assembled with fmt.Sprintf; see plainEmail below.
const brandedEmailTop = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:{{.Cream}};">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{.Cream}};padding:24px 12px;">
<tr><td align="center">
  <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border:1px solid {{.Border}};border-radius:10px;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;">
`

const brandedEmailBottom = `  </table>
</td></tr>
</table>
</body></html>`

// brandedEmail parses one email's card rows into the shared shell.
func brandedEmail(name, rows string) *template.Template {
	return template.Must(template.New(name).Parse(brandedEmailTop + rows + brandedEmailBottom))
}

// The plainer card used by the certification and referral emails: a
// beige page and a white 560px card. Each email formats only its card rows.
const plainEmailTop = `<!doctype html>
<html><body style="margin:0;padding:24px;background:#f7f3ec;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#2b2620;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;margin:0 auto;background:#ffffff;border-radius:12px;">`

// plainEmail wraps already-formatted card rows, which begin with their own
// newline; footer is any HTML that sits below the card (the certification
// email's small print).
func plainEmail(rows, footer string) string {
	return plainEmailTop + rows + "\n  </table>\n" + footer + "</body></html>"
}
