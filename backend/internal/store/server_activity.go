package store

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

const maxActivityIntervals = 100_000

type ActivityConcurrencyBucket struct {
	At             time.Time `json:"at"`
	PeakPlayers    int       `json:"peakPlayers"`
	AveragePlayers float64   `json:"averagePlayers"`
}

type ActivityPlayerRank struct {
	UID            string `json:"uid"`
	Name           string `json:"name"`
	GuildID        string `json:"guildId"`
	GuildName      string `json:"guildName"`
	DurationSec    int64  `json:"durationSec"`
	SessionCount   int    `json:"sessionCount"`
	CurrentSession bool   `json:"currentSession"`
	FirstObserved  bool   `json:"firstObserved"`
}

type ActivityGuildRank struct {
	GuildID       string `json:"guildId"`
	GuildName     string `json:"guildName"`
	DurationSec   int64  `json:"durationSec"`
	SessionCount  int    `json:"sessionCount"`
	ActivePlayers int    `json:"activePlayers"`
}

type ServerActivity struct {
	Coverage                string                      `json:"coverage"`
	TrackingSince           *time.Time                  `json:"trackingSince"`
	Window                  string                      `json:"window"`
	Since                   time.Time                   `json:"since"`
	Through                 time.Time                   `json:"through"`
	BucketSec               int64                       `json:"bucketSec"`
	AnalysisTruncated       bool                        `json:"analysisTruncated"`
	ActivePlayers           int                         `json:"activePlayers"`
	NewPlayers              int                         `json:"newPlayers"`
	ReturningPlayers        int                         `json:"returningPlayers"`
	PeakConcurrency         int                         `json:"peakConcurrency"`
	PeakAt                  *time.Time                  `json:"peakAt"`
	Concurrency             []ActivityConcurrencyBucket `json:"concurrency"`
	Players                 []ActivityPlayerRank        `json:"players"`
	Guilds                  []ActivityGuildRank         `json:"guilds"`
	GuildAttribution        string                      `json:"guildAttribution"`
	UnattributedPlayers     int                         `json:"unattributedPlayers"`
	UnattributedDurationSec int64                       `json:"unattributedDurationSec"`
}

type activityInterval struct {
	start, end                    int64
	uid, name, guildID, guildName string
	firstJoin                     int64
	open                          bool
}

type activityBoundary struct {
	at    int64
	delta int
}

type activityPlayerAccumulator struct {
	ActivityPlayerRank
}

type activityGuildAccumulator struct {
	ActivityGuildRank
	players map[string]struct{}
}

