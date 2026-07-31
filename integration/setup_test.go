//go:build integration

// Package integration validates go-garmin against the real Garmin Connect
// API. Tokens come from the environment and are never committed:
//
//	GARMINTOKENS=~/.garminconnect/garmin_tokens.json go test -tags integration ./integration/ -v
//
// GARMINTOKENS may hold a token file path or the token JSON inline (the CI
// form, injected from a repository secret). Tests are read-only unless
// GARMIN_INTEGRATION_WRITE=1 is also set.
package integration

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

var (
	clientOnce sync.Once
	client     *garmin.Client
	clientErr  error
)

// testClient returns a shared client, skipping the test when no token is
// provided by the environment.
func testClient(t *testing.T) *garmin.Client {
	t.Helper()
	env := os.Getenv("GARMINTOKENS")
	if env == "" {
		t.Skip("GARMINTOKENS not set; skipping integration tests")
	}
	clientOnce.Do(func() {
		creds, err := garmin.CredentialsFromEnv()
		if err != nil {
			clientErr = err
			return
		}
		opts := []garmin.ClientOption{
			garmin.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
		}
		// When GARMINTOKENS is a file path, persist rotated refresh
		// tokens back to it; inline JSON (CI secret) stays in memory.
		if !strings.HasPrefix(strings.TrimSpace(env), "{") {
			opts = append(opts, garmin.WithTokenStore(garmin.NewFileTokenStore(env)))
		}
		client = garmin.NewClient(creds, opts...)
	})
	if clientErr != nil {
		t.Fatalf("loading credentials from GARMINTOKENS: %v", clientErr)
	}
	return client
}

func writeEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("GARMIN_INTEGRATION_WRITE") != "1" {
		t.Skip("GARMIN_INTEGRATION_WRITE != 1; skipping write test")
	}
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}
