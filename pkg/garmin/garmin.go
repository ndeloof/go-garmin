// Package garmin is a Go client for the unofficial Garmin Connect API
// (connectapi.garmin.com) — the API used by the Garmin Connect mobile apps —
// authenticated with a Garmin account (email/password + 2FA), without any
// Garmin Developer Program token.
//
// It is a port of python-garminconnect: token files are interchangeable and
// the endpoint coverage mirrors it. The API surface follows the
// client+services layout of google/go-github:
//
//	creds, _ := garmin.CredentialsFromEnv()
//	client := garmin.NewClient(creds)
//	acts, err := client.Activities.List(ctx, nil)
//
// This is NOT a Garmin-supported API: endpoints can change without notice,
// and Garmin aggressively rate-limits datacenter IPs (expect ErrRateLimited
// when calling from cloud providers).
package garmin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the Garmin Connect API host.
	DefaultBaseURL = "https://connectapi.garmin.com"

	defaultUserAgent = "GCM-Android-5.23"
	xGarminUserAgent = "com.garmin.android.apps.connectmobile/5.23; ; Google/sdk_gphone64_arm64/google; Android/33; Dalvik/2.1.0"

	// refreshMargin is how long before expiry the access token is
	// proactively refreshed.
	refreshMargin = 15 * time.Minute
)

// Client is the Garmin Connect API client. Construct it with NewClient; the
// zero value is not usable. All methods are safe for concurrent use.
type Client struct {
	// Services accessing each API domain.
	UserProfile   *UserProfileService
	Summaries     *SummariesService
	Wellness      *WellnessService
	BloodPressure *BloodPressureService
	Weight        *WeightService
	Metrics       *MetricsService
	Biometrics    *BiometricsService
	Records       *RecordsService
	Goals         *GoalsService
	Devices       *DevicesService
	Activities    *ActivitiesService
	Upload        *UploadService
	Download      *DownloadService
	Gear          *GearService
	Workouts      *WorkoutsService
	Courses       *CoursesService
	Nutrition     *NutritionService
	WomensHealth  *WomensHealthService
	Golf          *GolfService
	GraphQL       *GraphQLService

	baseURL     string
	hc          *http.Client
	logger      *slog.Logger
	userAgent   string
	maxAttempts int
	backoff     time.Duration

	tokens *tokenSource

	dnMu        sync.Mutex
	displayName string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets the underlying *http.Client (also used for token
// refreshes). Useful to route through a proxy — Garmin rate-limits datacenter
// IPs, so a residential egress may be needed.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.hc = hc; c.tokens.hc = hc }
}

// WithBaseURL overrides https://connectapi.garmin.com (tests).
func WithBaseURL(u string) ClientOption {
	return func(c *Client) { c.baseURL = strings.TrimSuffix(u, "/") }
}

// WithTokenURL overrides the diauth token refresh endpoint (tests).
func WithTokenURL(u string) ClientOption {
	return func(c *Client) { c.tokens.tokenURL = u }
}

// WithUserAgent overrides the User-Agent sent on API calls.
func WithUserAgent(ua string) ClientOption {
	return func(c *Client) { c.userAgent = ua }
}

// WithLogger sets a logger (default: discard).
func WithLogger(l *slog.Logger) ClientOption {
	return func(c *Client) { c.logger = l; c.tokens.logger = l }
}

// WithRetry tunes the transport retry policy: maxAttempts total attempts with
// initialBackoff exponential backoff (plus jitter) between them. Only 5xx and
// network errors are retried.
func WithRetry(maxAttempts int, initialBackoff time.Duration) ClientOption {
	return func(c *Client) { c.maxAttempts = maxAttempts; c.backoff = initialBackoff }
}

// WithTokenStore attaches a store where rotated credentials are persisted
// after each token refresh. Without a store, rotated refresh tokens live only
// in memory (fine for short-lived processes and CI; long-lived ones should
// persist, or the token file on disk goes stale after the first refresh).
func WithTokenStore(store TokenStore) ClientOption {
	return func(c *Client) { c.tokens.store = store }
}

// NewClient builds a Garmin Connect client from DI credentials — the tokens
// are the construction parameter. Obtain them with the garmin CLI
// (cmd/garmin), Login/LoginWithMFA, LoadCredentials or CredentialsFromEnv.
func NewClient(creds *Credentials, opts ...ClientOption) *Client {
	hc := &http.Client{Timeout: 30 * time.Second}
	c := &Client{
		baseURL:     DefaultBaseURL,
		hc:          hc,
		logger:      slog.New(slog.DiscardHandler),
		userAgent:   defaultUserAgent,
		maxAttempts: 4,
		backoff:     500 * time.Millisecond,
		tokens: &tokenSource{
			tokenURL: DefaultTokenURL,
			hc:       hc,
			logger:   slog.New(slog.DiscardHandler),
			margin:   refreshMargin,
		},
	}
	if creds != nil {
		c.tokens.creds = creds.clone()
	}
	for _, o := range opts {
		o(c)
	}

	c.UserProfile = &UserProfileService{c}
	c.Summaries = &SummariesService{c}
	c.Wellness = &WellnessService{c}
	c.BloodPressure = &BloodPressureService{c}
	c.Weight = &WeightService{c}
	c.Metrics = &MetricsService{c}
	c.Biometrics = &BiometricsService{c}
	c.Records = &RecordsService{c}
	c.Goals = &GoalsService{c}
	c.Devices = &DevicesService{c}
	c.Activities = &ActivitiesService{c}
	c.Upload = &UploadService{c}
	c.Download = &DownloadService{c}
	c.Gear = &GearService{c}
	c.Workouts = &WorkoutsService{c}
	c.Courses = &CoursesService{c}
	c.Nutrition = &NutritionService{c}
	c.WomensHealth = &WomensHealthService{c}
	c.Golf = &GolfService{c}
	c.GraphQL = &GraphQLService{c}
	return c
}

// NewClientFromStore loads credentials from store and builds a client that
// persists rotated tokens back to it.
func NewClientFromStore(ctx context.Context, store TokenStore, opts ...ClientOption) (*Client, error) {
	creds, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	return NewClient(creds, append(opts, WithTokenStore(store))...), nil
}

// NewClientFromLogin performs a full email/password login (mfaPrompt is
// invoked when the account requires a 2FA code) and returns a ready client.
// Persist client.Credentials() if you want to reuse the session later.
func NewClientFromLogin(ctx context.Context, email, password string, mfaPrompt func(ctx context.Context, method string) (string, error), opts ...ClientOption) (*Client, error) {
	creds, err := LoginWithMFA(ctx, email, password, mfaPrompt)
	if err != nil {
		return nil, err
	}
	return NewClient(creds, opts...), nil
}

// Credentials returns a copy of the current credentials (including any
// rotated refresh token). Persist them to reuse the session across processes.
func (c *Client) Credentials() *Credentials {
	return c.tokens.credentials()
}

// DisplayName returns the account's Garmin displayName (a UUID-like string,
// not the human name), fetched once from the social profile and cached; it is
// interpolated in several endpoint paths.
func (c *Client) DisplayName(ctx context.Context) (string, error) {
	c.dnMu.Lock()
	defer c.dnMu.Unlock()
	if c.displayName != "" {
		return c.displayName, nil
	}
	p, err := c.UserProfile.SocialProfile(ctx)
	if err != nil {
		return "", err
	}
	if p.DisplayName == "" {
		return "", &APIError{StatusCode: http.StatusForbidden, Method: http.MethodGet,
			URL: c.baseURL + "/userprofile-service/socialProfile", Body: []byte("empty displayName in social profile")}
	}
	c.displayName = p.DisplayName
	return c.displayName, nil
}

// tokenSource hands out a valid access token, refreshing proactively (before
// expiry) or on demand (after a 401), and persisting rotations to the
// optional TokenStore. All under one mutex so concurrent requests trigger a
// single refresh.
type tokenSource struct {
	mu       sync.Mutex
	creds    *Credentials
	store    TokenStore
	tokenURL string
	hc       *http.Client
	logger   *slog.Logger
	margin   time.Duration
}

func (ts *tokenSource) credentials() *Credentials {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.creds == nil {
		return nil
	}
	return ts.creds.clone()
}

// token returns a valid access token, refreshing it first when it expires
// within the margin.
func (ts *tokenSource) token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.creds == nil {
		return "", ErrNoCredentials
	}
	if ts.creds.AccessToken == "" || ts.creds.Expired(ts.margin) {
		if err := ts.refreshLocked(ctx); err != nil {
			return "", err
		}
	}
	return ts.creds.AccessToken, nil
}

