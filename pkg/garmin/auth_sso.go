package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Native Garmin "DI" credential login (email / password / 2FA), reproduced
// from the python-garminconnect strategy cascade. It mints the same DI tokens
// (di_token / di_refresh_token / di_client_id).
//
// Several login strategies and token client ids are tried because individual
// ones get rate-limited / 409'd / Cloudflare-challenged. We reproduce the JSON
// flows (mobile + portal), the DI token exchange fallback over multiple client
// ids (the 409-prone step), and the MFA verify fallback over both buckets.
//
// The web "widget" HTML flow (CSRF scrape) is intentionally skipped: it needs
// TLS impersonation (curl_cffi) to pass Cloudflare, which net/http cannot do.

const (
	defaultSSOBase = "https://sso.garmin.com"

	diGrantType   = "https://connectapi.garmin.com/di-oauth2-service/oauth/grant/service_ticket"
	iosServiceURL = "https://mobile.integration.garmin.com/gcm/ios"
	portalSvcURL  = "https://connect.garmin.com/app"

	iosLoginUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"
	browserUA  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// diClientIDs are tried in order during the service-ticket → DI token
// exchange; some return 409/4xx, so we fall through to the next (the python
// lib does the same). The first that yields a token wins.
var diClientIDs = []string{
	"GARMIN_CONNECT_MOBILE_ANDROID_DI_2025Q2",
	"GARMIN_CONNECT_MOBILE_ANDROID_DI_2024Q4",
	"GARMIN_CONNECT_MOBILE_ANDROID_DI",
	"GARMIN_CONNECT_MOBILE_IOS_DI",
}

// LoginOption configures the login flow (mostly test hooks).
type LoginOption func(*loginConfig)

type loginConfig struct {
	ssoBase  string
	tokenURL string
	logger   *slog.Logger
	client   *http.Client // template; a cookie jar is attached per strategy
}

// WithLoginSSOBaseURL overrides https://sso.garmin.com (tests).
func WithLoginSSOBaseURL(u string) LoginOption {
	return func(c *loginConfig) { c.ssoBase = strings.TrimSuffix(u, "/") }
}

// WithLoginTokenURL overrides the diauth token endpoint (tests).
func WithLoginTokenURL(u string) LoginOption {
	return func(c *loginConfig) { c.tokenURL = u }
}

// WithLoginLogger sets a logger for the login flow (default: discard).
func WithLoginLogger(l *slog.Logger) LoginOption {
	return func(c *loginConfig) { c.logger = l }
}

// WithLoginHTTPClient sets the base HTTP client used by the login flow. A
// fresh cookie jar is attached per strategy attempt.
func WithLoginHTTPClient(hc *http.Client) LoginOption {
	return func(c *loginConfig) { c.client = hc }
}

func newLoginConfig(opts []LoginOption) *loginConfig {
	cfg := &loginConfig{
		ssoBase:  defaultSSOBase,
		tokenURL: DefaultTokenURL,
		logger:   slog.New(slog.DiscardHandler),
		client:   &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// newSSOClient clones the template client with a fresh cookie jar.
func (cfg *loginConfig) newSSOClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	hc := *cfg.client
	hc.Jar = jar
	return &hc, nil
}

// loginStrategy is one credential-submission flow (a JSON SSO endpoint).
type loginStrategy struct {
	flow       string // "mobile" | "portal"
	clientID   string // SSO clientId query param
	serviceURL string // SSO service query param (also used for the DI exchange)
	userAgent  string
	signinGET  string // optional GET to seed cookies before the POST; "" = none
	loginURL   string
	mfaURL     string
}

func (cfg *loginConfig) strategies() []loginStrategy {
	return []loginStrategy{
		{
			flow: "mobile", clientID: "GCM_IOS_DARK", serviceURL: iosServiceURL, userAgent: iosLoginUA,
			loginURL: cfg.ssoBase + "/mobile/api/login", mfaURL: cfg.ssoBase + "/mobile/api/mfa/verifyCode",
		},
		{
			flow: "portal", clientID: "GarminConnect", serviceURL: portalSvcURL, userAgent: browserUA,
			signinGET: cfg.ssoBase + "/portal/sso/en-US/sign-in",
			loginURL:  cfg.ssoBase + "/portal/api/login", mfaURL: cfg.ssoBase + "/portal/api/mfa/verifyCode",
		},
	}
}

// MFAChallenge carries the state needed to finish a 2FA login on a later
// request (no server-side state): the SSO flow, its cookies, the MFA method
// and the service URL for the DI exchange. It marshals small enough to
// round-trip in an encrypted browser cookie.
type MFAChallenge struct {
	Flow       string `json:"f"`
	Method     string `json:"m"`
	ServiceURL string `json:"s"`
	Cookies    string `json:"c"`
}

// ssoResponse is the JSON returned by the mobile/portal SSO login + MFA
// endpoints.
type ssoResponse struct {
	ResponseStatus struct {
		Type string `json:"type"`
	} `json:"responseStatus"`
	ServiceTicketID string `json:"serviceTicketId"`
	CustomerMfaInfo struct {
		MfaLastMethodUsed string `json:"mfaLastMethodUsed"`
	} `json:"customerMfaInfo"`
}

// Login submits the credentials through the SSO strategies in order. It
// returns the DI credentials on success, or a non-nil *MFAChallenge when the
// account needs a 2FA code (finish via ResumeMFA). ErrInvalidCredentials is
// returned for bad credentials; transient per-strategy failures (429,
// Cloudflare challenges) fall through to the next strategy.
func Login(ctx context.Context, email, password string, opts ...LoginOption) (*Credentials, *MFAChallenge, error) {
	cfg := newLoginConfig(opts)
	var lastErr error
	for _, st := range cfg.strategies() {
		client, err := cfg.newSSOClient()
		if err != nil {
			return nil, nil, err
		}
		if st.signinGET != "" {
			// Seed cookies (best-effort; ignore failures).
			seed, err := http.NewRequestWithContext(ctx, http.MethodGet, st.signinGET+"?"+url.Values{
				"clientId": {st.clientID}, "service": {st.serviceURL},
			}.Encode(), nil)
			if err == nil {
				seed.Header.Set("User-Agent", st.userAgent)
				if resp, e := client.Do(seed); e == nil {
					_ = resp.Body.Close()
				}
			}
		}

		res, status, err := ssoPostJSON(ctx, client, st.loginURL, st, map[string]any{
			"username": email, "password": password, "rememberMe": true, "captchaToken": "",
		})
		if err != nil {
			cfg.logger.Warn("garmin login attempt failed", "flow", st.flow, "status", status, "err", err)
			lastErr = fmt.Errorf("%s: %w", st.flow, err)
			continue // transient (429/403/non-JSON) — try next strategy
		}
		cfg.logger.Debug("garmin login response", "flow", st.flow, "status", status, "type", res.ResponseStatus.Type)

		switch res.ResponseStatus.Type {
		case "SUCCESSFUL":
			creds, err := exchangeServiceTicket(ctx, cfg, client, res.ServiceTicketID, st.serviceURL)
			if err != nil {
				lastErr = fmt.Errorf("%s: %w", st.flow, err)
				continue
			}
			return creds, nil, nil
		case "MFA_REQUIRED":
			method := res.CustomerMfaInfo.MfaLastMethodUsed
			if method == "" {
				method = "email"
			}
			return nil, &MFAChallenge{
				Flow: st.flow, Method: method, ServiceURL: st.serviceURL,
				Cookies: serializeCookies(cfg.ssoBase, client),
			}, nil
		case "INVALID_USERNAME_PASSWORD":
			return nil, nil, ErrInvalidCredentials
		default:
			lastErr = fmt.Errorf("%s: unexpected Garmin response type %q", st.flow, res.ResponseStatus.Type)
		}
	}
	if lastErr == nil {
		lastErr = ErrLoginFailed
	}
	return nil, nil, lastErr
}

// ResumeMFA finishes a 2FA login with the user's verification code, trying
// the challenge's own MFA endpoint first then the other bucket (rate-limit
// buckets differ, as python-garminconnect does).
func ResumeMFA(ctx context.Context, ch *MFAChallenge, code string, opts ...LoginOption) (*Credentials, error) {
	if ch == nil {
		return nil, fmt.Errorf("%w: missing MFA state", ErrLoginFailed)
	}
	cfg := newLoginConfig(opts)
	client, err := cfg.newSSOClient()
	if err != nil {
		return nil, err
	}
	restoreCookies(cfg.ssoBase, client, ch.Cookies)

	// Pick the strategy matching the flow, plus the other as fallback.
	var ordered []loginStrategy
	for _, st := range cfg.strategies() {
		if st.flow == ch.Flow {
			ordered = append([]loginStrategy{st}, ordered...)
		} else {
			ordered = append(ordered, st)
		}
	}

	var lastErr error
	for _, st := range ordered {
		res, status, err := ssoPostJSON(ctx, client, st.mfaURL, st, map[string]any{
			"mfaMethod": ch.Method, "mfaVerificationCode": code,
			"rememberMyBrowser": true, "reconsentList": []any{}, "mfaSetup": false,
		})
		if err != nil {
			cfg.logger.Warn("garmin MFA verify failed", "flow", st.flow, "status", status, "err", err)
			lastErr = err
			continue
		}
		cfg.logger.Debug("garmin MFA verify response", "flow", st.flow, "status", status, "type", res.ResponseStatus.Type)
		if res.ResponseStatus.Type == "SUCCESSFUL" {
			return exchangeServiceTicket(ctx, cfg, client, res.ServiceTicketID, ch.ServiceURL)
		}
		// Wrong/expired code (keep the sentinel, carry detail).
		lastErr = fmt.Errorf("MFA %s: HTTP %d type %q: %w", st.flow, status, res.ResponseStatus.Type, ErrInvalidCredentials)
	}
	if lastErr == nil {
		lastErr = ErrInvalidCredentials
	}
	return nil, lastErr
}

// LoginWithMFA is the blocking convenience for CLIs: when the account needs a
// 2FA code, prompt is invoked (with the MFA method, e.g. "email") and must
// return the verification code.
func LoginWithMFA(ctx context.Context, email, password string, prompt func(ctx context.Context, method string) (string, error), opts ...LoginOption) (*Credentials, error) {
	creds, ch, err := Login(ctx, email, password, opts...)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return creds, nil
	}
	if prompt == nil {
		return nil, ErrMFARequired
	}
	code, err := prompt(ctx, ch.Method)
	if err != nil {
		return nil, err
	}
	return ResumeMFA(ctx, ch, code, opts...)
}

// ssoPostJSON posts a JSON body to an SSO endpoint with the flow's query
// params / user agent and decodes the JSON response. A 429 / non-JSON body is
// an error so the caller can fall through to the next strategy.
func ssoPostJSON(ctx context.Context, client *http.Client, endpoint string, st loginStrategy, body map[string]any) (*ssoResponse, int, error) {
	payload, _ := json.Marshal(body)
	u := endpoint + "?" + url.Values{
		"clientId": {st.clientID}, "locale": {"en-US"}, "service": {st.serviceURL},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", st.userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", defaultSSOBase)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, resp.StatusCode, ErrRateLimited
	}
	var res ssoResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		// Include a short snippet to spot Cloudflare/HTML challenges.
		return nil, resp.StatusCode, fmt.Errorf("non-JSON Garmin response (HTTP %d): %.120s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return &res, resp.StatusCode, nil
}

// exchangeServiceTicket trades a CAS service ticket for DI tokens, trying
// each client id until one succeeds (the 409 fallback).
func exchangeServiceTicket(ctx context.Context, cfg *loginConfig, client *http.Client, ticket, serviceURL string) (*Credentials, error) {
	var lastErr error
	for _, cid := range diClientIDs {
		form := url.Values{
			"client_id":      {cid},
			"service_ticket": {ticket},
			"grant_type":     {diGrantType},
			"service_url":    {serviceURL},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.tokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", basicAuth(cid))
		req.Header.Set("User-Agent", defaultUserAgent)
		req.Header.Set("X-Garmin-User-Agent", xGarminUserAgent)
		req.Header.Set("Accept", "application/json,text/html;q=0.9,*/*;q=0.8")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cache-Control", "no-cache")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("%w: DI token exchange", ErrRateLimited)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			cfg.logger.Debug("garmin DI exchange fallback", "client_id", cid, "status", resp.StatusCode)
			lastErr = fmt.Errorf("DI exchange %s: HTTP %d", cid, resp.StatusCode)
			continue // e.g. 409 — try the next client id
		}
		var tok struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.Unmarshal(raw, &tok); err != nil || tok.AccessToken == "" {
			lastErr = fmt.Errorf("DI exchange %s: unreadable response", cid)
			continue
		}
		clientID := cid
		if claims, err := parseJWTClaims(tok.AccessToken); err == nil && claims.ClientID != "" {
			clientID = claims.ClientID
		}
		return &Credentials{
			AccessToken:  tok.AccessToken,
			RefreshToken: tok.RefreshToken,
			ClientID:     clientID,
		}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: DI token exchange failed", ErrLoginFailed)
	}
	return nil, lastErr
}

// cookieScopePaths cover the SSO paths where Garmin may set path-scoped
// cookies; we union over all of them so the 2FA step doesn't lose a session
// cookie.
var cookieScopePaths = []string{"/", "/sso", "/mobile/api/login", "/portal/api/login"}

// serializeCookies / restoreCookies round-trip the SSO session cookies (so
// the 2FA step can reuse the login session without server-side state).
// Cookies are gathered across all SSO paths and restored at "/" so they apply
// everywhere.
func serializeCookies(ssoBase string, client *http.Client) string {
	seen := map[string]string{}
	var order []string
	for _, p := range cookieScopePaths {
		u, err := url.Parse(ssoBase + p)
		if err != nil {
			continue
		}
		for _, c := range client.Jar.Cookies(u) {
			if _, ok := seen[c.Name]; !ok {
				order = append(order, c.Name)
			}
			seen[c.Name] = c.Value
		}
	}
	var parts []string
	for _, n := range order {
		parts = append(parts, n+"="+seen[n])
	}
	return strings.Join(parts, "\n")
}

func restoreCookies(ssoBase string, client *http.Client, blob string) {
	u, err := url.Parse(ssoBase + "/")
	if err != nil {
		return
	}
	var cookies []*http.Cookie
	for _, line := range strings.Split(blob, "\n") {
		if i := strings.IndexByte(line, '='); i > 0 {
			cookies = append(cookies, &http.Cookie{Name: line[:i], Value: line[i+1:], Path: "/"})
		}
	}
	client.Jar.SetCookies(u, cookies)
}
