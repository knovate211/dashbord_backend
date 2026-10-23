package resolvers

import (
	"bytes"

	"go.uber.org/zap"
)

// HTML bodies for the candidate-facing emails.
//
// Since the email is now the only way into the test, it has to survive being
// read anywhere — so these are built the way email actually works rather than
// the way a web page does: tables for layout, inline styles, no external CSS
// and no images. Gmail strips <style> blocks in some views and Outlook renders
// through Word; a div-and-class layout that looks right in a browser preview
// falls apart in both.
//
// Everything interpolated goes through html/template. The applicant's name
// comes from a public form, so it is attacker-controlled text arriving in a
// document we send to ourselves and to staff.

const (
	emailInk    = "#2b2620"
	emailMuted  = "#6c6455"
	emailGold   = "#c98a3a"
	emailCream  = "#f7f3ec"
	emailBorder = "#e4dccd"
)

type applicantMail struct {
	Name         string
	CourseName   string
	TestTitle    string
	TestURL      string
	Duration     int32
	TotalMarks   int32
	IsNewAccount bool
	Ink          string
	Muted        string
	Gold         string
	Cream        string
	Border       string
}

// applicantTemplate mirrors the shape candidates already recognise from other
// assessment invitations: a banded header, what the test is, what to expect,
// then one unmissable button with a copyable link underneath for the clients
// that strip buttons.
var applicantTemplate = brandedEmail("applicant", `
    <tr><td style="background:{{.Ink}};padding:26px 32px;">
      <div style="color:#ffffff;font-size:21px;font-weight:700;line-height:1.3;">{{.CourseName}} Scholarship Test</div>
      <div style="color:#b9b1a4;font-size:13px;padding-top:5px;">Powered by Knovate</div>
    </td></tr>

    <tr><td style="padding:32px;color:{{.Ink}};font-size:15px;line-height:1.65;">
      <p style="margin:0 0 16px;">Hi {{.Name}},</p>

      <p style="margin:0 0 16px;">
        Your application for the <strong>{{.CourseName}}</strong> scholarship has been received,
        and as the next step you have this assessment.
      </p>

      <p style="margin:0 0 20px;">
        It is <strong>{{.Duration}} minutes</strong> long and must be attempted in one sitting
        <strong>(without pauses or breaks)</strong>.
      </p>

      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid {{.Border}};border-radius:8px;margin:0 0 22px;">
        <tr><td style="padding:14px 18px;font-size:14px;color:{{.Muted}};">
          <div style="padding:3px 0;"><span style="display:inline-block;width:92px;color:{{.Muted}};">Test</span><strong style="color:{{.Ink}};">{{.TestTitle}}</strong></div>
          <div style="padding:3px 0;"><span style="display:inline-block;width:92px;color:{{.Muted}};">Duration</span><strong style="color:{{.Ink}};">{{.Duration}} minutes</strong></div>
          <div style="padding:3px 0;"><span style="display:inline-block;width:92px;color:{{.Muted}};">Marks</span><strong style="color:{{.Ink}};">{{.TotalMarks}}</strong></div>
          <div style="padding:3px 0;"><span style="display:inline-block;width:92px;color:{{.Muted}};">Format</span><strong style="color:{{.Ink}};">Multiple choice and live coding</strong></div>
        </td></tr>
      </table>

      <p style="margin:0 0 10px;font-weight:600;">Things to keep in mind about the test:</p>
      <ul style="margin:0 0 24px;padding-left:20px;color:{{.Muted}};font-size:14px;line-height:1.7;">
        <li>The coding questions test fundamentals, not competitive-programming tricks — no dynamic programming or graph algorithms.</li>
        <li>Make sure you have <em>stable power and an internet connection</em> before you start.</li>
        <li>Use a laptop or desktop. The code editor needs the screen space.</li>
        <li>The timer runs on our servers, so closing the tab does not stop it.</li>
      </ul>

      <table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 18px;">
        <tr><td align="center" style="background:{{.Gold}};border-radius:7px;">
          <a href="{{.TestURL}}" style="display:inline-block;padding:13px 34px;color:#ffffff;font-size:15px;font-weight:700;text-decoration:none;">Start Test</a>
        </td></tr>
      </table>

      <p style="margin:0 0 22px;font-size:13px;color:{{.Muted}};">
        Or copy this link into your browser:<br>
        <a href="{{.TestURL}}" style="color:{{.Gold}};word-break:break-all;">{{.TestURL}}</a>
      </p>

      <p style="margin:0 0 6px;font-size:13.5px;color:{{.Muted}};">
        <strong style="color:{{.Ink}};">This link is personal to you and works for the next 3 days.</strong>
        The clock only starts when you open the test, so there is no rush to click it now — but do
        set aside the full {{.Duration}} minutes when you do.
      </p>

      {{if .IsNewAccount}}
      <p style="margin:16px 0 0;padding-top:16px;border-top:1px solid {{.Border}};font-size:13px;color:{{.Muted}};">
        We have created an account for you. You do not need a password to start the test — use the
        button above. To sign in later, choose &ldquo;Forgot password&rdquo; on the login page to set one.
      </p>
      {{end}}
    </td></tr>

    <tr><td style="background:{{.Cream}};padding:16px 32px;font-size:12px;color:{{.Muted}};border-top:1px solid {{.Border}};">
      Good luck.<br>— Team Knovate
    </td></tr>

`)

