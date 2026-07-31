package garmin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultTokenURL is the diauth endpoint that mints and refreshes the "DI"
// OAuth2 tokens used by the Garmin Connect mobile apps.
const DefaultTokenURL = "https://diauth.garmin.com/di-oauth2-service/oauth/token"

// Credentials is a usable Garmin "DI" credential: a short-lived access token
// (a JWT bearer), the long-lived refresh token and the OAuth client id needed
// to refresh. The JSON form is exactly the token file written by
// python-garminconnect ({"di_token","di_refresh_token","di_client_id"}), so
// token files are interchangeable between the two libraries.
type Credentials struct {
	AccessToken  string `json:"di_token"`
	RefreshToken string `json:"di_refresh_token"`
	ClientID     string `json:"di_client_id"`
}

// Expiry returns the access token's expiration time, decoded from its `exp`
// JWT claim, or the zero time when it cannot be decoded.
func (c *Credentials) Expiry() time.Time {
	claims, err := parseJWTClaims(c.AccessToken)
	if err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

// Expired reports whether the access token expires within margin. An
// undecodable expiry is treated as not-expired: the client falls back to the
// reactive refresh-on-401 path.
func (c *Credentials) Expired(margin time.Duration) bool {
	exp := c.Expiry()
	if exp.IsZero() {
		return false
	}
	return time.Now().After(exp.Add(-margin))
}

func (c *Credentials) clone() *Credentials {
	cp := *c
	return &cp
}

// LoadCredentials accepts either a path to a token file or the inline JSON
// itself (the same heuristic python-garminconnect applies to GARMINTOKENS).
func LoadCredentials(pathOrJSON string) (*Credentials, error) {
	s := strings.TrimSpace(pathOrJSON)
	if s == "" {
		return nil, fmt.Errorf("%w: empty token source", ErrNoCredentials)
	}
	data := []byte(s)
	if !strings.HasPrefix(s, "{") {
		var err error
		if data, err = os.ReadFile(s); err != nil {
			return nil, fmt.Errorf("garmin: reading token file: %w", err)
		}
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("garmin: parsing tokens: %w", err)
	}
	if creds.AccessToken == "" && creds.RefreshToken == "" {
		return nil, fmt.Errorf("%w: token JSON has neither di_token nor di_refresh_token", ErrNoCredentials)
	}
	return &creds, nil
}

// CredentialsFromEnv loads credentials from the GARMINTOKENS environment
// variable, which may hold either a file path or the token JSON inline. The
// inline form is how CI provides tokens through repository secrets.
func CredentialsFromEnv() (*Credentials, error) {
	v := os.Getenv("GARMINTOKENS")
	if v == "" {
		return nil, fmt.Errorf("%w: GARMINTOKENS is not set", ErrNoCredentials)
	}
	return LoadCredentials(v)
}

// Refresh mints fresh credentials from the refresh token. Garmin ROTATES the
// refresh token: persist the returned credentials, the old refresh token is
// invalidated. Client does this automatically; Refresh is for tools (like the
// garmin CLI) that manage tokens directly.
func Refresh(ctx context.Context, creds *Credentials, opts ...LoginOption) (*Credentials, error) {
	cfg := newLoginConfig(opts)
	return refreshCredentials(ctx, cfg.client, cfg.tokenURL, creds)
}

type jwtClaims struct {
	Exp      int64  `json:"exp"`
	ClientID string `json:"client_id"`
}

// parseJWTClaims decodes the payload segment of a JWT, tolerating both padded
// and unpadded base64url.
func parseJWTClaims(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, errors.New("garmin: access token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		padded := parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4)
		if payload, err = base64.URLEncoding.DecodeString(padded); err != nil {
			return nil, err
		}
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

// basicAuth builds the public-OAuth-client Basic header (client id, empty
// secret) used by the diauth endpoints.
func basicAuth(clientID string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(clientID+":"))
}

// refreshCredentials mints a fresh access token from the refresh token (the
// diauth refresh_token grant).
//
// Garmin ROTATES the refresh token: the response carries a new refresh_token
// and the one just used is invalidated. Callers MUST persist the returned
// credentials — reusing the old refresh token on the next refresh fails with
// the whole credential rejected. When the response omits a refresh_token the
// input one is kept.
func refreshCredentials(ctx context.Context, hc *http.Client, tokenURL string, creds *Credentials) (*Credentials, error) {
	if creds == nil || creds.RefreshToken == "" || creds.ClientID == "" {
		return nil, fmt.Errorf("%w: refresh token or client id missing", ErrNoCredentials)
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {creds.ClientID},
		"refresh_token": {creds.RefreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", basicAuth(creds.ClientID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "com.garmin.android.apps.connectmobile")
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("%w: token refresh", ErrRateLimited)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newAPIError(http.MethodPost, tokenURL, resp.StatusCode, body)
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("garmin: token refresh response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, errors.New("garmin: token refresh response has no access_token")
	}
	next := &Credentials{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ClientID:     creds.ClientID,
	}
	if next.RefreshToken == "" {
		next.RefreshToken = creds.RefreshToken
	}
	return next, nil
}
