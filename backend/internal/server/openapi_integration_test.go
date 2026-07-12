package server

// Contract tests for the v0.4.0 Integration API's embedded OpenAPI document (spec §12.11).
// These assert the *documented* contract against the *real* router and handlers, not just
// that a path key exists (the M11 lesson referenced throughout docs/specs/integration-api.md):
// every status a live request actually produces must be one the document advertises for that
// path+method, and every 200/201 response body must structurally match its documented schema
// field-for-field - both "the doc doesn't under-promise" and "the doc doesn't over-promise".

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/palhelm/palhelm/internal/config"
	"github.com/palhelm/palhelm/internal/store"
)

// integrationOpenAPIDoc parses the embedded openapi.json once per test.
func integrationOpenAPIDoc(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(openapi, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestIntegrationOpenAPIPathsAdvertiseExactStatuses is the static half of the contract: the
// documented status set for every new path+method must be exactly what this release adds -
// same rigor as TestBackupsOpenAPIContract/TestConfigOpenAPIContract, extended to the bearer
// group and the admin key-management routes.
func TestIntegrationOpenAPIPathsAdvertiseExactStatuses(t *testing.T) {
	doc := integrationOpenAPIDoc(t)
	paths := object(t, doc, "paths")
	want := map[string]map[string][]string{
		"/api/v1/integration-keys":            {"post": {"201", "400", "401", "403", "409"}, "get": {"200", "401", "403"}},
		"/api/v1/integration-keys/{id}":       {"delete": {"200", "401", "403", "404"}},
		"/api/integration/v1/players":         {"get": {"200", "304", "400", "401", "429"}, "post": {"405"}},
		"/api/integration/v1/players/{uid}":   {"get": {"200", "304", "401", "404", "429"}, "post": {"405"}},
		"/api/integration/v1/pals":            {"get": {"200", "304", "400", "401", "429"}, "post": {"405"}},
		"/api/integration/v1/guilds":          {"get": {"200", "304", "401", "429"}, "post": {"405"}},
		"/api/integration/v1/map":             {"get": {"200", "304", "401", "429"}, "post": {"405"}},
		"/api/integration/v1/server":          {"get": {"200", "304", "401", "429"}, "post": {"405"}},
		"/api/integration/v1/metrics/current": {"get": {"200", "304", "401", "429"}, "post": {"405"}},
		"/api/integration/v1/events":          {"get": {"200", "304", "400", "401", "429"}, "post": {"405"}},
	}
	for path, methods := range want {
		pathItem := object(t, paths, path)
		for method, statuses := range methods {
			op := object(t, pathItem, method)
			responses := object(t, op, "responses")
			got := make([]string, 0, len(responses))
			for status := range responses {
				got = append(got, status)
			}
			sort.Strings(got)
			sort.Strings(statuses)
			if !reflect.DeepEqual(got, statuses) {
				t.Errorf("%s %s statuses = %v, want %v", method, path, got, statuses)
			}
		}
	}
}

// TestIntegrationOpenAPIBearerSecurityScheme proves spec §13: bearerAuth is a proper HTTP
// bearer scheme, every integration GET operation is scoped to it (and nothing else), and the
// admin key-management routes are NOT scoped to it - they stay on the default session cookie
// scheme, since bearer tokens must never reach adminOnly routes (spec §1).
func TestIntegrationOpenAPIBearerSecurityScheme(t *testing.T) {
	doc := integrationOpenAPIDoc(t)
	components := object(t, doc, "components")
	schemes := object(t, components, "securitySchemes")
	bearer := object(t, schemes, "bearerAuth")
	if bearer["type"] != "http" || bearer["scheme"] != "bearer" {
		t.Fatalf("bearerAuth scheme = %#v, want type=http scheme=bearer", bearer)
	}

	paths := object(t, doc, "paths")
	for _, path := range []string{
		"/api/integration/v1/players", "/api/integration/v1/players/{uid}", "/api/integration/v1/pals",
		"/api/integration/v1/guilds", "/api/integration/v1/map", "/api/integration/v1/server",
		"/api/integration/v1/metrics/current",
		"/api/integration/v1/events",
	} {
		op := object(t, object(t, paths, path), "get")
		if !hasBearerAuthSecurity(op) {
			t.Errorf("GET %s security = %#v, want exactly [{bearerAuth: []}]", path, op["security"])
		}
	}

	for _, tc := range []struct{ path, method string }{
		{"/api/v1/integration-keys", "post"},
		{"/api/v1/integration-keys", "get"},
		{"/api/v1/integration-keys/{id}", "delete"},
	} {
		op := object(t, object(t, paths, tc.path), tc.method)
		if _, overridden := op["security"]; overridden {
			t.Errorf("%s %s declares a security override %#v; admin key routes must stay on the default session scheme", tc.method, tc.path, op["security"])
		}
	}
}

func hasBearerAuthSecurity(op map[string]any) bool {
	sec, ok := op["security"].([]any)
	if !ok || len(sec) != 1 {
		return false
	}
	entry, ok := sec[0].(map[string]any)
	if !ok {
		return false
	}
	_, ok = entry["bearerAuth"]
	return ok
}

// tokenShapedKeyPattern is the exact real-key grammar (spec §3). The public document must
// never contain a string matching it: a real-looking example would itself look like a leaked
// credential to a secret scanner, and defeats the point of using an obviously-fake one.
var tokenShapedKeyPattern = regexp.MustCompile(`^phk_[0-9a-f]{8}_[A-Za-z0-9_-]{43}$`)
var tokenShapedKeyAnywhere = regexp.MustCompile(`phk_[0-9a-f]{8}_[A-Za-z0-9_-]{43}`)

// TestIntegrationOpenAPINoRealKeyShapedString proves spec §13's public-document requirement:
// /api/openapi.json is served unauthenticated, so any example key must be obviously fake and
// format-non-conforming. This scans the raw embedded bytes, not just a known example field, so
// it also catches an accidental real-shaped string anywhere else in the document.
func TestIntegrationOpenAPINoRealKeyShapedString(t *testing.T) {
	if m := tokenShapedKeyAnywhere.FindString(string(openapi)); m != "" {
		t.Fatalf("openapi.json contains a real-token-shaped string %q matching %s", m, tokenShapedKeyPattern.String())
	}
	// Sanity: prove the regex isn't trivially unsatisfiable by confirming a real minted token
	// (never written to the doc) does match it - otherwise the negative assertion above would
	// be vacuous.
	s, h, st := newIntegrationTestServer(t, nil)
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "regex-sanity")
	_ = h
	if !tokenShapedKeyPattern.MatchString(token) {
		t.Fatalf("sanity check failed: minted token %q does not match the pattern under test", token)
	}
}

// --- schema resolution: minimal $ref/allOf walker, enough to check field-level shape ---

func lookupSchemaRef(doc map[string]any, ref string) map[string]any {
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	var cur any = doc
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = m[p]
	}
	m, _ := cur.(map[string]any)
	return m
}

