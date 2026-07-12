package server

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/store"
)

// TestParseBearerAcceptsValidTokenCaseInsensitiveScheme proves spec §3: the scheme is
// matched ASCII case-insensitively, and a well-formed token is extracted.
func TestParseBearerAcceptsValidTokenCaseInsensitiveScheme(t *testing.T) {
	token := "phk_aaaaaaaa_" + strings.Repeat("A", 43)
	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "BeAreR"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", scheme+" "+token)
		got, ok := parseBearer(r)
		if !ok || got != token {
			t.Errorf("scheme %q: got=(%q,%v), want (%q,true)", scheme, got, ok, token)
		}
	}
}

// TestParseBearerRejectsMalformedVariants exercises spec §3's exact grammar: every
// deviation - missing/duplicate header, wrong spacing, trailing garbage, wrong length,
// wrong charset (uppercase hex id) - is rejected. This function allocates nothing that
// outlives the request and touches no store or limiter state (the H4 lesson), which is
// exactly why it can be tested with a bare *http.Request and no server at all.
func TestParseBearerRejectsMalformedVariants(t *testing.T) {
	token := "phk_aaaaaaaa_" + strings.Repeat("A", 43)
	cases := []struct {
		name   string
		values []string
	}{
		{"missing header", nil},
		{"duplicate headers", []string{"Bearer " + token, "Bearer " + token}},
		{"double space", []string{"Bearer  " + token}},
		{"tab between scheme and token", []string{"Bearer\t" + token}},
		{"trailing garbage", []string{"Bearer " + token + "x"}},
		{"55-char token", []string{"Bearer " + token[:len(token)-1]}},
		{"57-char token", []string{"Bearer " + token + "A"}},
		{"uppercase hex id", []string{"Bearer phk_AAAAAAAA_" + strings.Repeat("A", 43)}},
		{"wrong prefix", []string{"Basic " + token}},
		{"empty token", []string{"Bearer "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			for _, v := range tc.values {
				r.Header.Add("Authorization", v)
			}
			if _, ok := parseBearer(r); ok {
				t.Fatalf("parseBearer accepted a malformed header: %v", tc.values)
			}
		})
	}
}

// TestIntegrationAuthValidateMatrix proves spec §2.3: a known id with the right secret
// validates; an unknown id, a wrong secret, and a revoked key all fail - each via the same
// Validate call, so the not-found and found paths share one code path (spec §12.3).
func TestIntegrationAuthValidateMatrix(t *testing.T) {
	auth := newIntegrationAuth(&countingKeyStore{}, nil, 60, testLogger())
	hash, token := newTestKeyHash("aaaaaaaa")
	auth.Add("aaaaaaaa", hash, "bot")

	if _, ok := auth.Validate(token); !ok {
		t.Fatal("valid token rejected")
	}
	wrongSecret := "phk_aaaaaaaa_" + strings.Repeat("Z", 43)
	if _, ok := auth.Validate(wrongSecret); ok {
		t.Fatal("wrong secret accepted")
	}
	_, unknownToken := newTestKeyHash("ffffffff")
	if _, ok := auth.Validate(unknownToken); ok {
		t.Fatal("unknown id accepted")
	}
	auth.Revoke("aaaaaaaa")
	if _, ok := auth.Validate(token); ok {
		t.Fatal("revoked key still validates")
	}
}

// TestIntegrationAuthNoStateForInvalidToken proves the H4 property at the auth-core level
// (spec §8.2, §12.2): failed Validate calls never grow the cache, and Revoke on an unknown
// id is a harmless no-op rather than fabricating an entry.
func TestIntegrationAuthNoStateForInvalidToken(t *testing.T) {
	auth := newIntegrationAuth(&countingKeyStore{}, nil, 60, testLogger())
	before := len(auth.keys)
	_, unknown := newTestKeyHash("ffffffff")
	auth.Validate(unknown)
	auth.Validate("not-a-real-token")
	auth.Revoke("never-existed")
	if got := len(auth.keys); got != before {
		t.Fatalf("cache grew from invalid input: before=%d after=%d", before, got)
	}
}

// TestIntegrationAuthAddAndRevokeSameCriticalSection proves spec §2.6: Add makes a key
// immediately valid, and Revoke both invalidates it and deletes its limiter bucket in one
// call, so churned buckets cannot accumulate.
func TestIntegrationAuthAddAndRevokeSameCriticalSection(t *testing.T) {
	auth := newIntegrationAuth(&countingKeyStore{}, nil, 60, testLogger())
	hash, token := newTestKeyHash("bbbbbbbb")
	auth.Add("bbbbbbbb", hash, "bot")
	if _, ok := auth.Validate(token); !ok {
		t.Fatal("Add did not make the key valid")
	}
	if ok, _ := auth.Allow("bbbbbbbb"); !ok {
		t.Fatal("Allow unexpectedly denied a fresh key")
	}
	if got := auth.limiter.size(); got != 1 {
		t.Fatalf("limiter size after one request = %d, want 1", got)
	}
	auth.Revoke("bbbbbbbb")
	if _, ok := auth.Validate(token); ok {
		t.Fatal("key still validates after Revoke")
	}
	if got := auth.limiter.size(); got != 0 {
		t.Fatalf("limiter size after Revoke = %d, want 0 (bucket must be deleted synchronously)", got)
	}
}

