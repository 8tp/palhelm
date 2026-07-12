package server

// Adversarial auth/abuse-control audit for the v0.4.0 Integration API
// (docs/specs/integration-api.md §2, §3, §8, §9). These tests attack the areas the existing
// suite leaves under-proven: the uniform-401 oracle property and zero-state-allocation proven
// *through the real HTTP middleware* (not just the Validate core), limiter behavior beyond the
// implementer's happy-path cases (an existing bucket still served when the map is full, the
// documented in-flight bucket-recreation caveat, X-Real-IP evasion), plaintext-key containment
// across every response surface the server can emit, the 100-key cap under concurrent creates,
// and the guarantee that no slog line ever carries key material.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"log/slog"

	"github.com/8tp/palhelm/internal/config"
)

// TestUniform401AndZeroStateAcrossEveryFailureMode is the H4/oracle proof at the HTTP layer.
//
//	Threat: (a) any 401 variant (distinct body, missing WWW-Authenticate, distinct status)
//	        acting as an enumeration oracle - "format-valid key" distinguishable from garbage,
//	        or "known id" from unknown; (b) any failed auth allocating limiter/cache/lastUsedAt
//	        state (the H4 memory-growth primitive).
//	Test: drive the real router with six failure modes - missing header, malformed token,
//	      wrong scheme, unknown well-formed id, known id + wrong secret, revoked key - and
//	      assert byte-identical body, identical status 401, identical WWW-Authenticate: Bearer;
//	      then inspect the live integrationAuth internals and assert zero limiter buckets, an
//	      unchanged key-cache size, and that the valid key's lastUsedAt was never touched.
//	Closes it: every failure routes through integrationUnauthorized (one body, one header) and
//	           the middleware allocates the limiter bucket only *after* Validate succeeds, so a
//	           rejected request touches no map that outlives it.
//	Edge: the "known id + wrong secret" mode is the sharpest - it proves a *found* id whose
//	      compare fails still allocates no bucket and is byte-indistinguishable from an unknown id.
func TestUniform401AndZeroStateAcrossEveryFailureMode(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	validToken := issueTestAPIKey(t, s, st, "aaaaaaaa", "valid")
	// A separately created key that we revoke, to exercise the revoked-key failure mode.
	revokedToken := issueTestAPIKey(t, s, st, "bbbbbbbb", "revoked")
	s.integration.Revoke("bbbbbbbb")

	keyCacheBefore := len(s.integration.keys)

	wrongSecret := "phk_aaaaaaaa_" + strings.Repeat("Z", 43) // known id, wrong secret
	unknownID := "phk_ffffffff_" + strings.Repeat("Z", 43)   // well-formed, unknown id

	type mode struct{ name, header string }
	modes := []mode{
		{"missing header", ""},
		{"malformed token", "Bearer not-a-valid-token"},
		{"wrong scheme", "Basic " + validToken},
		{"unknown well-formed id", "Bearer " + unknownID},
		{"known id wrong secret", "Bearer " + wrongSecret},
		{"revoked key", "Bearer " + revokedToken},
	}

	var refBody string
	for i, m := range modes {
		req := httptest.NewRequest("GET", "/api/integration/v1/guilds", nil)
		if m.header != "" {
			req.Header.Set("Authorization", m.header)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("mode %q: status = %d, want 401", m.name, rr.Code)
		}
		if got := rr.Header().Get("WWW-Authenticate"); got != "Bearer" {
			t.Errorf("mode %q: WWW-Authenticate = %q, want Bearer", m.name, got)
		}
		if got := rr.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("mode %q: Cache-Control = %q, want no-store", m.name, got)
		}
		if i == 0 {
			refBody = rr.Body.String()
		} else if rr.Body.String() != refBody {
			t.Errorf("mode %q: body = %q, differs from reference %q (a distinguishable 401 is an oracle)", m.name, rr.Body.String(), refBody)
		}
	}

	// The reference body is exactly the documented uniform envelope.
	wantBody := `{"error":{"code":"unauthorized","message":"A valid API key is required."}}` + "\n"
	if refBody != wantBody {
		t.Errorf("uniform 401 body = %q, want %q", refBody, wantBody)
	}

	// Zero state: no failed request created a limiter bucket, grew the key cache, or advanced
	// any lastUsedAt.
	if got := s.integration.limiter.size(); got != 0 {
		t.Errorf("limiter allocated %d buckets across failed auth attempts, want 0 (H4)", got)
	}
	if got := len(s.integration.keys); got != keyCacheBefore {
		t.Errorf("key cache grew from %d to %d across failed auth attempts, want unchanged", keyCacheBefore, got)
	}
	s.integration.mu.Lock()
	entry := s.integration.keys["aaaaaaaa"]
	touched := !entry.lastUsed.IsZero() || !entry.lastPersisted.IsZero()
	s.integration.mu.Unlock()
	if touched {
		t.Errorf("valid key's lastUsedAt was advanced by a failed auth attempt using its id (wrong secret)")
	}
}

