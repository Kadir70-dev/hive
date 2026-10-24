package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// Template names stored in the outbox.
const (
	TemplateVerifyEmail     = "verify_email"
	TemplateResetPassword   = "reset_password"
	TemplateAccountExists   = "account_exists"
	TemplatePasswordChanged = "password_changed"
)

var htmlLayout = template.Must(template.New("email").Parse(`<!doctype html>
<html><body style="margin:0;background:#F7F2EA;font-family:Helvetica,Arial,sans-serif;color:#22201A">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">
<table role="presentation" width="100%" style="max-width:520px;background:#FFFDF8;border-radius:20px;padding:32px" cellpadding="0" cellspacing="0">
<tr><td style="font-family:Georgia,serif;font-size:13px;letter-spacing:.2em;text-transform:uppercase;color:#3F6E5E">The Hive Society</td></tr>
<tr><td style="padding-top:20px;font-family:Georgia,serif;font-size:26px;line-height:1.2">{{.Heading}}</td></tr>
{{range .Paragraphs}}<tr><td style="padding-top:14px;font-size:15px;line-height:1.6;color:#4A463C">{{.}}</td></tr>{{end}}
{{if .ButtonURL}}<tr><td style="padding-top:24px"><a href="{{.ButtonURL}}" style="display:inline-block;background:#3F6E5E;color:#FFFFFF;text-decoration:none;padding:12px 22px;border-radius:999px;font-weight:600">{{.ButtonLabel}}</a></td></tr>
<tr><td style="padding-top:16px;font-size:12px;color:#6B6558;word-break:break-all">If the button does not work, copy this link: {{.ButtonURL}}</td></tr>{{end}}
<tr><td style="padding-top:24px;font-size:12px;color:#6B6558">{{.Footer}}</td></tr>
</table></td></tr></table></body></html>`))

type content struct {
	Heading, ButtonLabel, ButtonURL, Footer string
	Paragraphs                              []string
}

func render(to, subject string, c content) Message {
	var html bytes.Buffer
	_ = htmlLayout.Execute(&html, c)
	var text strings.Builder
	text.WriteString(c.Heading + "\n\n")
	for _, p := range c.Paragraphs {
		text.WriteString(p + "\n\n")
	}
	if c.ButtonURL != "" {
		text.WriteString(c.ButtonLabel + ": " + c.ButtonURL + "\n\n")
	}
	text.WriteString(c.Footer + "\n")
	return Message{To: to, Subject: subject, Text: text.String(), HTML: html.String()}
}

const footer = "You received this because of activity on your The Hive Society account. Questions? Write to hello@thehivesociety.ae."

// VerifyEmail renders the address-confirmation email. The token travels in the
// URL fragment so it never reaches server or CDN logs.
func VerifyEmail(to, name, appURL, token string, validHours int) Message {
	return render(to, "Confirm your email for The Hive Society", content{
		Heading:     "Confirm your email",
		Paragraphs:  []string{fmt.Sprintf("Hello %s,", name), fmt.Sprintf("Please confirm this is your email address. The link works once and expires in %d hours.", validHours)},
		ButtonLabel: "Confirm my email",
		ButtonURL:   strings.TrimRight(appURL, "/") + "/verify-email#token=" + token,
		Footer:      "If you did not create an account, you can ignore this email. " + footer,
	})
}

// ResetPassword renders the password reset email.
func ResetPassword(to, appURL, token string, validMinutes int) Message {
	return render(to, "Reset your The Hive Society password", content{
		Heading:     "Reset your password",
		Paragraphs:  []string{fmt.Sprintf("Someone asked to reset the password for this account. The link works once and expires in %d minutes.", validMinutes), "Resetting signs you out on every device."},
		ButtonLabel: "Choose a new password",
		ButtonURL:   strings.TrimRight(appURL, "/") + "/reset-password#token=" + token,
		Footer:      "If this was not you, ignore this email; your password stays the same. " + footer,
	})
}

// AccountExists is sent when someone signs up with an address that already has an account.
func AccountExists(to, appURL string) Message {
	return render(to, "You already have a The Hive Society account", content{
		Heading:     "You already have an account",
		Paragraphs:  []string{"Someone tried to create a new account with this email address. You can sign in, or reset your password if you have forgotten it."},
		ButtonLabel: "Reset my password",
		ButtonURL:   strings.TrimRight(appURL, "/") + "/reset-password",
		Footer:      "If this was not you, no action is needed. " + footer,
	})
}

// PasswordChanged confirms a completed reset.
func PasswordChanged(to string) Message {
	return render(to, "Your The Hive Society password was changed", content{
		Heading:    "Your password was changed",
		Paragraphs: []string{"Your password was just reset and every device was signed out.", "If you did not do this, contact hello@thehivesociety.ae straight away."},
		Footer:     footer,
	})
}
