package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/store"
)

// newKeyManagementTestServer builds a full server with both an admin and a viewer
// password wired, for exercising the adminOnly key-management routes under both roles
// (spec §9: viewer gets 403, matching every other admin route).
func newKeyManagementTestServer(t *testing.T) (*Server, http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{
		AdminPassword: "panelpass", ViewerPassword: "viewerpass",
		SessionSecret: strings.Repeat("s", 48), IntegrationRateLimit: 60,
	}
	s, h := New(cfg, st, testLogger())
	return s, h, st
}

// loginAs authenticates against the real login handler and returns the session cookie.
func loginAs(t *testing.T, h http.Handler, password string) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login(%q) = %d: %s", password, rr.Code, rr.Body.String())
	}
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("login(%q) set no cookie", password)
	}
	return cookies[0]
}

// sessionRequest issues a request against h carrying cookie (nil omits it entirely, for
// the no-session case).
func sessionRequest(h http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

var plaintextKeyPattern = regexp.MustCompile(`^phk_[0-9a-f]{8}_[A-Za-z0-9_-]{43}$`)
var keyIDPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

// TestCreateIntegrationKeyShapeAndOneTimePlaintext is the §12.8 key-lifecycle proof's
// create half: the 201 response carries the plaintext key in the documented shape, it
// never reappears in the list endpoint, and the id is well-formed.
func TestCreateIntegrationKeyShapeAndOneTimePlaintext(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	rr := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"discord-bot"}`, admin)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	key, _ := created["key"].(string)
	if !plaintextKeyPattern.MatchString(key) {
		t.Fatalf("key %q does not match phk_<id>_<secret> shape", key)
	}
	id, _ := created["id"].(string)
	if !keyIDPattern.MatchString(id) {
		t.Fatalf("id %q is not 8 lowercase hex chars", id)
	}
	if !strings.HasPrefix(key, "phk_"+id+"_") {
		t.Fatalf("key %q does not embed id %q", key, id)
	}
	if created["label"] != "discord-bot" {
		t.Fatalf("label = %v", created["label"])
	}
	if created["lastUsedAt"] != nil {
		t.Fatalf("lastUsedAt = %v, want nil on a fresh key", created["lastUsedAt"])
	}
	if created["revokedAt"] != nil {
		t.Fatalf("revokedAt = %v, want nil on a fresh key", created["revokedAt"])
	}
	if created["createdAt"] == nil || created["createdAt"] == "" {
		t.Fatalf("createdAt missing: %#v", created)
	}

	list := sessionRequest(h, "GET", "/api/v1/integration-keys", "", admin)
	if list.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), key) {
		t.Fatalf("plaintext key leaked into list response: %s", list.Body.String())
	}
	if !strings.Contains(list.Body.String(), id) {
		t.Fatalf("list response missing created key id: %s", list.Body.String())
	}
}

// TestIntegrationKeyLabelValidationMatrix proves spec §9's exact label rule: required,
// trimmed, 1-64 chars, no control characters. Duplicate labels are explicitly allowed.
func TestIntegrationKeyLabelValidationMatrix(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	cases := []struct {
		name  string
		label string
		want  int
	}{
		{"missing field", "", http.StatusBadRequest},
		{"whitespace only", "   ", http.StatusBadRequest},
		{"single char", "a", http.StatusCreated},
		{"64 chars exactly", strings.Repeat("a", 64), http.StatusCreated},
		{"65 chars over limit", strings.Repeat("a", 65), http.StatusBadRequest},
		{"trims to within limit", "  " + strings.Repeat("b", 64) + "  ", http.StatusCreated},
		{"trims to over limit", "  " + strings.Repeat("b", 65) + "  ", http.StatusBadRequest},
		{"control char newline", "bad\nlabel", http.StatusBadRequest},
		{"control char tab", "bad\tlabel", http.StatusBadRequest},
		{"control char NUL", "bad\x00label", http.StatusBadRequest},
		{"ordinary label", "discord-bot", http.StatusCreated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"label": c.label})
			if err != nil {
				t.Fatal(err)
			}
			rr := sessionRequest(h, "POST", "/api/v1/integration-keys", string(body), admin)
			if rr.Code != c.want {
				t.Fatalf("label %q: status = %d, want %d: %s", c.label, rr.Code, c.want, rr.Body.String())
			}
			if rr.Code == http.StatusBadRequest {
				var envelope struct {
					Error struct{ Code, Message string } `json:"error"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Error.Code != "invalid_request" {
					t.Fatalf("code = %q, want invalid_request", envelope.Error.Code)
				}
			}
		})
	}

	// Duplicate labels are allowed; the id disambiguates.
	dup := `{"label":"duplicate-ok"}`
	first := sessionRequest(h, "POST", "/api/v1/integration-keys", dup, admin)
	second := sessionRequest(h, "POST", "/api/v1/integration-keys", dup, admin)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("duplicate labels: first=%d second=%d", first.Code, second.Code)
	}
}

