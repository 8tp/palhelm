package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/8tp/palhelm/internal/paldeck"
)

const maxGuildDetailPals = 500

type GuildDetailLocation struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type GuildDetailMember struct {
	UID                  string     `json:"uid"`
	Name                 string     `json:"name"`
	Level                int        `json:"level"`
	Online               bool       `json:"online"`
	LastSeenAt           *time.Time `json:"lastSeenAt"`
	PlaytimeSec          int64      `json:"playtimeSec"`
	CaptureTotal         *int64     `json:"captureTotal"`
	UniquePalsCaptured   *int       `json:"uniquePalsCaptured"`
	PaldeckUnlocked      *int       `json:"paldeckUnlocked"`
	ObservedDurationSec  int64      `json:"observedDurationSec"`
	ObservedSessionCount int        `json:"observedSessionCount"`
	CurrentSession       bool       `json:"currentSession"`
}

type GuildDetailBase struct {
	ID string `json:"id"`
	// Name is null when the base was never renamed (or the save predates name
	// decoding); consumers fall back to a positional "Base N" label.
	Name     *string              `json:"name"`
	Location *GuildDetailLocation `json:"location"`
	Level    int                  `json:"level"`
	PalCount int                  `json:"palCount"`
}

type GuildDetailPal struct {
	InstanceID    string  `json:"instanceId"`
	CharacterID   string  `json:"characterId"`
	DisplayName   string  `json:"displayName"`
	Level         int     `json:"level"`
	IsAlpha       bool    `json:"isAlpha"`
	IsLucky       bool    `json:"isLucky"`
	IsBoss        bool    `json:"isBoss"`
	Placement     string  `json:"placement"`
	BaseID        *string `json:"baseId"`
	OwnerUID      string  `json:"ownerUid"`
	OwnerName     string  `json:"ownerName"`
	OwnerSource   string  `json:"ownerSource"`
	OwnerResolved bool    `json:"ownerResolved"`
	Association   string  `json:"association"`
}

type GuildDetailActivity struct {
	Coverage          string     `json:"coverage"`
	Attribution       string     `json:"attribution"`
	Window            string     `json:"window"`
	Since             time.Time  `json:"since"`
	Through           time.Time  `json:"through"`
	TrackingSince     *time.Time `json:"trackingSince"`
	AnalysisTruncated bool       `json:"analysisTruncated"`
	DurationSec       int64      `json:"durationSec"`
	SessionCount      int        `json:"sessionCount"`
	ActivePlayers     int        `json:"activePlayers"`
}

type GuildDetail struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	AdminUID      string              `json:"adminUid"`
	MemberCount   int                 `json:"memberCount"`
	Members       []GuildDetailMember `json:"members"`
	Bases         []GuildDetailBase   `json:"bases"`
	PalCount      int                 `json:"palCount"`
	PalsTruncated bool                `json:"palsTruncated"`
	Pals          []GuildDetailPal    `json:"pals"`
	Activity      GuildDetailActivity `json:"activity"`
}

