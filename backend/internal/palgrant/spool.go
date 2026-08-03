package palgrant

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

var ErrBridgeBusy = errors.New("Pal bridge already has a pending request")

type Capability struct {
	Enabled         bool   `json:"enabled"`
	Ready           bool   `json:"ready"`
	Reason          string `json:"reason"`
	ProtocolVersion int    `json:"protocolVersion"`
	GameVersion     string `json:"gameVersion,omitempty"`
	CatalogVersion  string `json:"catalogVersion"`
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
	ProtocolVersion int            `json:"protocolVersion"`
	RequestID       string         `json:"requestId"`
	CreatedAt       string         `json:"createdAt"`
	PlayerUID       string         `json:"playerUid"`
	PlayerName      string         `json:"playerName"`
	CharacterID     string         `json:"characterId"`
	Spec            GenerationSpec `json:"spec"`
}

type Result struct {
	ProtocolVersion   int      `json:"protocolVersion"`
	RequestID         string   `json:"requestId"`
	Status            string   `json:"status"`
	InstanceID        string   `json:"instanceId,omitempty"`
	Level             int      `json:"level,omitempty"`
	Gender            string   `json:"gender,omitempty"`
	Alpha             bool     `json:"alpha,omitempty"`
	Lucky             bool     `json:"lucky,omitempty"`
	TalentHP          *int     `json:"talentHp,omitempty"`
	TalentMelee       *int     `json:"talentMelee,omitempty"`
	TalentShot        *int     `json:"talentShot,omitempty"`
	TalentDefense     *int     `json:"talentDefense,omitempty"`
	PassiveSkillIDs   []string `json:"passiveSkillIds,omitempty"`
	CondensationStars int      `json:"condensationStars,omitempty"`
	ErrorCode         string   `json:"errorCode,omitempty"`
	ErrorMessage      string   `json:"errorMessage,omitempty"`
	CompletedAt       string   `json:"completedAt"`
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
	c := Capability{Enabled: enabled, ProtocolVersion: BridgeProtocolVersion, CatalogVersion: CatalogVersion}
	if catalog != nil {
		c.CatalogItems = catalog.Count()
	}
	if !enabled {
		c.Reason = "Pal grants are disabled in Palhelm configuration."
		return c
	}
	if catalog == nil || catalog.Count() == 0 {
		c.Reason = "The safe Pal creation catalogue is unavailable."
		return c
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, "capability.json"))
	if err != nil {
		c.Reason = "The Pal creation bridge has not reported ready."
		return c
	}
	var h heartbeat
	if json.Unmarshal(b, &h) != nil || h.ProtocolVersion != BridgeProtocolVersion {
		c.Reason = "The Pal creation bridge protocol is incompatible."
		return c
	}
	c.GameVersion, c.LastHeartbeatAt = strings.TrimSpace(h.GameVersion), h.At
	at, err := time.Parse(time.RFC3339, h.At)
	if err != nil || now.Sub(at) < 0 || now.Sub(at) > heartbeatMaxAge {
		c.Reason = "The Pal creation bridge heartbeat is stale."
		return c
	}
	if h.State != "ready" {
		c.Reason = "The Pal creation bridge has not passed validation for this game build."
		return c
	}
	if h.CatalogVersion != CatalogVersion || h.CatalogItems != catalog.Count() {
		c.Reason = "The Pal bridge allowlist does not match Palhelm's Pal catalogue."
		return c
	}
	c.Ready = true
	return c
}

func (s Spool) Submit(req Request) error {
	if req.ProtocolVersion != BridgeProtocolVersion || !safeToken.MatchString(req.RequestID) || !safeToken.MatchString(req.PlayerUID) || !safeToken.MatchString(req.CharacterID) || req.Spec.Validate() != nil {
		return errors.New("invalid Pal grant request")
	}
	requests, results := filepath.Join(s.Dir, "requests"), filepath.Join(s.Dir, "results")
	if err := os.MkdirAll(requests, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(results, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	target := filepath.Join(requests, req.RequestID+".json")
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
	if !safeToken.MatchString(requestID) {
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
		return Result{}, fmt.Errorf("decode Pal bridge result: %w", err)
	}
	if result.ProtocolVersion != BridgeProtocolVersion || result.RequestID != requestID || (result.Status != "succeeded" && result.Status != "failed") {
		return Result{}, errors.New("invalid Pal bridge result")
	}
	if result.Status == "succeeded" {
		if !safeToken.MatchString(result.InstanceID) || result.Level < 1 || result.Level > MaxSupportedLevel || (result.Gender != "male" && result.Gender != "female") || result.CondensationStars < 0 || result.CondensationStars > 4 {
			return Result{}, errors.New("invalid successful Pal bridge result")
		}
		for _, value := range []*int{result.TalentHP, result.TalentMelee, result.TalentShot, result.TalentDefense} {
			if value == nil || *value < 0 || *value > 100 {
				return Result{}, errors.New("successful Pal bridge result has invalid IVs")
			}
		}
		if len(result.PassiveSkillIDs) > 4 {
			return Result{}, errors.New("successful Pal bridge result has too many passive skills")
		}
		seen := make(map[string]bool, len(result.PassiveSkillIDs))
		for _, id := range result.PassiveSkillIDs {
			key := strings.ToLower(id)
			if !safeToken.MatchString(id) || seen[key] {
				return Result{}, errors.New("successful Pal bridge result has invalid passive skills")
			}
			seen[key] = true
		}
	}
	if len(result.ErrorCode) > 100 || len(result.ErrorMessage) > 500 {
		return Result{}, errors.New("bridge result error fields are too long")
	}
	if _, err := time.Parse(time.RFC3339, result.CompletedAt); err != nil {
		return Result{}, errors.New("bridge result completedAt is invalid")
	}
	return result, nil
}