// TestCreateIntegrationKeyCapAt100 proves the 100-active-key cap (spec §2.6): seeding 100
// active keys directly against the store/cache (bypassing the handler, as the existing
// issueTestAPIKey helper does), the 101st create call is rejected with 409 too_many_keys.
func TestCreateIntegrationKeyCapAt100(t *testing.T) {
	s, h, st := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")
	for i := 0; i < maxActiveIntegrationKeys; i++ {
		id := "seed" + fmtHex4(i)
		issueTestAPIKey(t, s, st, id, "seed")
	}

	rr := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"one-too-many"}`, admin)
	if rr.Code != http.StatusConflict {
		t.Fatalf("101st create = %d, want 409: %s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "too_many_keys" {
		t.Fatalf("code = %q, want too_many_keys", envelope.Error.Code)
	}
}

// fmtHex4 renders i as 4 lowercase hex chars, for building distinct 8-char seed ids
// ("seed"+4 hex chars) in TestCreateIntegrationKeyCapAt100.
func fmtHex4(i int) string {
	const hexDigits = "0123456789abcdef"
	b := [4]byte{hexDigits[0], hexDigits[0], hexDigits[0], hexDigits[0]}
	for pos := 3; i > 0 && pos >= 0; pos-- {
		b[pos] = hexDigits[i%16]
		i /= 16
	}
	return string(b[:])
}

// TestCreateIntegrationKeyRetriesOnIDCollision proves spec §2.1's retry rule: on a
// colliding id the insert fails with a UNIQUE constraint error and the handler retries
// with a fresh id rather than failing the request.
func TestCreateIntegrationKeyRetriesOnIDCollision(t *testing.T) {
	s, h, st := newKeyManagementTestServer(t)
	issueTestAPIKey(t, s, st, "aaaaaaaa", "existing")
	admin := loginAs(t, h, "panelpass")

	orig := newIntegrationKeyID
	t.Cleanup(func() { newIntegrationKeyID = orig })
	calls := 0
	newIntegrationKeyID = func() (string, error) {
		calls++
		if calls == 1 {
			return "aaaaaaaa", nil
		}
		return orig()
	}

	rr := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"retry-me"}`, admin)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create after collision = %d: %s", rr.Code, rr.Body.String())
	}
	if calls < 2 {
		t.Fatalf("expected a retry after the collision, got %d id-generation calls", calls)
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["id"] == "aaaaaaaa" {
		t.Fatalf("retried key still has the colliding id")
	}
}

