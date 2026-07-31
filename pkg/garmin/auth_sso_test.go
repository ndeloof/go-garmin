package garmin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ssoServer simulates sso.garmin.com + diauth in one mux.
type ssoServer struct {
	srv  *httptest.Server
	mux  *http.ServeMux
	opts []LoginOption
}

func newSSOServer(t *testing.T) *ssoServer {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &ssoServer{
		srv: srv,
		mux: mux,
		opts: []LoginOption{
			WithLoginSSOBaseURL(srv.URL),
			WithLoginTokenURL(srv.URL + "/token"),
		},
	}
}

func TestLoginSuccessFirstStrategy(t *testing.T) {
	s := newSSOServer(t)
	jwt := makeJWT(t, "GARMIN_CONNECT_MOBILE_ANDROID_DI_2025Q2", time.Now().Add(time.Hour))
	s.mux.HandleFunc("POST /mobile/api/login", func(w http.ResponseWriter, r *http.Request) {
		body := jsonBody(t, r)
		if body["username"] != "user@example.com" || body["password"] != "s3cret" {
			t.Errorf("unexpected credentials in body: %v", body)
		}
		writeJSON(w, map[string]any{
			"responseStatus":  map[string]string{"type": "SUCCESSFUL"},
			"serviceTicketId": "ST-123",
		})
	})
	s.mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("service_ticket") != "ST-123" {
			t.Errorf("service_ticket = %q", r.Form.Get("service_ticket"))
		}
		writeJSON(w, map[string]string{"access_token": jwt, "refresh_token": "rt-1"})
	})

	creds, ch, err := Login(context.Background(), "user@example.com", "s3cret", s.opts...)
	if err != nil || ch != nil {
		t.Fatalf("Login: creds=%v ch=%v err=%v", creds, ch, err)
	}
	if creds.ClientID != "GARMIN_CONNECT_MOBILE_ANDROID_DI_2025Q2" {
		t.Fatalf("ClientID should come from the JWT claim, got %q", creds.ClientID)
	}
	if creds.RefreshToken != "rt-1" {
		t.Fatalf("RefreshToken = %q", creds.RefreshToken)
	}
}

func TestLoginFallsBackToPortalStrategy(t *testing.T) {
	s := newSSOServer(t)
	jwt := makeJWT(t, "", time.Now().Add(time.Hour))
	s.mux.HandleFunc("POST /mobile/api/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html>Cloudflare challenge</html>"))
	})
	var portalSeeded atomic.Bool
	s.mux.HandleFunc("GET /portal/sso/en-US/sign-in", func(w http.ResponseWriter, r *http.Request) {
		portalSeeded.Store(true)
	})
	s.mux.HandleFunc("POST /portal/api/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"responseStatus":  map[string]string{"type": "SUCCESSFUL"},
			"serviceTicketId": "ST-portal",
		})
	})
	s.mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"access_token": jwt, "refresh_token": "rt"})
	})

	creds, ch, err := Login(context.Background(), "u", "p", s.opts...)
	if err != nil || ch != nil || creds == nil {
		t.Fatalf("Login: creds=%v ch=%v err=%v", creds, ch, err)
	}
	if !portalSeeded.Load() {
		t.Fatal("portal sign-in GET was not used to seed cookies")
	}
	// JWT carries no client_id claim → fall back to the client id used.
	if creds.ClientID != "GARMIN_CONNECT_MOBILE_ANDROID_DI_2025Q2" {
		t.Fatalf("ClientID fallback = %q", creds.ClientID)
	}
}

func TestLoginInvalidCredentialsStopsCascade(t *testing.T) {
	s := newSSOServer(t)
	var portalCalled atomic.Bool
	s.mux.HandleFunc("POST /mobile/api/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"responseStatus": map[string]string{"type": "INVALID_USERNAME_PASSWORD"}})
	})
	s.mux.HandleFunc("POST /portal/api/login", func(w http.ResponseWriter, r *http.Request) {
		portalCalled.Store(true)
	})
	_, _, err := Login(context.Background(), "u", "wrong", s.opts...)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	if portalCalled.Load() {
		t.Fatal("cascade must stop on definitive invalid credentials")
	}
}

func TestLoginMFAFlow(t *testing.T) {
	s := newSSOServer(t)
	jwt := makeJWT(t, "GARMIN_CONNECT_MOBILE_IOS_DI", time.Now().Add(time.Hour))
	s.mux.HandleFunc("POST /mobile/api/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SSO_SESSION", Value: "abc", Path: "/"})
		writeJSON(w, map[string]any{
			"responseStatus":  map[string]string{"type": "MFA_REQUIRED"},
			"customerMfaInfo": map[string]string{"mfaLastMethodUsed": "email"},
		})
	})
	s.mux.HandleFunc("POST /mobile/api/mfa/verifyCode", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("SSO_SESSION"); err != nil || c.Value != "abc" {
			t.Error("MFA verify did not carry the SSO session cookie")
		}
		body := jsonBody(t, r)
		if body["mfaVerificationCode"] != "123456" {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]any{"responseStatus": map[string]string{"type": "FAILED"}})
			return
		}
		writeJSON(w, map[string]any{
			"responseStatus":  map[string]string{"type": "SUCCESSFUL"},
			"serviceTicketId": "ST-mfa",
		})
	})
	s.mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"access_token": jwt, "refresh_token": "rt-mfa"})
	})

	creds, ch, err := Login(context.Background(), "u", "p", s.opts...)
	if err != nil || creds != nil || ch == nil {
		t.Fatalf("Login: creds=%v ch=%v err=%v", creds, ch, err)
	}
	if ch.Flow != "mobile" || ch.Method != "email" || ch.Cookies == "" {
		t.Fatalf("challenge = %+v", ch)
	}

	// The challenge round-trips (e.g. via an encrypted cookie): resume in a
	// fresh flow with just its serialized state.
	got, err := ResumeMFA(context.Background(), ch, "123456", s.opts...)
	if err != nil {
		t.Fatalf("ResumeMFA: %v", err)
	}
	if got.RefreshToken != "rt-mfa" {
		t.Fatalf("RefreshToken = %q", got.RefreshToken)
	}
}