// ServerActivity summarizes only connection intervals observed by this panel. Raw sessions are
// never returned; rankings and time buckets are hard-capped while a defensive interval cap is
// reported explicitly if an unusually busy 30-day window exceeds it.
func (s *Store) ServerActivity(ctx context.Context, now time.Time, window, bucket time.Duration, windowName string, rankLimit int) (ServerActivity, error) {
	now = now.UTC().Truncate(time.Second)
	since := now.Add(-window)
	if rankLimit < 1 || rankLimit > 100 {
		rankLimit = 25
	}
	result := ServerActivity{
		Coverage: "panel_observed_sessions", Window: windowName, Since: since, Through: now,
		BucketSec: int64(bucket / time.Second), Concurrency: []ActivityConcurrencyBucket{},
		Players: []ActivityPlayerRank{}, Guilds: []ActivityGuildRank{},
		GuildAttribution: "current_player_guild",
	}
	var tracking sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT MIN(join_at) FROM sessions WHERE join_at<=?", now.Unix()).Scan(&tracking); err != nil {
		return ServerActivity{}, err
	}
	if tracking.Valid {
		value := time.Unix(tracking.Int64, 0).UTC()
		result.TrackingSince = &value
	}

	rows, err := s.db.QueryContext(ctx, `SELECT s.join_at,s.leave_at,s.player_uid,COALESCE(p.name,''),COALESCE(p.guild_id,''),COALESCE(p.guild_name,''),first_seen.first_join
FROM sessions s
LEFT JOIN players p ON p.uid=s.player_uid
JOIN (SELECT player_uid,MIN(join_at) AS first_join FROM sessions GROUP BY player_uid) first_seen ON first_seen.player_uid=s.player_uid
WHERE s.join_at<? AND COALESCE(s.leave_at,?)>?
ORDER BY s.join_at ASC,s.id ASC LIMIT ?`, now.Unix(), now.Unix(), since.Unix(), maxActivityIntervals+1)
	if err != nil {
		return ServerActivity{}, err
	}
	defer rows.Close()
	intervals := make([]activityInterval, 0)
	for rows.Next() {
		if len(intervals) == maxActivityIntervals {
			result.AnalysisTruncated = true
			break
		}
		var joined, firstJoin int64
		var left sql.NullInt64
		var interval activityInterval
		if err = rows.Scan(&joined, &left, &interval.uid, &interval.name, &interval.guildID, &interval.guildName, &firstJoin); err != nil {
			return ServerActivity{}, err
		}
		interval.start = max(joined, since.Unix())
		interval.end = now.Unix()
		interval.open = !left.Valid
		if left.Valid {
			interval.end = min(left.Int64, now.Unix())
		}
		interval.firstJoin = firstJoin
		if interval.end > interval.start {
			intervals = append(intervals, interval)
		}
	}
	if err = rows.Err(); err != nil {
		return ServerActivity{}, err
	}

	players := map[string]*activityPlayerAccumulator{}
	guilds := map[string]*activityGuildAccumulator{}
	boundaries := make([]activityBoundary, 0, len(intervals)*2)
	for _, interval := range intervals {
		duration := interval.end - interval.start
		player := players[interval.uid]
		if player == nil {
			player = &activityPlayerAccumulator{ActivityPlayerRank: ActivityPlayerRank{
				UID: interval.uid, Name: interval.name, GuildID: interval.guildID, GuildName: interval.guildName,
				FirstObserved: interval.firstJoin >= since.Unix(),
			}}
			players[interval.uid] = player
		}
		player.DurationSec += duration
		player.SessionCount++
		player.CurrentSession = player.CurrentSession || interval.open
		boundaries = append(boundaries, activityBoundary{at: interval.start, delta: 1}, activityBoundary{at: interval.end, delta: -1})

		if interval.guildID == "" {
			result.UnattributedDurationSec += duration
			continue
		}
		guild := guilds[interval.guildID]
		if guild == nil {
			guild = &activityGuildAccumulator{ActivityGuildRank: ActivityGuildRank{GuildID: interval.guildID, GuildName: interval.guildName}, players: map[string]struct{}{}}
			guilds[interval.guildID] = guild
		}
		guild.DurationSec += duration
		guild.SessionCount++
		guild.players[interval.uid] = struct{}{}
	}
	result.ActivePlayers = len(players)
	for _, player := range players {
		if player.FirstObserved {
			result.NewPlayers++
		} else {
			result.ReturningPlayers++
		}
		if player.GuildID == "" {
			result.UnattributedPlayers++
		}
		result.Players = append(result.Players, player.ActivityPlayerRank)
	}
	sort.Slice(result.Players, func(i, j int) bool {
		if result.Players[i].DurationSec != result.Players[j].DurationSec {
			return result.Players[i].DurationSec > result.Players[j].DurationSec
		}
		return result.Players[i].Name < result.Players[j].Name
	})
	if len(result.Players) > rankLimit {
		result.Players = result.Players[:rankLimit]
	}
	for _, guild := range guilds {
		guild.ActivePlayers = len(guild.players)
		result.Guilds = append(result.Guilds, guild.ActivityGuildRank)
	}
	sort.Slice(result.Guilds, func(i, j int) bool {
		if result.Guilds[i].DurationSec != result.Guilds[j].DurationSec {
			return result.Guilds[i].DurationSec > result.Guilds[j].DurationSec
		}
		return result.Guilds[i].GuildName < result.Guilds[j].GuildName
	})
	if len(result.Guilds) > rankLimit {
		result.Guilds = result.Guilds[:rankLimit]
	}
	result.Concurrency, result.PeakConcurrency, result.PeakAt = activityConcurrency(boundaries, since, now, bucket)
	return result, nil
}

func activityConcurrency(boundaries []activityBoundary, since, through time.Time, bucket time.Duration) ([]ActivityConcurrencyBucket, int, *time.Time) {
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i].at < boundaries[j].at })
	out := make([]ActivityConcurrencyBucket, 0, int(through.Sub(since)/bucket))
	current, index, overallPeak := 0, 0, 0
	var peakAt *time.Time
	for start := since; start.Before(through); start = start.Add(bucket) {
		end := start.Add(bucket)
		if end.After(through) {
			end = through
		}
		cursor := start.Unix()
		area, bucketPeak := int64(0), current
		for index < len(boundaries) && boundaries[index].at < end.Unix() {
			at := boundaries[index].at
			area += int64(current) * max(int64(0), at-cursor)
			delta := 0
			for index < len(boundaries) && boundaries[index].at == at {
				delta += boundaries[index].delta
				index++
			}
			current += delta
			cursor = at
			bucketPeak = max(bucketPeak, current)
			if current > overallPeak {
				overallPeak = current
				value := time.Unix(at, 0).UTC()
				peakAt = &value
			}
		}
		area += int64(current) * max(int64(0), end.Unix()-cursor)
		duration := max(float64(1), end.Sub(start).Seconds())
		out = append(out, ActivityConcurrencyBucket{At: start, PeakPlayers: bucketPeak, AveragePlayers: float64(area) / duration})
	}
	return out, overallPeak, peakAt
}