// TestValidateConstantTimeBothPathsAllocateNothing is the structural constant-time proof
// (spec §2.3, §12.3): the unknown-id path and the known-id-wrong-secret path both run the same
// sha256 + fixed-length subtle.ConstantTimeCompare and both return false without mutating state.
//
//	Verdict (by code review, stated in the audit report): Validate is constant-time by
//	construction - it always computes sha256(token) and always calls ConstantTimeCompare over
//	two 32-byte digests (the real digest on a hit, the package dummyDigest on a miss), with no
//	early return before the compare (the revoked check is evaluated after match). The only
//	non-constant step is the map lookup on the *public, non-secret* key id, which discloses
//	nothing the UI/logs do not already treat as public.
func TestValidateConstantTimeBothPathsAllocateNothing(t *testing.T) {
	auth := newIntegrationAuth(&countingKeyStore{}, nil, 60, testLogger())
	hash, token := newTestKeyHash("aaaaaaaa")
	auth.Add("aaaaaaaa", hash, "bot")
	sizeBefore := len(auth.keys)

	// Known id, wrong secret: same id substring as a real key, compare fails.
	wrongSecret := "phk_aaaaaaaa_" + strings.Repeat("Q", 43)
	if _, ok := auth.Validate(wrongSecret); ok {
		t.Fatal("known id with wrong secret validated")
	}
	// Unknown id: dummyDigest compare, fails.
	_, unknown := newTestKeyHash("ffffffff")
	if _, ok := auth.Validate(unknown); ok {
		t.Fatal("unknown id validated")
	}
	// The valid token still validates (sanity: the wrong-secret attempt did not corrupt state).
	if _, ok := auth.Validate(token); !ok {
		t.Fatal("valid token stopped validating after failed attempts")
	}
	if got := len(auth.keys); got != sizeBefore {
		t.Fatalf("key cache size changed from %d to %d across Validate calls", sizeBefore, got)
	}
	if got := auth.limiter.size(); got != 0 {
		t.Fatalf("Validate allocated %d limiter buckets, want 0 (Validate must never touch the limiter)", got)
	}
}

// TestLimiterExistingKeyStillServedWhenMapFull is the adversarial fail-closed case the
// implementer's test omits.
//
//	Threat: the 1024-entry fail-closed bound denying service to keys that ALREADY hold a bucket
//	        (a legitimate key starved because the map is full of others) - or, conversely, the
//	        bound not actually firing for a brand-new key.
//	Test: fill the map to exactly maxIntegrationLimiterEntries, then prove a key that already
//	      owns a bucket is still allowed, while a brand-new key id is denied with Retry-After
//	      equal to the full window.
//	Closes it: allow() looks up the existing bucket before the capacity check, so an in-map key
//	           is never subject to the fail-closed branch; only a *new* id arriving at capacity
//	           is denied.
func TestLimiterExistingKeyStillServedWhenMapFull(t *testing.T) {
	limiter := newIntegrationLimiter(60)
	now := time.Unix(1_700_000_000, 0)
	limiter.now = func() time.Time { return now }

	for i := 0; i < maxIntegrationLimiterEntries; i++ {
		if ok, _ := limiter.allow("key-" + strconv.Itoa(i)); !ok {
			t.Fatalf("entry %d denied while filling the map", i)
		}
	}
	if got := limiter.size(); got != maxIntegrationLimiterEntries {
		t.Fatalf("map size = %d, want %d (full)", got, maxIntegrationLimiterEntries)
	}
	// An existing key still gets served despite the map being full.
	if ok, _ := limiter.allow("key-5"); !ok {
		t.Fatal("a key with an existing bucket was denied while the map was full (starvation)")
	}
	// A brand-new key is fail-closed with the full-window Retry-After.
	ok, retryAfter := limiter.allow("brand-new-key")
	if ok {
		t.Fatal("a new key id was admitted past the fail-closed bound")
	}
	if retryAfter != 60 {
		t.Fatalf("fail-closed Retry-After = %d, want 60 (full window)", retryAfter)
	}
}

