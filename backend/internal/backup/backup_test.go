package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/store"
)

func testEngine(t *testing.T) (*Engine, *store.Store, string) {
	t.Helper()
	root := t.TempDir()
	save := filepath.Join(root, "Saved")
	world := filepath.Join(save, "SaveGames", "0", "GUID")
	if err := os.MkdirAll(world, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(world, "Level.sav"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(filepath.Join(root, "data"), save, st, nil, func(context.Context) bool { return false }, nil, nil), st, world
}

func TestRoundTripDryRunRestore(t *testing.T) {
	e, _, world := testEngine(t)
	ctx := context.Background()
	b, err := e.Create(ctx, "manual")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := e.Contents(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("contents=%v", entries)
	}
	if err = os.WriteFile(filepath.Join(world, "Level.sav"), []byte("modified and longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(world, "new.sav"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	diff, err := e.DryRun(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.RequiresStop || len(diff.Changes) != 2 {
		t.Fatalf("diff=%+v", diff)
	}
	if _, err = e.Restore(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(world, "Level.sav"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("restored=%q", got)
	}
	if _, err = os.Stat(filepath.Join(world, "new.sav")); !os.IsNotExist(err) {
		t.Fatalf("new file survived: %v", err)
	}
}

func TestBackupExcludesNestedWorldBackupsAndRetainsActiveSaveTree(t *testing.T) {
	e, _, world := testEngine(t)
	active := map[string]string{
		"Level.sav":                    "active-level",
		"LevelMeta.sav":                "active-level-meta",
		"Players/0000000000000001.sav": "active-player",
		"WorldOption.sav":              "active-world-option",
		"Config/WorldSettings.ini":     "active-config",
		"backup-old/keep.sav":          "not-the-wrapper-backup",
		"Players/backup/keep.sav":      "nested-non-wrapper-directory",
	}
	for rel, contents := range active {
		path := filepath.Join(world, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for rel, contents := range map[string]string{
		"backup/2026-07-15/Level.sav":                    "stale-level",
		"backup/2026-07-15/Players/0000000000000001.sav": "stale-player",
	} {
		path := filepath.Join(world, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	b, err := e.Create(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := e.Contents(context.Background(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	archived := make(map[string]bool, len(entries))
	for _, entry := range entries {
		archived[entry.Path] = true
		if strings.HasPrefix(entry.Path, "SaveGames/0/GUID/backup/") || entry.Path == "SaveGames/0/GUID/backup/" {
			t.Fatalf("nested wrapper backup was archived: %q", entry.Path)
		}
	}
	for rel := range active {
		want := "SaveGames/0/GUID/" + rel
		if !archived[want] {
			t.Errorf("active save entry %q is missing from archive", want)
		}
	}
}

func TestRestoreExtractFailurePreservesOriginal(t *testing.T) {
	e, st, world := testEngine(t)
	ctx := context.Background()
	if err := e.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(e.dir(), "world-bad.tar.gz")
	if err := writeArchive(bad, map[string][]byte{"../escape": []byte("bad")}, manifest{CreatedAt: time.Now(), WorldGUID: "GUID"}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(bad)
	b, err := st.AddBackup(ctx, store.Backup{File: filepath.Base(bad), CreatedAt: time.Now(), SizeBytes: info.Size(), Trigger: "imported"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Restore(ctx, b.ID); err == nil {
		t.Fatal("unsafe restore succeeded")
	}
	got, _ := os.ReadFile(filepath.Join(world, "Level.sav"))
	if string(got) != "original" {
		t.Fatalf("original changed: %q", got)
	}
}

func TestZipSlipRejected(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bad.tar.gz")
	if err := writeArchive(archive, map[string][]byte{"../../outside": []byte("owned")}, manifest{CreatedAt: time.Now(), WorldGUID: "GUID"}); err != nil {
		t.Fatal(err)
	}
	if _, err := extract(context.Background(), archive, filepath.Join(dir, "extract")); err == nil {
		t.Fatal("zip-slip path accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside")); !os.IsNotExist(err) {
		t.Fatalf("outside created: %v", err)
	}
}

func TestActiveWorldResolverOverridesNewerRetainedWorld(t *testing.T) {
	e, _, active := testEngine(t)
	root := filepath.Dir(active)
	inactive := filepath.Join(root, "DEADBEEF-0000-0000-0000-000000000000")
	if err := os.MkdirAll(inactive, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inactive, "Level.sav"), []byte("inactive"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(time.Hour)
	if err := os.Chtimes(inactive, newer, newer); err != nil {
		t.Fatal(err)
	}
	e.SetWorldGUIDResolver(func(context.Context) (string, error) { return "{g-u-i-d}", nil })
	b, err := e.Create(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := e.Path(context.Background(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := extract(context.Background(), p, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if m.WorldGUID != "GUID" {
		t.Fatalf("archived world %q, want active GUID", m.WorldGUID)
	}
}

func TestActiveWorldMismatchAndAmbiguityFailClosed(t *testing.T) {
	e, _, active := testEngine(t)
	e.SetWorldGUIDResolver(func(context.Context) (string, error) { return "missing", nil })
	if _, err := e.Create(context.Background(), "manual"); err == nil || !strings.Contains(err.Error(), "matched 0") {
		t.Fatalf("mismatch error = %v", err)
	}

	other := filepath.Join(filepath.Dir(active), "G-U-I-D")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	e.SetWorldGUIDResolver(func(context.Context) (string, error) { return "guid", nil })
	if _, err := e.Create(context.Background(), "manual"); err == nil || !strings.Contains(err.Error(), "matched 2") {
		t.Fatalf("ambiguity error = %v", err)
	}
}

func TestFallbackOnlyForRESTUnavailabilityAndEmitsWarning(t *testing.T) {
	e, _, _ := testEngine(t)
	var eventMeta map[string]any
	e.emit = func(_ string, meta any) { eventMeta, _ = meta.(map[string]any) }
	e.SetWorldGUIDResolver(func(context.Context) (string, error) {
		return "", fmt.Errorf("offline: %w", ErrWorldGUIDUnavailable)
	})
	if _, err := e.Create(context.Background(), "manual"); err != nil {
		t.Fatal(err)
	}
	if eventMeta["worldSelection"] != "newest-directory-fallback" || eventMeta["warning"] == nil {
		t.Fatalf("fallback event metadata = %#v", eventMeta)
	}

	e.SetWorldGUIDResolver(func(context.Context) (string, error) { return "", errors.New("REST unauthorized") })
	if _, err := e.Create(context.Background(), "manual"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("non-unavailable REST error did not fail closed: %v", err)
	}
}

func TestStoppedRestoreUsesCachedActiveWorldAcrossPreBackupAndSwap(t *testing.T) {
	e, _, originalWorld := testEngine(t)
	root := filepath.Dir(originalWorld)
	active := filepath.Join(root, "A-C-T-I-V-E")
	if err := os.Rename(originalWorld, active); err != nil {
		t.Fatal(err)
	}
	inactive := filepath.Join(root, "INACTIVE")
	if err := os.MkdirAll(inactive, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inactive, "Level.sav"), []byte("inactive-retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(time.Hour)
	if err := os.Chtimes(inactive, newer, newer); err != nil {
		t.Fatal(err)
	}
	offline := false
	e.SetWorldGUIDResolver(func(context.Context) (string, error) {
		if offline {
			return "", fmt.Errorf("stopped: %w", ErrWorldGUIDUnavailable)
		}
		return "{active}", nil
	})
	e.SetCachedWorldGUID(func() string { return "A-C-T-I-V-E" })
	target, err := e.Create(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(active, "Level.sav"), []byte("current-before-restore"), 0o600); err != nil {
		t.Fatal(err)
	}
	offline = true
	if _, err := e.Restore(context.Background(), target.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(active, "Level.sav"))
	if err != nil || string(restored) != "original" {
		t.Fatalf("active restored file = %q, %v", restored, err)
	}
	retained, err := os.ReadFile(filepath.Join(inactive, "Level.sav"))
	if err != nil || string(retained) != "inactive-retained" {
		t.Fatalf("inactive retained world changed = %q, %v", retained, err)
	}
	rows, err := e.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var pre store.Backup
	for _, row := range rows {
		if row.Trigger == "pre-restore" {
			pre = row
			break
		}
	}
	if pre.ID == 0 {
		t.Fatalf("pre-restore backup missing: %#v", rows)
	}
	prePath, _, err := e.Path(context.Background(), pre.ID)
	if err != nil {
		t.Fatal(err)
	}
	preDir := t.TempDir()
	m, err := extract(context.Background(), prePath, preDir)
	if err != nil {
		t.Fatal(err)
	}
	if normalizeWorldGUID(m.WorldGUID) != "active" || m.WorldSelection != "poller-rest-cache" {
		t.Fatalf("pre-restore manifest = %#v", m)
	}
	preBytes, err := os.ReadFile(filepath.Join(preDir, "SaveGames", "0", m.WorldGUID, "Level.sav"))
	if err != nil || string(preBytes) != "current-before-restore" {
		t.Fatalf("pre-restore active snapshot = %q, %v", preBytes, err)
	}
}

func TestArchiveEntryCountAndCopiedBytesAreBounded(t *testing.T) {
	dir := t.TempDir()
	m := manifest{CreatedAt: time.Now(), WorldGUID: "GUID"}
	many := filepath.Join(dir, "many.tar.gz")
	files := make(map[string][]byte, maxArchiveEntries)
	for i := 0; i < maxArchiveEntries; i++ {
		files[fmt.Sprintf("SaveGames/0/GUID/%05d", i)] = nil
	}
	if err := writeArchive(many, files, m); err != nil {
		t.Fatal(err)
	}
	if _, err := extract(context.Background(), many, filepath.Join(dir, "many")); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("entry-count error = %v", err)
	}

	truncated := filepath.Join(dir, "truncated.tar.gz")
	if err := writeTruncatedArchive(truncated, m); err != nil {
		t.Fatal(err)
	}
	if _, err := extract(context.Background(), truncated, filepath.Join(dir, "truncated")); err == nil {
		t.Fatal("truncated entry unexpectedly extracted")
	}
}

func TestArchivePerEntryExpandedSizeLimit(t *testing.T) {
	limits := archiveLimits{}
	if err := limits.add(&tar.Header{Name: "at-limit", Size: maxArchiveEntrySize}); err != nil {
		t.Fatalf("exact per-entry limit rejected: %v", err)
	}
	limits = archiveLimits{}
	if err := limits.add(&tar.Header{Name: "over-limit", Size: maxArchiveEntrySize + 1}); err == nil || !strings.Contains(err.Error(), "entry") {
		t.Fatalf("per-entry over-limit error = %v", err)
	}
}

func TestArchiveAggregateExpandedSizeLimit(t *testing.T) {
	limits := archiveLimits{}
	for i := int64(0); i < maxArchiveTotalSize/maxArchiveEntrySize; i++ {
		if err := limits.add(&tar.Header{Name: fmt.Sprintf("part-%d", i), Size: maxArchiveEntrySize}); err != nil {
			t.Fatalf("exact aggregate limit rejected at part %d: %v", i, err)
		}
	}
	if err := limits.add(&tar.Header{Name: "over-total", Size: 1}); err == nil || !strings.Contains(err.Error(), "extracted-size") {
		t.Fatalf("aggregate over-limit error = %v", err)
	}
}

func TestConflictingOperationsAreSerialized(t *testing.T) {
	e, _, _ := testEngine(t)
	e.running.Store(true)
	defer e.running.Store(false)
	if err := e.Delete(context.Background(), 1); !errors.Is(err, ErrRunning) {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := e.DryRun(context.Background(), 1); !errors.Is(err, ErrRunning) {
		t.Fatalf("dry-run error = %v", err)
	}
	if err := e.Prune(context.Background(), 1); !errors.Is(err, ErrRunning) {
		t.Fatalf("prune error = %v", err)
	}
}

func TestBackupWalkHonorsCancellation(t *testing.T) {
	e, _, _ := testEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Create(ctx, "manual"); !errors.Is(err, context.Canceled) {
		t.Fatalf("create error = %v, want context cancellation", err)
	}
}

func TestTreeHashHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.sav")
	if err := os.WriteFile(path, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hash(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("hash error = %v, want context cancellation", err)
	}
}

func TestScheduleDeadlinePersistsAndPUTResetsOrDisables(t *testing.T) {
	e, st, _ := testEngine(t)
	fc := newFakeClock(time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
	e.clock = fc
	s, err := e.PutSchedule(context.Background(), Schedule{Enabled: true, EveryMinutes: 60, KeepDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	if want := fc.Now().Add(time.Hour); s.NextRunAt == nil || !s.NextRunAt.Equal(want) {
		t.Fatalf("nextRunAt = %v, want %v", s.NextRunAt, want)
	}
	fc.Advance(10 * time.Minute)
	s, err = e.PutSchedule(context.Background(), Schedule{Enabled: true, EveryMinutes: 5, KeepDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	if want := fc.Now().Add(5 * time.Minute); s.NextRunAt == nil || !s.NextRunAt.Equal(want) {
		t.Fatalf("reset nextRunAt = %v, want %v", s.NextRunAt, want)
	}
	restarted := New(e.dataDir, e.saveDir, st, nil, nil, nil, nil)
	restarted.clock = fc
	loaded, err := restarted.GetSchedule(context.Background())
	if err != nil || loaded.NextRunAt == nil || s.NextRunAt == nil || !loaded.NextRunAt.Equal(*s.NextRunAt) {
		t.Fatalf("persisted schedule = %+v, %v", loaded, err)
	}
	disabled, err := restarted.PutSchedule(context.Background(), Schedule{Enabled: false, EveryMinutes: 5, KeepDays: 7})
	if err != nil || disabled.NextRunAt != nil {
		t.Fatalf("disabled schedule = %+v, %v", disabled, err)
	}
}

func TestSchedulerResetsTimerAndCoalescesMissedRun(t *testing.T) {
	e, _, _ := testEngine(t)
	e.flushDelay = 0
	fc := newFakeClock(time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
	e.clock = fc
	if _, err := e.PutSchedule(context.Background(), Schedule{Enabled: true, EveryMinutes: 60, KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	eventually(t, func() bool { return fc.TimerCount() > 0 })
	fc.Advance(30 * time.Minute)
	if _, err := e.PutSchedule(context.Background(), Schedule{Enabled: true, EveryMinutes: 5, KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return fc.TimerCount() > 1 })
	fc.Advance(5 * time.Minute)
	eventually(t, func() bool {
		rows, _ := e.List(context.Background())
		return len(rows) == 1
	})
	cancel()
	<-done

	// A persisted deadline missed while Palhelm was down runs once on restart,
	// then advances to the first future interval rather than replaying every miss.
	e2, _, _ := testEngine(t)
	e2.flushDelay = 0
	fc2 := newFakeClock(time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
	e2.clock = fc2
	if _, err := e2.PutSchedule(context.Background(), Schedule{Enabled: true, EveryMinutes: 5, KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	fc2.Advance(12 * time.Minute)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	done2 := make(chan struct{})
	go func() { e2.Run(ctx2); close(done2) }()
	eventually(t, func() bool {
		rows, _ := e2.List(context.Background())
		loaded, _ := e2.GetSchedule(context.Background())
		return len(rows) == 1 && loaded.NextRunAt != nil && loaded.NextRunAt.After(fc2.Now())
	})
	loaded, err := e2.GetSchedule(context.Background())
	if err != nil || loaded.NextRunAt == nil || !loaded.NextRunAt.After(fc2.Now()) {
		t.Fatalf("advanced missed schedule = %+v, %v", loaded, err)
	}
	cancel2()
	<-done2
}

func TestSchedulerDisableCancelsArmedDeadline(t *testing.T) {
	e, _, _ := testEngine(t)
	e.flushDelay = 0
	fc := newFakeClock(time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
	e.clock = fc
	if _, err := e.PutSchedule(context.Background(), Schedule{Enabled: true, EveryMinutes: 5, KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	eventually(t, func() bool { return fc.TimerCount() > 0 })
	if _, err := e.PutSchedule(context.Background(), Schedule{Enabled: false, EveryMinutes: 5, KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	fc.Advance(time.Hour)
	time.Sleep(10 * time.Millisecond)
	rows, err := e.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("disabled scheduler created %d backups", len(rows))
	}
	cancel()
	<-done
}

func FuzzArchiveExtraction(f *testing.F) {
	seedPath := filepath.Join(f.TempDir(), "seed.tar.gz")
	if err := writeArchive(seedPath, map[string][]byte{"SaveGames/0/GUID/Level.sav": []byte("seed")}, manifest{WorldGUID: "GUID"}); err != nil {
		f.Fatal(err)
	}
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte("not an archive"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		dir := t.TempDir()
		archive := filepath.Join(dir, "fuzz.tar.gz")
		if err := os.WriteFile(archive, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = extract(context.Background(), archive, filepath.Join(dir, "out"))
	})
}

func writeArchive(path string, files map[string][]byte, m manifest) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	mb, _ := json.Marshal(m)
	if err = tw.WriteHeader(&tar.Header{Name: "palhelm-backup.json", Mode: 0o600, Size: int64(len(mb))}); err == nil {
		_, err = tw.Write(mb)
	}
	for name, b := range files {
		if err != nil {
			break
		}
		if err = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b))}); err == nil {
			_, err = tw.Write(b)
		}
	}
	if er := tw.Close(); err == nil {
		err = er
	}
	if er := gz.Close(); err == nil {
		err = er
	}
	if er := f.Close(); err == nil {
		err = er
	}
	return err
}

func writeTruncatedArchive(path string, m manifest) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	mb, _ := json.Marshal(m)
	if err = tw.WriteHeader(&tar.Header{Name: "palhelm-backup.json", Mode: 0o600, Size: int64(len(mb))}); err == nil {
		_, err = tw.Write(mb)
	}
	if err == nil {
		err = tw.WriteHeader(&tar.Header{Name: "SaveGames/0/GUID/Level.sav", Mode: 0o600, Size: 10})
	}
	if err == nil {
		_, err = tw.Write([]byte("short"))
	}
	// tar.Writer deliberately reports the short payload. The partial stream is
	// still gzip-finalized so the extractor must verify actual copied bytes.
	_ = tw.Close()
	if closeErr := gz.Close(); err == nil {
		err = closeErr
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	clock   *fakeClock
	at      time.Time
	ch      chan time.Time
	stopped bool
	fired   bool
}

func newFakeClock(now time.Time) *fakeClock { return &fakeClock{now: now} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{clock: c, at: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	if d <= 0 {
		t.fired = true
		t.ch <- c.now
	}
	return t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	for _, t := range c.timers {
		if !t.stopped && !t.fired && !t.at.After(c.now) {
			t.fired = true
			t.ch <- c.now
		}
	}
	c.mu.Unlock()
}

func (c *fakeClock) TimerCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

func (t *fakeTimer) Chan() <-chan time.Time { return t.ch }
func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	wasActive := !t.stopped && !t.fired
	t.stopped = true
	return wasActive
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}