// resolveSchema follows $ref and flattens allOf (used by IntegrationPlayerDetail,
// IntegrationPalListItem, and IntegrationKeyCreated) into one properties/required object, so
// the field-level walker below can treat every schema uniformly.
func resolveSchema(doc map[string]any, schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{}
	}
	if ref, ok := schema["$ref"].(string); ok {
		return resolveSchema(doc, lookupSchemaRef(doc, ref))
	}
	if allOf, ok := schema["allOf"].([]any); ok {
		props := map[string]any{}
		var required []any
		for _, sub := range allOf {
			subSchema, ok := sub.(map[string]any)
			if !ok {
				continue
			}
			resolved := resolveSchema(doc, subSchema)
			if p, ok := resolved["properties"].(map[string]any); ok {
				for k, v := range p {
					props[k] = v
				}
			}
			if r, ok := resolved["required"].([]any); ok {
				required = append(required, r...)
			}
		}
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	return schema
}

// validateAgainstSchema is the field-level structural proof: every documented required field
// must be present, and every field actually present in the live response must be documented
// (an undocumented field is exactly the kind of drift path-presence checks miss - the M11
// lesson). It recurses into object properties and array items.
func validateAgainstSchema(t *testing.T, doc map[string]any, schema map[string]any, value any, path string) {
	t.Helper()
	schema = resolveSchema(doc, schema)
	switch v := value.(type) {
	case map[string]any:
		props, _ := schema["properties"].(map[string]any)
		required, _ := schema["required"].([]any)
		for _, r := range required {
			key, _ := r.(string)
			if _, ok := v[key]; !ok {
				t.Errorf("%s: response is missing documented required field %q", path, key)
			}
		}
		for key, fv := range v {
			propSchema, ok := props[key]
			if !ok {
				t.Errorf("%s: response field %q is not present in the documented schema", path, key)
				continue
			}
			ps, ok := propSchema.(map[string]any)
			if !ok {
				continue
			}
			validateAgainstSchema(t, doc, ps, fv, path+"."+key)
		}
	case []any:
		items, _ := schema["items"].(map[string]any)
		if items == nil {
			return
		}
		for i, item := range v {
			validateAgainstSchema(t, doc, items, item, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

// schemaAt resolves paths.<path>.<method>.responses.<status>.content["application/json"].schema,
// resolving a response-level $ref (used by the shared IntegrationUnauthorized/IntegrationRateLimited/
// etc. component responses) before drilling into "content".
func schemaAt(t *testing.T, doc map[string]any, path, method, status string) map[string]any {
	t.Helper()
	op := object(t, object(t, object(t, doc, "paths"), path), method)
	resp := object(t, object(t, op, "responses"), status)
	if ref, ok := resp["$ref"].(string); ok {
		resp = lookupSchemaRef(doc, ref)
	}
	content := object(t, resp, "content")
	media := object(t, content, "application/json")
	return object(t, media, "schema")
}

// headersAt resolves the header names documented on a response, resolving $ref where present
// (component responses like IntegrationUnauthorized keep their headers there).
func headersAt(t *testing.T, doc map[string]any, path, method, status string) map[string]any {
	t.Helper()
	op := object(t, object(t, doc, "paths"), path)
	opMethod := object(t, op, method)
	resp, ok := object(t, opMethod, "responses")[status].(map[string]any)
	if !ok {
		t.Fatalf("missing %s response for %s %s", status, method, path)
	}
	if ref, ok := resp["$ref"].(string); ok {
		resp = lookupSchemaRef(doc, ref)
	}
	headers, _ := resp["headers"].(map[string]any)
	return headers
}

// TestIntegrationOpenAPIGETResponsesMatchLiveHandlers is the dynamic half of the contract:
// for every one of the seven integration GET paths, real 200/401/404/405/429 (and 304 for a
// representative path) responses are asserted to (a) be a status the document advertises, (b)
// carry the headers the document promises, and (c) for 200s, match the documented schema
// field-for-field against a real, non-empty response body.
func TestIntegrationOpenAPIGETResponsesMatchLiveHandlers(t *testing.T) {
	doc := integrationOpenAPIDoc(t)
	s, h, st := newIntegrationTestServer(t, nil)
	uidA, _, _, _ := seedBasicWorld(t, st)
	if err := st.AddEvent(context.Background(), store.Event{
		At: time.Now().UTC(), Kind: "join", Message: "Hunter joined", Meta: map[string]any{"uid": uidA},
	}); err != nil {
		t.Fatal(err)
	}
	token := issueTestAPIKey(t, s, st, "aaaaaaaa", "contract-bot")

	type endpoint struct {
		path     string // request path (with a real uid substituted for {uid})
		docPath  string // the OpenAPI path key
		schemaOK bool   // whether to run the field-level 200 body check
	}
	endpoints := []endpoint{
		{"/api/integration/v1/players", "/api/integration/v1/players", true},
		{"/api/integration/v1/players/" + uidA, "/api/integration/v1/players/{uid}", true},
		{"/api/integration/v1/pals", "/api/integration/v1/pals", true},
		{"/api/integration/v1/guilds", "/api/integration/v1/guilds", true},
		{"/api/integration/v1/map", "/api/integration/v1/map", true},
		{"/api/integration/v1/server", "/api/integration/v1/server", true},
		{"/api/integration/v1/metrics/current", "/api/integration/v1/metrics/current", true},
		{"/api/integration/v1/events", "/api/integration/v1/events", true},
	}

	for _, ep := range endpoints {
		t.Run(ep.docPath, func(t *testing.T) {
			// 200: status is documented, headers match, body matches the schema field-for-field.
			rr := integrationRequest(h, "GET", ep.path, token)
			if rr.Code != http.StatusOK {
				t.Fatalf("GET %s = %d: %s", ep.path, rr.Code, rr.Body.String())
			}
			assertDocumentedStatus(t, doc, ep.docPath, "get", "200")
			headers := headersAt(t, doc, ep.docPath, "get", "200")
			if _, ok := headers["ETag"]; ok && rr.Header().Get("ETag") == "" {
				t.Errorf("%s: doc promises an ETag header but the live 200 carried none", ep.path)
			}
			if _, ok := headers["Cache-Control"]; ok && rr.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("%s: doc promises Cache-Control: no-store but got %q", ep.path, rr.Header().Get("Cache-Control"))
			}
			if ep.schemaOK {
				var body any
				if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
					t.Fatalf("GET %s: response is not valid JSON: %v", ep.path, err)
				}
				schema := schemaAt(t, doc, ep.docPath, "get", "200")
				validateAgainstSchema(t, doc, schema, body, ep.path)
			}

			// 304: the documented ETag round-trips to a bodyless 304 with the same ETag.
			etag := rr.Header().Get("ETag")
			req := httptest.NewRequest("GET", ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("If-None-Match", etag)
			rr304 := httptest.NewRecorder()
			h.ServeHTTP(rr304, req)
			if rr304.Code != http.StatusNotModified {
				t.Fatalf("revalidated GET %s = %d, want 304: %s", ep.path, rr304.Code, rr304.Body.String())
			}
			assertDocumentedStatus(t, doc, ep.docPath, "get", "304")
			if rr304.Body.Len() != 0 {
				t.Errorf("%s: 304 carried a body", ep.path)
			}
			if rr304.Header().Get("ETag") != etag {
				t.Errorf("%s: 304 ETag = %q, want %q", ep.path, rr304.Header().Get("ETag"), etag)
			}

			// 401: uniform unauthorized, matching the documented WWW-Authenticate header.
			rrAuth := integrationRequest(h, "GET", ep.path, "")
			if rrAuth.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated GET %s = %d, want 401", ep.path, rrAuth.Code)
			}
			assertDocumentedStatus(t, doc, ep.docPath, "get", "401")
			if rrAuth.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Errorf("%s: WWW-Authenticate = %q, want Bearer", ep.path, rrAuth.Header().Get("WWW-Authenticate"))
			}
			var authEnvelope map[string]any
			if err := json.Unmarshal(rrAuth.Body.Bytes(), &authEnvelope); err != nil {
				t.Fatal(err)
			}
			validateAgainstSchema(t, doc, schemaAt(t, doc, ep.docPath, "get", "401"), authEnvelope, ep.path+" (401 body)")

			// 405: any non-GET method on a real path.
			postReq := httptest.NewRequest("POST", ep.path, nil)
			postReq.Header.Set("Authorization", "Bearer "+token)
			rrPost := httptest.NewRecorder()
			h.ServeHTTP(rrPost, postReq)
			if rrPost.Code != http.StatusMethodNotAllowed {
				t.Fatalf("POST %s = %d, want 405", ep.path, rrPost.Code)
			}
			assertDocumentedStatus(t, doc, ep.docPath, "post", "405")
			if rrPost.Header().Get("Allow") != "GET" {
				t.Errorf("%s: Allow = %q, want GET", ep.path, rrPost.Header().Get("Allow"))
			}
		})
	}

	// 429: one round trip against a 1-req/min key, proving the shared limiter middleware
	// applies uniformly (it runs before route dispatch, so one proof covers every path).
	stLimited, err := storeOpenTemp(t)
	if err != nil {
		t.Fatal(err)
	}
	cfgLimited := config.Config{AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48), IntegrationRateLimit: 1}
	limited, hLimited := New(cfgLimited, stLimited, testLogger())
	limitedToken := issueTestAPIKey(t, limited, stLimited, "bbbbbbbb", "limited-bot")
	if rr := integrationRequest(hLimited, "GET", "/api/integration/v1/guilds", limitedToken); rr.Code != http.StatusOK {
		t.Fatalf("first request = %d", rr.Code)
	}
	rr429 := integrationRequest(hLimited, "GET", "/api/integration/v1/guilds", limitedToken)
	if rr429.Code != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429", rr429.Code)
	}
	assertDocumentedStatus(t, doc, "/api/integration/v1/guilds", "get", "429")
	if rr429.Header().Get("Retry-After") == "" {
		t.Errorf("429 missing Retry-After")
	}

	// 404: an unknown (but validly-shaped) uid on /players/{uid}.
	rrNotFound := integrationRequest(h, "GET", "/api/integration/v1/players/ffffffffffffffffffffffffffffffff", token)
	if rrNotFound.Code != http.StatusNotFound {
		t.Fatalf("unknown uid = %d, want 404: %s", rrNotFound.Code, rrNotFound.Body.String())
	}
	assertDocumentedStatus(t, doc, "/api/integration/v1/players/{uid}", "get", "404")

	// 400: invalid limit on /players and /pals.
	for _, path := range []string{"/api/integration/v1/players", "/api/integration/v1/pals", "/api/integration/v1/events"} {
		rrBad := integrationRequest(h, "GET", path+"?limit=0", token)
		if rrBad.Code != http.StatusBadRequest {
			t.Fatalf("GET %s?limit=0 = %d, want 400", path, rrBad.Code)
		}
		assertDocumentedStatus(t, doc, path, "get", "400")
	}
}

