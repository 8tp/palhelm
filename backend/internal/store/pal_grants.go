package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/8tp/palhelm/internal/palgrant"
)

type PalGrant struct {
	RequestID      string                  `json:"requestId"`
	IdempotencyKey string                  `json:"idempotencyKey,omitempty"`
	CreatedAt      time.Time               `json:"createdAt"`
	UpdatedAt      time.Time               `json:"updatedAt"`
	Actor          string                  `json:"actor"`
	PlayerUID      string                  `json:"playerUid"`
	PlayerName     string                  `json:"playerName"`
	CharacterID    string                  `json:"characterId"`
	DisplayName    string                  `json:"displayName"`
	Spec           palgrant.GenerationSpec `json:"spec"`
	Result         *palgrant.Result        `json:"result,omitempty"`
	Reason         string                  `json:"reason"`
	Status         string                  `json:"status"`
	ErrorCode      string                  `json:"errorCode,omitempty"`
	ErrorMessage   string                  `json:"errorMessage,omitempty"`
}

func (s *Store) CreatePalGrant(ctx context.Context, g PalGrant) (PalGrant, bool, error) {
	g.PlayerUID = NormalizeUID(g.PlayerUID)
	spec, err := json.Marshal(g.Spec)
	if err != nil {
		return PalGrant{}, false, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO pal_grants(request_id,idempotency_key,created_at,updated_at,actor,player_uid,player_name,character_id,display_name,spec_json,reason,status) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		g.RequestID, g.IdempotencyKey, g.CreatedAt.Unix(), g.UpdatedAt.Unix(), g.Actor, g.PlayerUID, g.PlayerName, g.CharacterID, g.DisplayName, string(spec), g.Reason, g.Status)
	if err == nil {
		return g, true, nil
	}
	existing, getErr := s.PalGrantByIdempotencyKey(ctx, g.IdempotencyKey)
	if getErr == nil {
		return existing, false, nil
	}
	return PalGrant{}, false, err
}

func (s *Store) PalGrantByIdempotencyKey(ctx context.Context, key string) (PalGrant, error) {
	return scanPalGrant(s.db.QueryRowContext(ctx, palGrantSelect+" WHERE idempotency_key=?", key))
}

func (s *Store) PalGrant(ctx context.Context, requestID string) (PalGrant, error) {
	return scanPalGrant(s.db.QueryRowContext(ctx, palGrantSelect+" WHERE request_id=?", requestID))
}

func (s *Store) PalGrants(ctx context.Context, limit int) ([]PalGrant, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, palGrantSelect+" ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PalGrant, 0)
	for rows.Next() {
		grant, err := scanPalGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, grant)
	}
	return out, rows.Err()
}

func (s *Store) CompletePalGrant(ctx context.Context, requestID, status string, result *palgrant.Result, code, message string, now time.Time) error {
	if status != "succeeded" && status != "failed" {
		return errors.New("invalid Pal grant status")
	}
	var resultJSON any
	if result != nil {
		body, err := json.Marshal(result)
		if err != nil {
			return err
		}
		resultJSON = string(body)
	}
	updated, err := s.db.ExecContext(ctx, `UPDATE pal_grants SET updated_at=?,status=?,result_json=?,error_code=?,error_message=? WHERE request_id=? AND status='queued'`, now.Unix(), status, resultJSON, code, message, requestID)
	if err != nil {
		return err
	}
	n, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, lookupErr := s.PalGrant(ctx, requestID)
		return lookupErr
	}
	return nil
}

const palGrantSelect = `SELECT request_id,idempotency_key,created_at,updated_at,actor,player_uid,player_name,character_id,display_name,spec_json,result_json,reason,status,error_code,error_message FROM pal_grants`

func scanPalGrant(row itemGrantScanner) (PalGrant, error) {
	var grant PalGrant
	var created, updated int64
	var specJSON string
	var resultJSON sql.NullString
	if err := row.Scan(&grant.RequestID, &grant.IdempotencyKey, &created, &updated, &grant.Actor, &grant.PlayerUID, &grant.PlayerName, &grant.CharacterID, &grant.DisplayName, &specJSON, &resultJSON, &grant.Reason, &grant.Status, &grant.ErrorCode, &grant.ErrorMessage); err != nil {
		return PalGrant{}, err
	}
	if err := json.Unmarshal([]byte(specJSON), &grant.Spec); err != nil {
		return PalGrant{}, fmt.Errorf("decode stored Pal grant spec: %w", err)
	}
	if resultJSON.Valid {
		var result palgrant.Result
		if err := json.Unmarshal([]byte(resultJSON.String), &result); err != nil {
			return PalGrant{}, fmt.Errorf("decode stored Pal grant result: %w", err)
		}
		grant.Result = &result
	}
	grant.CreatedAt, grant.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	return grant, nil
}
