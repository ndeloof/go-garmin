package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TokenStore persists Garmin credentials. Because Garmin rotates the refresh
// token on every refresh, the client saves the new credentials through the
// store immediately after each successful refresh; a store that fails to save
// makes the refresh fail (a silently-lost rotated token bricks the whole
// credential).
type TokenStore interface {
	Load(ctx context.Context) (*Credentials, error)
	Save(ctx context.Context, creds *Credentials) error
}

// FileTokenStore stores credentials in the python-garminconnect token file
// format: {"di_token","di_refresh_token","di_client_id"}, mode 0600. Files
// are interchangeable with python-garminconnect.
type FileTokenStore struct {
	path string
}

const defaultTokenFile = "garmin_tokens.json"

// NewFileTokenStore builds a file-backed store. An empty path resolves to
// $GARMINTOKENS (when it holds a path, not inline JSON) or
// ~/.garminconnect/garmin_tokens.json. A path that is a directory or lacks a
// .json extension gets garmin_tokens.json appended (python-garminconnect
// behavior).
func NewFileTokenStore(path string) *FileTokenStore {
	return &FileTokenStore{path: resolveTokenPath(path)}
}

// Path returns the resolved token file path.
func (s *FileTokenStore) Path() string { return s.path }

func resolveTokenPath(path string) string {
	if path == "" {
		if env := os.Getenv("GARMINTOKENS"); env != "" && !strings.HasPrefix(strings.TrimSpace(env), "{") {
			path = env
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				home = "."
			}
			path = filepath.Join(home, ".garminconnect")
		}
	}
	if st, err := os.Stat(path); (err == nil && st.IsDir()) || !strings.HasSuffix(path, ".json") {
		path = filepath.Join(path, defaultTokenFile)
	}
	return path
}

func (s *FileTokenStore) Load(_ context.Context) (*Credentials, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s does not exist (run the garmin CLI login first)", ErrNoCredentials, s.path)
		}
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("garmin: parsing %s: %w", s.path, err)
	}
	return &creds, nil
}

func (s *FileTokenStore) Save(_ context.Context, creds *Credentials) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	// Write-then-rename so a crash never leaves a truncated token file.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// MemoryTokenStore keeps credentials in memory only. Useful for CI (tokens
// injected from a secret) and tests; rotated refresh tokens survive only for
// the lifetime of the process.
type MemoryTokenStore struct {
	mu    sync.Mutex
	creds *Credentials
}

func NewMemoryTokenStore(creds *Credentials) *MemoryTokenStore {
	return &MemoryTokenStore{creds: creds}
}

func (s *MemoryTokenStore) Load(_ context.Context) (*Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creds == nil {
		return nil, ErrNoCredentials
	}
	return s.creds.clone(), nil
}

func (s *MemoryTokenStore) Save(_ context.Context, creds *Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds = creds.clone()
	return nil
}
