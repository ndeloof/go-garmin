package garmin

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors, matchable with errors.Is. API failures are returned as
// *APIError values whose Is method maps common status codes onto these
// sentinels, so both errors.Is(err, garmin.ErrNotFound) and
// errors.As(err, &apiErr) work.
var (
	// ErrRateLimited is returned when Garmin answers HTTP 429. Garmin
	// aggressively rate-limits datacenter/cloud egress IPs; retrying
	// immediately makes it worse, so the client never retries a 429.
	ErrRateLimited = errors.New("garmin: rate limited (429)")
	// ErrNotFound is returned for HTTP 404 responses.
	ErrNotFound = errors.New("garmin: not found (404)")
	// ErrUnauthorized is returned when a request still gets HTTP 401/403
	// after a token refresh was attempted.
	ErrUnauthorized = errors.New("garmin: unauthorized")
	// ErrInvalidCredentials is returned when Garmin rejects the
	// email/password pair or the MFA verification code.
	ErrInvalidCredentials = errors.New("garmin: invalid credentials")
	// ErrMFARequired is returned by helpers that cannot complete a login
	// because the account requires a 2FA code.
	ErrMFARequired = errors.New("garmin: MFA code required")
	// ErrLoginFailed is returned when every login strategy was exhausted
	// without a definitive answer from Garmin.
	ErrLoginFailed = errors.New("garmin: login failed")
	// ErrDuplicateUpload is returned when an uploaded activity already
	// exists on Garmin Connect (HTTP 409 on import).
	ErrDuplicateUpload = errors.New("garmin: duplicate activity (409)")
	// ErrNoCredentials is returned when the client has no credentials to
	// authenticate with.
	ErrNoCredentials = errors.New("garmin: no credentials")
)

// APIError is the error returned for any non-2xx Garmin API response.
type APIError struct {
	StatusCode int
	Method     string
	URL        string
	Body       []byte // response body, truncated to 4KiB
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("garmin: %s %s: HTTP %d", e.Method, e.URL, e.StatusCode)
	if len(e.Body) > 0 {
		msg += ": " + string(e.Body)
	}
	return msg
}

// Is maps well-known status codes onto the package sentinels so that
// errors.Is(err, garmin.ErrRateLimited) etc. work on any *APIError.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrRateLimited:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
	case ErrDuplicateUpload:
		return e.StatusCode == http.StatusConflict
	}
	return false
}

const maxErrBody = 4 << 10

func newAPIError(method, url string, status int, body []byte) *APIError {
	if len(body) > maxErrBody {
		body = body[:maxErrBody]
	}
	return &APIError{StatusCode: status, Method: method, URL: url, Body: body}
}
