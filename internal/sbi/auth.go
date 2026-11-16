package sbi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var errAuthentication = errors.New("SBI authentication failed")

const maxOAuthResponseBytes = int64(64 << 10)

type authFunc func(context.Context, *http.Request) error

type oauthTokenSource struct {
	profile OAuthProfile
	client  *http.Client
	timeout time.Duration

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newOAuthTokenSource(profile OAuthProfile, client *http.Client, timeout time.Duration) *oauthTokenSource {
	return &oauthTokenSource{profile: profile, client: client, timeout: timeout}
}

func (s *oauthTokenSource) bearerToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if s.token != "" && now.Add(tokenRefreshSkew(s.expiresAt.Sub(now))).Before(s.expiresAt) {
		return s.token, nil
	}
	token, expiresAt, err := s.fetch(ctx)
	if err != nil {
		return "", err
	}
	s.token = token
	s.expiresAt = expiresAt
	return token, nil
}

func tokenRefreshSkew(lifetime time.Duration) time.Duration {
	skew := 30 * time.Second
	if lifetime/10 < skew {
		skew = lifetime / 10
	}
	if skew < 0 {
		return 0
	}
	return skew
}

func (s *oauthTokenSource) fetch(ctx context.Context) (string, time.Time, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestContext, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	form := url.Values{"grant_type": {"client_credentials"}}
	if len(s.profile.Scopes) > 0 {
		form.Set("scope", strings.Join(s.profile.Scopes, " "))
	}
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		s.profile.TokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", time.Time{}, errAuthentication
	}
	request.SetBasicAuth(s.profile.ClientID, s.profile.ClientSecret)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "ca-client-tools-sbi/1")

	response, err := s.client.Do(request)
	if err != nil {
		if requestContext.Err() != nil {
			return "", time.Time{}, requestContext.Err()
		}
		return "", time.Time{}, errAuthentication
	}
	defer response.Body.Close()
	if response.ProtoMajor != 2 || response.StatusCode != http.StatusOK {
		return "", time.Time{}, errAuthentication
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthResponseBytes+1))
	if err != nil || int64(len(raw)) > maxOAuthResponseBytes {
		return "", time.Time{}, errAuthentication
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.Unmarshal(raw, &payload) != nil ||
		payload.AccessToken == "" ||
		strings.ContainsAny(payload.AccessToken, " \t\r\n") ||
		!strings.EqualFold(payload.TokenType, "Bearer") ||
		payload.ExpiresIn <= 0 || payload.ExpiresIn > int64((30*24*time.Hour)/time.Second) {
		return "", time.Time{}, errAuthentication
	}
	return payload.AccessToken, time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second), nil
}
