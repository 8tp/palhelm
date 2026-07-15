// Package store owns Palhelm's SQLite persistence and retention logic.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/8tp/palhelm/internal/paldeck"
	"github.com/8tp/palhelm/internal/sav"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store wraps the application's SQLite database.
type Store struct {
	db *sql.DB

	activityPruneMu sync.Mutex
	activityPruned  time.Time
}

// Open opens a database, enables WAL, and applies embedded migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	// busy_timeout must be set before journal_mode=WAL: otherwise a concurrent opener that
	// arrives while this process holds the lock can get an immediate SQLITE_BUSY instead of
	// waiting out the timeout (audit finding; spec §10.1's busy_timeout=5000 wait applies to
	// every PRAGMA/exec on this handle, not just migrations).
	for _, q := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err = migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// migration is one embedded, numbered schema step.
type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations parses embedded migrations/NNN_name.sql files, ascending by NNN.
func loadMigrations() ([]migration, error) {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	out := make([]migration, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		cut := strings.IndexByte(name, '_')
		if cut < 0 {
			return nil, fmt.Errorf("migrations: unexpected filename %q", name)
		}
		v, err := strconv.Atoi(name[:cut])
		if err != nil {
			return nil, fmt.Errorf("migrations: unexpected filename %q: %w", name, err)
		}
		b, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: name, sql: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// queryRower is satisfied by both *sql.DB and *sql.Conn, letting schemaVersion run
// either untransacted (the initial fail-closed check) or inside a migration's own
// BEGIN IMMEDIATE (the in-transaction re-read).
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// schemaVersion reads kv.schema_version, returning 0 for a fresh database where the
// kv table (or the row) does not exist yet.
func schemaVersion(ctx context.Context, q queryRower) (int, error) {
	var exists int
	err := q.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='kv'").Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var v string
	err = q.QueryRowContext(ctx, "SELECT value FROM kv WHERE key='schema_version'").Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid schema_version %q: %w", v, err)
	}
	return n, nil
}

// migrate applies embedded migrations in order, tracked by the existing kv table's
// schema_version row. A database whose version exceeds the newest embedded migration
// fails Open() closed rather than risk silently opening a future-versioned database.
func migrate(ctx context.Context, db *sql.DB) error {
	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	newest := 0
	for _, m := range migs {
		if m.version > newest {
			newest = m.version
		}
	}
	cur, err := schemaVersion(ctx, db)
	if err != nil {
		return err
	}
	if cur > newest {
		return fmt.Errorf("database schema version %d is newer than this binary supports (newest known migration is %d)", cur, newest)
	}
	for _, m := range migs {
		if m.version <= cur {
			continue
		}
		if err = applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("migrate %s: %w", m.name, err)
		}
	}
	return nil
}

// applyMigration runs one migration inside BEGIN IMMEDIATE, re-reading the schema
// version inside the transaction so a concurrently-applied migration (a second
// process on the same file) is a no-op instead of a double application.
func applyMigration(ctx context.Context, db *sql.DB, m migration) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	cur, err := schemaVersion(ctx, conn)
	if err != nil {
		return err
	}
	if cur >= m.version {
		_, err = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	if _, err = conn.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, "INSERT OR REPLACE INTO kv(key,value) VALUES('schema_version',?)", strconv.Itoa(m.version)); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// SetKV persists an internal configuration value.
func (s *Store) SetKV(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, "INSERT OR REPLACE INTO kv(key,value) VALUES(?,?)", key, value)
	return err
}

// GetKV returns an internal configuration value.
func (s *Store) GetKV(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM kv WHERE key=?", key).Scan(&value)
	return value, err
}

// Backup is one indexed archive.
type Backup struct {
	ID        int64     `json:"id"`
	File      string    `json:"file"`
	CreatedAt time.Time `json:"createdAt"`
	SizeBytes int64     `json:"sizeBytes"`
	Trigger   string    `json:"trigger"`
	WorldDay  *int64    `json:"worldDay,omitempty"`
}

// AddBackup indexes an archive and returns its database identity.
func (s *Store) AddBackup(ctx context.Context, b Backup) (Backup, error) {
	var day any
	if b.WorldDay != nil {
		day = *b.WorldDay
	}
	r, err := s.db.ExecContext(ctx, "INSERT INTO backups(file,created_at,size_bytes,trigger,world_day) VALUES(?,?,?,?,?)", b.File, b.CreatedAt.Unix(), b.SizeBytes, b.Trigger, day)
	if err == nil {
		b.ID, _ = r.LastInsertId()
	}
	return b, err
}

