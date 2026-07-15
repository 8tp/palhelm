package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/store"
)

func TestPlayerDetailExposesBoundedViewerSafeObservedActivity(t *testing.T) {
	_, h, st := newKeyManagementTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	uid := "activity-player"
	if err := st.UpsertLivePlayer(ctx, store.Player{UID: uid, Name: "Activity Player"}, now); err != nil {
		t.Fatal(err)
	}
	first := now.Add(-6 * time.Hour)
	for i := 0; i < 25; i++ {
		joined := first.Add(time.Duration(i) * 10 * time.Minute)
		if err := st.StartSession(ctx, uid, joined); err != nil {
			t.Fatal(err)
		}
		if err := st.EndSession(ctx, uid, joined.Add(5*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.StartSession(ctx, uid, now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	viewer := loginAs(t, h, "viewerpass")
	rr := sessionRequest(h, http.MethodGet, "/api/v1/players/"+uid, "", viewer)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Activity store.PlayerActivity    `json:"activity"`
		Sessions []store.ObservedSession `json:"sessions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	activity := doc.Activity
	if activity.Coverage != "panel_observed_sessions" || activity.TrackingSince == nil || !activity.TrackingSince.Equal(first) {
		t.Fatalf("coverage = %#v", activity)
	}
	if activity.CurrentSession == nil || activity.CurrentSession.DurationSec < 30*60 || activity.CurrentSession.DurationSec > 30*60+2 {
		t.Fatalf("current session = %#v", activity.CurrentSession)
	}
	if activity.Windows.Last24Hours.SessionCount != 26 || activity.Windows.Last24Hours.DurationSec < 155*60 || activity.Windows.Last24Hours.DurationSec > 155*60+2 {
		t.Fatalf("24-hour summary = %#v", activity.Windows.Last24Hours)
	}
	if len(activity.RecentSessions) != 20 || !activity.RecentSessionsTruncated || len(doc.Sessions) != 20 {
		t.Fatalf("recent=%d legacy=%d truncated=%v", len(activity.RecentSessions), len(doc.Sessions), activity.RecentSessionsTruncated)
	}
	activityJSON, err := json.Marshal(activity)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"steam", "account", "raw", "location", "platform"} {
		if strings.Contains(strings.ToLower(string(activityJSON)), forbidden) {
			t.Errorf("activity projection leaked %q: %s", forbidden, activityJSON)
		}
	}
}
