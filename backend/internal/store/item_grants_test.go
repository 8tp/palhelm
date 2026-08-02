package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestItemGrantAuditIsIdempotentAndFirstResultWins(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	want := ItemGrant{RequestID: "request123", IdempotencyKey: "panel-idem-123", CreatedAt: now, UpdatedAt: now, Actor: "admin", PlayerUID: "ABCDEF12", PlayerName: "Player", ItemID: "Stone", ItemName: "Stone", Quantity: 25, Reason: "replace lost materials", Status: "queued"}
	created, inserted, err := st.CreateItemGrant(ctx, want)
	if err != nil || !inserted || created.PlayerUID != "abcdef12" {
		t.Fatalf("create = %#v inserted=%v err=%v", created, inserted, err)
	}
	replayed := want
	replayed.RequestID = "different-request"
	got, inserted, err := st.CreateItemGrant(ctx, replayed)
	if err != nil || inserted || got.RequestID != want.RequestID {
		t.Fatalf("replay = %#v inserted=%v err=%v", got, inserted, err)
	}
	if err := st.CompleteItemGrant(ctx, want.RequestID, "succeeded", 25, "", "", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := st.CompleteItemGrant(ctx, want.RequestID, "failed", 0, "late", "late result", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err = st.ItemGrant(ctx, want.RequestID)
	if err != nil || got.Status != "succeeded" || got.GrantedQuantity == nil || *got.GrantedQuantity != 25 {
		t.Fatalf("completed grant = %#v err=%v", got, err)
	}
}
