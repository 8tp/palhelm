package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type ItemGrant struct {
	RequestID       string    `json:"requestId"`
	IdempotencyKey  string    `json:"idempotencyKey,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Actor           string    `json:"actor"`
	PlayerUID       string    `json:"playerUid"`
	PlayerName      string    `json:"playerName"`
	ItemID          string    `json:"itemId"`
	ItemName        string    `json:"itemName"`
	Quantity        int       `json:"quantity"`
	GrantedQuantity *int      `json:"grantedQuantity"`
	Reason          string    `json:"reason"`
	Status          string    `json:"status"`
	ErrorCode       string    `json:"errorCode,omitempty"`
	ErrorMessage    string    `json:"errorMessage,omitempty"`
}

func (s *Store) CreateItemGrant(ctx context.Context, g ItemGrant) (ItemGrant, bool, error) {
	g.PlayerUID = NormalizeUID(g.PlayerUID)
	_, err := s.db.ExecContext(ctx, `INSERT INTO item_grants(request_id,idempotency_key,created_at,updated_at,actor,player_uid,player_name,item_id,item_name,quantity,reason,status) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		g.RequestID, g.IdempotencyKey, g.CreatedAt.Unix(), g.UpdatedAt.Unix(), g.Actor, g.PlayerUID, g.PlayerName, g.ItemID, g.ItemName, g.Quantity, g.Reason, g.Status)
	if err == nil {
		return g, true, nil
	}
	var existing ItemGrant
	existing, getErr := s.ItemGrantByIdempotencyKey(ctx, g.IdempotencyKey)
	if getErr == nil {
		return existing, false, nil
	}
	return ItemGrant{}, false, err
}

func (s *Store) ItemGrantByIdempotencyKey(ctx context.Context, key string) (ItemGrant, error) {
	return scanItemGrant(s.db.QueryRowContext(ctx, itemGrantSelect+" WHERE idempotency_key=?", key))
}

func (s *Store) ItemGrant(ctx context.Context, requestID string) (ItemGrant, error) {
	return scanItemGrant(s.db.QueryRowContext(ctx, itemGrantSelect+" WHERE request_id=?", requestID))
}

func (s *Store) ItemGrants(ctx context.Context, limit int) ([]ItemGrant, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, itemGrantSelect+" ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ItemGrant, 0)
	for rows.Next() {
		g, err := scanItemGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CompleteItemGrant(ctx context.Context, requestID, status string, granted int, code, message string, now time.Time) error {
	if status != "succeeded" && status != "failed" {
		return errors.New("invalid item grant status")
	}
	var quantity any
	if status == "succeeded" {
		quantity = granted
	}
	result, err := s.db.ExecContext(ctx, `UPDATE item_grants SET updated_at=?,status=?,granted_quantity=?,error_code=?,error_message=? WHERE request_id=? AND status='queued'`, now.Unix(), status, quantity, code, message, requestID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, lookupErr := s.ItemGrant(ctx, requestID)
		return lookupErr
	}
	return nil
}

const itemGrantSelect = `SELECT request_id,idempotency_key,created_at,updated_at,actor,player_uid,player_name,item_id,item_name,quantity,granted_quantity,reason,status,error_code,error_message FROM item_grants`

type itemGrantScanner interface{ Scan(dest ...any) error }

func scanItemGrant(row itemGrantScanner) (ItemGrant, error) {
	var g ItemGrant
	var created, updated int64
	var granted sql.NullInt64
	err := row.Scan(&g.RequestID, &g.IdempotencyKey, &created, &updated, &g.Actor, &g.PlayerUID, &g.PlayerName, &g.ItemID, &g.ItemName, &g.Quantity, &granted, &g.Reason, &g.Status, &g.ErrorCode, &g.ErrorMessage)
	if err != nil {
		return ItemGrant{}, err
	}
	g.CreatedAt, g.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	if granted.Valid {
		v := int(granted.Int64)
		g.GrantedQuantity = &v
	}
	return g, nil
}