// assertDocumentedStatus fails the test if the OpenAPI document does not list status among the
// responses for path+method - the core "handler produced a status the doc doesn't advertise"
// check the M11 lesson is about.
func assertDocumentedStatus(t *testing.T, doc map[string]any, path, method, status string) {
	t.Helper()
	op := object(t, object(t, doc, "paths"), path)
	opMethod := object(t, op, method)
	responses := object(t, opMethod, "responses")
	if _, ok := responses[status]; !ok {
		t.Errorf("%s %s produced %s live, but the OpenAPI document does not list it", method, path, status)
	}
}

// TestIntegrationKeyManagementOpenAPIMatchesLiveHandlers is the key-management counterpart:
// POST/GET/DELETE /api/v1/integration-keys against the real admin handlers, checked against
// their documented schemas and status set.
func TestIntegrationKeyManagementOpenAPIMatchesLiveHandlers(t *testing.T) {
	doc := integrationOpenAPIDoc(t)
	_, h, _ := newKeyManagementTestServer(t)
	admin := loginAs(t, h, "panelpass")
	viewer := loginAs(t, h, "viewerpass")

	// 201 create: body matches IntegrationKeyCreated field-for-field.
	create := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"contract-bot"}`, admin)
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", create.Code, create.Body.String())
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys", "post", "201")
	if create.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("create Cache-Control = %q, want no-store", create.Header().Get("Cache-Control"))
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	validateAgainstSchema(t, doc, schemaAt(t, doc, "/api/v1/integration-keys", "post", "201"), created, "create response")
	id, _ := created["id"].(string)

	// 400: missing label.
	bad := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":""}`, admin)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad create = %d, want 400", bad.Code)
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys", "post", "400")

	// 409: over the active-key cap, on its own fresh server so the cap is exact.
	capServer, capHandler, capStore := newKeyManagementTestServer(t)
	capAdmin := loginAs(t, capHandler, "panelpass")
	for i := 0; i < maxActiveIntegrationKeys; i++ {
		issueTestAPIKey(t, capServer, capStore, "seed"+fmtHex4(i), "seed")
	}
	over := sessionRequest(capHandler, "POST", "/api/v1/integration-keys", `{"label":"one-too-many"}`, capAdmin)
	if over.Code != http.StatusConflict {
		t.Fatalf("over-cap create = %d, want 409: %s", over.Code, over.Body.String())
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys", "post", "409")

	// 401/403 on create, list, and revoke.
	noSession := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"x"}`, nil)
	if noSession.Code != http.StatusUnauthorized {
		t.Fatalf("no-session create = %d, want 401", noSession.Code)
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys", "post", "401")
	asViewer := sessionRequest(h, "POST", "/api/v1/integration-keys", `{"label":"x"}`, viewer)
	if asViewer.Code != http.StatusForbidden {
		t.Fatalf("viewer create = %d, want 403", asViewer.Code)
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys", "post", "403")

	// 200 list: array items match IntegrationKey field-for-field.
	list := sessionRequest(h, "GET", "/api/v1/integration-keys", "", admin)
	if list.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", list.Code, list.Body.String())
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys", "get", "200")
	if list.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("list Cache-Control = %q, want no-store", list.Header().Get("Cache-Control"))
	}
	var listBody []any
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody) == 0 {
		t.Fatal("list response is empty; schema check would be vacuous")
	}
	validateAgainstSchema(t, doc, schemaAt(t, doc, "/api/v1/integration-keys", "get", "200"), listBody, "list response")

	listNoSession := sessionRequest(h, "GET", "/api/v1/integration-keys", "", nil)
	if listNoSession.Code != http.StatusUnauthorized {
		t.Fatalf("no-session list = %d, want 401", listNoSession.Code)
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys", "get", "401")
	listAsViewer := sessionRequest(h, "GET", "/api/v1/integration-keys", "", viewer)
	if listAsViewer.Code != http.StatusForbidden {
		t.Fatalf("viewer list = %d, want 403", listAsViewer.Code)
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys", "get", "403")

	// 200 revoke: body matches IntegrationKey field-for-field.
	revoke := sessionRequest(h, "DELETE", "/api/v1/integration-keys/"+id, "", admin)
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", revoke.Code, revoke.Body.String())
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys/{id}", "delete", "200")
	if revoke.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("revoke Cache-Control = %q, want no-store", revoke.Header().Get("Cache-Control"))
	}
	var revoked map[string]any
	if err := json.Unmarshal(revoke.Body.Bytes(), &revoked); err != nil {
		t.Fatal(err)
	}
	validateAgainstSchema(t, doc, schemaAt(t, doc, "/api/v1/integration-keys/{id}", "delete", "200"), revoked, "revoke response")

	// 404 unknown id, 401/403 on revoke.
	unknown := sessionRequest(h, "DELETE", "/api/v1/integration-keys/deadbeef", "", admin)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown-id revoke = %d, want 404", unknown.Code)
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys/{id}", "delete", "404")
	revokeNoSession := sessionRequest(h, "DELETE", "/api/v1/integration-keys/"+id, "", nil)
	if revokeNoSession.Code != http.StatusUnauthorized {
		t.Fatalf("no-session revoke = %d, want 401", revokeNoSession.Code)
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys/{id}", "delete", "401")
	revokeAsViewer := sessionRequest(h, "DELETE", "/api/v1/integration-keys/"+id, "", viewer)
	if revokeAsViewer.Code != http.StatusForbidden {
		t.Fatalf("viewer revoke = %d, want 403", revokeAsViewer.Code)
	}
	assertDocumentedStatus(t, doc, "/api/v1/integration-keys/{id}", "delete", "403")
}