// Backups lists archives newest first.
func (s *Store) Backups(ctx context.Context) ([]Backup, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,file,created_at,size_bytes,trigger,world_day FROM backups ORDER BY created_at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Backup{}
	for rows.Next() {
		var b Backup
		var at int64
		var day sql.NullInt64
		if err = rows.Scan(&b.ID, &b.File, &at, &b.SizeBytes, &b.Trigger, &day); err != nil {
			return nil, err
		}
		b.CreatedAt = time.Unix(at, 0).UTC()
		if day.Valid {
			v := day.Int64
			b.WorldDay = &v
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BackupByID returns an indexed archive.
func (s *Store) BackupByID(ctx context.Context, id int64) (Backup, error) {
	var b Backup
	var at int64
	var day sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT id,file,created_at,size_bytes,trigger,world_day FROM backups WHERE id=?", id).Scan(&b.ID, &b.File, &at, &b.SizeBytes, &b.Trigger, &day)
	b.CreatedAt = time.Unix(at, 0).UTC()
	if day.Valid {
		v := day.Int64
		b.WorldDay = &v
	}
	return b, err
}

// DeleteBackupIndex removes an archive index row.
func (s *Store) DeleteBackupIndex(ctx context.Context, id int64) error {
	r, err := s.db.ExecContext(ctx, "DELETE FROM backups WHERE id=?", id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteBackupFileIndex removes an index row by filename.
func (s *Store) DeleteBackupFileIndex(ctx context.Context, file string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM backups WHERE file=?", file)
	return err
}

// Metric is a stored or aggregated sample.
type Metric struct {
	At               time.Time `json:"at"`
	FPS, FrameTimeMS float64
	Players          int
}

// AddMetric inserts a raw sample.
func (s *Store) AddMetric(ctx context.Context, m Metric) error {
	_, err := s.db.ExecContext(ctx, "INSERT OR REPLACE INTO metrics(ts,fps,frametime_ms,players) VALUES(?,?,?,?)", m.At.Unix(), m.FPS, m.FrameTimeMS, m.Players)
	return err
}

// MaintainMetrics creates completed one-minute rollups and applies retention.
func (s *Store) MaintainMetrics(ctx context.Context, now time.Time) error {
	cut := now.Add(-24 * time.Hour).Unix()
	rollCut := now.Add(-30 * 24 * time.Hour).Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO metrics_rollup(ts,fps_avg,fps_min,fps_max,frametime_avg,frametime_min,frametime_max,players_avg,players_min,players_max)
SELECT (ts/60)*60,AVG(fps),MIN(fps),MAX(fps),AVG(frametime_ms),MIN(frametime_ms),MAX(frametime_ms),AVG(players),MIN(players),MAX(players) FROM metrics WHERE ts < ? GROUP BY ts/60`, now.Truncate(time.Minute).Unix())
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM metrics WHERE ts < ?", cut); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM metrics_rollup WHERE ts < ?", rollCut); err != nil {
		return err
	}
	return tx.Commit()
}

// MetricsHistory returns raw samples for windows through 24h and minute averages beyond.
func (s *Store) MetricsHistory(ctx context.Context, since time.Time, rollup bool) ([]Metric, error) {
	q := "SELECT ts,fps,frametime_ms,players FROM metrics WHERE ts>=? ORDER BY ts"
	args := []any{since.Unix()}
	if rollup {
		q = `SELECT ts,fps,frametime,players FROM (
SELECT ts,fps_avg AS fps,frametime_avg AS frametime,CAST(players_avg AS INTEGER) AS players FROM metrics_rollup WHERE ts>=? AND ts<?
UNION ALL SELECT ts,fps,frametime_ms,players FROM metrics WHERE ts>=?) ORDER BY ts`
		cut := time.Now().Add(-24 * time.Hour).Unix()
		args = []any{since.Unix(), cut, cut}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Metric
	for rows.Next() {
		var m Metric
		var ts int64
		if err = rows.Scan(&ts, &m.FPS, &m.FrameTimeMS, &m.Players); err != nil {
			return nil, err
		}
		m.At = time.Unix(ts, 0).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// Player is the merged REST/save representation.
type Player struct {
	UID                     string  `json:"uid"`
	SteamID                 string  `json:"steamId"`
	Name                    string  `json:"name"`
	AccountName             string  `json:"accountName"`
	Online                  bool    `json:"online"`
	Level                   int     `json:"level"`
	GuildID                 string  `json:"guildId"`
	GuildName               string  `json:"guildName"`
	Ping                    float64 `json:"ping"`
	X, Y                    *float64
	FirstSeenAt, LastSeenAt time.Time
	PlaytimeSec             int64           `json:"playtimeSec"`
	CaptureTotal            *int64          `json:"captureTotal,omitempty"`
	UniquePalsCaptured      *int            `json:"uniquePalsCaptured,omitempty"`
	PaldeckUnlocked         *int            `json:"paldeckUnlocked,omitempty"`
	Banned                  bool            `json:"banned"`
	Whitelisted             bool            `json:"whitelisted"`
	Raw                     json.RawMessage `json:"-"`
}

// NormalizeUID normalizes GUID identity for joins across REST and save data.
func NormalizeUID(v string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(v), "-", ""))
}

// ResolveUID returns an existing full UID when value is its unique normalized prefix.
func (s *Store) ResolveUID(ctx context.Context, value string) string {
	uid := NormalizeUID(value)
	if uid == "" {
		return ""
	}
	var existing string
	if err := s.db.QueryRowContext(ctx, "SELECT uid FROM players WHERE uid LIKE ? OR ? LIKE uid || '%' ORDER BY length(uid) DESC LIMIT 1", uid+"%", uid).Scan(&existing); err == nil {
		return existing
	}
	return uid
}

// UpsertLivePlayer records a current REST player without erasing save-derived fields.
func (s *Store) UpsertLivePlayer(ctx context.Context, p Player, now time.Time) error {
	p.UID = NormalizeUID(p.UID)
	raw := p.Raw
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO players(uid,steam_id,name,account_name,first_seen,last_seen,level,ping,location_x,location_y,raw_json,whitelisted) VALUES(?,?,?,?,?,?,?,?,?,?,?,EXISTS(SELECT 1 FROM whitelist WHERE steam_id=?)) ON CONFLICT(uid) DO UPDATE SET steam_id=excluded.steam_id,name=excluded.name,account_name=excluded.account_name,last_seen=excluded.last_seen,level=MAX(players.level,excluded.level),ping=excluded.ping,location_x=COALESCE(excluded.location_x,players.location_x),location_y=COALESCE(excluded.location_y,players.location_y),raw_json=excluded.raw_json,whitelisted=excluded.whitelisted`, p.UID, p.SteamID, p.Name, p.AccountName, now.Unix(), now.Unix(), p.Level, p.Ping, p.X, p.Y, string(raw), p.SteamID)
	return err
}

// SetPlayerFlags updates moderation flags.
func (s *Store) SetPlayerFlags(ctx context.Context, uid string, banned, whitelisted *bool) error {
	if banned != nil {
		_, err := s.db.ExecContext(ctx, "UPDATE players SET banned=? WHERE uid=?", *banned, NormalizeUID(uid))
		if err != nil {
			return err
		}
	}
	if whitelisted != nil {
		_, err := s.db.ExecContext(ctx, "UPDATE players SET whitelisted=? WHERE uid=?", *whitelisted, NormalizeUID(uid))
		return err
	}
	return nil
}

// PlayerByUID finds one player.
func (s *Store) PlayerByUID(ctx context.Context, uid string) (Player, error) {
	rows, err := s.listPlayers(ctx, " WHERE uid=?", s.ResolveUID(ctx, uid))
	if err != nil {
		return Player{}, err
	}
	if len(rows) == 0 {
		return Player{}, sql.ErrNoRows
	}
	return rows[0], nil
}

// Players lists merged player records and marks supplied UIDs online.
func (s *Store) Players(ctx context.Context, online map[string]bool) ([]Player, error) {
	p, err := s.listPlayers(ctx, "")
	if err == nil {
		for i := range p {
			p[i].Online = online[p[i].UID]
		}
	}
	return p, err
}

// playerColumns is the column list shared by every players query below; keeping it in one
// place means listPlayers, PlayersPage, and scanPlayerRows can never drift out of sync.
// playtime_sec stores completed-session time. The projection also includes the
// elapsed portion of an open session without mutating or closing that session;
// otherwise a player's first long session misleadingly reads as zero until leave.
const playerColumns = "uid,steam_id,name,account_name,level,guild_id,guild_name,ping,location_x,location_y,first_seen,last_seen,playtime_sec+COALESCE((SELECT MAX(0,CAST(strftime('%s','now') AS INTEGER)-join_at) FROM sessions WHERE player_uid=players.uid AND leave_at IS NULL),0) AS effective_playtime_sec,banned,whitelisted,raw_json,capture_total,unique_pals_captured,paldeck_unlocked"

func (s *Store) listPlayers(ctx context.Context, where string, args ...any) ([]Player, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+playerColumns+" FROM players"+where+" ORDER BY name", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlayerRows(rows)
}

// PlayersPage returns players ordered by uid ascending for keyset pagination (integration
// API spec §7.1), one page at a time: WHERE uid > after ORDER BY uid ASC LIMIT limit. When
// onlyUIDs is non-nil it additionally restricts results to that uid set (the ?online=true
// snapshot predicate); an empty, non-nil slice matches nothing and issues no query, since
// SQLite rejects an empty IN() list — callers relying on the zero-online short-circuit
// should prefer skipping this call entirely, but the guard here keeps the method safe either way.
func (s *Store) PlayersPage(ctx context.Context, after string, limit int, onlyUIDs []string) ([]Player, error) {
	if onlyUIDs != nil && len(onlyUIDs) == 0 {
		return []Player{}, nil
	}
	q := "SELECT " + playerColumns + " FROM players WHERE uid > ?"
	args := []any{after}
	if onlyUIDs != nil {
		q += " AND uid IN (" + placeholders(len(onlyUIDs)) + ")"
		for _, uid := range onlyUIDs {
			args = append(args, uid)
		}
	}
	q += " ORDER BY uid ASC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanPlayerRows(rows)
	if out == nil {
		out = []Player{}
	}
	return out, err
}

func scanPlayerRows(rows *sql.Rows) ([]Player, error) {
	var out []Player
	for rows.Next() {
		var p Player
		var x, y sql.NullFloat64
		var first, last sql.NullInt64
		var captureTotal, uniqueCaptured, unlocked sql.NullInt64
		var raw string
		if err := rows.Scan(&p.UID, &p.SteamID, &p.Name, &p.AccountName, &p.Level, &p.GuildID, &p.GuildName, &p.Ping, &x, &y, &first, &last, &p.PlaytimeSec, &p.Banned, &p.Whitelisted, &raw, &captureTotal, &uniqueCaptured, &unlocked); err != nil {
			return nil, err
		}
		if x.Valid {
			p.X = &x.Float64
		}
		if y.Valid {
			p.Y = &y.Float64
		}
		if first.Valid {
			p.FirstSeenAt = time.Unix(first.Int64, 0).UTC()
		}
		if last.Valid {
			p.LastSeenAt = time.Unix(last.Int64, 0).UTC()
		}
		p.Raw = json.RawMessage(raw)
		if captureTotal.Valid {
			p.CaptureTotal = &captureTotal.Int64
		}
		if uniqueCaptured.Valid {
			v := int(uniqueCaptured.Int64)
			p.UniquePalsCaptured = &v
		}
		if unlocked.Valid {
			v := int(unlocked.Int64)
			p.PaldeckUnlocked = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// placeholders returns n comma-separated "?" SQL placeholders.
func placeholders(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('?')
	}
	return b.String()
}

// StartSession opens a session unless one is already open.
func (s *Store) StartSession(ctx context.Context, uid string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO sessions(player_uid,join_at) SELECT ?,? WHERE NOT EXISTS(SELECT 1 FROM sessions WHERE player_uid=? AND leave_at IS NULL)", NormalizeUID(uid), at.Unix(), NormalizeUID(uid))
	return err
}

// EndSession closes the current session and accrues playtime.
func (s *Store) EndSession(ctx context.Context, uid string, at time.Time) error {
	uid = NormalizeUID(uid)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE sessions SET leave_at=? WHERE player_uid=? AND leave_at IS NULL", at.Unix(), uid); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE players SET playtime_sec=playtime_sec+MAX(0,?-COALESCE((SELECT MAX(join_at) FROM sessions WHERE player_uid=?),?)) WHERE uid=?", at.Unix(), uid, at.Unix(), uid); err != nil {
		return err
	}
	return tx.Commit()
}

// Session is a player connection interval.
type Session struct {
	JoinAt  time.Time  `json:"joinAt"`
	LeaveAt *time.Time `json:"leaveAt"`
}

// ObservedSession is one bounded, panel-observed connection interval. DurationSec is derived
// at read time so an open session remains current without mutating the stored row.
type ObservedSession struct {
	JoinedAt    time.Time  `json:"joinedAt"`
	LeftAt      *time.Time `json:"leftAt"`
	DurationSec int64      `json:"durationSec"`
}

// ActivityWindow summarizes sessions overlapping a rolling window. Both duration and count are
// clamped to that window, so a session spanning the boundary contributes only its overlap.
type ActivityWindow struct {
	DurationSec  int64 `json:"durationSec"`
	SessionCount int   `json:"sessionCount"`
}

type PlayerActivityWindows struct {
	Last24Hours ActivityWindow `json:"last24Hours"`
	Last7Days   ActivityWindow `json:"last7Days"`
	Last30Days  ActivityWindow `json:"last30Days"`
}

// PlayerActivity is intentionally observation-scoped. It describes only join/leave intervals
// seen by this panel and must not be presented as lifetime Palworld playtime.
type PlayerActivity struct {
	Coverage                string                `json:"coverage"`
	TrackingSince           *time.Time            `json:"trackingSince"`
	CurrentSession          *ObservedSession      `json:"currentSession"`
	Windows                 PlayerActivityWindows `json:"windows"`
	RecentSessions          []ObservedSession     `json:"recentSessions"`
	RecentSessionsTruncated bool                  `json:"recentSessionsTruncated"`
}

// Sessions lists a player's recent sessions.
func (s *Store) Sessions(ctx context.Context, uid string) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT join_at,leave_at FROM sessions WHERE player_uid=? ORDER BY join_at DESC", NormalizeUID(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var a int64
		var b sql.NullInt64
		if err = rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		v := Session{JoinAt: time.Unix(a, 0).UTC()}
		if b.Valid {
			t := time.Unix(b.Int64, 0).UTC()
			v.LeaveAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// PlayerActivity returns exact rolling aggregates plus a bounded recent sample from the existing
// sessions table. now is supplied by the caller to make clamping deterministic and testable.
func (s *Store) PlayerActivity(ctx context.Context, uid string, now time.Time, recentLimit int) (PlayerActivity, error) {
	uid = NormalizeUID(uid)
	now = now.UTC().Truncate(time.Second)
	if recentLimit < 1 || recentLimit > 100 {
		recentLimit = 20
	}
	activity := PlayerActivity{
		Coverage:       "panel_observed_sessions",
		RecentSessions: []ObservedSession{},
	}

	var first, current sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MIN(join_at),MAX(CASE WHEN leave_at IS NULL THEN join_at END)
FROM sessions WHERE player_uid=? AND join_at<=?`, uid, now.Unix()).Scan(&first, &current)
	if err != nil {
		return PlayerActivity{}, err
	}
	if first.Valid {
		value := time.Unix(first.Int64, 0).UTC()
		activity.TrackingSince = &value
	}
	if current.Valid {
		joined := time.Unix(current.Int64, 0).UTC()
		activity.CurrentSession = &ObservedSession{
			JoinedAt: joined, DurationSec: max(0, now.Unix()-current.Int64),
		}
	}

	if activity.Windows.Last24Hours, err = s.playerActivityWindow(ctx, uid, now, 24*time.Hour); err != nil {
		return PlayerActivity{}, err
	}
	if activity.Windows.Last7Days, err = s.playerActivityWindow(ctx, uid, now, 7*24*time.Hour); err != nil {
		return PlayerActivity{}, err
	}
	if activity.Windows.Last30Days, err = s.playerActivityWindow(ctx, uid, now, 30*24*time.Hour); err != nil {
		return PlayerActivity{}, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT join_at,leave_at FROM sessions
WHERE player_uid=? AND join_at<=? ORDER BY join_at DESC,id DESC LIMIT ?`, uid, now.Unix(), recentLimit+1)
	if err != nil {
		return PlayerActivity{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var joined int64
		var left sql.NullInt64
		if err = rows.Scan(&joined, &left); err != nil {
			return PlayerActivity{}, err
		}
		if len(activity.RecentSessions) == recentLimit {
			activity.RecentSessionsTruncated = true
			break
		}
		end := now.Unix()
		var leftAt *time.Time
		if left.Valid {
			end = min(left.Int64, end)
			value := time.Unix(left.Int64, 0).UTC()
			leftAt = &value
		}
		activity.RecentSessions = append(activity.RecentSessions, ObservedSession{
			JoinedAt: time.Unix(joined, 0).UTC(), LeftAt: leftAt, DurationSec: max(0, end-joined),
		})
	}
	return activity, rows.Err()
}

func (s *Store) playerActivityWindow(ctx context.Context, uid string, now time.Time, window time.Duration) (ActivityWindow, error) {
	nowUnix := now.Unix()
	since := now.Add(-window).Unix()
	var result ActivityWindow
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(
MAX(0,MIN(COALESCE(leave_at,?),?)-MAX(join_at,?))
),0) FROM sessions
WHERE player_uid=? AND join_at<? AND COALESCE(leave_at,?)>?`,
		nowUnix, nowUnix, since, uid, nowUnix, nowUnix, since,
	).Scan(&result.SessionCount, &result.DurationSec)
	return result, err
}

// Event is an audit or observed activity item.
type Event struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
	Meta    any       `json:"meta"`
}

// AddEvent stores an event.
func (s *Store) AddEvent(ctx context.Context, e Event) error {
	b, _ := json.Marshal(e.Meta)
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO events(at,kind,message,meta_json) VALUES(?,?,?,?)", e.At.Unix(), e.Kind, e.Message, string(b))
	return err
}

// Events returns newest events, optionally filtered by kind.
func (s *Store) Events(ctx context.Context, limit int, kind string) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q := "SELECT at,kind,message,meta_json FROM events"
	args := []any{}
	if kind != "" {
		q += " WHERE kind=?"
		args = append(args, kind)
	}
	q += " ORDER BY at DESC,id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var at int64
		var raw string
		if err = rows.Scan(&at, &e.Kind, &e.Message, &raw); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0).UTC()
		var meta any
		if json.Unmarshal([]byte(raw), &meta) == nil {
			e.Meta = meta
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ConsoleEntry is one panel-originated RCON invocation.
type ConsoleEntry struct {
	At      time.Time `json:"at"`
	User    string    `json:"user"`
	Command string    `json:"command"`
	Output  string    `json:"output"`
	IsError bool      `json:"isError"`
}

// AddConsole stores a console invocation.
func (s *Store) AddConsole(ctx context.Context, e ConsoleEntry) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO console_log(at,user,command,output,is_error) VALUES(?,?,?,?,?)", e.At.Unix(), e.User, e.Command, e.Output, e.IsError)
	return err
}

// ConsoleLog returns recent console history.
func (s *Store) ConsoleLog(ctx context.Context, limit int) ([]ConsoleEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, "SELECT at,user,command,output,is_error FROM console_log ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConsoleEntry{}
	for rows.Next() {
		var e ConsoleEntry
		var at int64
		if err = rows.Scan(&at, &e.User, &e.Command, &e.Output, &e.IsError); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// SavedCommand is a reusable console command.
type SavedCommand struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
}

// SavedCommands lists saved commands.
func (s *Store) SavedCommands(ctx context.Context) ([]SavedCommand, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,command FROM saved_commands ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedCommand{}
	for rows.Next() {
		var v SavedCommand
		if err = rows.Scan(&v.ID, &v.Name, &v.Command); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveCommand creates a command.
func (s *Store) SaveCommand(ctx context.Context, v SavedCommand) (SavedCommand, error) {
	r, err := s.db.ExecContext(ctx, "INSERT INTO saved_commands(name,command) VALUES(?,?)", v.Name, v.Command)
	if err == nil {
		v.ID, _ = r.LastInsertId()
	}
	return v, err
}

// DeleteCommand deletes a saved command.
func (s *Store) DeleteCommand(ctx context.Context, id int64) error {
	r, err := s.db.ExecContext(ctx, "DELETE FROM saved_commands WHERE id=?", id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// WorldState is save parser status exposed by /world.
type WorldState struct {
	Day                                 int64     `json:"day"`
	LastParseAt                         time.Time `json:"lastParseAt"`
	ParseDurationMS                     int64     `json:"parseDurationMs"`
	Players, Pals, Guilds, SkippedProps int
	FormatDrift                         bool `json:"formatDrift"`
}

// ReplaceWorld atomically replaces save-derived entities and parse status.
func (s *Store) ReplaceWorld(ctx context.Context, w *sav.World, at time.Time, d time.Duration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Palworld clears OwnerPlayerUId while a Pal is deployed to a guild base.
	// Preserve the latest authoritative player ownership previously observed for
	// the same stable instance ID before replacing the snapshot. A later explicit
	// owner or unique personal-container match always supersedes this value.
	lastOwners, err := loadLastPalOwners(ctx, tx)
	if err != nil {
		return err
	}
	for _, q := range []string{"DELETE FROM pals", "DELETE FROM guild_members", "DELETE FROM guilds", "DELETE FROM bases"} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	players := make(map[string]sav.Player, len(w.Players))
	playerOrder := make([]string, 0, len(w.Players))
	for _, incoming := range w.Players {
		uid := NormalizeUID(incoming.UID)
		incoming.UID = uid
		incoming.GuildID = NormalizeUID(incoming.GuildID)
		incoming.OtomoContainerID = NormalizeUID(incoming.OtomoContainerID)
		incoming.PalStorageContainerID = NormalizeUID(incoming.PalStorageContainerID)
		merged, exists := players[uid]
		if !exists {
			playerOrder = append(playerOrder, uid)
		}
		if incoming.Nickname != "" {
			merged.Nickname = incoming.Nickname
		}
		if incoming.Level > merged.Level {
			merged.Level = incoming.Level
		}
		if incoming.GuildID != "" {
			merged.GuildID = incoming.GuildID
		}
		if incoming.Location != nil {
			merged.Location = incoming.Location
		}
		if incoming.OtomoContainerID != "" {
			merged.OtomoContainerID = incoming.OtomoContainerID
		}
		if incoming.PalStorageContainerID != "" {
			merged.PalStorageContainerID = incoming.PalStorageContainerID
		}
		if incoming.CaptureTotal != nil {
			merged.CaptureTotal = incoming.CaptureTotal
		}
		if incoming.UniquePalsCaptured != nil {
			merged.UniquePalsCaptured = incoming.UniquePalsCaptured
		}
		if incoming.PaldeckUnlocked != nil {
			merged.PaldeckUnlocked = incoming.PaldeckUnlocked
		}
		if incoming.PalCaptureCounts != nil {
			merged.PalCaptureCounts = incoming.PalCaptureCounts
			merged.PalCaptureCountsTruncated = incoming.PalCaptureCountsTruncated
		}
		if incoming.PaldeckUnlockFlags != nil {
			merged.PaldeckUnlockFlags = incoming.PaldeckUnlockFlags
			merged.PaldeckUnlockFlagsTruncated = incoming.PaldeckUnlockFlagsTruncated
		}
		merged.UID = uid
		players[uid] = merged
	}
	for _, uid := range playerOrder {
		p := players[uid]
		var x, y any
		if p.Location != nil {
			x = p.Location.X
			y = p.Location.Y
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO players(uid,name,level,guild_id,location_x,location_y,first_seen,last_seen,capture_total,unique_pals_captured,paldeck_unlocked) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(uid) DO UPDATE SET name=CASE WHEN excluded.name='' THEN players.name ELSE excluded.name END,level=MAX(players.level,excluded.level),guild_id=CASE WHEN excluded.guild_id='' THEN players.guild_id ELSE excluded.guild_id END,location_x=COALESCE(excluded.location_x,players.location_x),location_y=COALESCE(excluded.location_y,players.location_y),capture_total=COALESCE(excluded.capture_total,players.capture_total),unique_pals_captured=COALESCE(excluded.unique_pals_captured,players.unique_pals_captured),paldeck_unlocked=COALESCE(excluded.paldeck_unlocked,players.paldeck_unlocked)`, uid, p.Nickname, p.Level, p.GuildID, x, y, at.Unix(), at.Unix(), p.CaptureTotal, p.UniquePalsCaptured, p.PaldeckUnlocked)
		if err != nil {
			return err
		}
		if err = replacePlayerPaldeck(ctx, tx, p, at); err != nil {
			return err
		}
	}
	containerOwners := indexPersonalContainers(players)
	for _, p := range w.Pals {
		b, _ := json.Marshal(p)
		instanceID := NormalizeUID(p.InstanceID)
		ownerUID := normalizedNonzeroUID(p.OwnerUID)
		ownerSource := "unresolved"
		if players[ownerUID].UID != "" {
			ownerSource = "save"
		}
		containerID := normalizedNonzeroUID(p.ContainerID)
		if candidate, ok := containerOwners[containerID]; ok && !candidate.ambiguous {
			// A party or personal Palbox container is authoritative current
			// possession, including after a direct player-to-player transfer.
			ownerUID = candidate.uid
			ownerSource = "personal_container"
		} else if ownerUID == "" || players[ownerUID].UID == "" {
			// Base/cage containers identify a guild or world object, not the
			// contributing player. Carry the last player owner only for the same
			// stable Pal instance; never guess from guild membership or history.
			if previous := lastOwners[instanceID]; players[previous].UID != "" {
				ownerUID = previous
				ownerSource = "last_observed"
			} else {
				ownerUID = ""
				ownerSource = "unresolved"
			}
		}
		inParty := false
		var partySlot, boxPage, boxSlot any
		if owner, ok := players[ownerUID]; ok && p.SlotIndex >= 0 {
			switch {
			case owner.OtomoContainerID != "" && containerID == owner.OtomoContainerID:
				inParty = true
				partySlot = p.SlotIndex
			case owner.PalStorageContainerID != "" && containerID == owner.PalStorageContainerID:
				boxPage = p.SlotIndex / 30
				boxSlot = p.SlotIndex % 30
			}
		}
		passiveSkillIDs, _ := json.Marshal(p.PassiveSkillIDs)
		equippedSkillIDs, _ := json.Marshal(p.EquippedSkillIDs)
		_, err = tx.ExecContext(ctx, "INSERT INTO pals(instance_id,owner_uid,owner_source,character_id,display_name,level,is_alpha,is_lucky,in_party,party_slot,box_page,box_slot,hp,gender,talent_hp,talent_melee,talent_shot,talent_defense,passive_skill_ids,equipped_skill_ids,base_id,raw_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", instanceID, ownerUID, ownerSource, p.CharacterID, paldeck.Name(p.CharacterID), p.Level, p.IsBoss || paldeck.IsBossID(p.CharacterID), p.IsLucky, inParty, partySlot, boxPage, boxSlot, p.HP, p.Gender, nullableTalent(p.Talents, "Talent_HP"), nullableTalent(p.Talents, "Talent_Melee"), nullableTalent(p.Talents, "Talent_Shot"), nullableTalent(p.Talents, "Talent_Defense"), string(passiveSkillIDs), string(equippedSkillIDs), NormalizeUID(p.BaseID), string(b))
		if err != nil {
			return err
		}
	}
	for _, g := range w.Guilds {
		b, _ := json.Marshal(g)
		gid := NormalizeUID(g.ID)
		_, err = tx.ExecContext(ctx, "INSERT INTO guilds(id,name,admin_uid,raw_json) VALUES(?,?,?,?)", gid, g.Name, NormalizeUID(g.AdminUID), string(b))
		if err != nil {
			return err
		}
		for _, m := range g.Members {
			_, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO guild_members(guild_id,player_uid,name) VALUES(?,?,?)", gid, NormalizeUID(m.UID), m.Name)
			if err != nil {
				return err
			}
		}
		for _, uid := range g.MemberUIDs {
			if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO guild_members(guild_id,player_uid,name) VALUES(?,?,?)", gid, NormalizeUID(uid), ""); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE players SET guild_name=COALESCE((SELECT name FROM guilds WHERE id=players.guild_id),'')`); err != nil {
		return err
	}
	for _, b := range w.Bases {
		var x, y, name any
		if b.Position != nil {
			x = b.Position.X
			y = b.Position.Y
		}
		if b.Name != "" { // unnamed stays NULL, never "" or a synthetic label
			name = b.Name
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO bases(id,guild_id,name,x,y) VALUES(?,?,?,?,?)", NormalizeUID(b.ID), NormalizeUID(b.GuildID), name, x, y)
		if err != nil {
			return err
		}
	}
	drift := len(w.Stats.DecodeFailures) > 0 || w.Stats.SkippedProperties > 0 || w.Stats.SkippedStructs > 0
	_, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO world_state(singleton,day,last_parse_at,parse_duration_ms,players,pals,guilds,skipped_props,format_drift) VALUES(1,?,?,?,?,?,?,?,?)", w.Meta.Day, at.Unix(), d.Milliseconds(), len(players), len(w.Pals), len(w.Guilds), w.Stats.SkippedProperties+w.Stats.SkippedStructs, drift)
	if err != nil {
		return err
	}
	return tx.Commit()
}

type personalContainerOwner struct {
	uid       string
	kind      string
	ambiguous bool
}

func indexPersonalContainers(players map[string]sav.Player) map[string]personalContainerOwner {
	result := make(map[string]personalContainerOwner, len(players)*2)
	add := func(raw, uid, kind string) {
		id := normalizedNonzeroUID(raw)
		if id == "" {
			return
		}
		current, exists := result[id]
		if exists && (current.uid != uid || current.kind != kind) {
			current.ambiguous = true
			result[id] = current
			return
		}
		if !exists {
			result[id] = personalContainerOwner{uid: uid, kind: kind}
		}
	}
	for uid, player := range players {
		add(player.OtomoContainerID, uid, "party")
		add(player.PalStorageContainerID, uid, "box")
	}
	return result
}

func loadLastPalOwners(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT instance_id,owner_uid FROM pals WHERE owner_uid IS NOT NULL AND owner_uid<>''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var instanceID, ownerUID string
		if err = rows.Scan(&instanceID, &ownerUID); err != nil {
			return nil, err
		}
		if ownerUID = normalizedNonzeroUID(ownerUID); ownerUID != "" {
			result[NormalizeUID(instanceID)] = ownerUID
		}
	}
	return result, rows.Err()
}

func normalizedNonzeroUID(value string) string {
	value = NormalizeUID(value)
	if value == "" || strings.Trim(value, "0") == "" {
		return ""
	}
	return value
}

func nullableTalent(talents map[string]int, name string) any {
	value, ok := talents[name]
	if !ok {
		return nil
	}
	return value
}

// WorldState returns the most recent parse status.
func (s *Store) WorldState(ctx context.Context) (WorldState, error) {
	var v WorldState
	var at sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT day,last_parse_at,parse_duration_ms,players,pals,guilds,skipped_props,format_drift FROM world_state WHERE singleton=1").Scan(&v.Day, &at, &v.ParseDurationMS, &v.Players, &v.Pals, &v.Guilds, &v.SkippedProps, &v.FormatDrift)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if at.Valid {
		v.LastParseAt = time.Unix(at.Int64, 0).UTC()
	}
	return v, err
}

// guildListRealFilter restricts a guild-list query to genuine player guilds.
// Palworld's save writes a group record into GroupSaveDataMap for things that are
// not player guilds — a solo player's auto-created organization and other non-guild
// group types — and those decode into guild rows with no base placed and no member
// whose save identity resolves to a known player. Requiring at least one placed base
// AND at least one member with a confirmed player identity drops those empty records
// so every panel consumer of the list (guilds page, players "Guilds" tab, dashboard
// count, map bases) agrees on the same set. The guild detail path deliberately does
// NOT apply this, so a player row can still open its guild even when the guild is
// filtered out of the list.
const guildListRealFilter = `WHERE EXISTS (SELECT 1 FROM bases b WHERE b.guild_id=guilds.id)
	AND EXISTS (SELECT 1 FROM guild_members gm JOIN players p ON p.uid=gm.player_uid WHERE gm.guild_id=guilds.id)`

// GuildJSON returns API-ready guild objects including members and bases. Only guilds
// with at least one placed base and one confirmed player member are listed; see
// guildListRealFilter.
func (s *Store) GuildJSON(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,admin_uid FROM guilds "+guildListRealFilter+" ORDER BY name")
	if err != nil {
		return nil, err
	}
	type guild struct{ id, name, admin string }
	var guilds []guild
	for rows.Next() {
		var g guild
		if err = rows.Scan(&g.id, &g.name, &g.admin); err != nil {
			return nil, err
		}
		guilds = append(guilds, g)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, g := range guilds {
		members := []map[string]any{}
		mr, e := s.db.QueryContext(ctx, "SELECT player_uid,name FROM guild_members WHERE guild_id=?", g.id)
		if e != nil {
			return nil, e
		}
		for mr.Next() {
			var u, n string
			if e = mr.Scan(&u, &n); e != nil {
				mr.Close()
				return nil, e
			}
			members = append(members, map[string]any{"uid": u, "name": n})
		}
		mr.Close()
		bases := []map[string]any{}
		br, e := s.db.QueryContext(ctx, "SELECT id,name,x,y,level FROM bases WHERE guild_id=?", g.id)
		if e != nil {
			return nil, e
		}
		for br.Next() {
			var bid string
			var baseName sql.NullString
			var x, y sql.NullFloat64
			var level int
			if e = br.Scan(&bid, &baseName, &x, &y, &level); e != nil {
				br.Close()
				return nil, e
			}
			var location any // null, not (0,0), when the base transform was never decoded.
			if x.Valid && y.Valid {
				location = map[string]any{"x": x.Float64, "y": y.Float64}
			}
			var name any // null, never "" or a synthetic label, for an unnamed base.
			if baseName.Valid && baseName.String != "" {
				name = baseName.String
			}
			bases = append(bases, map[string]any{"id": bid, "name": name, "location": location, "level": level})
		}
		br.Close()
		out = append(out, map[string]any{"id": g.id, "name": g.name, "adminUid": g.admin, "memberCount": len(members), "members": members, "bases": bases})
	}
	return out, nil
}

// Guild is a save-derived guild with its members and bases, typed for the integration
// surface (spec §4/§6): unlike GuildJSON, which returns map[string]any shaped for the
// session UI and must never be reused on the token surface, this is a genuine allowlist by
// construction.
type Guild struct {
	ID, Name, AdminUID string
	Members            []GuildMember
	Bases              []GuildBase
}

// GuildMember is one guild roster entry.
type GuildMember struct{ UID, Name string }

// GuildBase is one persistent guild-owned base. HasLocation is false when the
// base transform was never decoded (a pre-decoding save); X and Y are then zero
// and must be surfaced as a null location rather than a misleading (0,0).
// Name is empty when the base was never renamed (or the save predates name
// decoding) and must likewise be surfaced as null, never a synthetic label.
type GuildBase struct {
	ID          string
	Name        string
	X, Y        float64
	HasLocation bool
	Level       int
}

// Guilds returns every guild with its members and bases, typed (the integration-surface
// counterpart to GuildJSON).
func (s *Store) Guilds(ctx context.Context) ([]Guild, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,admin_uid FROM guilds ORDER BY name")
	if err != nil {
		return nil, err
	}
	var guilds []Guild
	for rows.Next() {
		var g Guild
		if err = rows.Scan(&g.ID, &g.Name, &g.AdminUID); err != nil {
			rows.Close()
			return nil, err
		}
		guilds = append(guilds, g)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	out := make([]Guild, 0, len(guilds))
	for _, g := range guilds {
		mr, e := s.db.QueryContext(ctx, "SELECT player_uid,name FROM guild_members WHERE guild_id=?", g.ID)
		if e != nil {
			return nil, e
		}
		for mr.Next() {
			var m GuildMember
			if e = mr.Scan(&m.UID, &m.Name); e != nil {
				mr.Close()
				return nil, e
			}
			g.Members = append(g.Members, m)
		}
		if e = mr.Close(); e != nil {
			return nil, e
		}
		br, e := s.db.QueryContext(ctx, "SELECT id,name,x,y,level FROM bases WHERE guild_id=?", g.ID)
		if e != nil {
			return nil, e
		}
		for br.Next() {
			var b GuildBase
			var name sql.NullString
			var x, y sql.NullFloat64
			if e = br.Scan(&b.ID, &name, &x, &y, &b.Level); e != nil {
				br.Close()
				return nil, e
			}
			b.Name = name.String
			b.X, b.Y = x.Float64, y.Float64
			b.HasLocation = x.Valid && y.Valid
			g.Bases = append(g.Bases, b)
		}
		if e = br.Close(); e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, nil
}

// Pal is one save-derived pal, typed for the integration surface (spec §4/§6): no raw_json,
// no owner (added by PalWithOwner for the bulk /pals endpoint).
type Pal struct {
	InstanceID, CharacterID, DisplayName string
	Level                                int
	IsAlpha, IsLucky                     bool
	InParty                              bool
	PartySlot, BoxPage, BoxSlot          *int
	HP                                   *float64
	Gender                               string
	TalentHP, TalentMelee                *int
	TalentShot, TalentDefense            *int
	PassiveSkillIDs, EquippedSkillIDs    []string
	BaseID                               string
}

// PalWithOwner is one bulk-paginated pal row with its owner uid/name joined in, so the
// integration /pals endpoint avoids an N+1 call per player.
type PalWithOwner struct {
	Pal
	OwnerUID, OwnerName string
	OwnerSource         string
	OwnerResolved       bool
}

// PalExplorerQuery contains the viewer-safe filters used by the authenticated panel's
// server-wide Pal explorer. The server validates the enum values before they reach the store;
// levels are pointers so zero remains a meaningful bound.
type PalExplorerQuery struct {
	After       string
	Limit       int
	Search      string
	OwnerSource string
	Placement   string
	Specimen    string
	MinLevel    *int
	MaxLevel    *int
}

// LivePalIndex returns the save-derived identity and ownership rows needed to reconcile one
// Game Data API snapshot. The caller performs an exact instance-id join; this deliberately
// avoids location/name heuristics and loads the table only once per background poll.
func (s *Store) LivePalIndex(ctx context.Context) (map[string]PalWithOwner, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.instance_id,p.character_id,p.display_name,p.level,p.is_alpha,p.is_lucky,p.base_id,p.owner_uid,COALESCE(pl.name,''),p.owner_source,pl.uid IS NOT NULL
FROM pals p LEFT JOIN players pl ON pl.uid=p.owner_uid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]PalWithOwner)
	for rows.Next() {
		var p PalWithOwner
		if err = rows.Scan(&p.InstanceID, &p.CharacterID, &p.DisplayName, &p.Level, &p.IsAlpha, &p.IsLucky, &p.BaseID, &p.OwnerUID, &p.OwnerName, &p.OwnerSource, &p.OwnerResolved); err != nil {
			return nil, err
		}
		out[strings.ToLower(p.InstanceID)] = p
	}
	return out, rows.Err()
}

// GameDataActivity is one aggregate-only live-world sample. It intentionally persists no
// actor identifiers, names, guilds, health, or locations.
type GameDataActivity struct {
	At            time.Time `json:"at"`
	FPS           float64   `json:"fps"`
	FPSAvg        float64   `json:"fpsAvg"`
	Players       int       `json:"players"`
	BasePals      int       `json:"basePals"`
	Linked        int       `json:"linkedBasePals"`
	Working       int       `json:"working"`
	Transporting  int       `json:"transporting"`
	Eating        int       `json:"eating"`
	Sleeping      int       `json:"sleeping"`
	Idle          int       `json:"idle"`
	Inactive      int       `json:"inactive"`
	Combat        int       `json:"combat"`
	Incapacitated int       `json:"incapacitated"`
	Moving        int       `json:"moving"`
	Unknown       int       `json:"unknown"`
}

// AddGameDataActivity records one aggregate snapshot and bounds retention to 30 days.
func (s *Store) AddGameDataActivity(ctx context.Context, m GameDataActivity) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO game_data_activity(at,fps,fps_avg,players,base_pals,linked_base_pals,working,transporting,eating,sleeping,idle,inactive,combat,incapacitated,moving,unknown)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, m.At.Unix(), m.FPS, m.FPSAvg, m.Players, m.BasePals, m.Linked, m.Working, m.Transporting, m.Eating, m.Sleeping, m.Idle, m.Inactive, m.Combat, m.Incapacitated, m.Moving, m.Unknown)
	if err != nil {
		return err
	}
	// Retention cleanup is cheap but does not need to run on every 15–30 second
	// poll. The first accepted sample after a process start prunes immediately;
	// later samples prune at most hourly.
	s.activityPruneMu.Lock()
	prune := s.activityPruned.IsZero() || m.At.Before(s.activityPruned) || m.At.Sub(s.activityPruned) >= time.Hour
	if prune {
		s.activityPruned = m.At
	}
	s.activityPruneMu.Unlock()
	if !prune {
		return nil
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM game_data_activity WHERE at < ?", m.At.Add(-30*24*time.Hour).Unix())
	return err
}

// GameDataActivityHistory returns the newest sample in each time bucket for
// operator diagnostics. The hard limit bounds both database work and response
// size even if a caller or future poll cadence is misconfigured.
func (s *Store) GameDataActivityHistory(ctx context.Context, since time.Time, bucket time.Duration) ([]GameDataActivity, error) {
	bucketSeconds := int64(bucket / time.Second)
	if bucketSeconds < 1 {
		bucketSeconds = 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.at,a.fps,a.fps_avg,a.players,a.base_pals,a.linked_base_pals,a.working,a.transporting,a.eating,a.sleeping,a.idle,a.inactive,a.combat,a.incapacitated,a.moving,a.unknown
FROM game_data_activity a
JOIN (
  SELECT MAX(at) AS at
  FROM game_data_activity
  WHERE at >= ?
  GROUP BY at / ?
  ORDER BY at DESC
  LIMIT 500
) sampled ON sampled.at = a.at
ORDER BY a.at`, since.Unix(), bucketSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameDataActivity{}
	for rows.Next() {
		var m GameDataActivity
		var unix int64
		if err = rows.Scan(&unix, &m.FPS, &m.FPSAvg, &m.Players, &m.BasePals, &m.Linked, &m.Working, &m.Transporting, &m.Eating, &m.Sleeping, &m.Idle, &m.Inactive, &m.Combat, &m.Incapacitated, &m.Moving, &m.Unknown); err != nil {
			return nil, err
		}
		m.At = time.Unix(unix, 0).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// PalsTyped returns one player's save-derived pals, typed (the integration-surface
// counterpart to Pals, which returns map[string]any for the session UI).
func (s *Store) PalsTyped(ctx context.Context, uid string) ([]Pal, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT instance_id,character_id,display_name,level,is_alpha,is_lucky,in_party,party_slot,box_page,box_slot,hp,gender,talent_hp,talent_melee,talent_shot,talent_defense,passive_skill_ids,equipped_skill_ids,base_id FROM pals WHERE owner_uid=?", NormalizeUID(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Pal{}
	for rows.Next() {
		var p Pal
		var passiveJSON, equippedJSON string
		if err = rows.Scan(&p.InstanceID, &p.CharacterID, &p.DisplayName, &p.Level, &p.IsAlpha, &p.IsLucky, &p.InParty, &p.PartySlot, &p.BoxPage, &p.BoxSlot, &p.HP, &p.Gender, &p.TalentHP, &p.TalentMelee, &p.TalentShot, &p.TalentDefense, &passiveJSON, &equippedJSON, &p.BaseID); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(passiveJSON), &p.PassiveSkillIDs)
		_ = json.Unmarshal([]byte(equippedJSON), &p.EquippedSkillIDs)
		out = append(out, p)
	}
	return out, rows.Err()
}

// PalsPage returns pals ordered by instance_id ascending for keyset pagination (spec §7.1),
// with owner uid/name left-joined from players (owner name is empty when the owner row has
// no name, which the LEFT JOIN's COALESCE also covers if the owner row is somehow absent).
func (s *Store) PalsPage(ctx context.Context, after string, limit int) ([]PalWithOwner, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.instance_id,p.character_id,p.display_name,p.level,p.is_alpha,p.is_lucky,p.in_party,p.party_slot,p.box_page,p.box_slot,p.hp,p.gender,p.talent_hp,p.talent_melee,p.talent_shot,p.talent_defense,p.passive_skill_ids,p.equipped_skill_ids,p.base_id,p.owner_uid,COALESCE(pl.name,''),p.owner_source,pl.uid IS NOT NULL
FROM pals p LEFT JOIN players pl ON pl.uid=p.owner_uid
WHERE p.instance_id > ? ORDER BY p.instance_id ASC LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PalWithOwner{}
	for rows.Next() {
		var p PalWithOwner
		var passiveJSON, equippedJSON string
		if err = rows.Scan(&p.InstanceID, &p.CharacterID, &p.DisplayName, &p.Level, &p.IsAlpha, &p.IsLucky, &p.InParty, &p.PartySlot, &p.BoxPage, &p.BoxSlot, &p.HP, &p.Gender, &p.TalentHP, &p.TalentMelee, &p.TalentShot, &p.TalentDefense, &passiveJSON, &equippedJSON, &p.BaseID, &p.OwnerUID, &p.OwnerName, &p.OwnerSource, &p.OwnerResolved); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(passiveJSON), &p.PassiveSkillIDs)
		_ = json.Unmarshal([]byte(equippedJSON), &p.EquippedSkillIDs)
		out = append(out, p)
	}
	return out, rows.Err()
}

// PalsExplorerPage returns a stable, keyset-paginated slice of the server roster after applying
// filters in SQLite. Filtering before pagination is important: a client-side filter over one page
// would silently omit matching Pals later in the roster.
func (s *Store) PalsExplorerPage(ctx context.Context, filter PalExplorerQuery) ([]PalWithOwner, error) {
	const selectPals = `SELECT p.instance_id,p.character_id,p.display_name,p.level,p.is_alpha,p.is_lucky,p.in_party,p.party_slot,p.box_page,p.box_slot,p.hp,p.gender,p.talent_hp,p.talent_melee,p.talent_shot,p.talent_defense,p.passive_skill_ids,p.equipped_skill_ids,p.base_id,p.owner_uid,COALESCE(pl.name,''),p.owner_source,pl.uid IS NOT NULL
FROM pals p LEFT JOIN players pl ON pl.uid=p.owner_uid`
	var query strings.Builder
	query.WriteString(selectPals)
	query.WriteString(" WHERE p.instance_id > ?")
	args := []any{filter.After}

	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := "%" + escapeSQLLike(strings.ToLower(search)) + "%"
		query.WriteString(` AND (LOWER(p.display_name) LIKE ? ESCAPE '\' OR LOWER(p.character_id) LIKE ? ESCAPE '\' OR LOWER(COALESCE(pl.name,'')) LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern, pattern)
	}
	if filter.OwnerSource != "" {
		query.WriteString(" AND p.owner_source = ?")
		args = append(args, filter.OwnerSource)
	}
	switch filter.Placement {
	case "party":
		query.WriteString(" AND p.in_party = 1")
	case "box":
		query.WriteString(" AND p.in_party = 0 AND p.box_page IS NOT NULL")
	case "base":
		query.WriteString(" AND p.in_party = 0 AND p.box_page IS NULL AND p.base_id <> ''")
	case "unknown":
		query.WriteString(" AND p.in_party = 0 AND p.box_page IS NULL AND p.base_id = ''")
	}
	switch filter.Specimen {
	case "standard":
		query.WriteString(` AND p.is_alpha = 0 AND p.is_lucky = 0 AND LOWER(p.character_id) NOT LIKE 'boss\_%' ESCAPE '\'`)
	case "alpha":
		query.WriteString(` AND p.is_alpha = 1 AND LOWER(p.character_id) NOT LIKE 'boss\_%' ESCAPE '\'`)
	case "lucky":
		query.WriteString(" AND p.is_lucky = 1")
	case "boss":
		query.WriteString(` AND LOWER(p.character_id) LIKE 'boss\_%' ESCAPE '\'`)
	}
	if filter.MinLevel != nil {
		query.WriteString(" AND p.level >= ?")
		args = append(args, *filter.MinLevel)
	}
	if filter.MaxLevel != nil {
		query.WriteString(" AND p.level <= ?")
		args = append(args, *filter.MaxLevel)
	}
	query.WriteString(" ORDER BY p.instance_id ASC LIMIT ?")
	args = append(args, filter.Limit)

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PalWithOwner{}
	for rows.Next() {
		var p PalWithOwner
		var passiveJSON, equippedJSON string
		if err = rows.Scan(&p.InstanceID, &p.CharacterID, &p.DisplayName, &p.Level, &p.IsAlpha, &p.IsLucky, &p.InParty, &p.PartySlot, &p.BoxPage, &p.BoxSlot, &p.HP, &p.Gender, &p.TalentHP, &p.TalentMelee, &p.TalentShot, &p.TalentDefense, &passiveJSON, &equippedJSON, &p.BaseID, &p.OwnerUID, &p.OwnerName, &p.OwnerSource, &p.OwnerResolved); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(passiveJSON), &p.PassiveSkillIDs)
		_ = json.Unmarshal([]byte(equippedJSON), &p.EquippedSkillIDs)
		out = append(out, p)
	}
	return out, rows.Err()
}

func escapeSQLLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// Pals returns a player's save-derived pals.
func (s *Store) Pals(ctx context.Context, uid string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT instance_id,character_id,display_name,level,is_alpha,is_lucky,in_party,party_slot,box_page,box_slot,base_id,hp,gender,talent_hp,talent_melee,talent_shot,talent_defense,passive_skill_ids,equipped_skill_ids FROM pals WHERE owner_uid=?", NormalizeUID(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var i, c, n, baseID string
		var l int
		var a, k, inParty bool
		var partySlot, boxPage, boxSlot *int
		var hp *float64
		var gender, passiveJSON, equippedJSON string
		var talentHP, talentMelee, talentShot, talentDefense *int
		if err = rows.Scan(&i, &c, &n, &l, &a, &k, &inParty, &partySlot, &boxPage, &boxSlot, &baseID, &hp, &gender, &talentHP, &talentMelee, &talentShot, &talentDefense, &passiveJSON, &equippedJSON); err != nil {
			return nil, err
		}
		passives, equipped := []string{}, []string{}
		_ = json.Unmarshal([]byte(passiveJSON), &passives)
		_ = json.Unmarshal([]byte(equippedJSON), &equipped)
		if passives == nil {
			passives = []string{}
		}
		if equipped == nil {
			equipped = []string{}
		}
		out = append(out, map[string]any{
			"instanceId": i, "characterId": c, "displayName": n, "level": l,
			"isAlpha": a, "isLucky": k, "inParty": inParty, "partySlot": partySlot,
			"boxPage": boxPage, "boxSlot": boxSlot, "baseId": nullableString(baseID),
			"placement": palPlacement(inParty, boxPage, baseID), "hp": hp, "gender": gender,
			"talents":         map[string]any{"hp": talentHP, "melee": talentMelee, "shot": talentShot, "defense": talentDefense},
			"passiveSkillIds": passives, "equippedSkillIds": equipped,
		})
	}
	return out, rows.Err()
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func palPlacement(inParty bool, boxPage *int, baseID string) string {
	switch {
	case inParty:
		return "party"
	case boxPage != nil:
		return "box"
	case baseID != "":
		return "base"
	default:
		return "unknown"
	}
}

// Whitelist returns whitelisted Steam identities.
func (s *Store) Whitelist(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT steam_id,name FROM whitelist ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"steamId": id, "name": name})
	}
	return out, rows.Err()
}

