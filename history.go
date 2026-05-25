package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// backendRefreshWait is how long HistoryService.Vocal waits
// before issuing the history POST, to let Amazon's backend propagate
// recent records. Mirrors upstream BACKEND_REFRESH_WAIT_SECONDS.
const backendRefreshWait = 2 * time.Second

// HistoryService fetches the user's vocal history (the last 7 days of
// utterances) per device. Backed by /alexa-privacy/apd/rah (POST,
// bearer-authed) with a CSRF token scraped from /alexa-privacy/apd/rvh.
//
// Safe for concurrent use.
type HistoryService struct {
	wrapper *httpwrapper.Wrapper

	mu        sync.RWMutex
	session   *Session
	csrfToken string

	// Test seams. Production leaves these nil.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

func (s *HistoryService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	// Clear any cached CSRF token - a new session implies a new
	// website-cookies set, so the old CSRF may be invalid.
	s.csrfToken = ""
	s.mu.Unlock()
}

// nowFn returns the current time, honoring the test seam if set.
func (s *HistoryService) nowFn() time.Time {
	s.mu.RLock()
	fn := s.now
	s.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return time.Now()
}

// sleepFn pauses for d unless ctx is cancelled. Honors the test seam
// (which can return immediately for fast tests).
func (s *HistoryService) sleepFn(ctx context.Context, d time.Duration) error {
	s.mu.RLock()
	fn := s.sleep
	s.mu.RUnlock()
	if fn != nil {
		return fn(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Vocal returns the latest captured utterance per device serial,
// covering the last 7 days. Utterance types ASR_TIMEOUT,
// DEVICE_ARBITRATION, NO_EXPRESSED_INTENT, and WAKE_WORD_ONLY are
// filtered out before construction.
func (s *HistoryService) Vocal(ctx context.Context) (map[string]*VocalRecord, error) {
	if err := s.sleepFn(ctx, backendRefreshWait); err != nil {
		return nil, err
	}
	historyJSON, err := s.vocalHistoryJSON(ctx)
	if err != nil {
		return nil, err
	}

	out := map[string]*VocalRecord{}
	records, _ := historyJSON["alexaHistoryRecords"].([]any)
	for _, ri := range records {
		rec, _ := ri.(map[string]any)
		if rec == nil {
			continue
		}
		ut, _ := rec["utteranceType"].(string)
		switch ut {
		case "ASR_TIMEOUT", "DEVICE_ARBITRATION", "NO_EXPRESSED_INTENT", "WAKE_WORD_ONLY":
			continue
		}
		di, _ := rec["deviceInfo"].(map[string]any)
		serial, _ := di["deviceSerialNumber"].(string)
		if serial == "" {
			continue
		}
		tsMs, ok := numberAsInt64(rec["timestamp"])
		if !ok {
			continue
		}
		ts := time.UnixMilli(tsMs).UTC()
		intent, _ := rec["intent"].(string)
		title, _ := rec["title"].(string)
		subTitle, _ := rec["subTitle"].(string)
		entry := &VocalRecord{
			Timestamp:     ts,
			UtteranceType: ut,
			Intent:        intent,
			Title:         title,
			SubTitle:      subTitle,
		}
		if existing, present := out[serial]; !present || entry.Timestamp.After(existing.Timestamp) {
			out[serial] = entry
		}
	}
	return out, nil
}

// vocalHistoryJSON does the CSRF + bearer-POST dance and returns the
// decoded JSON body.
func (s *HistoryService) vocalHistoryJSON(ctx context.Context) (map[string]any, error) {
	if err := s.ensureCSRFToken(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	sess := s.session
	csrf := s.csrfToken
	s.mu.RUnlock()

	if sess == nil || sess.AccessToken == "" {
		return nil, fmt.Errorf("%w: no access token", ErrCannotAuthenticate)
	}

	now := s.nowFn().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(),
		12, 0, 0, 0, time.UTC)
	endMS := now.UnixMilli()
	startMS := startOfDay.AddDate(0, 0, -7).UnixMilli()

	url := fmt.Sprintf(
		"https://www.amazon.%s%s?startTime=%d&endTime=%d",
		s.wrapper.Domain(), amzconst.URIHistoryData, startMS, endMS,
	)

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodPost,
		URL:    url,
		JSONBody: map[string]any{
			"previousRequestToken": nil,
		},
		ExtraHeaders: http.Header{
			"Authorization":  []string{"Bearer " + sess.AccessToken},
			amzconst.CSRFA2Z: []string{csrf},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	if resp.StatusCode >= 400 {
		body := resp.Body
		defer body.Close()
		return nil, &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodPost,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}
	return data, nil
}

// ensureCSRFToken populates s.csrfToken if it is empty. Idempotent.
func (s *HistoryService) ensureCSRFToken(ctx context.Context) error {
	s.mu.RLock()
	have := s.csrfToken != ""
	s.mu.RUnlock()
	if have {
		return nil
	}
	tok, err := s.fetchCSRFToken(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.csrfToken = tok
	s.mu.Unlock()
	return nil
}

// fetchCSRFToken scrapes <meta name="csrf-token" content="..."> from the
// alexa-privacy frontend HTML page.
func (s *HistoryService) fetchCSRFToken(ctx context.Context) (string, error) {
	url := fmt.Sprintf("https://www.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URIHistoryFrontend)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodGet,
		URL:    url,
	})
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodGet,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: history frontend parse: %w",
			ErrCannotRetrieveData, err)
	}
	tok, ok := doc.Find(`meta[name="csrf-token"]`).Attr("content")
	if !ok || tok == "" {
		return "", fmt.Errorf("%w: history frontend missing csrf-token meta",
			ErrCannotRetrieveData)
	}
	return tok, nil
}
