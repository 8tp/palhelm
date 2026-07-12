package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/8tp/palhelm/internal/store"
)

// apiKeyPrincipal identifies a validated bearer request. It is a distinct type from
// principal (auth.go) and is stored under a distinct context key (integrationKey{}, never
// authKey{}) so no session-gated handler can ever mistake a bearer request for an
// authenticated session, and no integration handler can read a session cookie's identity
// (spec §1).
type apiKeyPrincipal struct{ ID, Label string }

// integrationKey is the context key holding apiKeyPrincipal on integration requests.
type integrationKey struct{}

// tokenPattern is the exact grammar of spec §3: "phk_" + 8 lowercase hex + "_" + 43
// base64url characters, 56 bytes total. Anything else is a malformed token.
var tokenPattern = regexp.MustCompile(`^phk_[0-9a-f]{8}_[A-Za-z0-9_-]{43}$`)

// dummyDigest is compared against on an unknown key id so the not-found path performs the
// exact same hash-and-compare shape as the known-id path, equalizing found-vs-not-found
// timing (spec §2.3, §12.3). Its value is arbitrary; only its fixed 32-byte length matters.
var dummyDigest = sha256.Sum256([]byte("phk_00000000_dummy-comparison-digest-do-not-use"))

// integrationKeyEntry is one cached, validated key's mutable state. Fields are only ever
// read or written while integrationAuth.mu is held.
type integrationKeyEntry struct {
	digest        [32]byte
	label         string
	revoked       bool
	lastUsed      time.Time // freshest known lastUsedAt, merged by the admin list handler
	lastPersisted time.Time // CAS marker for the §2.5 write-coalescing rule
}

// apiKeyStore is the store subset integrationAuth needs for lastUsedAt persistence,
// narrowed to an interface (rather than *store.Store directly) so the §2.5 write-coalescing
// CAS can be tested with a call-counting fake instead of a real database (spec §12.9).
// *store.Store satisfies this structurally.
type apiKeyStore interface {
	TouchAPIKeyLastUsed(ctx context.Context, id string, at time.Time) error
}

// integrationAuth is the in-memory validation cache, rate limiter, and lastUsedAt
// coalescer for the bearer-token integration API (spec §2). It is the reusable core the
// admin key-management handlers (POST/GET/DELETE /api/v1/integration-keys, built
// separately) call into via Add and Revoke to keep the cache synchronously consistent with
// the store.
type integrationAuth struct {
	store   apiKeyStore
	limiter *integrationLimiter
	log     *slog.Logger
	now     func() time.Time
	mu      sync.Mutex
	keys    map[string]*integrationKeyEntry
}

// newIntegrationAuth builds the cache from a startup snapshot of active keys (store.
// ActiveAPIKeys). Callers that fail to load that snapshot should pass nil rather than
// fail startup: an empty cache fails every bearer request closed (uniform 401) rather than
// crashing the whole panel over a transient DB read.
func newIntegrationAuth(st apiKeyStore, active []store.APIKey, rateLimit int, log *slog.Logger) *integrationAuth {
	keys := make(map[string]*integrationKeyEntry, len(active))
	for _, k := range active {
		entry := &integrationKeyEntry{digest: k.Hash, label: k.Label}
		if k.LastUsedAt != nil {
			entry.lastUsed = *k.LastUsedAt
			entry.lastPersisted = *k.LastUsedAt
		}
		keys[k.ID] = entry
	}
	return &integrationAuth{store: st, limiter: newIntegrationLimiter(rateLimit), log: log, now: time.Now, keys: keys}
}

// Validate checks a full plaintext token (already format-validated by parseBearer) against
// the cache. The hash-and-compare happens exactly once, against the real digest on a known
// id or the dummy digest on an unknown one, so the two paths are structurally identical in
// the work they perform (spec §2.3, §12.3). The cache lookup itself is a short, mutex-held
// copy; comparison happens outside the lock.
func (a *integrationAuth) Validate(token string) (apiKeyPrincipal, bool) {
	id := token[4:12]
	digest := sha256.Sum256([]byte(token))
	a.mu.Lock()
	entry, ok := a.keys[id]
	stored, label, revoked := dummyDigest, "", false
	if ok {
		stored, label, revoked = entry.digest, entry.label, entry.revoked
	}
	a.mu.Unlock()
	match := subtle.ConstantTimeCompare(digest[:], stored[:]) == 1
	if !ok || !match || revoked {
		return apiKeyPrincipal{}, false
	}
	return apiKeyPrincipal{ID: id, Label: label}, true
}

// Add inserts or replaces a key's cache entry. Called by the create handler after
// store.CreateAPIKey succeeds, synchronizing the cache in the same request.
func (a *integrationAuth) Add(id string, hash [32]byte, label string) {
	a.mu.Lock()
	a.keys[id] = &integrationKeyEntry{digest: hash, label: label}
	a.mu.Unlock()
}

// Revoke flips a key's cache entry to revoked and deletes its limiter bucket in the same
// critical section (spec §2.6, §8.1): the next request for this key fails validation before
// any handler runs, and no revoked key can leave a dead limiter bucket behind. Unknown ids
// are a no-op (idempotent revoke is a store-layer concern; the cache simply has nothing to
// flip).
func (a *integrationAuth) Revoke(id string) {
	a.mu.Lock()
	if entry, ok := a.keys[id]; ok {
		entry.revoked = true
	}
	a.limiter.delete(id)
	a.mu.Unlock()
}

