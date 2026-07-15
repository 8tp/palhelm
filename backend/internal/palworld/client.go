// Package palworld implements the official Palworld REST API and Source RCON protocols.
package palworld

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrorKind classifies REST failures for health reporting and HTTP translation.
type ErrorKind string

const (
	ErrorUnreachable  ErrorKind = "unreachable"
	ErrorUnauthorized ErrorKind = "unauthorized"
	ErrorUnsupported  ErrorKind = "unsupported"
	ErrorResponse     ErrorKind = "response"
)

// APIError is a classified response or transport error.
type APIError struct {
	Kind   ErrorKind
	Status int
	Err    error
}

func (e *APIError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("palworld REST status %d: %v", e.Status, e.Err)
	}
	return "palworld REST: " + e.Err.Error()
}
func (e *APIError) Unwrap() error { return e.Err }

// Client is a typed official REST API client.
type Client struct {
	base, user, password string
	http                 *http.Client
	gameDataHTTP         *http.Client
}

// NewClient constructs a client with the required five-second timeout.
func NewClient(base, user, password string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"), user: user, password: password,
		http: &http.Client{Timeout: 5 * time.Second},
		gameDataHTTP: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Info is the server identity response.
type Info struct {
	Version     string `json:"version"`
	ServerName  string `json:"servername"`
	Description string `json:"description"`
	WorldGUID   string `json:"worldguid"`
	Uptime      int64  `json:"uptime"`
}

// Metrics is the live server metric response.
type Metrics struct {
	ServerFPS        float64 `json:"serverfps"`
	CurrentPlayerNum int     `json:"currentplayernum"`
	ServerFrameTime  float64 `json:"serverframetime"`
	MaxPlayerNum     int     `json:"maxplayernum"`
	Uptime           int64   `json:"uptime"`
	Days             int64   `json:"days"`
	BaseCampNum      int     `json:"basecampnum"`
}

// Player is one online player returned by REST.
type Player struct {
	Name        string  `json:"name"`
	AccountName string  `json:"accountName"`
	PlayerID    string  `json:"playerId"`
	UserID      string  `json:"userId"`
	IP          string  `json:"ip"`
	Ping        float64 `json:"ping"`
	LocationX   float64 `json:"location_x"`
	LocationY   float64 `json:"location_y"`
	Level       int     `json:"level"`
	// BuildingCount is the number of buildings owned by the player. Documented at
	// docs.palworldgame.com/api/rest-api/players/ as a 1.0-era addition; omitted (zero value)
	// on older server builds that don't send it, since json.Unmarshal leaves missing fields
	// at their zero value.
	BuildingCount int `json:"building_count"`
}

// Players is the REST player-list envelope.
type Players struct {
	Players []Player `json:"players"`
}

// Info returns server metadata.
func (c *Client) Info(ctx context.Context) (Info, error) {
	var v Info
	return v, c.do(ctx, http.MethodGet, "info", nil, &v)
}

// Metrics returns current server metrics.
func (c *Client) Metrics(ctx context.Context) (Metrics, error) {
	var v Metrics
	return v, c.do(ctx, http.MethodGet, "metrics", nil, &v)
}

// Players returns currently connected players.
func (c *Client) Players(ctx context.Context) ([]Player, error) {
	var v Players
	err := c.do(ctx, http.MethodGet, "players", nil, &v)
	return v.Players, err
}

// Settings returns the raw settings object.
func (c *Client) Settings(ctx context.Context) (map[string]any, error) {
	var v map[string]any
	return v, c.do(ctx, http.MethodGet, "settings", nil, &v)
}

// Announce broadcasts a message.
func (c *Client) Announce(ctx context.Context, message string) error {
	return c.do(ctx, http.MethodPost, "announce", map[string]any{"message": message}, nil)
}

// Kick disconnects a REST user ID.
func (c *Client) Kick(ctx context.Context, userID, message string) error {
	return c.do(ctx, http.MethodPost, "kick", map[string]any{"userid": userID, "message": message}, nil)
}

// Ban bans a REST user ID.
func (c *Client) Ban(ctx context.Context, userID, message string) error {
	return c.do(ctx, http.MethodPost, "ban", map[string]any{"userid": userID, "message": message}, nil)
}

// Unban removes a REST user ID ban.
func (c *Client) Unban(ctx context.Context, userID string) error {
	return c.do(ctx, http.MethodPost, "unban", map[string]any{"userid": userID}, nil)
}

// Save triggers a world save.
func (c *Client) Save(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "save", struct{}{}, nil)
}

// Shutdown requests graceful shutdown.
func (c *Client) Shutdown(ctx context.Context, waitSec int, message string) error {
	return c.do(ctx, http.MethodPost, "shutdown", map[string]any{"waittime": waitSec, "message": message}, nil)
}

// Stop requests an immediate stop.
func (c *Client) Stop(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "stop", struct{}{}, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	if c.base == "" {
		return &APIError{Kind: ErrorUnreachable, Err: errors.New("PALWORLD_REST_URL is unset")}
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	u := c.base + "/v1/api/" + path
	if _, err := url.Parse(u); err != nil {
		return &APIError{Kind: ErrorUnreachable, Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.user, c.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) {
			return &APIError{Kind: ErrorUnreachable, Err: err}
		}
		return &APIError{Kind: ErrorUnreachable, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		kind := ErrorResponse
		if resp.StatusCode == http.StatusUnauthorized {
			kind = ErrorUnauthorized
		}
		return &APIError{Kind: kind, Status: resp.StatusCode, Err: errors.New(strings.TrimSpace(string(b)))}
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// IsKind reports whether err has the given REST failure classification.
func IsKind(err error, kind ErrorKind) bool {
	var e *APIError
	return errors.As(err, &e) && e.Kind == kind
}