func TestLoginWithMFAPrompt(t *testing.T) {
	s := newSSOServer(t)
	jwt := makeJWT(t, "X", time.Now().Add(time.Hour))
	s.mux.HandleFunc("POST /mobile/api/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"responseStatus":  map[string]string{"type": "MFA_REQUIRED"},
			"customerMfaInfo": map[string]string{"mfaLastMethodUsed": "sms"},
		})
	})
	s.mux.HandleFunc("POST /mobile/api/mfa/verifyCode", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"responseStatus":  map[string]string{"type": "SUCCESSFUL"},
			"serviceTicketId": "ST-x",
		})
	})
	s.mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"access_token": jwt, "refresh_token": "rt"})
	})
	var promptedMethod string
	creds, err := LoginWithMFA(context.Background(), "u", "p", func(ctx context.Context, method string) (string, error) {
		promptedMethod = method
		return "000000", nil
	}, s.opts...)
	if err != nil || creds == nil {
		t.Fatalf("LoginWithMFA: %v", err)
	}
	if promptedMethod != "sms" {
		t.Fatalf("prompted method = %q", promptedMethod)
	}
}

func TestExchangeTriesClientIDsInOrder(t *testing.T) {
	s := newSSOServer(t)
	jwt := makeJWT(t, "GARMIN_CONNECT_MOBILE_ANDROID_DI", time.Now().Add(time.Hour))
	s.mux.HandleFunc("POST /mobile/api/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"responseStatus":  map[string]string{"type": "SUCCESSFUL"},
			"serviceTicketId": "ST-1",
		})
	})
	var tried []string
	s.mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		cid := r.Form.Get("client_id")
		tried = append(tried, cid)
		if cid != "GARMIN_CONNECT_MOBILE_ANDROID_DI" {
			w.WriteHeader(http.StatusConflict) // 409 → next client id
			return
		}
		writeJSON(w, map[string]string{"access_token": jwt, "refresh_token": "rt"})
	})

	creds, _, err := Login(context.Background(), "u", "p", s.opts...)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	want := []string{
		"GARMIN_CONNECT_MOBILE_ANDROID_DI_2025Q2",
		"GARMIN_CONNECT_MOBILE_ANDROID_DI_2024Q4",
		"GARMIN_CONNECT_MOBILE_ANDROID_DI",
	}
	if len(tried) != len(want) {
		t.Fatalf("tried %v", tried)
	}
	for i := range want {
		if tried[i] != want[i] {
			t.Fatalf("tried %v, want %v", tried, want)
		}
	}
	if creds.ClientID != "GARMIN_CONNECT_MOBILE_ANDROID_DI" {
		t.Fatalf("ClientID = %q", creds.ClientID)
	}
}

func TestLoginAllStrategiesRateLimited(t *testing.T) {
	s := newSSOServer(t)
	limited := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}
	s.mux.HandleFunc("POST /mobile/api/login", limited)
	s.mux.HandleFunc("POST /portal/api/login", limited)
	_, _, err := Login(context.Background(), "u", "p", s.opts...)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestRefreshRotatesToken(t *testing.T) {
	s := newSSOServer(t)
	jwt := makeJWT(t, "CID", time.Now().Add(time.Hour))
	s.mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "rt-old" {
			t.Errorf("unexpected form: %v", r.Form)
		}
		writeJSON(w, map[string]string{"access_token": jwt, "refresh_token": "rt-new"})
	})
	next, err := Refresh(context.Background(), &Credentials{
		AccessToken: "x", RefreshToken: "rt-old", ClientID: "CID",
	}, s.opts...)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if next.RefreshToken != "rt-new" || next.AccessToken != jwt {
		t.Fatalf("next = %+v", next)
	}
}

func TestRefreshKeepsOldTokenWhenResponseOmitsIt(t *testing.T) {
	s := newSSOServer(t)
	jwt := makeJWT(t, "CID", time.Now().Add(time.Hour))
	s.mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"access_token": jwt})
	})
	next, err := Refresh(context.Background(), &Credentials{
		AccessToken: "x", RefreshToken: "rt-old", ClientID: "CID",
	}, s.opts...)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if next.RefreshToken != "rt-old" {
		t.Fatalf("RefreshToken = %q, want rt-old kept", next.RefreshToken)
	}
}
