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
	TemplateNewMemberAlert  = "new_member_alert"
	TemplateMemberApproved  = "member_approved"
	TemplateBookingConfirm  = "booking_confirmation"
	TemplateBookingCancel   = "booking_cancelled"
	TemplateWaitlistPromote = "waitlist_promoted"
	TemplateGatheringUpdate = "gathering_updated"
	TemplateGatheringCancel = "gathering_cancelled"
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
	// Subjects can include admin-entered titles; never let them carry header breaks.
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
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

// Gathering is the member-facing description of a gathering.
type Gathering struct {
	Title, Date, Time, Place string
}

func (g Gathering) line() string {
	return fmt.Sprintf("%s — %s at %s, %s", g.Title, g.Date, g.Time, g.Place)
}

const memberFooter = "This is a service message about your The Hive Society membership. Questions? Write to hello@thehivesociety.ae."

// NewMemberAlert tells an admin that a member confirmed their email and awaits
// review. Only the first name and area are included; details stay in the admin page.
func NewMemberAlert(to, firstName, area, appURL string) Message {
	where := ""
	if area != "" {
		where = " from " + area
	}
	return render(to, "New member awaiting approval", content{
		Heading:     "A new member is waiting for you",
		Paragraphs:  []string{fmt.Sprintf("%s%s confirmed their email address and is waiting for approval.", firstName, where)},
		ButtonLabel: "Review in the admin page",
		ButtonURL:   strings.TrimRight(appURL, "/") + "/admin",
		Footer:      "You receive this because you are a The Hive Society administrator.",
	})
}

// MemberApproved welcomes an approved member.
func MemberApproved(to, name, appURL string) Message {
	return render(to, "Welcome to The Hive Society", content{
		Heading:     "Your membership is approved",
		Paragraphs:  []string{fmt.Sprintf("Hello %s,", name), "Your membership has been approved. You can now keep a place at our gatherings."},
		ButtonLabel: "See gatherings",
		ButtonURL:   strings.TrimRight(appURL, "/") + "/gatherings",
		Footer:      memberFooter,
	})
}

// BookingConfirmation confirms a kept place or a waitlist position.
func BookingConfirmation(to, name, appURL string, g Gathering, waitlisted bool) Message {
	if waitlisted {
		return render(to, "You are on the waitlist: "+g.Title, content{
			Heading:     "You are on the waitlist",
			Paragraphs:  []string{fmt.Sprintf("Hello %s,", name), g.line(), "This gathering is full. If a place opens, it goes to the next person in line and we will email you straight away."},
			ButtonLabel: "My gatherings", ButtonURL: strings.TrimRight(appURL, "/") + "/gatherings", Footer: memberFooter,
		})
	}
	return render(to, "Your place is kept: "+g.Title, content{
		Heading:     "Your place is kept",
		Paragraphs:  []string{fmt.Sprintf("Hello %s,", name), g.line(), "If you can no longer come, please cancel so someone on the waitlist can take your place."},
		ButtonLabel: "My gatherings", ButtonURL: strings.TrimRight(appURL, "/") + "/gatherings", Footer: memberFooter,
	})
}

// BookingCancelled confirms a member's own cancellation.
func BookingCancelled(to, name string, g Gathering) Message {
	return render(to, "Cancelled: "+g.Title, content{
		Heading:    "Your place is cancelled",
		Paragraphs: []string{fmt.Sprintf("Hello %s,", name), g.line(), "Your place has been released. Thank you for letting us know."},
		Footer:     memberFooter,
	})
}

// WaitlistPromoted tells a waitlisted member they now have a confirmed place.
func WaitlistPromoted(to, name, appURL string, g Gathering) Message {
	return render(to, "A place opened up: "+g.Title, content{
		Heading:     "Good news: you have a place",
		Paragraphs:  []string{fmt.Sprintf("Hello %s,", name), g.line(), "A place opened up and it is now yours. If you can no longer come, please cancel so the next person can join."},
		ButtonLabel: "My gatherings", ButtonURL: strings.TrimRight(appURL, "/") + "/gatherings", Footer: memberFooter,
	})
}

// GatheringUpdated tells members with a reservation that details changed.
func GatheringUpdated(to, name, appURL string, g Gathering) Message {
	return render(to, "Updated details: "+g.Title, content{
		Heading:     "Gathering details changed",
		Paragraphs:  []string{fmt.Sprintf("Hello %s,", name), "The details of a gathering you reserved have changed. The latest details are:", g.line()},
		ButtonLabel: "My gatherings", ButtonURL: strings.TrimRight(appURL, "/") + "/gatherings", Footer: memberFooter,
	})
}

// GatheringCancelled tells members that a gathering they reserved was cancelled.
func GatheringCancelled(to, name string, g Gathering) Message {
	return render(to, "Gathering cancelled: "+g.Title, content{
		Heading:    "This gathering is cancelled",
		Paragraphs: []string{fmt.Sprintf("Hello %s,", name), g.line(), "We are sorry: this gathering will not take place. Your reservation has been removed."},
		Footer:     memberFooter,
	})
}
