package mcp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

func testHTTPHandler(t *testing.T) http.Handler {
	t.Helper()
	s := testServer(t) // reuses the fake Garmin backend wiring
	return NewHTTPHandler(func(r *http.Request) (*garmin.Client, error) {
		if r.Header.Get("Authorization") != "Bearer good" {
			return nil, errors.New("bad token")
		}
		return s.client, nil
	})
}

func doPost(t *testing.T, h http.Handler, auth, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHTTPToolCall(t *testing.T) {
	h := testHTTPHandler(t)
	rec := doPost(t, h, "Bearer good",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_profile","arguments":{}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "Jane Doe") {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestHTTPInitializeAndList(t *testing.T) {
	h := testHTTPHandler(t)
	rec := doPost(t, h, "Bearer good",
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "go-garmin") {
		t.Fatalf("initialize: %d %s", rec.Code, rec.Body)
	}
	rec = doPost(t, h, "Bearer good", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "list_activities") {
		t.Fatalf("tools/list: %d %s", rec.Code, truncateStr(rec.Body.String(), 200))
	}
}

func TestHTTPUnauthorized(t *testing.T) {
	h := testHTTPHandler(t)
	rec := doPost(t, h, "Bearer wrong", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHTTPNotificationAccepted(t *testing.T) {
	h := testHTTPHandler(t)
	rec := doPost(t, h, "Bearer good", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
		t.Fatalf("notification: %d %q", rec.Code, rec.Body)
	}
}

func TestHTTPMethodNotAllowed(t *testing.T) {
	h := testHTTPHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHTTPParseError(t *testing.T) {
	h := testHTTPHandler(t)
	rec := doPost(t, h, "Bearer good", `{not json`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "parse error") {
		t.Fatalf("parse error: %d %s", rec.Code, rec.Body)
	}
}

func truncateStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
