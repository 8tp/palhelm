package itemgrant

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const BridgeProtocolVersion = 1
const heartbeatMaxAge = 45 * time.Second

var ErrBridgeBusy = errors.New("server bridge already has a pending request")

type Capability struct {
	Enabled         bool   `json:"enabled"`
	Ready           bool   `json:"ready"`
	Reason          string `json:"reason"`
	ProtocolVersion int    `json:"protocolVersion"`
	GameVersion     string `json:"gameVersion,omitempty"`
	CatalogVersion  string `json:"catalogVersion,omitempty"`
	CatalogItems    int    `json:"catalogItems"`
	LastHeartbeatAt string `json:"lastHeartbeatAt,omitempty"`
}

type heartbeat struct {
	ProtocolVersion int    `json:"protocolVersion"`
	State           string `json:"state"`
	GameVersion     string `json:"gameVersion"`
	CatalogVersion  string `json:"catalogVersion"`
	CatalogItems    int    `json:"catalogItems"`
	At              string `json:"at"`
}

type Request struct {
	ProtocolVersion int    `json:"protocolVersion"`
	RequestID       string `json:"requestId"`
	CreatedAt       string `json:"createdAt"`
	PlayerUID       string `json:"playerUid"`
	PlayerName      string `json:"playerName"`
	ItemID          string `json:"itemId"`
	Quantity        int    `json:"quantity"`
}

type Result struct {
	ProtocolVersion int    `json:"protocolVersion"`
	RequestID       string `json:"requestId"`
	Status          string `json:"status"`
	GrantedQuantity int    `json:"grantedQuantity,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
	CompletedAt     string `json:"completedAt"`
}

type Spool struct{ Dir string }

func NewRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s Spool) Capability(enabled bool, catalog *Catalog, now time.Time) Capability {
	c := Capability{Enabled: enabled, ProtocolVersion: BridgeProtocolVersion}
	if catalog != nil {
		c.CatalogVersion = catalog.GameVersion()
		c.CatalogItems = catalog.GrantableCount()
	}
	if !enabled {
		c.Reason = "Item grants are disabled in Palhelm configuration."
		return c
	}
	if catalog == nil {
		c.Reason = "No validated item catalogue is installed."
		return c
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, "capability.json"))
	if err != nil {
		c.Reason = "The server bridge has not reported ready."
		return c
	}
	var h heartbeat
	if json.Unmarshal(b, &h) != nil || h.ProtocolVersion != BridgeProtocolVersion {
		c.Reason = "The server bridge protocol is incompatible."
		return c
	}
	c.GameVersion, c.LastHeartbeatAt = strings.TrimSpace(h.GameVersion), h.At
	at, err := time.Parse(time.RFC3339, h.At)
	if err != nil || now.Sub(at) < 0 || now.Sub(at) > heartbeatMaxAge {
		c.Reason = "The server bridge heartbeat is stale."
		return c
	}
	if h.State != "ready" {
		c.Reason = "The server bridge is not ready."
		return c
	}
	if h.CatalogVersion != catalog.GameVersion() || h.GameVersion != catalog.GameVersion() {
		c.Reason = "The server and item catalogue versions do not match."
		return c
	}
	if h.CatalogItems != catalog.GrantableCount() {
		c.Reason = "The server bridge allowlist does not match the item catalogue."
		return c
	}
	c.Ready = true
	return c
}

func (s Spool) Submit(req Request) error {
	if req.ProtocolVersion != BridgeProtocolVersion || req.RequestID == "" || !safeID.MatchString(req.PlayerUID) || !safeID.MatchString(req.ItemID) || req.Quantity < 1 || req.Quantity > 9999 {
		return errors.New("invalid grant request")
	}
	dir := filepath.Join(s.Dir, "requests")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, "results"), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, req.RequestID+".json")
	if _, err := os.Stat(target); err == nil {
		return os.ErrExist
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Link(target, filepath.Join(s.Dir, "inbox.json")); err != nil {
		if os.IsExist(err) {
			return ErrBridgeBusy
		}
		return err
	}
	return nil
}

func (s Spool) Result(requestID string) (Result, error) {
	if !safeID.MatchString(requestID) {
		return Result{}, errors.New("invalid request id")
	}
	f, err := os.Open(filepath.Join(s.Dir, "results", requestID+".json"))
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return Result{}, err
	}
	if len(b) > 64<<10 {
		return Result{}, errors.New("bridge result exceeds 64 KiB")
	}
	var result Result
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("decode bridge result: %w", err)
	}
	if result.ProtocolVersion != BridgeProtocolVersion || result.RequestID != requestID || (result.Status != "succeeded" && result.Status != "failed") {
		return Result{}, errors.New("invalid bridge result")
	}
	if len(result.ErrorCode) > 100 || len(result.ErrorMessage) > 500 {
		return Result{}, errors.New("bridge result error fields are too long")
	}
	if _, err := time.Parse(time.RFC3339, result.CompletedAt); err != nil {
		return Result{}, errors.New("bridge result completedAt is invalid")
	}
	return result, nil
}
