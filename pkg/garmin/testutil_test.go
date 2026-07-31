package garmin

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// makeJWT builds an unsigned JWT with the given claims (enough for the
// client, which only decodes the payload segment).
func makeJWT(t *testing.T, clientID string, exp time.Time) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"client_id": clientID,
		"exp":       exp.Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

// setupTest returns a client wired to a test server, with fast retries.
func setupTest(t *testing.T) (*Client, *http.ServeMux) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds := &Credentials{
		AccessToken:  makeJWT(t, "TEST_CLIENT", time.Now().Add(time.Hour)),
		RefreshToken: "refresh-0",
		ClientID:     "TEST_CLIENT",
	}
	c := NewClient(creds,
		WithBaseURL(srv.URL),
		WithTokenURL(srv.URL+"/token"),
		WithRetry(4, time.Millisecond),
	)
	return c, mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		t.Fatalf("decoding request body: %v", err)
	}
	return m
}