// ReplaceWhitelist atomically replaces whitelist flags, creating identity placeholders as needed.
func (s *Store) ReplaceWhitelist(ctx context.Context, entries []map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM whitelist"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE players SET whitelisted=0"); err != nil {
		return err
	}
	for _, e := range entries {
		id := e["steamId"]
		if id == "" {
			return errors.New("steamId is required")
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO whitelist(steam_id,name) VALUES(?,?)", id, e["name"])
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE players SET whitelisted=1 WHERE steam_id=?", id)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// APIKey is one integration bearer-token credential. Hash is the 32-byte SHA-256
// digest of the full plaintext key; the plaintext itself is never stored.
type APIKey struct {
	ID         string     `json:"id"`
	Hash       [32]byte   `json:"-"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

// MaxActiveAPIKeys bounds the number of unrevoked integration API keys (spec §2.6, §8.1).
// CreateAPIKey enforces it atomically, so this is the authoritative cap.
const MaxActiveAPIKeys = 100

// ErrAPIKeyCapReached is returned by CreateAPIKey when the active (unrevoked) key count
// already meets MaxActiveAPIKeys.
var ErrAPIKeyCapReached = errors.New("api key cap reached")

// CreateAPIKey inserts a new key row and returns the stored record. id and hash are
// caller-supplied (the key id is public/non-secret; the hash is the full-key SHA-256).
// On a primary-key collision the insert fails with the driver's constraint error and
// the caller is expected to retry with a fresh id.
//
// The active-key count check and the insert run inside one transaction (count-then-insert),
// which is race-free: SetMaxOpenConns(1) means this Store has exactly one pooled connection,
// and database/sql holds that connection exclusively for a Tx's entire lifetime, so no other
// CreateAPIKey call (or any other write) can interleave between the count and the insert.
// This closes a TOCTOU where concurrent creates at 99 active could each observe count<100
// before either inserted, pushing the active count past the documented cap (spec §2.6).
func (s *Store) CreateAPIKey(ctx context.Context, id string, hash [32]byte, label string, now time.Time) (APIKey, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return APIKey{}, err
	}
	defer tx.Rollback()
	var active int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys WHERE revoked_at IS NULL").Scan(&active); err != nil {
		return APIKey{}, err
	}
	if active >= MaxActiveAPIKeys {
		return APIKey{}, ErrAPIKeyCapReached
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO api_keys(id,hash,label,created_at) VALUES(?,?,?,?)", id, hash[:], label, now.Unix()); err != nil {
		return APIKey{}, err
	}
	if err = tx.Commit(); err != nil {
		return APIKey{}, err
	}
	return APIKey{ID: id, Hash: hash, Label: label, CreatedAt: now.UTC()}, nil
}

// ListAPIKeys returns every key, including revoked ones, newest first.
func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	return s.listAPIKeys(ctx, "")
}

// ActiveAPIKeys returns unrevoked keys, for populating the in-memory validation
// cache at startup. Revoked keys are omitted deliberately: an id absent from the
// cache is indistinguishable from an unknown id, which already returns the uniform
// 401 via the dummy-compare path.
func (s *Store) ActiveAPIKeys(ctx context.Context) ([]APIKey, error) {
	return s.listAPIKeys(ctx, " WHERE revoked_at IS NULL")
}

func (s *Store) listAPIKeys(ctx context.Context, where string) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,hash,label,created_at,last_used_at,revoked_at FROM api_keys"+where+" ORDER BY created_at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey soft-revokes a key and returns the updated record. Idempotent:
// revoking an already-revoked key leaves its revoked_at untouched and returns it
// unchanged. Unknown id returns sql.ErrNoRows.
func (s *Store) RevokeAPIKey(ctx context.Context, id string, now time.Time) (APIKey, error) {
	if _, err := s.db.ExecContext(ctx, "UPDATE api_keys SET revoked_at=? WHERE id=? AND revoked_at IS NULL", now.Unix(), id); err != nil {
		return APIKey{}, err
	}
	return s.apiKeyByID(ctx, id)
}

// TouchAPIKeyLastUsed persists lastUsedAt for one key. Callers are responsible for
// the at-most-once-per-60s coalescing (a mutex-guarded compare-and-set in the
// in-memory validation cache); this method itself is an unconditional write.
func (s *Store) TouchAPIKeyLastUsed(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at=? WHERE id=?", at.Unix(), id)
	return err
}

func (s *Store) apiKeyByID(ctx context.Context, id string) (APIKey, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id,hash,label,created_at,last_used_at,revoked_at FROM api_keys WHERE id=?", id)
	return scanAPIKey(row)
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanAPIKey(row rowScanner) (APIKey, error) {
	var k APIKey
	var hash []byte
	var created int64
	var lastUsed, revoked sql.NullInt64
	if err := row.Scan(&k.ID, &hash, &k.Label, &created, &lastUsed, &revoked); err != nil {
		return APIKey{}, err
	}
	copy(k.Hash[:], hash)
	k.CreatedAt = time.Unix(created, 0).UTC()
	if lastUsed.Valid {
		t := time.Unix(lastUsed.Int64, 0).UTC()
		k.LastUsedAt = &t
	}
	if revoked.Valid {
		t := time.Unix(revoked.Int64, 0).UTC()
		k.RevokedAt = &t
	}
	return k, nil
}
