package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

func testHTTPHandler(t *testing.T) http.Handler {
	t.Helper()
	client := testClient(t) // fake Garmin backend wiring
	return NewHTTPHandler(func(r *http.Request) (*garmin.Client, error) {
		if r.Header.Get("Authorization") != "Bearer good" {
			return nil, errors.New("bad token")
		}
		return client, nil
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

// TestHTTPCustomToolServer exercises the domain-neutral plumbing: a server
// built with NewToolServer and application tools, served over HTTP.
func TestHTTPCustomToolServer(t *testing.T) {
	s := NewToolServer("my-app", "9.9", Tool{
		Name:        "echo",
		Description: "Echo the input.",
		Schema:      objectSchema(map[string]any{"msg": strProp("message")}, "msg"),
		Handler: func(_ context.Context, raw json.RawMessage) (any, error) {
			var a struct {
				Msg string `json:"msg"`
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return nil, err
			}
			return map[string]string{"echo": a.Msg}, nil
		},
	})
	h := NewHTTPServerHandler(func(r *http.Request) (*Server, error) { return s, nil })

	rec := doPost(t, h, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if !strings.Contains(rec.Body.String(), "my-app") || !strings.Contains(rec.Body.String(), "9.9") {
		t.Fatalf("initialize should carry the custom serverInfo: %s", rec.Body)
	}
	rec = doPost(t, h, "", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"msg":"bonjour"}}}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "bonjour") {
		t.Fatalf("echo call: %d %s", rec.Code, rec.Body)
	}
}
