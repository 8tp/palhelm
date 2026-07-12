package poller

import (
	"context"
	"github.com/palhelm/palhelm/internal/palworld"
	"github.com/palhelm/palhelm/internal/store"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestDiffPlayers(t *testing.T) {
	old := map[string]palworld.Player{"a": {}, "b": {}}
	next := map[string]palworld.Player{"b": {}, "c": {}}
	joins, leaves := DiffPlayers(old, next)
	if !reflect.DeepEqual(joins, []string{"c"}) || !reflect.DeepEqual(leaves, []string{"a"}) {
		t.Fatalf("joins=%v leaves=%v", joins, leaves)
	}
}

type recordingPublisher struct {
	mu     sync.Mutex
	events []store.Event
}

func (p *recordingPublisher) Publish(event string, value any) {
	if event != "event" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, value.(store.Event))
}

func testService(t *testing.T, client *palworld.Client) (*Service, *store.Store, *recordingPublisher) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "poller.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	pub := &recordingPublisher{}
	service := New(client, st, pub, &Health{}, time.Hour, time.Hour, time.Hour, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	return service, st, pub
}

func TestFormatDriftTransitionEvents(t *testing.T) {
	cases := []struct {
		name           string
		previous, next bool
		wantMessage    string
		wantSkipped    any
	}{
		{name: "entering drift", previous: false, next: true, wantMessage: "world save format drift detected", wantSkipped: float64(7)},
		{name: "leaving drift", previous: true, next: false, wantMessage: "world save format drift resolved"},
		{name: "steady clean", previous: false, next: false},
		{name: "steady drift", previous: true, next: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, st, pub := testService(t, palworld.NewClient("", "", ""))
			s.emitFormatDriftTransition(context.Background(), tc.previous, tc.next, 7, time.Now().UTC())
			events, err := st.Events(context.Background(), 10, "system")
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if tc.wantMessage != "" {
				wantCount = 1
			}
			if len(events) != wantCount || len(pub.events) != wantCount {
				t.Fatalf("stored/published event counts = %d/%d, want %d", len(events), len(pub.events), wantCount)
			}
			if wantCount == 0 {
				return
			}
			if events[0].Kind != "system" || events[0].Message != tc.wantMessage || pub.events[0].Message != tc.wantMessage {
				t.Fatalf("stored/published events = %#v / %#v", events[0], pub.events[0])
			}
			if tc.wantSkipped != nil {
				meta, ok := events[0].Meta.(map[string]any)
				if !ok || meta["skippedProps"] != tc.wantSkipped {
					t.Fatalf("detected meta = %#v, want skippedProps=%v", events[0].Meta, tc.wantSkipped)
				}
			}
		})
	}
}

func TestPollPlayersSkipsPendingIdentities(t *testing.T) {
	for _, playerID := range []string{"", " none ", "NONE"} {
		t.Run("playerId="+playerID, func(t *testing.T) {
			s, st, _ := testService(t, nil)
			s.syncPlayers(context.Background(), []palworld.Player{{Name: "Pending", PlayerID: playerID, UserID: "steam-pending"}})
			players, err := st.Players(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(players) != 0 || len(s.Online()) != 0 {
				t.Fatalf("pending identity participated in poll: players=%#v online=%#v", players, s.Online())
			}
			events, err := st.Events(context.Background(), 10, "join")
			if err != nil || len(events) != 0 {
				t.Fatalf("join events = %#v, %v", events, err)
			}
		})
	}
}
