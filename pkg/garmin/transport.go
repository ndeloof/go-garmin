package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Do executes an authenticated request against connectapi.garmin.com and
// decodes the JSON response into v. It is the exported low-level escape hatch:
// any endpoint not covered by a typed service method can be called directly,
// and passing a *json.RawMessage as v captures the raw payload of any call.
//
// path is the endpoint path (e.g. "/activity-service/activity/123"), query
// the optional URL parameters. body may be nil, a []byte, an io.Reader, or
// any value to JSON-marshal. v may be nil (discard), a *[]byte (raw bytes),
// an io.Writer (raw copy), a *json.RawMessage, or a pointer to unmarshal
// into. A 204/empty response leaves v untouched.
//
// Retries: network errors and 5xx are retried with exponential backoff and
// jitter. 429 returns ErrRateLimited immediately (no retry — Garmin
// rate-limits datacenter IPs persistently). A 401 triggers one token refresh
// then one retry.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, v any) error {
	var payload []byte
	contentType := ""
	switch b := body.(type) {
	case nil:
	case []byte:
		payload = b
		contentType = "application/json"
	case io.Reader:
		data, err := io.ReadAll(b)
		if err != nil {
			return err
		}
		payload = data
		contentType = "application/json"
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return err
		}
		payload = data
		contentType = "application/json"
	}
	return c.do(ctx, method, path, query, contentType, payload, nil, v)
}

// do is the transport core shared by Do and the multipart/download helpers.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, contentType string, payload []byte, headers http.Header, v any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	token, err := c.tokens.token(ctx)
	if err != nil {
		return err
	}

	backoff := c.backoff
	refreshed := false
	var resp *http.Response
	var respBody []byte
	var lastNetErr error

	for attempt := 0; attempt < c.maxAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter; abort early on ctx cancel.
			sleep := backoff/2 + time.Duration(rand.Int64N(int64(backoff)))
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				return ctx.Err()
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("X-Garmin-User-Agent", xGarminUserAgent)
		req.Header.Set("X-App-Ver", "10861")
		req.Header.Set("X-Garmin-Client-Platform", "Android")
		req.Header.Set("X-Lang", "en")
		req.Header.Set("Accept", "application/json")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for k, vals := range headers {
			req.Header.Del(k)
			for _, hv := range vals {
				req.Header.Add(k, hv)
			}
		}

		resp, err = c.hc.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logger.Debug("garmin request network error", "method", method, "url", u, "attempt", attempt, "err", err)
			lastNetErr = err
			continue
		}
		respBody, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusUnauthorized && !refreshed:
			// One reactive refresh, then one immediate retry.
			refreshed = true
			token, err = c.tokens.invalidate(ctx, token)
			if err != nil {
				return err
			}
			attempt--
			continue
		case resp.StatusCode >= 500:
			if ra := retryAfter(resp.Header.Get("Retry-After")); ra > 0 && ra <= 5*time.Second {
				backoff = ra
			}
			c.logger.Debug("garmin request server error", "method", method, "url", u, "status", resp.StatusCode, "attempt", attempt)
			continue
		}
		lastNetErr = nil
		break
	}

	if lastNetErr != nil {
		return fmt.Errorf("garmin: %s %s: %w", method, u, lastNetErr)
	}
	if resp == nil {
		return fmt.Errorf("garmin: %s %s: no response after %d attempts", method, u, c.maxAttempts)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newAPIError(method, u, resp.StatusCode, respBody)
	}
	return decodeInto(v, respBody)
}

func decodeInto(v any, body []byte) error {
	if v == nil {
		return nil
	}
	switch dst := v.(type) {
	case *[]byte:
		*dst = body
		return nil
	case io.Writer:
		_, err := dst.Write(body)
		return err
	case *json.RawMessage:
		*dst = json.RawMessage(bytes.Clone(body))
		return nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil // 204 / empty body: leave v untouched
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("garmin: decoding response: %w (body: %.200s)", err, body)
	}
	return nil
}

// getJSON is the shorthand used by typed service methods.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, v any) error {
	return c.Do(ctx, http.MethodGet, path, query, nil, v)
}

// download GETs raw bytes with Accept: */*.
func (c *Client) download(ctx context.Context, path string, query url.Values) ([]byte, error) {
	var data []byte
	h := http.Header{"Accept": {"*/*"}}
	if err := c.do(ctx, http.MethodGet, path, query, "", nil, h, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// retryAfter parses a Retry-After header expressed as a number of seconds
// (the HTTP-date form is ignored — we keep the default backoff).
func retryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(h); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}
