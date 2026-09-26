package resolvers

import (
	"fmt"
	"html/template"
	"regexp"
)

// Referral codes are uppercase letters and digits only — the alphabet the
// generator uses, so anything else is a typo or a probe.
var referralCodeRe = regexp.MustCompile(`^[A-Z0-9]+$`)

// nonAlnum strips everything but letters and digits when building a code stem
// from somebody's name.
var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// referralLinkHTML is the email a new referrer gets. Table layout and inline
// styles, like every other template here: mail clients are the one place where
// 2005 HTML is still correct. Every value is escaped — names come from a public
// form.
func referralLinkHTML(name, link, code string, p referralProgram) string {
	esc := template.HTMLEscapeString
	return fmt.Sprintf(`<!doctype html>
<html><body style="margin:0;padding:24px;background:#f7f3ec;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#2b2620;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;margin:0 auto;background:#ffffff;border-radius:12px;">
    <tr><td style="padding:32px 32px 8px;">
      <p style="margin:0 0 4px;font-size:12px;letter-spacing:.12em;text-transform:uppercase;color:#b5701f;font-weight:700;">Knovate referrals</p>
      <h1 style="margin:0;font-size:24px;line-height:1.25;color:#2b2620;">Your referral link is ready</h1>
    </td></tr>
    <tr><td style="padding:12px 32px 0;font-size:15px;line-height:1.6;color:#4a443c;">
      <p style="margin:0 0 14px;">Hi %s,</p>
      <p style="margin:0 0 14px;">Share this link with anyone thinking about a course or a certification exam:</p>
    </td></tr>
    <tr><td style="padding:4px 32px 12px;">
      <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f7f3ec;border-radius:10px;">
        <tr><td style="padding:16px 18px;font-size:15px;color:#b5701f;word-break:break-all;">%s</td></tr>
      </table>
      <p style="margin:10px 0 0;font-size:13px;color:#6c6455;">Your code: <strong style="color:#2b2620;">%s</strong></p>
    </td></tr>
    <tr><td style="padding:8px 32px 0;font-size:15px;line-height:1.6;color:#4a443c;">
      <p style="margin:0 0 10px;"><strong style="color:#2b2620;">What they get:</strong> %.0f%% off their first purchase.</p>
      <p style="margin:0 0 14px;"><strong style="color:#2b2620;">What you get:</strong> ₹%d when they enrol on a course, ₹%d for a certification exam — paid by UPI once we have confirmed the purchase.</p>
    </td></tr>
    <tr><td style="padding:8px 32px 32px;font-size:13px;line-height:1.6;color:#6c6455;">
      <p style="margin:0;">Check your referrals any time with your code and this email address. We review each referral before paying, so rewards are not instant.</p>
    </td></tr>
  </table>
</body></html>`,
		esc(name), esc(link), esc(code), p.FriendDiscountPercent, p.CourseRewardPaise/100, p.ExamRewardPaise/100)
}

// referralEarnedHTML tells a referrer a reward is owed. It deliberately says
// "we check each referral" rather than implying money is already moving.
func referralEarnedHTML(name, friendFirstName, itemName string, rewardRupees int64) string {
	esc := template.HTMLEscapeString
	return fmt.Sprintf(`<!doctype html>
<html><body style="margin:0;padding:24px;background:#f7f3ec;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#2b2620;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;margin:0 auto;background:#ffffff;border-radius:12px;">
    <tr><td style="padding:32px 32px 8px;">
      <p style="margin:0 0 4px;font-size:12px;letter-spacing:.12em;text-transform:uppercase;color:#b5701f;font-weight:700;">Knovate referrals</p>
      <h1 style="margin:0;font-size:24px;line-height:1.25;color:#2b2620;">You earned ₹%d</h1>
    </td></tr>
    <tr><td style="padding:12px 32px 32px;font-size:15px;line-height:1.6;color:#4a443c;">
      <p style="margin:0 0 14px;">Hi %s,</p>
      <p style="margin:0 0 14px;">%s just enrolled in <strong style="color:#2b2620;">%s</strong> using your referral link.</p>
      <p style="margin:0;color:#6c6455;font-size:13px;">We check each referral before paying, and send payouts by UPI. Nothing more for you to do.</p>
    </td></tr>
  </table>
</body></html>`, rewardRupees, esc(name), esc(friendFirstName), esc(itemName))
}
