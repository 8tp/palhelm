package server

// Adversarial scope audit for the v0.4.0 Integration API (docs/specs/integration-api.md §1,
// §12.1). These tests are the flagship proof: they walk the REAL chi router with
// chi.Walk - never a hand-kept list - and prove that a valid bearer token confers access to
// exactly the nine integration GETs and nothing else, that no non-GET handler lives inside
// the integration mount, and that a session cookie authenticates nothing on that mount. The
// suite fails automatically if a future edit adds a route reachable by both principals, adds a
// non-GET handler under the integration mount, or lets a bearer token cross into a session or
// admin route.
//
// Every added test states the threat it closes and is enumerated from the router itself.

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// paramRe substitutes any chi path parameter (e.g. {uid}, {id}, {characterId}) with a concrete,
// harmless value so a walked route pattern becomes a requestable path. Gated routes reject the
// request at the auth middleware long before the value is ever parsed, so any placeholder works.
var paramRe = regexp.MustCompile(`\{[^}]+\}`)

// requestablePath turns a chi route pattern into a concrete request path.
func requestablePath(pattern string) string {
	p := paramRe.ReplaceAllString(pattern, "1")
	// Trailing wildcard mounts (spaHandler at "/*", map-tiles at "/map-tiles/*") become a
	// concrete child path.
	p = strings.ReplaceAll(p, "/*", "/x")
	if p == "" {
		p = "/"
	}
	return p
}

const integrationPrefix = "/api/integration/v1"

// walkRoutes enumerates every (method, pattern) pair the real router exposes, recursing into
// mounted sub-routers (the integration group, the tiles mount) exactly as chi.Walk does.
func walkRoutes(t *testing.T, h http.Handler) [][2]string {
	t.Helper()
	routes, ok := h.(chi.Routes)
	if !ok {
		t.Fatalf("router handler %T does not implement chi.Routes; cannot enumerate", h)
	}
	var out [][2]string
	err := chi.Walk(routes, func(method, route string, handler http.Handler, mw ...func(http.Handler) http.Handler) error {
		out = append(out, [2]string{method, route})
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("chi.Walk enumerated zero routes; the walk is vacuous")
	}
	return out
}

// TestFullRouterScopeEnumerationBearerConfersNothingOutsideGroup is the spec §12.1 flagship
// proof, enumerated from the real router.
//
//	Threat: a bearer principal reaching any session/admin/mutating route (scope creep), or a
//	        session cookie authenticating on the bearer mount, or a non-GET handler smuggled
//	        into the integration group.
//	Test: chi.Walk the real routes() handler; for every route outside /api/integration/v1
//	      assert status(valid-bearer) == status(no-auth) - a token confers nothing anywhere
//	      outside its group, so any gated route that is 401 unauthenticated stays 401 with a
//	      bearer, and public routes (200 either way) are not wrongly demanded to 401. For every
//	      route inside the mount, assert the method is GET, that a valid bearer is NOT rejected,
//	      and that a valid admin *session cookie* alone IS rejected (401).
//	Closes it: routing is by literal subtree; the integration mount's middleware only accepts
//	           the bearer principal type and the session group's middleware only accepts the
//	           cookie, so neither principal crosses. The enumeration (not a spot-check) makes a
//	           later regression fail here.
//	Edge: chi.Walk does not descend into non-chi mounts (spaHandler), so the SPA appears as a
//	      single "/*" leaf - still exercised via the equality check.
func TestFullRouterScopeEnumerationBearerConfersNothingOutsideGroup(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "scope-audit")
	adminCookie := loginAs(t, h, "panelpass")

	routes := walkRoutes(t, h)

	var integrationSeen []string
	for _, mr := range routes {
		method, pattern := mr[0], mr[1]
		path := requestablePath(pattern)

		if strings.HasPrefix(pattern, integrationPrefix) {
			integrationSeen = append(integrationSeen, method+" "+pattern)

			// (1) No non-GET handler may live inside the integration mount.
			if method != http.MethodGet {
				t.Errorf("integration mount exposes a non-GET route: %s %s (spec §1 forbids any non-GET handler in this group)", method, pattern)
			}

			// (2) A valid admin session cookie alone must NOT authenticate here.
			cookieReq := httptest.NewRequest(method, path, nil)
			cookieReq.AddCookie(adminCookie)
			cookieRec := httptest.NewRecorder()
			h.ServeHTTP(cookieRec, cookieReq)
			if cookieRec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with an admin session cookie (no bearer) = %d, want 401: a session grants nothing on the bearer mount", method, path, cookieRec.Code)
			}

			// (3) A valid bearer token must be accepted here (not the uniform 401).
			bearerRec := integrationRequest(h, method, path, token)
			if bearerRec.Code == http.StatusUnauthorized {
				t.Errorf("%s %s with a valid bearer token = 401; the token must confer access inside its own group", method, path)
			}
			continue
		}

		// Every route OUTSIDE the integration mount: a bearer token must change nothing.
		bearer := integrationRequest(h, method, path, token)
		noAuth := integrationRequest(h, method, path, "")
		if bearer.Code != noAuth.Code {
			t.Errorf("%s %s: status with bearer token = %d, without = %d; the token must confer nothing outside its group", method, path, bearer.Code, noAuth.Code)
		}
	}

	// Sanity: the enumeration actually reached the integration group (guards against a walk
	// that silently skipped the mount, which would make the GET-only assertions vacuous).
	if len(integrationSeen) != 10 {
		t.Fatalf("expected exactly 10 enumerated integration routes, walked %d: %v", len(integrationSeen), integrationSeen)
	}
}

