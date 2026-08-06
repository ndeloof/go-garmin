package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(testClient(t))
}

// testClient wires a garmin client to a fake backend.
func testClient(t *testing.T) *garmin.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/userprofile-service/socialProfile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayName":"dn-1","fullName":"Jane Doe"}`))
	})
	mux.HandleFunc("/activitylist-service/activities/search/activities", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "3" {
			t.Errorf("limit = %q, want 3", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"activityId":1,"activityName":"Run"}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	creds := &garmin.Credentials{AccessToken: unsignedJWT(t), RefreshToken: "r", ClientID: "c"}
	return garmin.NewClient(creds, garmin.WithBaseURL(srv.URL))
}

// unsignedJWT builds a JWT that expires far in the future so no refresh fires.
func unsignedJWT(t *testing.T) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc(payload) + ".sig"
}

func roundTrip(t *testing.T, s *Server, req string) rpcResponse {
	t.Helper()
	var out strings.Builder
	if err := s.ServeStdio(context.Background(), strings.NewReader(req+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	var resp rpcResponse
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("decoding response %q: %v", out.String(), err)
	}
	return resp
}

func TestInitialize(t *testing.T) {
	s := testServer(t)
	resp := roundTrip(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	res, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(res), `"serverInfo"`) || !strings.Contains(string(res), "go-garmin") {
		t.Fatalf("initialize result = %s", res)
	}
}

func TestToolsListNonEmpty(t *testing.T) {
	s := testServer(t)
	resp := roundTrip(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	res, _ := json.Marshal(resp.Result)
	var parsed struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(res, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tools) < 20 {
		t.Fatalf("expected a rich tool set, got %d", len(parsed.Tools))
	}
	for _, tl := range parsed.Tools {
		if tl.Name == "" || tl.InputSchema["type"] != "object" {
			t.Fatalf("bad tool: %+v", tl)
		}
	}
}

func TestToolCallProfile(t *testing.T) {
	s := testServer(t)
	resp := roundTrip(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_profile","arguments":{}}}`)
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	res, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(res), "Jane Doe") {
		t.Fatalf("tool result = %s", res)
	}
}

func TestToolCallWithArgs(t *testing.T) {
	s := testServer(t)
	resp := roundTrip(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_activities","arguments":{"limit":3}}}`)
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	res, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(res), "Run") {
		t.Fatalf("tool result = %s", res)
	}
}

func TestUnknownToolIsError(t *testing.T) {
	s := testServer(t)
	resp := roundTrip(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	if resp.Error == nil {
		t.Fatal("expected an error for unknown tool")
	}
}

func TestNotificationGetsNoReply(t *testing.T) {
	s := testServer(t)
	var out strings.Builder
	// A notification (no id) must not produce a response line.
	if err := s.ServeStdio(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("notification produced output: %q", out.String())
	}
}

func TestMissingRequiredArgReportedInBand(t *testing.T) {
	s := testServer(t)
	resp := roundTrip(t, s, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_activity","arguments":{}}}`)
	if resp.Error != nil {
		t.Fatalf("execution errors should be in-band, got protocol error %+v", resp.Error)
	}
	res, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(res), "isError") || !strings.Contains(string(res), "activity_id is required") {
		t.Fatalf("want in-band error, got %s", res)
	}
}