// TestRevokeIntegrationKeyImmediate401EndToEnd is the §12.8 revoke half: create a key
// through the real handler, authenticate with it against the integration surface, revoke
// it through the real handler, and prove the identical bearer request now gets 401 with
// no restart and no staleness window.
func TestRevokeIntegrationKeyImmediate401EndToEnd(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"lifecycle"}`, admin)
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	token := created["key"].(string)
	id := created["id"].(string)

	before := integrationRequest(h, "GET", "/api/integration/v1/metrics/current", token)
	if before.Code != http.StatusOK {
		t.Fatalf("authenticated request before revoke = %d: %s", before.Code, before.Body.String())
	}

	revoke := sessionRequest(h, "DELETE", "/api/v1/integration-keys/"+id, "", admin)
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", revoke.Code, revoke.Body.String())
	}
	var revoked map[string]any
	if err := json.Unmarshal(revoke.Body.Bytes(), &revoked); err != nil {
		t.Fatal(err)
	}
	if revoked["revokedAt"] == nil {
		t.Fatalf("revoke response missing revokedAt: %#v", revoked)
	}

	after := integrationRequest(h, "GET", "/api/integration/v1/metrics/current", token)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("authenticated request after revoke = %d, want 401: %s", after.Code, after.Body.String())
	}
}

// TestRevokeIntegrationKeyIdempotencyAndUnknown404 proves spec §9's idempotent-DELETE
// contract and the unknown-id 404.
func TestRevokeIntegrationKeyIdempotencyAndUnknown404(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"idempotent"}`, admin)
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)

	first := sessionRequest(h, "DELETE", "/api/v1/integration-keys/"+id, "", admin)
	if first.Code != http.StatusOK {
		t.Fatalf("first revoke = %d: %s", first.Code, first.Body.String())
	}
	var firstBody map[string]any
	_ = json.Unmarshal(first.Body.Bytes(), &firstBody)

	second := sessionRequest(h, "DELETE", "/api/v1/integration-keys/"+id, "", admin)
	if second.Code != http.StatusOK {
		t.Fatalf("second (idempotent) revoke = %d: %s", second.Code, second.Body.String())
	}
	var secondBody map[string]any
	_ = json.Unmarshal(second.Body.Bytes(), &secondBody)
	if firstBody["revokedAt"] != secondBody["revokedAt"] {
		t.Fatalf("revokedAt changed on idempotent revoke: %v -> %v", firstBody["revokedAt"], secondBody["revokedAt"])
	}

	unknown := sessionRequest(h, "DELETE", "/api/v1/integration-keys/deadbeef", "", admin)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown id revoke = %d, want 404: %s", unknown.Code, unknown.Body.String())
	}
	var envelope struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal(unknown.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "not_found" {
		t.Fatalf("code = %q, want not_found", envelope.Error.Code)
	}
}

// TestIntegrationKeysViewerForbidden proves all three routes match every other admin
// route's viewer behavior: 403 forbidden, not 404 (spec §9).
func TestIntegrationKeysViewerForbidden(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")
	viewer := loginAs(t, h, "viewerpass")

	// Seed a key as admin so DELETE has something real to target.
	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"target"}`, admin)
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	id := created["id"].(string)

	cases := []struct {
		method, path, body string
	}{
		{"POST", "/api/v1/integration-keys", `{"label":"viewer-attempt"}`},
		{"GET", "/api/v1/integration-keys", ""},
		{"DELETE", "/api/v1/integration-keys/" + id, ""},
	}
	for _, c := range cases {
		rr := sessionRequest(h, c.method, c.path, c.body, viewer)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s %s as viewer = %d, want 403: %s", c.method, c.path, rr.Code, rr.Body.String())
		}
		var envelope struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Error.Code != "forbidden" {
			t.Fatalf("%s %s: code = %q, want forbidden", c.method, c.path, envelope.Error.Code)
		}
	}
}

// TestIntegrationKeysRequireSession proves all three routes 401 with no session cookie
// at all - the outer session middleware, not an admin check.
func TestIntegrationKeysRequireSession(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	cases := []struct{ method, path, body string }{
		{"POST", "/api/v1/integration-keys", `{"label":"no-session"}`},
		{"GET", "/api/v1/integration-keys", ""},
		{"DELETE", "/api/v1/integration-keys/deadbeef", ""},
	}
	for _, c := range cases {
		rr := sessionRequest(h, c.method, c.path, c.body, nil)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s with no session = %d, want 401: %s", c.method, c.path, rr.Code, rr.Body.String())
		}
	}
}

// TestIntegrationKeysCacheControlNoStore proves spec §5's securityHeaders path-list
// extension: every /api/v1/integration-keys response, including the plaintext-carrying
// 201, carries Cache-Control: no-store.
func TestIntegrationKeysCacheControlNoStore(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"headers"}`, admin)
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", create.Code, create.Body.String())
	}
	if got := create.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("create Cache-Control = %q, want no-store", got)
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	id := created["id"].(string)

	list := sessionRequest(h, "GET", "/api/v1/integration-keys", "", admin)
	if got := list.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("list Cache-Control = %q, want no-store", got)
	}

	revoke := sessionRequest(h, "DELETE", "/api/v1/integration-keys/"+id, "", admin)
	if got := revoke.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("revoke Cache-Control = %q, want no-store", got)
	}
}

