package garmin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// pythonTokenFile is a token file as written by python-garminconnect.
const pythonTokenFile = `{"di_token": "access-abc", "di_refresh_token": "refresh-def", "di_client_id": "GARMIN_CONNECT_MOBILE_ANDROID_DI_2025Q2"}`

func TestFileTokenStoreReadsPythonFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garmin_tokens.json")
	if err := os.WriteFile(path, []byte(pythonTokenFile), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, err := NewFileTokenStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.AccessToken != "access-abc" || creds.RefreshToken != "refresh-def" ||
		creds.ClientID != "GARMIN_CONNECT_MOBILE_ANDROID_DI_2025Q2" {
		t.Fatalf("creds = %+v", creds)
	}
}

func TestFileTokenStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := NewFileTokenStore(path)
	in := &Credentials{AccessToken: "a", RefreshToken: "r", ClientID: "c"}
	if err := store.Save(context.Background(), in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode = %v, want 0600", st.Mode().Perm())
	}
	out, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *out != *in {
		t.Fatalf("round trip: %+v != %+v", out, in)
	}
}

func TestFileTokenStoreAppendsDefaultFilename(t *testing.T) {
	dir := t.TempDir()
	store := NewFileTokenStore(dir)
	if got, want := store.Path(), filepath.Join(dir, "garmin_tokens.json"); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestFileTokenStoreMissingFile(t *testing.T) {
	store := NewFileTokenStore(filepath.Join(t.TempDir(), "none.json"))
	_, err := store.Load(context.Background())
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("want ErrNoCredentials, got %v", err)
	}
}

func TestLoadCredentialsInlineJSON(t *testing.T) {
	creds, err := LoadCredentials(pythonTokenFile)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds.AccessToken != "access-abc" {
		t.Fatalf("creds = %+v", creds)
	}
}

func TestCredentialsFromEnvInlineAndPath(t *testing.T) {
	t.Setenv("GARMINTOKENS", pythonTokenFile)
	creds, err := CredentialsFromEnv()
	if err != nil || creds.RefreshToken != "refresh-def" {
		t.Fatalf("inline: creds=%+v err=%v", creds, err)
	}

	path := filepath.Join(t.TempDir(), "tokens.json")
	if err := os.WriteFile(path, []byte(pythonTokenFile), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GARMINTOKENS", path)
	creds, err = CredentialsFromEnv()
	if err != nil || creds.AccessToken != "access-abc" {
		t.Fatalf("path: creds=%+v err=%v", creds, err)
	}

	t.Setenv("GARMINTOKENS", "")
	if _, err := CredentialsFromEnv(); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("empty env: want ErrNoCredentials, got %v", err)
	}
}
