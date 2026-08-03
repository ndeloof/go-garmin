package garmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileTokenStoreLockExcludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garmin_tokens.json")
	a, b := NewFileTokenStore(path), NewFileTokenStore(path)

	unlockA, err := a.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var criticalDone atomic.Bool
	acquired := make(chan struct{})
	go func() {
		unlockB, err := b.Lock(context.Background())
		if err != nil {
			t.Error(err)
			return
		}
		defer unlockB()
		if !criticalDone.Load() {
			t.Error("second Lock acquired while the first was still held")
		}
		close(acquired)
	}()

	time.Sleep(150 * time.Millisecond) // let B block on the lock
	criticalDone.Store(true)
	unlockA()

	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("second Lock never acquired after release")
	}
}

func TestFileTokenStoreLockHonorsContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garmin_tokens.json")
	a, b := NewFileTokenStore(path), NewFileTokenStore(path)

	unlock, err := a.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := b.Lock(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Lock err = %v, want context.DeadlineExceeded", err)
	}
}

// rotatingTokenServer mimics diauth's single-use refresh tokens: spending a
// stale one revokes the whole family, exactly like Garmin's reuse detection.
type rotatingTokenServer struct {
	t       *testing.T
	mu      sync.Mutex
	current string
	minted  int
	revoked bool
}

func (s *rotatingTokenServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = r.ParseForm()
	if s.revoked || r.Form.Get("refresh_token") != s.current {
		s.revoked = true
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	s.minted++
	s.current = s.current + "'"
	writeJSON(w, map[string]string{
		"access_token":  makeJWT(s.t, "TEST_CLIENT", time.Now().Add(time.Hour)),
		"refresh_token": s.current,
	})
}

// TestConcurrentClientsSpendOneRotation is the regression test for the
// credential-bricking scenario: several clients (processes, in real life)
// share one token file, all start with the same expired access token, and
// all request a token at once. Exactly one refresh must reach the server;
// everyone else must adopt the rotated credentials from the store.
func TestConcurrentClientsSpendOneRotation(t *testing.T) {
	tokens := &rotatingTokenServer{t: t, current: "refresh-0"}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", tokens.handle)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "garmin_tokens.json")
	expired := &Credentials{
		AccessToken:  makeJWT(t, "TEST_CLIENT", time.Now().Add(-time.Hour)),
		RefreshToken: "refresh-0",
		ClientID:     "TEST_CLIENT",
	}
	if err := NewFileTokenStore(path).Save(context.Background(), expired); err != nil {
		t.Fatal(err)
	}

	const clients = 6
	var wg sync.WaitGroup
	errs := make(chan error, clients)
	for i := 0; i < clients; i++ {
		// A distinct store per client mirrors distinct processes.
		c := NewClient(expired.clone(),
			WithTokenURL(srv.URL+"/token"),
			WithTokenStore(NewFileTokenStore(path)))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.tokens.token(context.Background()); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("token: %v", err)
	}

	tokens.mu.Lock()
	defer tokens.mu.Unlock()
	if tokens.minted != 1 {
		t.Errorf("refresh endpoint minted %d tokens, want exactly 1", tokens.minted)
	}
	if tokens.revoked {
		t.Error("a stale refresh token was reused: credential family revoked")
	}
}

// TestRefreshStoreLoadsUnderLock verifies RefreshStore spends the refresh
// token found on disk at lock time, not a stale caller-side copy.
func TestRefreshStoreLoadsUnderLock(t *testing.T) {
	tokens := &rotatingTokenServer{t: t, current: "refresh-0"}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", tokens.handle)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "garmin_tokens.json")
	store := NewFileTokenStore(path)
	creds := &Credentials{
		AccessToken:  makeJWT(t, "TEST_CLIENT", time.Now().Add(-time.Hour)),
		RefreshToken: "refresh-0",
		ClientID:     "TEST_CLIENT",
	}
	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ { // second call must use the rotated token from disk
		if _, err := RefreshStore(context.Background(), store, WithLoginTokenURL(srv.URL+"/token")); err != nil {
			t.Fatalf("RefreshStore #%d: %v", i+1, err)
		}
	}
	tokens.mu.Lock()
	defer tokens.mu.Unlock()
	if tokens.minted != 2 || tokens.revoked {
		t.Errorf("minted = %d, revoked = %v; want 2 mints and no revocation", tokens.minted, tokens.revoked)
	}
}