// TestLimiterRetryAfterCountsDownAndWindowRestoresExactly proves Retry-After sanity and exact
// window restoration (spec §8.1).
//
//	Threat: a nonsensical Retry-After (<=0, or larger than the window) that either invites a
//	        tight retry loop or over-throttles; and a window that does not restore service after
//	        exactly one window elapses.
//	Test: with a fixed clock and limit=2, exhaust the window, then read Retry-After at several
//	      offsets and assert it counts down monotonically within [1,60]; at exactly window+epsilon
//	      the key is served again.
//	Closes it: allow() computes Retry-After = ceil(oldest + window - now), floored at 1, and
//	           prunes hits older than the window before counting.
func TestLimiterRetryAfterCountsDownAndWindowRestoresExactly(t *testing.T) {
	limiter := newIntegrationLimiter(2)
	base := time.Unix(1_700_000_000, 0)
	cur := base
	limiter.now = func() time.Time { return cur }

	// Two hits at t=base fill the window (limit 2).
	if ok, _ := limiter.allow("k"); !ok {
		t.Fatal("first hit denied")
	}
	if ok, _ := limiter.allow("k"); !ok {
		t.Fatal("second hit denied")
	}
	// Third hit at t=base is denied; oldest hit ages out in 60s.
	ok, ra := limiter.allow("k")
	if ok || ra != 60 {
		t.Fatalf("denied hit at t=base: ok=%v retryAfter=%d, want denied with 60", ok, ra)
	}
	// 30s later, Retry-After should be ~30 and strictly less than before.
	cur = base.Add(30 * time.Second)
	ok, ra30 := limiter.allow("k")
	if ok || ra30 < 1 || ra30 > 30 {
		t.Fatalf("denied hit at t=base+30s: ok=%v retryAfter=%d, want denied with 1<=ra<=30", ok, ra30)
	}
	// At exactly one window past the oldest hit, the bucket restores.
	cur = base.Add(time.Minute + time.Nanosecond)
	if ok, _ := limiter.allow("k"); !ok {
		t.Fatal("key not served after exactly one window elapsed (window did not restore)")
	}
}

// TestLimiterIgnoresXRealIP extends the H4 lesson to X-Real-IP (the existing suite only covers
// X-Forwarded-For): the limiter keys on token identity, so no forwarded header can fragment or
// evade it. Two requests with different spoofed X-Real-IP values but the same key still share a
// single bucket, so the second is throttled.
func TestLimiterIgnoresXRealIP(t *testing.T) {
	st, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48), IntegrationRateLimit: 1}
	s, h := New(cfg, st, testLogger())
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "bot")

	req1 := httptest.NewRequest("GET", "/api/integration/v1/guilds", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	req1.Header.Set("X-Real-IP", "10.0.0.1")
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request = %d: %s", rr1.Code, rr1.Body.String())
	}
	req2 := httptest.NewRequest("GET", "/api/integration/v1/guilds", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("X-Real-IP", "10.0.0.2") // different spoofed source, same key
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request with a different X-Real-IP = %d, want 429 (limiter must key on the token, not IP)", rr2.Code)
	}
}

// TestRevokeInFlightBucketRecreationMatchesSpecBound verifies the honest bound the spec
// documents in §8.1: revocation deletes the bucket in the same critical section as the cache
// flip, but a request already in flight (one that passed Validate before the revoke) can call
// Allow afterward and transiently re-create the bucket.
//
//	Threat: either dead buckets accumulating across revoke/create churn (the ">=100 by
//	        construction" claim being false), or the re-creation being unbounded.
//	Test: Add a key, Allow (bucket exists), Revoke (bucket deleted AND key invalidated), then
//	      Allow again on the same id (the in-flight caveat) - the bucket reappears, exactly once,
//	      and a fresh request can no longer reach Allow because Validate now fails.
//	Closes it: this is the spec's stated behavior, not a defect - the transient bucket is bounded
//	           by in-flight concurrency, expires after one window, and is absorbed by the 1024
//	           map bound. The audit confirms behavior matches the documented bound.
func TestRevokeInFlightBucketRecreationMatchesSpecBound(t *testing.T) {
	auth := newIntegrationAuth(&countingKeyStore{}, nil, 60, testLogger())
	hash, token := newTestKeyHash("aaaaaaaa")
	auth.Add("aaaaaaaa", hash, "bot")

	if ok, _ := auth.Allow("aaaaaaaa"); !ok {
		t.Fatal("fresh key denied")
	}
	if got := auth.limiter.size(); got != 1 {
		t.Fatalf("limiter size after one request = %d, want 1", got)
	}

	auth.Revoke("aaaaaaaa")
	// Revoke both invalidates the key and deletes its bucket in one critical section.
	if _, ok := auth.Validate(token); ok {
		t.Fatal("revoked key still validates")
	}
	if got := auth.limiter.size(); got != 0 {
		t.Fatalf("limiter size after Revoke = %d, want 0 (bucket must be deleted synchronously)", got)
	}

	// The documented in-flight caveat: an already-validated request calling Allow re-creates the
	// bucket transiently. It is bounded (one entry) and, crucially, unreachable by any *new*
	// request because Validate now fails before Allow is ever called.
	if ok, _ := auth.Allow("aaaaaaaa"); !ok {
		t.Fatal("Allow denied on the in-flight recreation path")
	}
	if got := auth.limiter.size(); got != 1 {
		t.Fatalf("in-flight recreation left %d buckets, want exactly 1 (bounded caveat)", got)
	}
}

