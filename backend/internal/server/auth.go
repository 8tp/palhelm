package server

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const sessionCookie = "palhelm_session"

type principal struct{ Role, Username string }
type authKey struct{}
type auth struct {
	secret        []byte
	admin, viewer string
	now           func() time.Time
	mu            sync.Mutex
	attempts      map[string]attemptBucket
	trusted       []*net.IPNet
}

type attemptBucket struct {
	attempts []time.Time
	expires  time.Time
}

const (
	loginAttemptLimit = 5
	loginWindow       = time.Minute
	maxLimiterEntries = 4096
)

func newAuth(secret, admin, viewer string, trustedCIDRs ...string) *auth {
	a := &auth{secret: []byte(secret), admin: admin, viewer: viewer, now: time.Now, attempts: make(map[string]attemptBucket)}
	for _, cidr := range trustedCIDRs {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			a.trusted = append(a.trusted, network)
		}
	}
	return a
}
func (a *auth) token(role string, expiry time.Time) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"role": role, "username": role, "exp": expiry.Unix(), "iat": a.now().Unix()}).SignedString(a.secret)
}
func (a *auth) parse(v string) (principal, error) {
	t, err := jwt.Parse(v, func(t *jwt.Token) (any, error) { return a.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(a.now))
	if err != nil || !t.Valid {
		return principal{}, err
	}
	c := t.Claims.(jwt.MapClaims)
	role, _ := c["role"].(string)
	user, _ := c["username"].(string)
	if role != "admin" && role != "viewer" {
		return principal{}, jwt.ErrTokenInvalidClaims
	}
	return principal{role, user}, nil
}
func (a *auth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "A valid session is required.")
			return
		}
		p, err := a.parse(c.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "The session is invalid or expired.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authKey{}, p)))
	})
}
func adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := r.Context().Value(authKey{}).(principal)
		if p.Role != "admin" {
			writeError(w, http.StatusForbidden, "forbidden", "Administrator access is required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *auth) allow(ip string) bool {
	now := a.now()
	cut := now.Add(-loginWindow)
	a.mu.Lock()
	defer a.mu.Unlock()
	for key, bucket := range a.attempts {
		if !bucket.expires.After(now) {
			delete(a.attempts, key)
		}
	}
	bucket, exists := a.attempts[ip]
	if !exists && len(a.attempts) >= maxLimiterEntries {
		return false
	}
	v := bucket.attempts[:0]
	for _, t := range bucket.attempts {
		if t.After(cut) {
			v = append(v, t)
		}
	}
	if len(v) >= loginAttemptLimit {
		a.attempts[ip] = attemptBucket{attempts: v, expires: now.Add(loginWindow)}
		return false
	}
	a.attempts[ip] = attemptBucket{attempts: append(v, now), expires: now.Add(loginWindow)}
	return true
}

func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(remoteAddr)
}

func (a *auth) isTrusted(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, network := range a.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP returns a forwarding header identity only when the transport peer is
// trusted. Walking from the nearest hop prevents a client-supplied leftmost value
// from overriding an address appended by a correctly configured proxy chain.
func (a *auth) clientIP(r *http.Request) string {
	peer := remoteIP(r.RemoteAddr)
	if !a.isTrusted(peer) {
		if peer != nil {
			return peer.String()
		}
		return r.RemoteAddr
	}
	raw := r.Header.Get("X-Forwarded-For")
	if raw == "" {
		raw = r.Header.Get("X-Real-IP")
	}
	if raw == "" {
		return peer.String()
	}
	parts := strings.Split(raw, ",")
	chain := make([]net.IP, 0, len(parts))
	for _, part := range parts {
		ip := net.ParseIP(strings.TrimSpace(part))
		if ip == nil {
			return peer.String()
		}
		chain = append(chain, ip)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if !a.isTrusted(chain[i]) {
			return chain[i].String()
		}
	}
	return chain[0].String()
}

func (a *auth) forwardedHTTPS(r *http.Request) bool {
	if !a.isTrusted(remoteIP(r.RemoteAddr)) {
		return false
	}
	values := strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(values[len(values)-1]), "https")
}
func secureEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func principalFrom(r *http.Request) principal {
	p, _ := r.Context().Value(authKey{}).(principal)
	return p
}