// invalidate drops the access token that just got a 401 and refreshes,
// unless another goroutine already refreshed it (stale comparison).
func (ts *tokenSource) invalidate(ctx context.Context, stale string) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.creds == nil {
		return "", ErrNoCredentials
	}
	if ts.creds.AccessToken != stale {
		return ts.creds.AccessToken, nil
	}
	if err := ts.refreshLocked(ctx); err != nil {
		return "", err
	}
	return ts.creds.AccessToken, nil
}

func (ts *tokenSource) refreshLocked(ctx context.Context) error {
	if ls, ok := ts.store.(LockingTokenStore); ok {
		unlock, err := ls.Lock(ctx)
		if err != nil {
			return fmt.Errorf("garmin: locking token store: %w", err)
		}
		defer unlock()
		// While we waited for the lock another process may have refreshed
		// (and rotated) the credential. Adopt the stored version when it is
		// fresh; otherwise refresh from it — its refresh token is the only
		// one still valid.
		if stored, err := ls.Load(ctx); err == nil && stored.RefreshToken != "" {
			if stored.AccessToken != "" && stored.AccessToken != ts.creds.AccessToken && !stored.Expired(ts.margin) {
				ts.creds = stored
				ts.logger.Debug("garmin credentials adopted from token store (refreshed by another process)")
				return nil
			}
			ts.creds = stored
		}
	}
	next, err := refreshCredentials(ctx, ts.hc, ts.tokenURL, ts.creds)
	if err != nil {
		return err
	}
	ts.creds = next
	ts.logger.Debug("garmin access token refreshed", "expires", next.Expiry())
	if ts.store != nil {
		// The refresh token just rotated: losing the new one bricks the
		// credential, so a failed save fails the refresh loudly.
		if err := ts.store.Save(ctx, next.clone()); err != nil {
			return &tokenSaveError{err}
		}
	}
	return nil
}

type tokenSaveError struct{ err error }

func (e *tokenSaveError) Error() string {
	return "garmin: token refreshed but saving rotated credentials failed (persist Client.Credentials() manually or the refresh token is lost): " + e.err.Error()
}
func (e *tokenSaveError) Unwrap() error { return e.err }
