package alexactl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"

	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
	"github.com/Fishwaldo/alexactl/internal/oauth"
)

// oauthURLBuilder is a seam used by tests to point the OAuth flow at a
// stub server. Production uses buildOAuthURL directly.
type oauthURLBuilder func(verifier, clientID, language string) string

// doOAuthDance performs the GET → password POST → OTP POST sequence and
// returns the authorization code from the maplanding redirect URL.
//
// The HTTP client's cookie jar collects state cookies as the dance
// progresses; httpwrapper.Wrapper.Do also captures the CSRF token.
func (s *AuthService) doOAuthDance(
	ctx context.Context,
	email, password string,
	otpProvider OTPProvider,
	language string,
	urlBuilder oauthURLBuilder,
) (authcode, codeVerifier, serial string, _ error) {
	if urlBuilder == nil {
		urlBuilder = buildOAuthURL
	}

	codeVerifier, err := oauth.NewCodeVerifier(nil, 32)
	if err != nil {
		return "", "", "", err
	}

	serial, err = newSerialNumber()
	if err != nil {
		return "", "", "", err
	}
	clientID := buildClientID(serial)

	loginPageURL := urlBuilder(codeVerifier, clientID, language)

	logger := s.wrapper.Logger(ctx)

	// Step 1: GET the OAuth page → parse the email/password form.
	form, err := s.getForm(ctx, http.MethodGet, loginPageURL, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("auth: oauth get: %w", err)
	}
	logger.DebugContext(ctx, "alexactl: OAuth password page served")

	// Step 2: POST email + password to the form action.
	pwForm := mergeForm(form.Inputs, map[string]string{
		"email":    email,
		"password": password,
	})
	otpForm, err := s.getForm(ctx, form.Method, form.Action, pwForm)
	if err != nil {
		return "", "", "", fmt.Errorf("auth: password post: %w", err)
	}

	// Step 2a: handle Japanese-locale captcha by retrying with ja_JP.
	if otpForm.IsCaptcha && language != "ja_JP" {
		logger.DebugContext(ctx, "alexactl: captcha detected, retrying with ja_JP")
		s.wrapper.ClearCSRF()
		loginPageURL2 := urlBuilder(codeVerifier, clientID, "ja_JP")
		form2, err := s.getForm(ctx, http.MethodGet, loginPageURL2, nil)
		if err != nil {
			return "", "", "", fmt.Errorf("auth: oauth get (ja_JP): %w", err)
		}
		pwForm2 := mergeForm(form2.Inputs, map[string]string{
			"email":    email,
			"password": password,
		})
		otpForm, err = s.getForm(ctx, form2.Method, form2.Action, pwForm2)
		if err != nil {
			return "", "", "", fmt.Errorf("auth: password post (ja_JP): %w", err)
		}
	}

	if !otpForm.HasOTPField {
		return "", "", "", fmt.Errorf("%w: MFA OTP page not served (check credentials)",
			ErrCannotAuthenticate)
	}
	logger.DebugContext(ctx, "alexactl: MFA OTP page detected")

	// Step 3: invoke caller's OTPProvider — now that the password worked
	// and the SMS (if any) has been sent.
	otp, err := otpProvider.OTP(ctx)
	if err != nil {
		return "", "", "", err
	}

	otpFields := mergeForm(otpForm.Inputs, map[string]string{
		"otpCode":        otp,
		"mfaSubmit":      "Submit",
		"rememberDevice": "false",
	})

	// Step 4: POST OTP — the final response is a redirect to maplanding
	// with the authcode in the query string.
	finalURL, err := s.submitAndGetFinalURL(ctx, otpForm.Method, otpForm.Action, otpFields)
	if err != nil {
		return "", "", "", fmt.Errorf("auth: otp post: %w", err)
	}

	code, err := extractAuthCodeFromURL(finalURL)
	if err != nil {
		return "", "", "", err
	}
	return code, codeVerifier, serial, nil
}

// getForm performs an HTTP request and parses the response body as an
// Amazon login form.
func (s *AuthService) getForm(
	ctx context.Context,
	method, urlStr string,
	form map[string]string,
) (*oauth.LoginForm, error) {
	req := httpwrapper.Request{Method: method, URL: urlStr, Form: form}
	resp, err := s.wrapper.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		return nil, &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     method,
			StatusCode: resp.StatusCode,
			URL:        urlStr,
		}
	}
	var base *urlpkg.URL
	if resp.Request != nil {
		base = resp.Request.URL
	}
	return oauth.ParseLoginForm(resp.Body, base)
}

// submitAndGetFinalURL POSTs the form and returns the final URL after any
// redirects. The body is read and discarded.
func (s *AuthService) submitAndGetFinalURL(
	ctx context.Context,
	method, urlStr string,
	form map[string]string,
) (*urlpkg.URL, error) {
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{Method: method, URL: urlStr, Form: form})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.Request == nil || resp.Request.URL == nil {
		return nil, errors.New("auth: response has no request URL")
	}
	return resp.Request.URL, nil
}

func mergeForm(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