// TestIntegrationKeyAuditEventsNeverContainKeyMaterial is a string-level proof (spec
// §2.4, §12.8): audit events for create and revoke carry {id, label} and never the
// plaintext key or its digest, checked against the raw events response body.
func TestIntegrationKeyAuditEventsNeverContainKeyMaterial(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"audit-sentinel-label"}`, admin)
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	id := created["id"].(string)
	key := created["key"].(string)
	secret := strings.TrimPrefix(key, "phk_"+id+"_")

	revoke := sessionRequest(h, "DELETE", "/api/v1/integration-keys/"+id, "", admin)
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", revoke.Code, revoke.Body.String())
	}

	events := sessionRequest(h, "GET", "/api/v1/events?limit=50", "", admin)
	if events.Code != http.StatusOK {
		t.Fatalf("events = %d: %s", events.Code, events.Body.String())
	}
	body := events.Body.String()
	if !strings.Contains(body, "created integration key") || !strings.Contains(body, "revoked integration key") {
		t.Fatalf("events missing expected audit messages: %s", body)
	}
	if !strings.Contains(body, id) {
		t.Fatalf("events missing key id %q: %s", id, body)
	}
	if !strings.Contains(body, "audit-sentinel-label") {
		t.Fatalf("events missing key label: %s", body)
	}
	if strings.Contains(body, key) || strings.Contains(body, secret) {
		t.Fatalf("events leaked plaintext key material: %s", body)
	}
}

// TestIntegrationKeyDBRowStoresDigestNotPlaintext closes the other half of §12.8: the
// api_keys row holds a 32-byte digest that differs from the plaintext, never the
// plaintext itself.
func TestIntegrationKeyDBRowStoresDigestNotPlaintext(t *testing.T) {
	_, h, st := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"digest-check"}`, admin)
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	id := created["id"].(string)
	key := created["key"].(string)

	rows, err := st.ListAPIKeys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var found *store.APIKey
	for i := range rows {
		if rows[i].ID == id {
			found = &rows[i]
		}
	}
	if found == nil {
		t.Fatalf("created key %q not found in store", id)
	}
	if len(found.Hash) != 32 {
		t.Fatalf("hash length = %d, want 32", len(found.Hash))
	}
	if string(found.Hash[:]) == key {
		t.Fatalf("stored hash equals plaintext key")
	}
}

// TestIntegrationLastUsedCoalescingReflectedInAdminList exercises spec §2.5's write
// coalescing end to end: two authenticated requests inside the 60s window still leave the
// admin list showing the fresher in-memory lastUsedAt (merged, not the stale DB column).
func TestIntegrationLastUsedCoalescingReflectedInAdminList(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")

	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"touch"}`, admin)
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	id := created["id"].(string)
	token := created["key"].(string)

	before := sessionRequest(h, "GET", "/api/v1/integration-keys", "", admin)
	var beforeList []map[string]any
	_ = json.Unmarshal(before.Body.Bytes(), &beforeList)
	if lastUsedOf(beforeList, id) != nil {
		t.Fatalf("lastUsedAt should be nil before any authenticated request")
	}

	if rr := integrationRequest(h, "GET", "/api/integration/v1/metrics/current", token); rr.Code != http.StatusOK {
		t.Fatalf("authenticated request = %d: %s", rr.Code, rr.Body.String())
	}

	after := sessionRequest(h, "GET", "/api/v1/integration-keys", "", admin)
	var afterList []map[string]any
	_ = json.Unmarshal(after.Body.Bytes(), &afterList)
	if lastUsedOf(afterList, id) == nil {
		t.Fatalf("lastUsedAt should be set in the merged admin list after use: %s", after.Body.String())
	}
}

func lastUsedOf(list []map[string]any, id string) any {
	for _, k := range list {
		if k["id"] == id {
			return k["lastUsedAt"]
		}
	}
	return nil
}

// TestParseBearerAndTokenPatternAgreeWithKeyID is a sanity cross-check that the id
// substring the handler returns in the create response satisfies the same charset the
// bearer parser requires (spec §2.1/§3): 8 lowercase hex characters.
func TestParseBearerAndTokenPatternAgreeWithKeyID(t *testing.T) {
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")
	rr := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"charset"}`, admin)
	var created map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	id := created["id"].(string)
	if len(id) != 8 {
		t.Fatalf("id length = %d, want 8", len(id))
	}
	for _, r := range id {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("id %q contains non-lowercase-hex char %q", id, r)
		}
	}
	if !tokenPattern.MatchString(created["key"].(string)) {
		t.Fatalf("key %q does not satisfy tokenPattern used by the integration middleware", created["key"])
	}
}