// GuildDetail returns a bounded current-save projection. Pals are associated only by an exact
// guild base join or a current guild member's stored owner UID; activity is attributed only to
// the current member roster and is explicitly limited to panel-observed sessions.
func (s *Store) GuildDetail(ctx context.Context, guildID string, now time.Time) (GuildDetail, error) {
	now = now.UTC().Truncate(time.Second)
	guildID = NormalizeUID(guildID)
	result := GuildDetail{
		ID: guildID, Members: []GuildDetailMember{}, Bases: []GuildDetailBase{}, Pals: []GuildDetailPal{},
		Activity: GuildDetailActivity{Coverage: "panel_observed_sessions", Attribution: "current_guild_membership", Window: "30d", Since: now.Add(-30 * 24 * time.Hour), Through: now},
	}
	if err := s.db.QueryRowContext(ctx, `SELECT name,admin_uid FROM guilds WHERE id=?`, guildID).Scan(&result.Name, &result.AdminUID); err != nil {
		return GuildDetail{}, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT gm.player_uid,COALESCE(NULLIF(p.name,''),gm.name,''),COALESCE(p.level,0),p.last_seen,COALESCE(p.playtime_sec,0),p.capture_total,p.unique_pals_captured,p.paldeck_unlocked
FROM guild_members gm LEFT JOIN players p ON p.uid=gm.player_uid WHERE gm.guild_id=? ORDER BY COALESCE(NULLIF(p.name,''),gm.name,''),gm.player_uid`, guildID)
	if err != nil {
		return GuildDetail{}, err
	}
	memberIndex := map[string]int{}
	for rows.Next() {
		var member GuildDetailMember
		var lastSeen, capture, unique, unlocked sql.NullInt64
		if err = rows.Scan(&member.UID, &member.Name, &member.Level, &lastSeen, &member.PlaytimeSec, &capture, &unique, &unlocked); err != nil {
			rows.Close()
			return GuildDetail{}, err
		}
		if lastSeen.Valid {
			v := time.Unix(lastSeen.Int64, 0).UTC()
			member.LastSeenAt = &v
		}
		if capture.Valid {
			member.CaptureTotal = &capture.Int64
		}
		if unique.Valid {
			v := int(unique.Int64)
			member.UniquePalsCaptured = &v
		}
		if unlocked.Valid {
			v := int(unlocked.Int64)
			member.PaldeckUnlocked = &v
		}
		memberIndex[member.UID] = len(result.Members)
		result.Members = append(result.Members, member)
	}
	if err = rows.Close(); err != nil {
		return GuildDetail{}, err
	}
	result.MemberCount = len(result.Members)

	baseRows, err := s.db.QueryContext(ctx, `SELECT b.id,b.name,b.x,b.y,b.level,COUNT(p.instance_id) FROM bases b LEFT JOIN pals p ON p.base_id=b.id WHERE b.guild_id=? GROUP BY b.id,b.name,b.x,b.y,b.level ORDER BY b.id`, guildID)
	if err != nil {
		return GuildDetail{}, err
	}
	for baseRows.Next() {
		var base GuildDetailBase
		var name sql.NullString
		var x, y sql.NullFloat64
		if err = baseRows.Scan(&base.ID, &name, &x, &y, &base.Level, &base.PalCount); err != nil {
			baseRows.Close()
			return GuildDetail{}, err
		}
		if name.Valid && name.String != "" {
			base.Name = &name.String
		}
		if x.Valid && y.Valid {
			base.Location = &GuildDetailLocation{X: x.Float64, Y: y.Float64}
		}
		result.Bases = append(result.Bases, base)
	}
	if err = baseRows.Close(); err != nil {
		return GuildDetail{}, err
	}

	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pals p LEFT JOIN guild_members gm ON gm.player_uid=p.owner_uid AND gm.guild_id=? LEFT JOIN bases b ON b.id=p.base_id AND b.guild_id=? WHERE b.id IS NOT NULL OR gm.player_uid IS NOT NULL`, guildID, guildID).Scan(&result.PalCount); err != nil {
		return GuildDetail{}, err
	}
	result.PalsTruncated = result.PalCount > maxGuildDetailPals
	palRows, err := s.db.QueryContext(ctx, `SELECT p.instance_id,p.character_id,p.display_name,p.level,p.is_alpha,p.is_lucky,p.in_party,p.box_page,p.base_id,p.owner_uid,COALESCE(owner.name,''),p.owner_source,owner.uid IS NOT NULL,b.id IS NOT NULL
FROM pals p
LEFT JOIN players owner ON owner.uid=p.owner_uid
LEFT JOIN guild_members gm ON gm.player_uid=p.owner_uid AND gm.guild_id=?
LEFT JOIN bases b ON b.id=p.base_id AND b.guild_id=?
WHERE b.id IS NOT NULL OR gm.player_uid IS NOT NULL
ORDER BY p.display_name,p.instance_id LIMIT ?`, guildID, guildID, maxGuildDetailPals)
	if err != nil {
		return GuildDetail{}, err
	}
	for palRows.Next() {
		var pal GuildDetailPal
		var inParty, hasBase bool
		var boxPage sql.NullInt64
		var baseID string
		if err = palRows.Scan(&pal.InstanceID, &pal.CharacterID, &pal.DisplayName, &pal.Level, &pal.IsAlpha, &pal.IsLucky, &inParty, &boxPage, &baseID, &pal.OwnerUID, &pal.OwnerName, &pal.OwnerSource, &pal.OwnerResolved, &hasBase); err != nil {
			palRows.Close()
			return GuildDetail{}, err
		}
		pal.IsBoss = paldeck.IsBossID(pal.CharacterID)
		switch {
		case inParty:
			pal.Placement = "party"
		case boxPage.Valid:
			pal.Placement = "box"
		case baseID != "":
			pal.Placement = "base"
		default:
			pal.Placement = "unknown"
		}
		if baseID != "" {
			pal.BaseID = &baseID
		}
		if hasBase {
			pal.Association = "guild_base"
		} else {
			pal.Association = "current_member_owner"
		}
		result.Pals = append(result.Pals, pal)
	}
	if err = palRows.Close(); err != nil {
		return GuildDetail{}, err
	}
	if len(memberIndex) == 0 {
		return result, nil
	}
	var tracking sql.NullInt64
	if err = s.db.QueryRowContext(ctx, `SELECT MIN(join_at) FROM sessions WHERE player_uid IN (SELECT player_uid FROM guild_members WHERE guild_id=?)`, guildID).Scan(&tracking); err != nil {
		return GuildDetail{}, err
	}
	if tracking.Valid {
		v := time.Unix(tracking.Int64, 0).UTC()
		result.Activity.TrackingSince = &v
	}
	sessionRows, err := s.db.QueryContext(ctx, `SELECT player_uid,join_at,leave_at FROM sessions WHERE player_uid IN (SELECT player_uid FROM guild_members WHERE guild_id=?) AND join_at<? AND COALESCE(leave_at,?)>? ORDER BY join_at,id LIMIT ?`, guildID, now.Unix(), now.Unix(), result.Activity.Since.Unix(), maxActivityIntervals+1)
	if err != nil {
		return GuildDetail{}, err
	}
	active := map[string]struct{}{}
	for sessionRows.Next() {
		if result.Activity.SessionCount == maxActivityIntervals {
			result.Activity.AnalysisTruncated = true
			break
		}
		var uid string
		var joined int64
		var left sql.NullInt64
		if err = sessionRows.Scan(&uid, &joined, &left); err != nil {
			sessionRows.Close()
			return GuildDetail{}, err
		}
		end := now.Unix()
		if left.Valid {
			end = min(end, left.Int64)
		}
		duration := max(int64(0), end-max(joined, result.Activity.Since.Unix()))
		result.Activity.DurationSec += duration
		result.Activity.SessionCount++
		active[uid] = struct{}{}
		if index, ok := memberIndex[uid]; ok {
			result.Members[index].ObservedDurationSec += duration
			result.Members[index].ObservedSessionCount++
			result.Members[index].CurrentSession = result.Members[index].CurrentSession || !left.Valid
		}
	}
	if err = sessionRows.Close(); err != nil {
		return GuildDetail{}, err
	}
	result.Activity.ActivePlayers = len(active)
	return result, nil
}