// TestParseBearerRejectsNULAndSurroundingWhitespace covers header-parsing pathologies the
// existing variant matrix omits: an embedded NUL byte and leading/trailing whitespace around
// the value (spec §3 - anything not matching the exact grammar is the uniform 401).
func TestParseBearerRejectsNULAndSurroundingWhitespace(t *testing.T) {
	good := "phk_aaaaaaaa_" + strings.Repeat("A", 43)
	cases := []struct{ name, value string }{
		{"embedded NUL in secret", "Bearer phk_aaaaaaaa_" + strings.Repeat("A", 42) + "\x00"},
		{"NUL between scheme and token", "Bearer\x00" + good},
		{"leading space before scheme", " Bearer " + good},
		{"trailing space after token", "Bearer " + good + " "},
		{"internal NUL in id", "Bearer phk_aaaa\x00aaa_" + strings.Repeat("A", 43)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", c.value)
			if _, ok := parseBearer(r); ok {
				t.Fatalf("parseBearer accepted a pathological header: %q", c.value)
			}
		})
	}
}

// TestPlaintextKeyAppearsInExactlyOneResponseEver is the string-level containment proof across
// every response surface the server can emit (spec §2.2, §2.4, §12.8).
//
//	Threat: the plaintext key or its secret leaking into any response other than the single 201
//	        create body - the admin list, the audit-events feed, the public OpenAPI document, an
//	        integration 401, or a validation-error body.
//	Test: create a key (the one legitimate appearance), then scan the raw bytes of the list,
//	      events, /api/openapi.json, an integration 401, and a bad-create 400 response and assert
//	      neither the full key nor its 43-char secret substring appears in any of them.
//	Closes it: the key is written only into the create response struct; every other surface
//	           carries {id, label} or a digest.
func TestPlaintextKeyAppearsInExactlyOneResponseEver(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"containment"}`, admin)
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	key := created["key"].(string)
	id := created["id"].(string)
	secret := strings.TrimPrefix(key, "phk_"+id+"_")
	if len(secret) != 43 {
		t.Fatalf("unexpected secret length %d", len(secret))
	}

	// The created key can actually authenticate, so this is not a vacuous fixture.
	if rr := integrationRequest(h, "GET", "/api/integration/v1/metrics/current", key); rr.Code != http.StatusOK {
		t.Fatalf("created key failed to authenticate = %d: %s", rr.Code, rr.Body.String())
	}

	surfaces := map[string]string{
		"admin list":        sessionRequest(h, "GET", "/api/v1/integration-keys", "", admin).Body.String(),
		"audit events":      sessionRequest(h, "GET", "/api/v1/events?limit=100", "", admin).Body.String(),
		"public openapi":    sessionRequest(h, "GET", "/api/openapi.json", "", nil).Body.String(),
		"integration 401":   integrationRequest(h, "GET", "/api/integration/v1/guilds", "").Body.String(),
		"bad create 400":    sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":""}`, admin).Body.String(),
		"unknown-id revoke": sessionRequest(h, "DELETE", "/api/v1/integration-keys/deadbeef", "", admin).Body.String(),
	}
	for name, body := range surfaces {
		if strings.Contains(body, key) {
			t.Errorf("%s response leaked the full plaintext key", name)
		}
		if strings.Contains(body, secret) {
			t.Errorf("%s response leaked the key's secret substring", name)
		}
	}

}

