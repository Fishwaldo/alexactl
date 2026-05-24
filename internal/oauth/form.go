package oauth

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// LoginForm describes a parsed Amazon login page form. Only hidden inputs
// are captured into Inputs — callers fill email/password/otp explicitly
// before submitting the form to Action via Method.
type LoginForm struct {
	// Method is the HTML form method (typically "POST"), uppercased.
	Method string

	// Action is the absolute URL the form should POST/GET to.
	Action string

	// Inputs maps hidden input name → value as extracted from the form.
	Inputs map[string]string

	// HasOTPField is true if the page contains an <input id="auth-mfa-otpcode">
	// field. Used to detect that the password step succeeded and Amazon is
	// now prompting for MFA.
	HasOTPField bool

	// IsCaptcha is true if the page presents the cvf-aamation-challenge-form
	// captcha (observed for Japan users when language is set to English).
	IsCaptcha bool
}

// ParseLoginForm reads an Amazon login HTML page and extracts the primary
// form's method, action, hidden inputs, and a couple of state flags.
// Returns an error if no <form> element is present.
//
// The action attribute is resolved against base (typically the URL of the
// response that produced the HTML), so Amazon's relative or protocol-relative
// form actions emerge as absolute URLs in LoginForm.Action.
func ParseLoginForm(r io.Reader, base *url.URL) (*LoginForm, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("oauth: parse html: %w", err)
	}

	form := &LoginForm{
		Inputs: map[string]string{},
	}

	// Captcha challenge: detected before form selection
	if doc.Find("form#cvf-aamation-challenge-form").Length() > 0 {
		form.IsCaptcha = true
	}

	// MFA OTP field: detected before form selection
	if doc.Find("input#auth-mfa-otpcode").Length() > 0 {
		form.HasOTPField = true
	}

	// Prefer name="signIn" form; otherwise first <form>.
	sel := doc.Find(`form[name="signIn"]`)
	if sel.Length() == 0 {
		sel = doc.Find("form")
	}
	if sel.Length() == 0 {
		return nil, fmt.Errorf("oauth: no form element in document")
	}
	primary := sel.First()

	method, _ := primary.Attr("method")
	form.Method = strings.ToUpper(strings.TrimSpace(method))
	if form.Method == "" {
		form.Method = "GET"
	}

	action, _ := primary.Attr("action")
	actionStr := strings.TrimSpace(action)
	if actionStr == "" {
		form.Action = ""
	} else {
		actionURL, parseErr := url.Parse(actionStr)
		if parseErr != nil {
			return nil, fmt.Errorf("oauth: parse form action %q: %w", actionStr, parseErr)
		}
		if base != nil {
			actionURL = base.ResolveReference(actionURL)
		}
		form.Action = actionURL.String()
	}

	primary.Find("input").Each(func(_ int, s *goquery.Selection) {
		typ, _ := s.Attr("type")
		if !strings.EqualFold(typ, "hidden") {
			return
		}
		name, hasName := s.Attr("name")
		if !hasName {
			return
		}
		val, _ := s.Attr("value")
		form.Inputs[name] = val
	})

	return form, nil
}
