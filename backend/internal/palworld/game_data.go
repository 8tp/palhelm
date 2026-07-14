package palworld

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GameDataMaxResponseBytes bounds the official world-actor snapshot. The endpoint has no
// pagination contract and can include every currently loaded wild Pal and NPC, so it must
// never be decoded through the unbounded generic REST path.
const GameDataMaxResponseBytes int64 = 32 << 20

// GameDataMaxActors is a second defensive bound after JSON decoding. The byte limit is the
// primary memory guard; this limit also rejects pathological arrays of tiny objects.
const GameDataMaxActors = 250_000

// GameDataSnapshot is the documented Palworld 1.0 /game-data response. Time is deliberately
// retained as an opaque string: Pocketpair documents it as server-local time without an
// offset, not an RFC 3339 timestamp.
type GameDataSnapshot struct {
	Time       string          `json:"Time"`
	FPS        float64         `json:"FPS"`
	AverageFPS float64         `json:"AverageFPS"`
	ActorData  []GameDataActor `json:"ActorData"`
}

// GameDataActor tolerantly models both Character and PalBox variants. Unknown JSON fields are
// intentionally ignored for forward compatibility. IP and userid are intentionally absent:
// encoding/json discards them at the credential boundary, so Palhelm cannot accidentally log,
// cache, or project them later.
type GameDataActor struct {
	Type              string  `json:"Type"`
	UnitType          string  `json:"UnitType"`
	InstanceID        string  `json:"InstanceID"`
	NickName          string  `json:"NickName"`
	TrainerInstanceID string  `json:"TrainerInstanceID"`
	TrainerNickName   string  `json:"TrainerNickName"`
	Level             int     `json:"level"`
	HP                int     `json:"HP"`
	MaxHP             int     `json:"MaxHP"`
	GuildID           string  `json:"GuildID"`
	GuildName         string  `json:"GuildName"`
	Class             string  `json:"Class"`
	Action            string  `json:"Action"`
	AIAction          string  `json:"AI_Action"`
	LocationX         float64 `json:"LocationX"`
	LocationY         float64 `json:"LocationY"`
	LocationZ         float64 `json:"LocationZ"`
	RotationX         float64 `json:"RotationX"`
	RotationY         float64 `json:"RotationY"`
	RotationZ         float64 `json:"RotationZ"`
	Stage             string  `json:"Stage"`
	IsActive          string  `json:"IsActive"`
}

// Active normalizes the documented string-valued IsActive field. Unknown or missing values
// remain nil rather than being guessed.
func (a GameDataActor) Active() *bool {
	switch strings.ToLower(strings.TrimSpace(a.IsActive)) {
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	default:
		return nil
	}
}

// SetGameDataTimeout changes only the large snapshot request timeout. It must be called during
// startup, before the client begins polling. Ordinary REST methods retain Client.http's
// five-second timeout.
func (c *Client) SetGameDataTimeout(timeout time.Duration) {
	if timeout > 0 {
		c.gameDataHTTP.Timeout = timeout
	}
}

// GameData fetches one bounded world-actor snapshot. It never retains the raw response bytes.
func (c *Client) GameData(ctx context.Context) (GameDataSnapshot, error) {
	var out GameDataSnapshot
	if c.base == "" {
		return out, &APIError{Kind: ErrorUnreachable, Err: errors.New("PALWORLD_REST_URL is unset")}
	}
	u := c.base + "/v1/api/game-data"
	parsed, err := url.Parse(u)
	if err != nil {
		return out, &APIError{Kind: ErrorUnreachable, Err: err}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return out, &APIError{Kind: ErrorUnreachable, Err: errors.New("PALWORLD_REST_URL must use http or https")}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return out, err
	}
	req.SetBasicAuth(c.user, c.password)
	req.Header.Set("Accept", "application/json")

	// This dedicated client is created before any request and never copied after first use.
	// Refusing redirects keeps Basic credentials pinned to the configured REST host.
	resp, err := c.gameDataHTTP.Do(req)
	if err != nil {
		return out, &APIError{Kind: ErrorUnreachable, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		kind := ErrorResponse
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			kind = ErrorUnauthorized
		case http.StatusNotFound:
			kind = ErrorUnsupported
		}
		// Do not echo or log an upstream response body: future server builds may include actor or
		// credential-adjacent data in errors.
		return out, &APIError{Kind: kind, Status: resp.StatusCode, Err: errors.New(http.StatusText(resp.StatusCode))}
	}
	if resp.ContentLength > GameDataMaxResponseBytes {
		return out, &APIError{Kind: ErrorResponse, Status: resp.StatusCode, Err: errors.New("game-data response exceeds size limit")}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, GameDataMaxResponseBytes+1))
	if err != nil {
		return out, &APIError{Kind: ErrorResponse, Status: resp.StatusCode, Err: errors.New("read game-data response")}
	}
	if int64(len(b)) > GameDataMaxResponseBytes {
		return out, &APIError{Kind: ErrorResponse, Status: resp.StatusCode, Err: errors.New("game-data response exceeds size limit")}
	}
	if err = json.Unmarshal(b, &out); err != nil {
		return GameDataSnapshot{}, &APIError{Kind: ErrorResponse, Status: resp.StatusCode, Err: errors.New("invalid game-data JSON")}
	}
	if len(out.ActorData) > GameDataMaxActors {
		return GameDataSnapshot{}, &APIError{Kind: ErrorResponse, Status: resp.StatusCode, Err: errors.New("game-data actor count exceeds limit")}
	}
	if !math.IsNaN(out.FPS) && !math.IsInf(out.FPS, 0) && !math.IsNaN(out.AverageFPS) && !math.IsInf(out.AverageFPS, 0) {
		for _, actor := range out.ActorData {
			if !finite(actor.LocationX) || !finite(actor.LocationY) || !finite(actor.LocationZ) ||
				!finite(actor.RotationX) || !finite(actor.RotationY) || !finite(actor.RotationZ) {
				return GameDataSnapshot{}, &APIError{Kind: ErrorResponse, Status: resp.StatusCode, Err: errors.New("game-data contains non-finite coordinates")}
			}
		}
		return out, nil
	}
	return GameDataSnapshot{}, &APIError{Kind: ErrorResponse, Status: resp.StatusCode, Err: errors.New("game-data contains non-finite FPS")}
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
