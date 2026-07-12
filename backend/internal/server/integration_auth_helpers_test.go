package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
)

// testLogger is a discard logger for constructing an integrationAuth directly in unit tests
// that don't go through New().
func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newTestKeyHash mints a syntactically valid phk_<id>_<secret> token for the given id and
// returns its SHA-256 digest alongside the plaintext, for tests that build an
// integrationAuth directly (via newIntegrationAuth/Add) rather than through the store.
func newTestKeyHash(id string) (hash [32]byte, token string) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic(err)
	}
	token = "phk_" + id + "_" + base64.RawURLEncoding.EncodeToString(secret)
	return sha256.Sum256([]byte(token)), token
}