// TestGatedRoutesRejectBearerTokenWith401 is the sharpened companion to the equality proof:
// every route in the session/admin subtree must return exactly 401 to a bearer token, so the
// enumeration does not merely prove "same as anonymous" but the stronger "actively rejected".
//
//	Threat: a mutating/admin route silently treating a bearer token as some weaker-but-nonzero
//	        principal (e.g. 200/403 instead of 401).
//	Test: for every walked route under /api/v1, /map-tiles, or /api/v1/auth/{logout,session}
//	      (the cookie-gated subtree), a bearer-token request returns 401.
//	Closes it: the session middleware ignores the Authorization header entirely; a bearer
//	           request carries no cookie, so it is indistinguishable from anonymous - 401.
func TestGatedRoutesRejectBearerTokenWith401(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "gated-audit")

	// Public routes legitimately answer a bearer request with a non-401 status. The SPA
	// catch-all is mounted at "/" for every method (chi reports it as "<VERB> /*"), so it is
	// public regardless of verb.
	public := map[string]bool{
		"GET /healthz":            true,
		"GET /api/openapi.json":   true,
		"POST /api/v1/auth/login": true,
	}

	gatedChecked := 0
	for _, mr := range walkRoutes(t, h) {
		method, pattern := mr[0], mr[1]
		if strings.HasPrefix(pattern, integrationPrefix) {
			continue
		}
		if pattern == "/*" || public[method+" "+pattern] {
			continue
		}
		path := requestablePath(pattern)
		rr := integrationRequest(h, method, path, token)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("gated route %s %s with a bearer token = %d, want 401", method, path, rr.Code)
			continue
		}
		gatedChecked++
	}
	if gatedChecked == 0 {
		t.Fatal("no gated routes were checked; the enumeration is vacuous")
	}
}

// TestIntegrationMountPathTraversalCannotEscapeToSessionRoutes probes URL-encoded and raw
// dot-segment traversal against the real router.
//
//	Threat: a bearer holder escaping the mount to a session/admin route via
//	        /api/integration/v1/../../v1/config-style traversal (path confusion).
//	Test: request several raw and percent-encoded traversal variants with a valid bearer token
//	      and assert none returns a session handler's output - never 200, never config content.
//	Closes it: no CleanPath middleware rewrites the request path, so chi routes on the literal
//	           (still integration-prefixed) path; the traversal tail matches no GET route and
//	           gets the group's 404, and even a hypothetical escape would hit the cookie-gated
//	           subtree, which rejects the bearer.
//	Edge: url.Parse keeps dot segments literal in Path; %2e forms populate RawPath, which chi
//	      also resolves under the same mount - both covered below.
func TestIntegrationMountPathTraversalCannotEscapeToSessionRoutes(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "traversal-audit")

	variants := []string{
		"/api/integration/v1/../v1/config",
		"/api/integration/v1/../../api/v1/config",
		"/api/integration/v1/%2e%2e/v1/config",
		"/api/integration/v1/..%2f..%2fapi%2fv1%2fconfig",
		"/api/integration/v1/players/../../../api/v1/config/raw",
	}
	for _, v := range variants {
		rr := integrationRequest(h, "GET", v, token)
		if rr.Code == http.StatusOK {
			t.Errorf("traversal %q returned 200 with a bearer token: %s", v, rr.Body.String())
		}
		body := rr.Body.String()
		for _, leak := range []string{"panelVersion", "manualCommand", "ADMIN_PASSWORD", "\"raw\""} {
			if strings.Contains(body, leak) {
				t.Errorf("traversal %q leaked session/config content (%q): %s", v, leak, body)
			}
		}
	}
}

// TestIntegrationMountRejectsNonGETVerbsAfterAuth pins the observable OPTIONS/HEAD/PUT/DELETE
// behavior inside the mount.
//
//	Threat: an alternate verb (HEAD auto-served, OPTIONS pre-flight) bypassing the GET-only
//	        contract or reaching a handler without a token.
//	Test: for HEAD/OPTIONS/PUT/DELETE/PATCH on a real integration path: without a token every
//	      verb is the uniform 401 (auth precedes routing); with a valid token every non-GET verb
//	      is 405 with Allow: GET (the group registers only Get()).
//	Closes it: the bearer middleware runs via Use(...) on all methods, so no verb skips auth;
//	           chi does not auto-register HEAD for Get(), so every non-GET falls to the group's
//	           MethodNotAllowed handler.
func TestIntegrationMountRejectsNonGETVerbsAfterAuth(t *testing.T) {
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "verb-audit")
	const path = "/api/integration/v1/guilds"

	for _, method := range []string{"HEAD", "OPTIONS", "PUT", "DELETE", "PATCH"} {
		// No token: uniform 401 regardless of verb (auth before routing).
		if rr := integrationRequest(h, method, path, ""); rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401 (auth must precede routing on every verb)", method, path, rr.Code)
		}
		// Valid token: 405 method_not_allowed with Allow: GET.
		rr := integrationRequest(h, method, path, token)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s with a valid token = %d, want 405", method, path, rr.Code)
			continue
		}
		if got := rr.Header().Get("Allow"); got != "GET" {
			t.Errorf("%s %s: Allow = %q, want GET", method, path, got)
		}
		if got := rr.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s %s: Cache-Control = %q, want no-store (group middleware sets it on every response)", method, path, got)
		}
	}
}
