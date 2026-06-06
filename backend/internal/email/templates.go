package email

import (
	"bytes"
	"embed"
	htmltpl "html/template"
	"strings"
	texttpl "text/template"
)

//go:embed templates
var templateFS embed.FS

// verifyEmailData is the template data for verification emails.
type verifyEmailData struct {
	UserName  string
	VerifyURL string
	ExpiresIn string
	AppName   string
}

// resetPasswordData is the template data for password-reset emails.
type resetPasswordData struct {
	UserName  string
	ResetURL  string
	ExpiresIn string
	AppName   string
}

// renderVerifyEmail renders the verification email templates. Returns
// (subject, textBody, htmlBody, err). On error all string fields are empty.
func renderVerifyEmail(data verifyEmailData) (subject, textBody, htmlBody string, err error) {
	return renderPair("templates/verify_email.txt", "templates/verify_email.html", data)
}

// renderPasswordReset renders the password-reset email templates.
func renderPasswordReset(data resetPasswordData) (subject, textBody, htmlBody string, err error) {
	return renderPair("templates/password_reset.txt", "templates/password_reset.html", data)
}

// notificationEventData is the template data for notification event emails.
type notificationEventData struct {
	UserName         string
	EventLabel       string
	NotificationsURL string
	AppName          string
}

// renderNotificationEvent renders the notification event email templates.
func renderNotificationEvent(data notificationEventData) (subject, textBody, htmlBody string, err error) {
	return renderPair("templates/notification_event.txt", "templates/notification_event.html", data)
}

// renderPair renders a matched .txt + .html template pair.
// The subject is the first non-empty line of the rendered text body.
func renderPair(txtPath, htmlPath string, data any) (subject, textBody, htmlBody string, err error) {
	// Render text template.
	txtSrc, err := templateFS.ReadFile(txtPath)
	if err != nil {
		return "", "", "", err
	}
	tt, err := texttpl.New("text").Parse(string(txtSrc))
	if err != nil {
		return "", "", "", err
	}
	var textBuf bytes.Buffer
	if err = tt.Execute(&textBuf, data); err != nil {
		return "", "", "", err
	}
	rendered := textBuf.String()

	// Extract subject from the first non-empty line.
	for _, line := range strings.Split(rendered, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			subject = line
			break
		}
	}
	// Text body is everything after the subject line + blank separator.
	if idx := strings.Index(rendered, "\n"); idx >= 0 {
		textBody = strings.TrimLeft(rendered[idx:], "\n")
	} else {
		textBody = rendered
	}

	// Render HTML template.
	htmlSrc, err := templateFS.ReadFile(htmlPath)
	if err != nil {
		return "", "", "", err
	}
	ht, err := htmltpl.New("html").Parse(string(htmlSrc))
	if err != nil {
		return "", "", "", err
	}
	var htmlBuf bytes.Buffer
	if err = ht.Execute(&htmlBuf, data); err != nil {
		return "", "", "", err
	}
	htmlBody = htmlBuf.String()

	return subject, textBody, htmlBody, nil
}
