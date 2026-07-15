package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/8tp/palhelm/internal/paldeck"
	"github.com/8tp/palhelm/internal/sav"
)

const paldeckCoverageSource = "player_save_record_data"

type PaldeckSpecies struct {
	CharacterID       string `json:"characterId"`
	DisplayName       string `json:"displayName"`
	Known             bool   `json:"known"`
	CaptureCount      *int64 `json:"captureCount"`
	CapturedByPlayers *int   `json:"capturedByPlayers"`
	UnlockedByPlayers *int   `json:"unlockedByPlayers"`
}

type PlayerPaldeckSpecies struct {
	CharacterID  string `json:"characterId"`
	DisplayName  string `json:"displayName"`
	Known        bool   `json:"known"`
	CaptureCount *int64 `json:"captureCount"`
	Unlocked     *bool  `json:"unlocked"`
}

type PaldeckCatalog struct {
	Version                string `json:"version"`
	KnownSpecies           int    `json:"knownSpecies"`
	ObservedUnknownSpecies int    `json:"observedUnknownSpecies"`
}

type PaldeckCoverage struct {
	Source                   string     `json:"source"`
	PlayersTotal             int        `json:"playersTotal"`
	PlayersWithCaptureCounts int        `json:"playersWithCaptureCounts"`
	PlayersWithUnlockFlags   int        `json:"playersWithUnlockFlags"`
	CaptureCountsTruncated   bool       `json:"captureCountsTruncated"`
	UnlockFlagsTruncated     bool       `json:"unlockFlagsTruncated"`
	OldestObservedAt         *time.Time `json:"oldestObservedAt"`
	LatestObservedAt         *time.Time `json:"latestObservedAt"`
}

type ServerPaldeck struct {
	Coverage              PaldeckCoverage  `json:"coverage"`
	Catalog               PaldeckCatalog   `json:"catalog"`
	CaptureTotal          *int64           `json:"captureTotal"`
	UniqueSpeciesCaptured *int             `json:"uniqueSpeciesCaptured"`
	SpeciesUnlocked       *int             `json:"speciesUnlocked"`
	Species               []PaldeckSpecies `json:"species"`
}

type PlayerPaldeckCoverage struct {
	Source                 string     `json:"source"`
	CaptureCountsAvailable bool       `json:"captureCountsAvailable"`
	UnlockFlagsAvailable   bool       `json:"unlockFlagsAvailable"`
	CaptureCountsTruncated bool       `json:"captureCountsTruncated"`
	UnlockFlagsTruncated   bool       `json:"unlockFlagsTruncated"`
	CaptureObservedAt      *time.Time `json:"captureObservedAt"`
	UnlockObservedAt       *time.Time `json:"unlockObservedAt"`
}

type PlayerPaldeckIdentity struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
}

type PlayerPaldeck struct {
	Player             PlayerPaldeckIdentity  `json:"player"`
	Coverage           PlayerPaldeckCoverage  `json:"coverage"`
	Catalog            PaldeckCatalog         `json:"catalog"`
	CaptureTotal       *int64                 `json:"captureTotal"`
	UniquePalsCaptured *int                   `json:"uniquePalsCaptured"`
	PaldeckUnlocked    *int                   `json:"paldeckUnlocked"`
	Species            []PlayerPaldeckSpecies `json:"species"`
}

// replacePlayerPaldeck persists only authoritative RecordData maps. A nil map leaves the last
// successful observation untouched; an empty non-nil map authoritatively clears that side.
func replacePlayerPaldeck(ctx context.Context, tx *sql.Tx, p sav.Player, at time.Time) error {
	if p.PalCaptureCounts == nil && p.PaldeckUnlockFlags == nil {
		return nil
	}
	uid := NormalizeUID(p.UID)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO player_paldeck_state(player_uid) VALUES(?)`, uid); err != nil {
		return err
	}
	if p.PalCaptureCounts != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE player_paldeck SET capture_count=NULL WHERE player_uid=?`, uid); err != nil {
			return err
		}
		counts := normalizeCaptureCounts(p.PalCaptureCounts)
		for characterID, count := range counts {
			if _, err := tx.ExecContext(ctx, `INSERT INTO player_paldeck(player_uid,character_id,capture_count) VALUES(?,?,?) ON CONFLICT(player_uid,character_id) DO UPDATE SET capture_count=excluded.capture_count`, uid, characterID, count); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE player_paldeck_state SET capture_counts_available=1,capture_counts_truncated=?,capture_observed_at=? WHERE player_uid=?`, p.PalCaptureCountsTruncated, at.Unix(), uid); err != nil {
			return err
		}
	}
	if p.PaldeckUnlockFlags != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE player_paldeck SET unlocked=NULL WHERE player_uid=?`, uid); err != nil {
			return err
		}
		flags := normalizeUnlockFlags(p.PaldeckUnlockFlags)
		for characterID, unlocked := range flags {
			if _, err := tx.ExecContext(ctx, `INSERT INTO player_paldeck(player_uid,character_id,unlocked) VALUES(?,?,?) ON CONFLICT(player_uid,character_id) DO UPDATE SET unlocked=excluded.unlocked`, uid, characterID, unlocked); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE player_paldeck_state SET unlock_flags_available=1,unlock_flags_truncated=?,unlock_observed_at=? WHERE player_uid=?`, p.PaldeckUnlockFlagsTruncated, at.Unix(), uid); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM player_paldeck WHERE player_uid=? AND capture_count IS NULL AND unlocked IS NULL`, uid)
	return err
}

