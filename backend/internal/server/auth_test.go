package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/palhelm/palhelm/internal/config"
	"github.com/palhelm/palhelm/internal/gameconfig"
)

func TestAuthRoleMatrix(t *testing.T) {
	a := newAuth("secret-secret-secret-secret-secret-secret", "admin", "viewer")
	for _, tc := range []struct {
		role string
		want int
	}{{"admin", 204}, {"viewer", 403}} {
		tok, err := a.token(tc.role, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		h := a.middleware(adminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })))
		req := httptest.NewRequest("POST", "/", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("role %s status=%d", tc.role, rr.Code)
		}
	}
}
func TestAuthExpiredToken(t *testing.T) {
	a := newAuth("secret-secret-secret-secret-secret-secret", "admin", "")
	base := time.Now()
	a.now = func() time.Time { return base }
	tok, err := a.token("admin", base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	a.now = func() time.Time { return base.Add(2 * time.Minute) }
	h := a.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("called") }))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("status=%d", rr.Code)
	}
}

func authHandler(cfg config.Config) (*Server, http.Handler) {
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = strings.Repeat("s", 48)
	}
	if cfg.AdminPassword == "" {
		cfg.AdminPassword = "adminpass"
	}
	s := &Server{
		cfg:     cfg,
		auth:    newAuth(cfg.SessionSecret, cfg.AdminPassword, cfg.ViewerPassword, cfg.TrustedProxies...),
		hub:     NewHub(),
		gamecfg: &gameconfig.Editor{SaveDir: cfg.SaveDir},
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return s, s.routes()
}

func loginRequest(h http.Handler, remote, forwardedFor, password string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"password":"`+password+`"}`))
	req.RemoteAddr = remote
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestLoginRateLimitIgnoresSpoofedForwardingHeaders(t *testing.T) {
	_, h := authHandler(config.Config{})
	for i := 0; i < loginAttemptLimit; i++ {
		rr := loginRequest(h, "192.0.2.10:1234", "198.51.100."+strconv.Itoa(i+1), "wrong")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status=%d body=%s", i+1, rr.Code, rr.Body.String())
		}
	}
	if rr := loginRequest(h, "192.0.2.10:1234", "203.0.113.200", "wrong"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed sixth attempt status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLoginRateLimitHonorsForwardingFromTrustedProxy(t *testing.T) {
	_, h := authHandler(config.Config{TrustedProxies: []string{"192.0.2.0/24"}})
	for i := 1; i <= 6; i++ {
		rr := loginRequest(h, "192.0.2.10:1234", "198.51.100."+strconv.Itoa(i), "wrong")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("client %d status=%d body=%s", i, rr.Code, rr.Body.String())
		}
	}
}

func TestTrustedProxyUsesNearestUntrustedForwardedHop(t *testing.T) {
	a := newAuth(strings.Repeat("s", 48), "admin", "", "192.0.2.0/24")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	// The leftmost value is attacker-supplied. A trusted proxy appended the
	// actual client address on the right.
	req.Header.Set("X-Forwarded-For", "203.0.113.99, 198.51.100.5")
	if got := a.clientIP(req); got != "198.51.100.5" {
		t.Fatalf("client IP=%q", got)
	}
}

func TestInvalidLoginJSONDoesNotAllocateLimiterState(t *testing.T) {
	s, h := authHandler(config.Config{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{"))
	req.RemoteAddr = "192.0.2.10:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := len(s.auth.attempts); got != 0 {
		t.Fatalf("invalid JSON allocated %d limiter buckets", got)
	}
}

func TestLoginLimiterExpiresAndBoundsState(t *testing.T) {
	a := newAuth(strings.Repeat("s", 48), "admin", "")
	now := time.Unix(1000, 0)
	a.now = func() time.Time { return now }
	for i := 0; i < loginAttemptLimit; i++ {
		if !a.allow("192.0.2.1") {
			t.Fatalf("attempt %d unexpectedly denied", i+1)
		}
	}
	if a.allow("192.0.2.1") {
		t.Fatal("sixth attempt unexpectedly allowed")
	}
	now = now.Add(loginWindow + time.Nanosecond)
	if !a.allow("192.0.2.1") {
		t.Fatal("expired bucket was not allowed")
	}

	a.attempts = make(map[string]attemptBucket)
	for i := 0; i < maxLimiterEntries; i++ {
		if !a.allow("client-" + strconv.Itoa(i)) {
			t.Fatalf("entry %d unexpectedly denied", i)
		}
	}
	if a.allow("over-capacity") {
		t.Fatal("new key was allowed beyond limiter capacity")
	}
	now = now.Add(loginWindow + time.Nanosecond)
	if !a.allow("after-expiry") {
		t.Fatal("expired limiter state was not reclaimed")
	}
	if got := len(a.attempts); got != 1 {
		t.Fatalf("limiter retained %d entries after expiry, want 1", got)
	}
}

func TestViewerCannotReadRawConfig(t *testing.T) {
	_, h := authHandler(config.Config{ViewerPassword: "viewerpass"})
	login := loginRequest(h, "192.0.2.10:1234", "", "viewerpass")
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/raw", nil)
	req.AddCookie(login.Result().Cookies()[0])
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("viewer raw config status=%d body=%s", rr.Code, rr.Body.String())
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control=%q", cc)
	}
}

func TestViewerStructuredConfigRedactsCredentials(t *testing.T) {
	saveDir := t.TempDir()
	ini := filepath.Join(saveDir, "Config", "LinuxServer", "PalWorldSettings.ini")
	if err := os.MkdirAll(filepath.Dir(ini), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ini, []byte(`[/Script/Pal.PalGameWorldSettings]
