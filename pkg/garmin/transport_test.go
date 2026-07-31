package garmin

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoRetriesOn5xx(t *testing.T) {
	c, mux := setupTest(t)
	var calls atomic.Int32
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"hello": "world"})
	})
	var res map[string]string
	if err := c.Do(context.Background(), http.MethodGet, "/ok", nil, nil, &res); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if res["hello"] != "world" || calls.Load() != 3 {
		t.Fatalf("got %v after %d calls", res, calls.Load())
	}
}

func TestDoDoesNotRetry429(t *testing.T) {
	c, mux := setupTest(t)
	var calls atomic.Int32
	mux.HandleFunc("/limited", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	err := c.Do(context.Background(), http.MethodGet, "/limited", nil, nil, nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("429 was retried %d times", calls.Load())
	}
}

func TestDoDoesNotRetry404(t *testing.T) {
	c, mux := setupTest(t)
	var calls atomic.Int32
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	err := c.Do(context.Background(), http.MethodGet, "/missing", nil, nil, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("want *APIError 404, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("404 was retried %d times", calls.Load())
	}
}

func TestDoRefreshesOn401(t *testing.T) {
	c, mux := setupTest(t)
	// A different expiry than setupTest's token, so the JWTs differ.
	fresh := makeJWT(t, "TEST_CLIENT", time.Now().Add(2*time.Hour))
	var refreshes, calls atomic.Int32
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		writeJSON(w, map[string]string{"access_token": fresh, "refresh_token": "refresh-1"})
	})
	mux.HandleFunc("/profile", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+fresh {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]string{"ok": "yes"})
	})
	var res map[string]string
	if err := c.Do(context.Background(), http.MethodGet, "/profile", nil, nil, &res); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if refreshes.Load() != 1 || calls.Load() != 2 {
		t.Fatalf("refreshes=%d calls=%d, want 1 and 2", refreshes.Load(), calls.Load())
	}
	if got := c.Credentials().RefreshToken; got != "refresh-1" {
		t.Fatalf("rotated refresh token not kept: %q", got)
	}
}

func TestDoRefreshesOnlyOncePer401(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{
			"access_token":  makeJWT(t, "TEST_CLIENT", time.Now().Add(time.Hour)),
			"refresh_token": "refresh-1",
		})
	})
	var calls atomic.Int32
	mux.HandleFunc("/always401", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	})
	err := c.Do(context.Background(), http.MethodGet, "/always401", nil, nil, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("endpoint called %d times, want 2 (initial + one post-refresh retry)", calls.Load())
	}
}

func TestProactiveRefreshPersistsRotationToStore(t *testing.T) {
	// An expired access token must trigger a refresh before the request,
	// and the rotated refresh token must reach the TokenStore.
	c, muxBase := setupTest(t)
	store := NewMemoryTokenStore(nil)
	expired := &Credentials{
		AccessToken:  makeJWT(t, "TEST_CLIENT", time.Now().Add(-time.Hour)),
		RefreshToken: "refresh-old",
		ClientID:     "TEST_CLIENT",
	}
	c.tokens.creds = expired
	c.tokens.store = store

	fresh := makeJWT(t, "TEST_CLIENT", time.Now().Add(time.Hour))
	muxBase.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("refresh_token") != "refresh-old" {
			t.Errorf("refresh used token %q, want refresh-old", r.Form.Get("refresh_token"))
		}
		writeJSON(w, map[string]string{"access_token": fresh, "refresh_token": "refresh-new"})
	})
	muxBase.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"ok": "yes"})
	})

	if err := c.Do(context.Background(), http.MethodGet, "/data", nil, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	saved, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if saved.RefreshToken != "refresh-new" {
		t.Fatalf("store has refresh token %q, want refresh-new", saved.RefreshToken)
	}
}

func TestFailedStoreSaveFailsTheRequest(t *testing.T) {
	c, mux := setupTest(t)
	c.tokens.creds.AccessToken = makeJWT(t, "TEST_CLIENT", time.Now().Add(-time.Hour))
	c.tokens.store = failingStore{}
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{
			"access_token":  makeJWT(t, "TEST_CLIENT", time.Now().Add(time.Hour)),
			"refresh_token": "refresh-new",
		})
	})
	err := c.Do(context.Background(), http.MethodGet, "/data", nil, nil, nil)
	if err == nil || !errors.Is(err, errSaveBoom) {
		t.Fatalf("want save failure to surface, got %v", err)
	}
}

var errSaveBoom = errors.New("boom")

type failingStore struct{}

func (failingStore) Load(context.Context) (*Credentials, error) { return nil, ErrNoCredentials }
func (failingStore) Save(context.Context, *Credentials) error   { return errSaveBoom }

func TestRetryAfterHeader(t *testing.T) {
	if got := retryAfter("3"); got != 3*time.Second {
		t.Fatalf("retryAfter(3) = %v", got)
	}
	if got := retryAfter("Wed, 21 Oct 2015 07:28:00 GMT"); got != 0 {
		t.Fatalf("HTTP-date form should be ignored, got %v", got)
	}
}