func normalizePaldeckCharacterID(value string) string {
	return strings.ToLower(strings.TrimSpace(paldeck.BaseCharacterID(value)))
}

func normalizeCaptureCounts(input map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(input))
	for raw, value := range input {
		id := normalizePaldeckCharacterID(raw)
		if id == "" || value < 0 {
			continue
		}
		if value > math.MaxInt64-out[id] {
			out[id] = math.MaxInt64
		} else {
			out[id] += value
		}
	}
	return out
}

func normalizeUnlockFlags(input map[string]bool) map[string]bool {
	out := make(map[string]bool, len(input))
	for raw, value := range input {
		id := normalizePaldeckCharacterID(raw)
		if id != "" {
			out[id] = out[id] || value
		}
	}
	return out
}

func (s *Store) ServerPaldeck(ctx context.Context) (ServerPaldeck, error) {
	result := ServerPaldeck{Coverage: PaldeckCoverage{Source: paldeckCoverageSource}, Catalog: PaldeckCatalog{Version: "palworld_1.0_pinned"}, Species: []PaldeckSpecies{}}
	var oldest, latest sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT
  (SELECT COUNT(*) FROM players),
  COALESCE(SUM(capture_counts_available),0),COALESCE(SUM(unlock_flags_available),0),
  COALESCE(MAX(capture_counts_truncated),0),COALESCE(MAX(unlock_flags_truncated),0),
  MIN(CASE WHEN capture_observed_at IS NULL THEN unlock_observed_at WHEN unlock_observed_at IS NULL THEN capture_observed_at ELSE MIN(capture_observed_at,unlock_observed_at) END),
  MAX(CASE WHEN capture_observed_at IS NULL THEN unlock_observed_at WHEN unlock_observed_at IS NULL THEN capture_observed_at ELSE MAX(capture_observed_at,unlock_observed_at) END)
FROM player_paldeck_state`).Scan(&result.Coverage.PlayersTotal, &result.Coverage.PlayersWithCaptureCounts, &result.Coverage.PlayersWithUnlockFlags, &result.Coverage.CaptureCountsTruncated, &result.Coverage.UnlockFlagsTruncated, &oldest, &latest)
	if err != nil {
		return ServerPaldeck{}, err
	}
	if oldest.Valid {
		v := time.Unix(oldest.Int64, 0).UTC()
		result.Coverage.OldestObservedAt = &v
	}
	if latest.Valid {
		v := time.Unix(latest.Int64, 0).UTC()
		result.Coverage.LatestObservedAt = &v
	}
	var aggregate sql.NullInt64
	var aggregatePlayers int
	if err = s.db.QueryRowContext(ctx, `SELECT SUM(capture_total),COUNT(capture_total) FROM players`).Scan(&aggregate, &aggregatePlayers); err != nil {
		return ServerPaldeck{}, err
	}
	if aggregatePlayers > 0 && aggregate.Valid {
		result.CaptureTotal = &aggregate.Int64
	}
	rows, err := s.db.QueryContext(ctx, `SELECT character_id,SUM(capture_count),COUNT(CASE WHEN capture_count>0 THEN 1 END),COUNT(capture_count),COUNT(CASE WHEN unlocked=1 THEN 1 END),COUNT(unlocked) FROM player_paldeck GROUP BY character_id ORDER BY character_id LIMIT 2048`)
	if err != nil {
		return ServerPaldeck{}, err
	}
	defer rows.Close()
	observed := map[string]PaldeckSpecies{}
	captured, unlocked := 0, 0
	for rows.Next() {
		var species PaldeckSpecies
		var captureCount sql.NullInt64
		var capturedBy, captureRows, unlockedBy, unlockRows int
		if err = rows.Scan(&species.CharacterID, &captureCount, &capturedBy, &captureRows, &unlockedBy, &unlockRows); err != nil {
			return ServerPaldeck{}, err
		}
		species.DisplayName = paldeck.Name(species.CharacterID)
		if captureRows > 0 || result.Coverage.PlayersWithCaptureCounts > 0 && !result.Coverage.CaptureCountsTruncated {
			value := captureCount.Int64
			species.CaptureCount, species.CapturedByPlayers = &value, &capturedBy
		}
		if unlockRows > 0 || result.Coverage.PlayersWithUnlockFlags > 0 && !result.Coverage.UnlockFlagsTruncated {
			species.UnlockedByPlayers = &unlockedBy
		}
		if captureCount.Valid && captureCount.Int64 > 0 {
			captured++
		}
		if unlockedBy > 0 {
			unlocked++
		}
		observed[species.CharacterID] = species
	}
	if err = rows.Err(); err != nil {
		return ServerPaldeck{}, err
	}
	if result.Coverage.PlayersWithCaptureCounts > 0 {
		result.UniqueSpeciesCaptured = &captured
	}
	if result.Coverage.PlayersWithUnlockFlags > 0 {
		result.SpeciesUnlocked = &unlocked
	}
	known := paldeckCatalog()
	result.Catalog.KnownSpecies = len(known)
	for _, entry := range known {
		species, ok := observed[entry.CharacterID]
		if ok {
			delete(observed, entry.CharacterID)
		} else {
			species = PaldeckSpecies{CharacterID: entry.CharacterID, DisplayName: entry.DisplayName}
			if result.Coverage.PlayersWithCaptureCounts > 0 && !result.Coverage.CaptureCountsTruncated {
				zero, players := int64(0), 0
				species.CaptureCount, species.CapturedByPlayers = &zero, &players
			}
			if result.Coverage.PlayersWithUnlockFlags > 0 && !result.Coverage.UnlockFlagsTruncated {
				zero := 0
				species.UnlockedByPlayers = &zero
			}
		}
		if species.CaptureCount == nil && result.Coverage.PlayersWithCaptureCounts > 0 && !result.Coverage.CaptureCountsTruncated {
			zero, players := int64(0), 0
			species.CaptureCount, species.CapturedByPlayers = &zero, &players
		}
		if species.UnlockedByPlayers == nil && result.Coverage.PlayersWithUnlockFlags > 0 && !result.Coverage.UnlockFlagsTruncated {
			zero := 0
			species.UnlockedByPlayers = &zero
		}
		species.Known = true
		result.Species = append(result.Species, species)
	}
	result.Catalog.ObservedUnknownSpecies = len(observed)
	for _, species := range observed {
		if species.CaptureCount == nil && result.Coverage.PlayersWithCaptureCounts > 0 && !result.Coverage.CaptureCountsTruncated {
			zero, players := int64(0), 0
			species.CaptureCount, species.CapturedByPlayers = &zero, &players
		}
		if species.UnlockedByPlayers == nil && result.Coverage.PlayersWithUnlockFlags > 0 && !result.Coverage.UnlockFlagsTruncated {
			zero := 0
			species.UnlockedByPlayers = &zero
		}
		result.Species = append(result.Species, species)
	}
	sort.Slice(result.Species, func(i, j int) bool { return result.Species[i].DisplayName < result.Species[j].DisplayName })
	return result, nil
}

func (s *Store) PlayerPaldeck(ctx context.Context, uid string) (PlayerPaldeck, error) {
	p, err := s.PlayerByUID(ctx, uid)
	if err != nil {
		return PlayerPaldeck{}, err
	}
	result := PlayerPaldeck{
		Player:       PlayerPaldeckIdentity{UID: p.UID, Name: p.Name},
		Coverage:     PlayerPaldeckCoverage{Source: paldeckCoverageSource},
		Catalog:      PaldeckCatalog{Version: "palworld_1.0_pinned"},
		CaptureTotal: p.CaptureTotal, UniquePalsCaptured: p.UniquePalsCaptured, PaldeckUnlocked: p.PaldeckUnlocked,
		Species: []PlayerPaldeckSpecies{},
	}
	var captureAt, unlockAt sql.NullInt64
	err = s.db.QueryRowContext(ctx, `SELECT capture_counts_available,unlock_flags_available,capture_counts_truncated,unlock_flags_truncated,capture_observed_at,unlock_observed_at FROM player_paldeck_state WHERE player_uid=?`, p.UID).Scan(
		&result.Coverage.CaptureCountsAvailable, &result.Coverage.UnlockFlagsAvailable, &result.Coverage.CaptureCountsTruncated, &result.Coverage.UnlockFlagsTruncated, &captureAt, &unlockAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PlayerPaldeck{}, err
	}
	if captureAt.Valid {
		v := time.Unix(captureAt.Int64, 0).UTC()
		result.Coverage.CaptureObservedAt = &v
	}
	if unlockAt.Valid {
		v := time.Unix(unlockAt.Int64, 0).UTC()
		result.Coverage.UnlockObservedAt = &v
	}
	rows, err := s.db.QueryContext(ctx, `SELECT character_id,capture_count,unlocked FROM player_paldeck WHERE player_uid=? ORDER BY character_id LIMIT 2048`, p.UID)
	if err != nil {
		return PlayerPaldeck{}, err
	}
	defer rows.Close()
	observed := map[string]PlayerPaldeckSpecies{}
	for rows.Next() {
		var species PlayerPaldeckSpecies
		var capture sql.NullInt64
		var unlocked sql.NullBool
		if err = rows.Scan(&species.CharacterID, &capture, &unlocked); err != nil {
			return PlayerPaldeck{}, err
		}
		species.DisplayName = paldeck.Name(species.CharacterID)
		if capture.Valid {
			species.CaptureCount = &capture.Int64
		}
		if unlocked.Valid {
			species.Unlocked = &unlocked.Bool
		}
		observed[species.CharacterID] = species
	}
	if err = rows.Err(); err != nil {
		return PlayerPaldeck{}, err
	}
	known := paldeckCatalog()
	result.Catalog.KnownSpecies = len(known)
	for _, entry := range known {
		species, ok := observed[entry.CharacterID]
		if ok {
			delete(observed, entry.CharacterID)
		} else {
			species = PlayerPaldeckSpecies{CharacterID: entry.CharacterID, DisplayName: entry.DisplayName}
			if result.Coverage.CaptureCountsAvailable && !result.Coverage.CaptureCountsTruncated {
				zero := int64(0)
				species.CaptureCount = &zero
			}
			if result.Coverage.UnlockFlagsAvailable && !result.Coverage.UnlockFlagsTruncated {
				value := false
				species.Unlocked = &value
			}
		}
		if species.CaptureCount == nil && result.Coverage.CaptureCountsAvailable && !result.Coverage.CaptureCountsTruncated {
			zero := int64(0)
			species.CaptureCount = &zero
		}
		if species.Unlocked == nil && result.Coverage.UnlockFlagsAvailable && !result.Coverage.UnlockFlagsTruncated {
			value := false
			species.Unlocked = &value
		}
		species.Known = true
		result.Species = append(result.Species, species)
	}
	result.Catalog.ObservedUnknownSpecies = len(observed)
	for _, species := range observed {
		if species.CaptureCount == nil && result.Coverage.CaptureCountsAvailable && !result.Coverage.CaptureCountsTruncated {
			zero := int64(0)
			species.CaptureCount = &zero
		}
		if species.Unlocked == nil && result.Coverage.UnlockFlagsAvailable && !result.Coverage.UnlockFlagsTruncated {
			value := false
			species.Unlocked = &value
		}
		result.Species = append(result.Species, species)
	}
	sort.Slice(result.Species, func(i, j int) bool { return result.Species[i].DisplayName < result.Species[j].DisplayName })
	return result, nil
}

type paldeckCatalogEntry struct {
	CharacterID string
	DisplayName string
}

func paldeckCatalog() []paldeckCatalogEntry {
	byID := map[string]string{}
	for _, entry := range paldeck.All() {
		id := normalizePaldeckCharacterID(entry.ID)
		if id != "" {
			byID[id] = entry.Name
		}
	}
	out := make([]paldeckCatalogEntry, 0, len(byID))
	for id, name := range byID {
		out = append(out, paldeckCatalogEntry{CharacterID: id, DisplayName: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CharacterID < out[j].CharacterID })
	return out
}