// TestCreateKeyCapUnderConcurrentCreatesIsBoundedAndConsistent honestly probes the 100-active
// cap for a TOCTOU race (spec §2.6, §9).
//
//	Threat: concurrent creates at 99 active each reading count<100 before any insert, blowing
//	        past the cap without bound (limiter/cache cardinality unbounded).
//	Test: seed 99 active keys, fire N concurrent creates, then assert the store and cache stay
//	      mutually consistent and the active count never exceeds the strict cap of 100.
//	Fixed: store.CreateAPIKey now enforces the cap atomically - it counts active rows and
//	       inserts inside one transaction, and SetMaxOpenConns(1) means database/sql holds this
//	       Store's single pooled connection exclusively for that transaction's lifetime, so no
//	       concurrent CreateAPIKey call can observe count<100 while another's insert is still
//	       pending. The HTTP handler (integration_keys.go) no longer pre-checks with a separate
//	       ActiveAPIKeys() read; the store's count-then-insert is the sole, authoritative gate.
func TestCreateKeyCapUnderConcurrentCreatesIsBoundedAndConsistent(t *testing.T) {
	s, h, st := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")
	const seeded = maxActiveIntegrationKeys - 1 // 99
	for i := 0; i < seeded; i++ {
		issueTestAPIKey(t, s, st, "seed"+fmtHex4(i), "seed")
	}

	const goroutines = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	created, conflict := 0, 0
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rr := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"race`+strconv.Itoa(n)+`"}`, admin)
			mu.Lock()
			switch rr.Code {
			case http.StatusCreated:
				created++
			case http.StatusConflict:
				conflict++
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	active, err := st.ActiveAPIKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	finalActive := len(active)

	// Consistency: the store's active count equals the seed plus every 201 the handlers returned
	// (no lost or double-counted writes), and the in-memory cache matches the store.
	if finalActive != seeded+created {
		t.Errorf("active count = %d, want seeded(%d)+created(%d)=%d (lost or double writes)", finalActive, seeded, created, seeded+created)
	}
	s.integration.mu.Lock()
	cacheSize := len(s.integration.keys)
	s.integration.mu.Unlock()
	if cacheSize != finalActive {
		t.Errorf("cache size %d != store active count %d (cache/store divergence)", cacheSize, finalActive)
	}
	if created+conflict != goroutines {
		t.Errorf("created(%d)+conflict(%d) != goroutines(%d)", created, conflict, goroutines)
	}
	// Strict: the cap is now atomic (store.CreateAPIKey's count-then-insert transaction), so
	// concurrent creates at 99 active can never push the active count past 100.
	if finalActive > maxActiveIntegrationKeys {
		t.Errorf("active count %d exceeds the %d cap (created=%d conflict=%d) - the store-level cap must be atomic, not a soft bound", finalActive, maxActiveIntegrationKeys, created, conflict)
	}
	t.Logf("cap held: %d active keys (created=%d conflict=%d)", finalActive, created, conflict)
}

// syncBuffer is a mutex-guarded writer so a slog handler can be scanned from a test without a
// data race, even if a log line is emitted from a handler goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}
func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestSlogLinesNeverContainKeyMaterial proves the request-attribution log line (spec §2.4)
// carries the public key id but never the token or secret, and that a failed auth logs no token.
//
//	Threat: the "integration request" slog line (or any startup/error line) echoing the bearer
//	        token into logs, defeating the "logs carry {id,label} only" invariant.
//	Test: wire the server's logger to an in-memory buffer, make one authenticated request and one
//	      failed request, then assert the buffer contains the key id but neither the full token
//	      nor its secret substring.
//	Closes it: the middleware logs keyId (the public id) only; parseBearer/Validate failures log
//	           nothing about the token.
func TestSlogLinesNeverContainKeyMaterial(t *testing.T) {
	st, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48), IntegrationRateLimit: 60}
	s, h := New(cfg, st, logger)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "log-audit")
	secret := strings.TrimPrefix(token, "phk_aaaaaaaa_")

	if rr := integrationRequest(h, "GET", "/api/integration/v1/metrics/current", token); rr.Code != http.StatusOK {
		t.Fatalf("authenticated request = %d: %s", rr.Code, rr.Body.String())
	}
	// A failed auth using a well-formed but unknown token must not log the token either.
	unknown := "phk_ffffffff_" + strings.Repeat("Z", 43)
	_ = integrationRequest(h, "GET", "/api/integration/v1/metrics/current", unknown)

	logs := buf.String()
	if !strings.Contains(logs, "aaaaaaaa") {
		t.Errorf("expected the request-attribution log to carry the public key id; logs:\n%s", logs)
	}
	if strings.Contains(logs, token) {
		t.Errorf("slog output leaked the full bearer token")
	}
	if strings.Contains(logs, secret) {
		t.Errorf("slog output leaked the token's secret substring")
	}
	if strings.Contains(logs, "ffffffff") {
		t.Errorf("slog output logged the unknown-key id from a failed auth (no state, no attribution should occur)")
	}
}