func applicantHTML(name, courseName, testTitle, testURL string, duration, totalMarks int32, isNewAccount bool) string {
	var buf bytes.Buffer
	data := applicantMail{
		Name: name, CourseName: courseName, TestTitle: testTitle, TestURL: testURL,
		Duration: duration, TotalMarks: totalMarks, IsNewAccount: isNewAccount,
		Ink: emailInk, Muted: emailMuted, Gold: emailGold, Cream: emailCream, Border: emailBorder,
	}
	if err := applicantTemplate.Execute(&buf, data); err != nil {
		// The plain-text part is already built and will still be sent, so a
		// template fault degrades the email rather than losing it.
		zap.L().Error("render scholarship applicant email", zap.Error(err))
		return ""
	}
	return buf.String()
}

var awardTemplate = brandedEmail("award", `    <tr><td style="background:{{.Ink}};padding:26px 32px;">
      <div style="color:#ffffff;font-size:21px;font-weight:700;">Knovate Scholarship</div>
      <div style="color:#b9b1a4;font-size:13px;padding-top:5px;">{{.CourseName}}</div>
    </td></tr>
    <tr><td style="padding:32px;color:{{.Ink}};font-size:15px;line-height:1.65;">
      <p style="margin:0 0 20px;">Hi {{.Name}},</p>
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 22px;">
        <tr><td align="center" style="border:1px solid {{.Gold}};border-radius:9px;padding:22px;">
          <div style="font-size:40px;font-weight:700;color:{{.Gold}};line-height:1;">{{.AwardPercent}}%</div>
          <div style="font-size:14px;color:{{.Muted}};padding-top:6px;">off your course fee</div>
        </td></tr>
      </table>
      <p style="margin:0 0 16px;">
        You have been awarded a <strong>{{.AwardPercent}}% scholarship</strong> on {{.CourseName}}.
      </p>
      <p style="margin:0 0 16px;color:{{.Muted}};font-size:14px;">
        The discount is applied to your fee — there is nothing you need to do to claim it. One of
        our counsellors will call you shortly to confirm your batch and take you through enrolment.
      </p>
      <p style="margin:0;">Congratulations, and welcome.</p>
    </td></tr>
    <tr><td style="background:{{.Cream}};padding:16px 32px;font-size:12px;color:{{.Muted}};border-top:1px solid {{.Border}};">— Team Knovate</td></tr>
`)

func awardHTML(name, courseName string, awardPercent int) string {
	var buf bytes.Buffer
	data := struct {
		Name, CourseName                string
		AwardPercent                    int
		Ink, Muted, Gold, Cream, Border string
	}{name, courseName, awardPercent, emailInk, emailMuted, emailGold, emailCream, emailBorder}
	if err := awardTemplate.Execute(&buf, data); err != nil {
		zap.L().Error("render scholarship award email", zap.Error(err))
		return ""
	}
	return buf.String()
}