// LastUsed returns the freshest known lastUsedAt for id, for the admin list handler to
// merge over the store's (coalesced, so possibly stale) column value.
func (a *integrationAuth) LastUsed(id string) (time.Time, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.keys[id]
	if !ok || entry.lastUsed.IsZero() {
		return time.Time{}, false
	}
	return entry.lastUsed, true
}

// Touch records a key's use and persists lastUsedAt to the store at most once per 60
// seconds (spec §2.5), via compare-and-set under the mutex: the persisted marker advances
// before the mutex releases, so exactly one caller among any number of concurrent requests
// on the same key performs the write, which happens outside the mutex.
func (a *integrationAuth) Touch(ctx context.Context, id string, at time.Time) {
	a.mu.Lock()
	entry, ok := a.keys[id]
	write := false
	if ok {
		entry.lastUsed = at
		if at.Sub(entry.lastPersisted) > time.Minute {
			entry.lastPersisted = at
			write = true
		}
	}
	a.mu.Unlock()
	if write {
		if err := a.store.TouchAPIKeyLastUsed(ctx, id, at); err != nil {
			a.log.Error("persist integration api key last used", "id", id, "error", err)
		}
	}
}

// Flush best-effort persists every entry whose in-memory lastUsed is newer than what was
// last written, so the final minute of activity before a graceful shutdown is not lost
// (spec §2.5). Errors are logged, never returned: shutdown must not block on them.
func (a *integrationAuth) Flush(ctx context.Context) {
	if a == nil {
		// Some tests build a bare &Server{} for a single handler under test and never wire
		// an integrationAuth; CloseStreams still calls Flush unconditionally on shutdown, so
		// this must be a safe no-op rather than a nil-pointer panic.
		return
	}
	type pending struct {
		id string
		at time.Time
	}
	a.mu.Lock()
	var writes []pending
	for id, entry := range a.keys {
		if entry.lastUsed.After(entry.lastPersisted) {
			entry.lastPersisted = entry.lastUsed
			writes = append(writes, pending{id, entry.lastUsed})
		}
	}
	a.mu.Unlock()
	for _, w := range writes {
		if err := a.store.TouchAPIKeyLastUsed(ctx, w.id, w.at); err != nil {
			a.log.Error("flush integration api key last used", "id", w.id, "error", err)
		}
	}
}

// Allow reports whether id may proceed under the per-key rate limiter, and the seconds a
// caller should wait before retrying when it may not (spec §8.1).
func (a *integrationAuth) Allow(id string) (bool, int) { return a.limiter.allow(id) }

// parseBearer extracts and validates a bearer token per spec §3's exact grammar. Any
// deviation - missing/duplicate header, wrong scheme, wrong spacing, wrong length or
// charset - returns ok=false, uniformly mapped to a 401 by the caller. This function
// allocates nothing that outlives the request and touches no store or limiter state
// (spec §8.2, the H4 lesson).
func parseBearer(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	if !tokenPattern.MatchString(token) {
		return "", false
	}
	return token, true
}

// integrationUnauthorized writes the uniform 401 required for every bearer failure mode
// (spec §2.3, §3): identical body and headers regardless of why validation failed, so no
// response variant can act as an oracle.
func integrationUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthorized", "A valid API key is required.")
}

// integrationLimiter is the per-key-id sliding-window rate limiter (spec §8.1), mirroring
// auth.go's login limiter: expiring buckets, bounded map, fail-closed when full.
type integrationLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]integrationBucket
	now     func() time.Time
}

type integrationBucket struct {
	hits    []time.Time
	expires time.Time
}

// maxIntegrationLimiterEntries bounds the limiter map even under pathological revoke/create
// churn; steady-state cardinality tracks the active-key count (<=100, spec §2.6), far below
// this bound, which remains as defense in depth (spec §8.1).
const maxIntegrationLimiterEntries = 1024

func newIntegrationLimiter(limit int) *integrationLimiter {
	return &integrationLimiter{limit: limit, window: time.Minute, buckets: make(map[string]integrationBucket), now: time.Now}
}

// allow reports whether id may proceed, and on denial the seconds until retry is sensible.
// A request for a key with no existing bucket, arriving when the map is already at its
// bound, is denied fail-closed with the full window as Retry-After (spec §8.1) - there is
// no oldest timestamp to compute a tighter value from.
func (l *integrationLimiter) allow(id string) (bool, int) {
	now := l.now()
	cut := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if !b.expires.After(now) {
			delete(l.buckets, k)
		}
	}
	bucket, exists := l.buckets[id]
	if !exists && len(l.buckets) >= maxIntegrationLimiterEntries {
		return false, int(l.window.Seconds())
	}
	v := bucket.hits[:0]
	for _, t := range bucket.hits {
		if t.After(cut) {
			v = append(v, t)
		}
	}
	if len(v) >= l.limit {
		l.buckets[id] = integrationBucket{hits: v, expires: now.Add(l.window)}
		retryAfter := int(math.Ceil(v[0].Add(l.window).Sub(now).Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}
	l.buckets[id] = integrationBucket{hits: append(v, now), expires: now.Add(l.window)}
	return true, 0
}

// delete removes id's bucket, if any. Called by integrationAuth.Revoke in the same critical
// section as the cache flip (spec §2.6).
func (l *integrationLimiter) delete(id string) {
	l.mu.Lock()
	delete(l.buckets, id)
	l.mu.Unlock()
}

// size reports the current bucket count, for tests proving no state is allocated before
// validation succeeds (spec §12.2).
func (l *integrationLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