// TestIntegrationRateLimiterPerKeyIsolationAndExpiry proves spec §8.1: exhausting one key's
// window does not affect another key, and the bucket recovers after the window elapses.
func TestIntegrationRateLimiterPerKeyIsolationAndExpiry(t *testing.T) {
	limiter := newIntegrationLimiter(3)
	now := time.Unix(1_700_000_000, 0)
	limiter.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if ok, _ := limiter.allow("key-a"); !ok {
			t.Fatalf("attempt %d on key-a unexpectedly denied", i+1)
		}
	}
	ok, retryAfter := limiter.allow("key-a")
	if ok || retryAfter <= 0 {
		t.Fatalf("key-a fourth attempt: ok=%v retryAfter=%d, want denied with positive Retry-After", ok, retryAfter)
	}
	// A different key id is unaffected by key-a's exhaustion.
	if ok, _ := limiter.allow("key-b"); !ok {
		t.Fatal("key-b denied despite having its own window")
	}
	// After the window elapses, key-a recovers.
	now = now.Add(time.Minute + time.Second)
	if ok, _ := limiter.allow("key-a"); !ok {
		t.Fatal("key-a still denied after window expiry")
	}
}

// TestIntegrationRateLimiterFailClosedAtBound proves the 1024-entry bound is fail-closed
// (spec §8.1): a brand-new key id arriving when the map is already full is denied, with
// Retry-After equal to the full window since there is no bucket to compute a tighter value
// from.
func TestIntegrationRateLimiterFailClosedAtBound(t *testing.T) {
	limiter := newIntegrationLimiter(60)
	now := time.Unix(1_700_000_000, 0)
	limiter.now = func() time.Time { return now }
	for i := 0; i < maxIntegrationLimiterEntries; i++ {
		if ok, _ := limiter.allow("key-" + strconv.Itoa(i)); !ok {
			t.Fatalf("entry %d unexpectedly denied while filling the map", i)
		}
	}
	ok, retryAfter := limiter.allow("one-too-many")
	if ok || retryAfter != 60 {
		t.Fatalf("over-capacity key: ok=%v retryAfter=%d, want denied with Retry-After=60", ok, retryAfter)
	}
}

// TestIntegrationLastUsedAtCoalescing proves spec §2.5/§12.9: two Touch calls on the same
// key within the 60s window, including two issued concurrently, produce exactly one store
// write, via the mutex-guarded compare-and-set.
func TestIntegrationLastUsedAtCoalescing(t *testing.T) {
	fake := &countingKeyStore{}
	auth := newIntegrationAuth(fake, []store.APIKey{{ID: "ffffffff", Hash: [32]byte{1}, Label: "bot"}}, 60, testLogger())

	base := time.Now().UTC()
	auth.Touch(context.Background(), "ffffffff", base)
	auth.Touch(context.Background(), "ffffffff", base.Add(time.Second))
	if got := fake.callCount(); got != 1 {
		t.Fatalf("sequential touches within window wrote %d times, want 1", got)
	}

	// Two concurrent touches must still coalesce to one write: the mutex-guarded CAS
	// advances the marker before either goroutine's write happens.
	fake2 := &countingKeyStore{}
	auth2 := newIntegrationAuth(fake2, []store.APIKey{{ID: "gggggggg", Hash: [32]byte{2}, Label: "bot"}}, 60, testLogger())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			auth2.Touch(context.Background(), "gggggggg", base)
		}()
	}
	wg.Wait()
	if got := fake2.callCount(); got != 1 {
		t.Fatalf("concurrent touches within window wrote %d times, want 1", got)
	}

	// After the window elapses, a subsequent touch writes again.
	auth.Touch(context.Background(), "ffffffff", base.Add(2*time.Minute))
	if got := fake.callCount(); got != 2 {
		t.Fatalf("touch after window elapsed wrote %d times total, want 2", got)
	}
}

// TestIntegrationAuthFlushPersistsOnlyPendingEntries proves spec §2.5's graceful-shutdown
// flush: an entry already persisted at its current lastUsed value is not rewritten, but one
// with unpersisted activity is.
func TestIntegrationAuthFlushPersistsOnlyPendingEntries(t *testing.T) {
	fake := &countingKeyStore{}
	auth := newIntegrationAuth(fake, []store.APIKey{{ID: "aaaaaaaa", Hash: [32]byte{1}, Label: "bot"}}, 60, testLogger())
	now := time.Now().UTC()

	// Flush with no activity at all writes nothing.
	auth.Flush(context.Background())
	if got := fake.callCount(); got != 0 {
		t.Fatalf("Flush with no activity wrote %d times, want 0", got)
	}

	auth.Touch(context.Background(), "aaaaaaaa", now)
	if got := fake.callCount(); got != 1 {
		t.Fatalf("Touch wrote %d times, want 1", got)
	}
	// Flush immediately after a Touch that already persisted has nothing pending.
	auth.Flush(context.Background())
	if got := fake.callCount(); got != 1 {
		t.Fatalf("Flush with nothing pending wrote %d times, want still 1", got)
	}
}

// countingKeyStore is a minimal apiKeyStore fake that counts writes without touching SQLite.
type countingKeyStore struct {
	mu    sync.Mutex
	calls int
}

func (f *countingKeyStore) TouchAPIKeyLastUsed(ctx context.Context, id string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return nil
}
func (f *countingKeyStore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