OptionSettings=(AdminPassword="game-admin",ServerPassword="join-secret",ServerName="test")
`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, h := authHandler(config.Config{ViewerPassword: "viewerpass", SaveDir: saveDir})
	login := loginRequest(h, "192.0.2.10:1234", "", "viewerpass")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.AddCookie(login.Result().Cookies()[0])
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("config status=%d body=%s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); strings.Contains(body, "game-admin") || strings.Contains(body, "join-secret") {
		t.Fatalf("viewer response disclosed a credential: %s", body)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control=%q", cc)
	}
}

func TestMapTilesRequireAuthentication(t *testing.T) {
	_, h := authHandler(config.Config{DataDir: t.TempDir()})
	req := httptest.NewRequest(http.MethodGet, "/map-tiles/0/0/0.png", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated tile status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSecurityHeadersAndSecureProxyCookie(t *testing.T) {
	_, h := authHandler(config.Config{TrustedProxies: []string{"192.0.2.0/24"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"password":"adminpass"}`))
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rr.Code, rr.Body.String())
	}
	for _, name := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Permissions-Policy"} {
		if rr.Header().Get(name) == "" {
			t.Errorf("missing %s", name)
		}
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache-control=%q", cc)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("trusted HTTPS proxy cookie=%#v", cookies)
	}

	_, untrusted := authHandler(config.Config{})
	untrustedReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"password":"adminpass"}`))
	untrustedReq.RemoteAddr = "192.0.2.10:1234"
	untrustedReq.Header.Set("X-Forwarded-Proto", "https")
	untrustedRR := httptest.NewRecorder()
	untrusted.ServeHTTP(untrustedRR, untrustedReq)
	if untrustedRR.Result().Cookies()[0].Secure {
		t.Fatal("untrusted forwarded proto marked cookie Secure")
	}

	_, forced := authHandler(config.Config{SecureCookies: true})
	forcedRR := loginRequest(forced, "192.0.2.10:1234", "", "adminpass")
	if !forcedRR.Result().Cookies()[0].Secure {
		t.Fatal("PALHELM_SECURE_COOKIES did not mark cookie Secure")
	}
}
