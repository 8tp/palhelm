package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/palgrant"
)

func TestPalGrantPersistenceAndIdempotency(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "pal-grants.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UTC().Truncate(time.Second)
	spec := palgrant.DefaultSpec()
	spec.LevelMin, spec.LevelMax = 20, 25
	grant := PalGrant{RequestID: "request123", IdempotencyKey: "key-12345", CreatedAt: now, UpdatedAt: now, Actor: "admin", PlayerUID: "ABCDEF12000000000000000000000000", PlayerName: "Player", CharacterID: "anubis", DisplayName: "Anubis", Spec: spec, Reason: "event reward", Status: "queued"}
	created, fresh, err := st.CreatePalGrant(context.Background(), grant)
	if err != nil || !fresh || created.PlayerUID != "abcdef12000000000000000000000000" {
		t.Fatalf("create = %#v fresh=%v err=%v", created, fresh, err)
	}
	replayed, fresh, err := st.CreatePalGrant(context.Background(), grant)
	if err != nil || fresh || replayed.RequestID != grant.RequestID {
		t.Fatalf("replay = %#v fresh=%v err=%v", replayed, fresh, err)
	}
	result := &palgrant.Result{ProtocolVersion: 1, RequestID: grant.RequestID, Status: "succeeded", InstanceID: "12345678abcdef00", Level: 23, Gender: "female", CompletedAt: now.Format(time.RFC3339)}
	if err := st.CompletePalGrant(context.Background(), grant.RequestID, "succeeded", result, "", "", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err := st.PalGrant(context.Background(), grant.RequestID)
	if err != nil || got.Result == nil || got.Result.Level != 23 || got.Status != "succeeded" {
		t.Fatalf("stored = %#v err=%v", got, err)
	}
}
