// Package backup creates, indexes, inspects, and restores Palworld save archives.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/8tp/palhelm/internal/store"
)

var (
	// ErrRunning indicates that another backup operation owns the engine.
	ErrRunning = errors.New("a backup operation is already running")
	// ErrWorldGUIDUnavailable tells the engine that REST cannot currently report
	// the active world. Only this error permits the explicit newest-directory fallback.
	ErrWorldGUIDUnavailable = errors.New("active world GUID is unavailable")
)

const (
	maxArchiveEntries   = 10_000
	maxArchiveEntrySize = int64(8 << 30)
	maxArchiveTotalSize = int64(32 << 30)
)

// WorldGUIDResolver reports the active world GUID from REST or the poller's
// normalized cache. It must wrap ErrWorldGUIDUnavailable only when REST is unreachable.
type WorldGUIDResolver func(context.Context) (string, error)

type timer interface {
	Chan() <-chan time.Time
	Stop() bool
}

type clock interface {
	Now() time.Time
	NewTimer(time.Duration) timer
}

type realClock struct{}
type realTimer struct{ *time.Timer }

func (realClock) Now() time.Time                 { return time.Now() }
func (realClock) NewTimer(d time.Duration) timer { return realTimer{time.NewTimer(d)} }
func (t realTimer) Chan() <-chan time.Time       { return t.C }

// Repository is the persistence needed by Engine.
type Repository interface {
	AddBackup(context.Context, store.Backup) (store.Backup, error)
	Backups(context.Context) ([]store.Backup, error)
	BackupByID(context.Context, int64) (store.Backup, error)
	DeleteBackupIndex(context.Context, int64) error
	DeleteBackupFileIndex(context.Context, string) error
	GetKV(context.Context, string) (string, error)
	SetKV(context.Context, string, string) error
	WorldState(context.Context) (store.WorldState, error)
}

// Schedule controls automatic backups and retention.
type Schedule struct {
	Enabled      bool       `json:"enabled"`
	EveryMinutes int        `json:"everyMinutes"`
	KeepDays     int        `json:"keepDays"`
	NextRunAt    *time.Time `json:"nextRunAt"`
}

// Entry is one member of an archive.
type Entry struct {
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

// Change is one restore difference.
type Change struct {
	Path, Kind       string
	FromSize, ToSize *int64
}

func (c Change) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Path     string `json:"path"`
		Kind     string `json:"kind"`
		FromSize *int64 `json:"fromSize,omitempty"`
		ToSize   *int64 `json:"toSize,omitempty"`
	}{c.Path, c.Kind, c.FromSize, c.ToSize})
}

// Diff is a restore preview.
type Diff struct {
	Changes      []Change `json:"changes"`
	RequiresStop bool     `json:"requiresStop"`
}

type manifest struct {
	CreatedAt      time.Time `json:"createdAt"`
	Trigger        string    `json:"trigger"`
	WorldGUID      string    `json:"worldGuid"`
	WorldSelection string    `json:"worldSelection,omitempty"`
	Warning        string    `json:"warning,omitempty"`
	PanelVersion   string    `json:"panelVersion"`
	WorldDay       *int64    `json:"worldDay,omitempty"`
}

// Engine owns serialized backup operations.
type Engine struct {
	dataDir, saveDir string
	repo             Repository
	save             func(context.Context) error
	reachable        func(context.Context) bool
	emit             func(string, any)
	log              *slog.Logger
	running          atomic.Bool
	flushDelay       time.Duration
	worldGUID        WorldGUIDResolver
	cachedWorldGUID  func() string
	clock            clock
	scheduleMu       sync.Mutex
	scheduleReset    chan struct{}
	scheduleRevision atomic.Uint64
}

// New constructs an Engine. save and reachable may be nil for offline use.
func New(dataDir, saveDir string, repo Repository, save func(context.Context) error, reachable func(context.Context) bool, emit func(string, any), log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	return &Engine{
		dataDir: dataDir, saveDir: saveDir, repo: repo, save: save,
		reachable: reachable, emit: emit, log: log, flushDelay: 2 * time.Second,
		clock: realClock{}, scheduleReset: make(chan struct{}, 1),
	}
}

// SetWorldGUIDResolver supplies the authoritative active-world lookup. The
// resolver should distinguish REST unavailability from authentication or data
// errors by wrapping ErrWorldGUIDUnavailable only for the former.
func (e *Engine) SetWorldGUIDResolver(resolve WorldGUIDResolver) { e.worldGUID = resolve }

// SetCachedWorldGUID supplies the poller's last REST-observed active world for
// operations, such as restore, that intentionally require the server to be stopped.
func (e *Engine) SetCachedWorldGUID(resolve func() string) { e.cachedWorldGUID = resolve }

func (e *Engine) dir() string { return filepath.Join(e.dataDir, "backups") }

// Reconcile imports and prunes archive index rows to match disk.
func (e *Engine) Reconcile(ctx context.Context) error {
	if err := os.MkdirAll(e.dir(), 0o700); err != nil {
		return err
	}
	rows, err := e.repo.Backups(ctx)
	if err != nil {
		return err
	}
	known := map[string]store.Backup{}
	for _, b := range rows {
		known[b.File] = b
	}
	files, err := os.ReadDir(e.dir())
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".tar.gz") {
			continue
		}
		seen[f.Name()] = true
		if _, ok := known[f.Name()]; ok {
			continue
		}
		info, er := f.Info()
		if er != nil {
			return er
		}
		_, er = e.repo.AddBackup(ctx, store.Backup{File: f.Name(), CreatedAt: info.ModTime().UTC(), SizeBytes: info.Size(), Trigger: "imported"})
		if er != nil {
			return er
		}
	}
	for file, b := range known {
		if !seen[file] {
			if err = e.repo.DeleteBackupIndex(ctx, b.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// List returns indexed backups.
func (e *Engine) List(ctx context.Context) ([]store.Backup, error) { return e.repo.Backups(ctx) }

// Create performs a manual/scheduled/pre-restore archive operation.
func (e *Engine) Create(ctx context.Context, trigger string) (store.Backup, error) {
	if !e.running.CompareAndSwap(false, true) {
		return store.Backup{}, ErrRunning
	}
	defer e.running.Store(false)
	selection, err := e.resolveWorld(ctx)
	if err != nil {
		return store.Backup{}, err
	}
	b, err := e.create(ctx, trigger, trigger != "pre-restore", selection)
	if err == nil && trigger != "pre-restore" {
		if schedule, scheduleErr := e.GetSchedule(ctx); scheduleErr != nil {
			e.log.Warn("load retention after backup", "error", scheduleErr)
		} else if pruneErr := e.prune(ctx, schedule.KeepDays); pruneErr != nil {
			e.log.Warn("retention after backup", "error", pruneErr)
		}
	}
	return b, err
}

func (e *Engine) create(ctx context.Context, trigger string, flush bool, selection worldSelection) (store.Backup, error) {
	if flush && e.save != nil {
		if err := e.save(ctx); err != nil {
			e.log.Warn("backup save request failed; continuing", "error", err)
		} else if e.flushDelay > 0 {
			select {
			case <-ctx.Done():
				return store.Backup{}, ctx.Err()
			case <-time.After(e.flushDelay):
			}
		}
	}
	world, guid := selection.Path, selection.GUID
	state, _ := e.repo.WorldState(ctx)
	var day *int64
	if !state.LastParseAt.IsZero() {
		v := state.Day
		day = &v
	}
	now := time.Now().UTC()
	name := now.Format("world-2006-01-02-1504") + ".tar.gz"
	var err error
	if _, err = os.Stat(filepath.Join(e.dir(), name)); err == nil {
		name = now.Format("world-2006-01-02-150405") + ".tar.gz"
	}
	if err = os.MkdirAll(e.dir(), 0o700); err != nil {
		return store.Backup{}, err
	}
	tmp, err := os.CreateTemp(e.dir(), ".backup-*.tmp")
	if err != nil {
		return store.Backup{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)
	m := manifest{CreatedAt: now, Trigger: trigger, WorldGUID: guid, WorldSelection: selection.Source, Warning: selection.Warning, PanelVersion: "dev", WorldDay: day}
	mb, _ := json.Marshal(m)
	manifestHeader := &tar.Header{Name: "palhelm-backup.json", Mode: 0o600, Size: int64(len(mb)), ModTime: now}
	limits := archiveLimits{}
	if err = limits.add(manifestHeader); err == nil {
		err = tw.WriteHeader(manifestHeader)
	}
	if err == nil {
		_, err = tw.Write(mb)
	}
	base := filepath.Join(e.saveDir, "SaveGames", "0")
	if err == nil {
		err = filepath.WalkDir(world, func(path string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			rel, er := filepath.Rel(base, path)
			if er != nil {
				return er
			}
			info, er := d.Info()
			if er != nil {
				return er
			}
			h, er := tar.FileInfoHeader(info, "")
			if er != nil {
				return er
			}
			h.Name = filepath.ToSlash(filepath.Join("SaveGames", "0", rel))
			if d.IsDir() {
				h.Name += "/"
			}
			if er = limits.add(h); er != nil {
				return er
			}
			if er = tw.WriteHeader(h); er != nil {
				return er
			}
			if info.Mode().IsRegular() {
				f, er := os.Open(path)
				if er != nil {
					return er
				}
				var copied int64
				copied, er = copyContext(ctx, tw, f)
				closeErr := f.Close()
				if er != nil {
					return er
				}
				if copied != info.Size() {
					return fmt.Errorf("backup source %q copied %d bytes, expected %d", path, copied, info.Size())
				}
				return closeErr
			}
			return nil
		})
	}
	if er := tw.Close(); err == nil {
		err = er
	}
	if er := gz.Close(); err == nil {
		err = er
	}
	if er := tmp.Sync(); err == nil {
		err = er
	}
	if er := tmp.Close(); err == nil {
		err = er
	}
	if err != nil {
		return store.Backup{}, err
	}
	final := filepath.Join(e.dir(), name)
	if err = os.Rename(tmpName, final); err != nil {
		return store.Backup{}, err
	}
	info, err := os.Stat(final)
	if err != nil {
		return store.Backup{}, err
	}
	b, err := e.repo.AddBackup(ctx, store.Backup{File: name, CreatedAt: now, SizeBytes: info.Size(), Trigger: trigger, WorldDay: day})
	if err != nil {
		_ = os.Remove(final)
		return store.Backup{}, err
	}
	meta := map[string]any{"id": b.ID, "trigger": trigger, "worldGuid": guid, "worldSelection": selection.Source}
	if selection.Warning != "" {
		meta["warning"] = selection.Warning
		e.log.Warn("backup used active-world fallback", "worldGuid", guid, "warning", selection.Warning)
	}
	e.event("backup created", meta)
	return b, nil
}

type worldSelection struct {
	Path, GUID, Source, Warning string
}

func normalizeWorldGUID(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "{}")
	v = strings.ReplaceAll(v, "-", "")
	return strings.ToLower(v)
}

func (e *Engine) resolveWorld(ctx context.Context) (worldSelection, error) {
	if e.worldGUID == nil {
		return discoverWorldFallback(e.saveDir, "active-world REST resolver is not configured")
	}
	guid, err := e.worldGUID(ctx)
	if err != nil {
		if errors.Is(err, ErrWorldGUIDUnavailable) {
			if e.cachedWorldGUID != nil {
				if cached := strings.TrimSpace(e.cachedWorldGUID()); cached != "" {
					return e.matchWorld(cached, "poller-rest-cache", "Palworld REST is unavailable; selected the last REST-observed active world")
				}
			}
			return discoverWorldFallback(e.saveDir, "Palworld REST is unavailable; selected the newest world directory")
		}
		return worldSelection{}, fmt.Errorf("resolve active world GUID: %w", err)
	}
	return e.matchWorld(guid, "rest", "")
}

func (e *Engine) matchWorld(guid, source, warning string) (worldSelection, error) {
	normalized := normalizeWorldGUID(guid)
	if normalized == "" {
		return worldSelection{}, errors.New("resolve active world GUID: source returned an empty world GUID")
	}
	root := filepath.Join(e.saveDir, "SaveGames", "0")
	entries, err := os.ReadDir(root)
	if err != nil {
		return worldSelection{}, fmt.Errorf("discover world: %w", err)
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() && normalizeWorldGUID(entry.Name()) == normalized {
			matches = append(matches, entry.Name())
		}
	}
	if len(matches) != 1 {
		return worldSelection{}, fmt.Errorf("active world GUID %q matched %d directories under SaveGames/0", guid, len(matches))
	}
	return worldSelection{Path: filepath.Join(root, matches[0]), GUID: matches[0], Source: source, Warning: warning}, nil
}

func discoverWorldFallback(saveDir, warning string) (worldSelection, error) {
	root := filepath.Join(saveDir, "SaveGames", "0")
	entries, err := os.ReadDir(root)
	if err != nil {
		return worldSelection{}, fmt.Errorf("discover world: %w", err)
	}
	var best fs.FileInfo
	var paths []string
	for _, d := range entries {
		if !d.IsDir() {
			continue
		}
		i, er := d.Info()
		if er != nil {
			continue
		}
		if best == nil || i.ModTime().After(best.ModTime()) {
			best = i
			paths = []string{filepath.Join(root, d.Name())}
		} else if i.ModTime().Equal(best.ModTime()) {
			paths = append(paths, filepath.Join(root, d.Name()))
		}
	}
	if best == nil {
		return worldSelection{}, errors.New("no world directory found under SaveGames/0")
	}
	if len(paths) != 1 {
		return worldSelection{}, fmt.Errorf("active world unavailable and newest-directory fallback is ambiguous (%d directories share the newest timestamp)", len(paths))
	}
	return worldSelection{Path: paths[0], GUID: filepath.Base(paths[0]), Source: "newest-directory-fallback", Warning: warning}, nil
}

// Contents lists at most 10,000 archive members.
func (e *Engine) Contents(ctx context.Context, id int64) ([]Entry, error) {
	b, err := e.repo.BackupByID(ctx, id)
	if err != nil {
		return nil, err
	}
	tr, closeFn, err := openTar(filepath.Join(e.dir(), b.File))
	if err != nil {
		return nil, err
	}
	defer closeFn()
	out := []Entry{}
	limits := archiveLimits{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		h, er := tr.Next()
		if errors.Is(er, io.EOF) {
			break
		}
		if er != nil {
			return nil, er
		}
		if er = limits.add(h); er != nil {
			return nil, er
		}
		out = append(out, Entry{Path: h.Name, SizeBytes: h.Size, ModifiedAt: h.ModTime.UTC()})
	}
	return out, nil
}

// Path returns the verified archive path for streaming.
func (e *Engine) Path(ctx context.Context, id int64) (string, store.Backup, error) {
	b, err := e.repo.BackupByID(ctx, id)
	if err != nil {
		return "", b, err
	}
	p := filepath.Join(e.dir(), b.File)
	if _, err = os.Stat(p); err != nil {
		return "", b, err
	}
	return p, b, nil
}

// Delete removes an archive and index entry.
func (e *Engine) Delete(ctx context.Context, id int64) error {
	if !e.running.CompareAndSwap(false, true) {
		return ErrRunning
	}
	defer e.running.Store(false)
	return e.delete(ctx, id)
}

func (e *Engine) delete(ctx context.Context, id int64) error {
	p, _, err := e.Path(ctx, id)
	if err != nil {
		return err
	}
	if err = os.Remove(p); err != nil {
		return err
	}
	return e.repo.DeleteBackupIndex(ctx, id)
}

// DryRun safely extracts and compares an archive with the live save tree.
func (e *Engine) DryRun(ctx context.Context, id int64) (Diff, error) {
	if !e.running.CompareAndSwap(false, true) {
		return Diff{}, ErrRunning
	}
	defer e.running.Store(false)
	selection, err := e.resolveWorld(ctx)
	if err != nil {
		return Diff{}, err
	}
	p, _, err := e.Path(ctx, id)
	if err != nil {
		return Diff{}, err
	}
	tmp, err := os.MkdirTemp(e.dataDir, "restore-tmp-")
	if err != nil {
		return Diff{}, err
	}
	defer os.RemoveAll(tmp)
	m, err := extract(ctx, p, tmp)
	if err != nil {
		return Diff{}, err
	}
	arch := filepath.Join(tmp, "SaveGames", "0", m.WorldGUID)
	changes, err := compareTrees(ctx, selection.Path, arch)
	return Diff{Changes: changes, RequiresStop: true}, err
}

// Restore validates stoppage, takes a safety backup, then swaps the restored world atomically.
func (e *Engine) Restore(ctx context.Context, id int64) (Diff, error) {
	if !e.running.CompareAndSwap(false, true) {
		return Diff{}, ErrRunning
	}
	defer e.running.Store(false)
	if e.reachable != nil && e.reachable(ctx) {
		return Diff{}, errors.New("Palworld REST is reachable; stop the game server before restoring")
	}
	p, _, err := e.Path(ctx, id)
	if err != nil {
		return Diff{}, err
	}
	selection, err := e.resolveWorld(ctx)
	if err != nil {
		return Diff{}, err
	}
	if _, err = e.create(ctx, "pre-restore", false, selection); err != nil {
		return Diff{}, fmt.Errorf("pre-restore backup: %w", err)
	}
	tmp, err := os.MkdirTemp(e.dataDir, "restore-tmp-")
	if err != nil {
		return Diff{}, err
	}
	defer os.RemoveAll(tmp)
	m, err := extract(ctx, p, tmp)
	if err != nil {
		return Diff{}, err
	}
	src := filepath.Join(tmp, "SaveGames", "0", m.WorldGUID)
	if i, er := os.Stat(src); er != nil || !i.IsDir() {
		return Diff{}, errors.New("backup does not contain its manifest world directory")
	}
	live := selection.Path
	changes, err := compareTrees(ctx, live, src)
	if err != nil {
		return Diff{}, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(live), ".palhelm-restored-")
	if err != nil {
		return Diff{}, err
	}
	if err = os.Remove(staging); err != nil {
		return Diff{}, err
	}
	defer os.RemoveAll(staging)
	if err = copyTree(ctx, src, staging); err != nil {
		return Diff{}, fmt.Errorf("stage restored world: %w", err)
	}
	asid := live + fmt.Sprintf(".palhelm-restore-%d", time.Now().UnixNano())
	if err = os.Rename(live, asid); err != nil {
		return Diff{}, err
	}
	if err = os.Rename(staging, live); err != nil {
		if rollback := os.Rename(asid, live); rollback != nil {
			return Diff{}, fmt.Errorf("install restored world: %v; rollback failed: %v (original retained at %s)", err, rollback, asid)
		}
		return Diff{}, fmt.Errorf("install restored world: %w", err)
	}
	if err = os.RemoveAll(asid); err != nil {
		e.log.Warn("restore succeeded but old world cleanup failed", "path", asid, "error", err)
	}
	e.event("backup restored", map[string]any{"id": id, "changes": len(changes)})
	return Diff{Changes: changes, RequiresStop: true}, nil
}

func openTar(path string) (*tar.Reader, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return tar.NewReader(gz), func() { gz.Close(); f.Close() }, nil
}
func safeName(name string) (string, error) {
	name = strings.ReplaceAll(filepath.ToSlash(name), "\\", "/")
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("unsafe absolute tar path %q", name)
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe tar path %q", name)
	}
	return clean, nil
}

type archiveLimits struct {
	entries int
	total   int64
}

func (l *archiveLimits) add(h *tar.Header) error {
	l.entries++
	if l.entries > maxArchiveEntries {
		return fmt.Errorf("archive contains more than %d entries", maxArchiveEntries)
	}
	if h.Size < 0 {
		return fmt.Errorf("archive entry %q has a negative size", h.Name)
	}
	if h.Size > maxArchiveEntrySize {
		return fmt.Errorf("archive entry %q exceeds the %d-byte limit", h.Name, maxArchiveEntrySize)
	}
	if h.Size > maxArchiveTotalSize-l.total {
		return fmt.Errorf("archive exceeds the %d-byte extracted-size limit", maxArchiveTotalSize)
	}
	l.total += h.Size
	return nil
}

func extract(ctx context.Context, archive, dst string) (manifest, error) {
	tr, closeFn, err := openTar(archive)
	if err != nil {
		return manifest{}, err
	}
	defer closeFn()
	var m manifest
	found := false
	limits := archiveLimits{}
	for {
		if err := ctx.Err(); err != nil {
			return m, err
		}
		h, er := tr.Next()
		if errors.Is(er, io.EOF) {
			break
		}
		if er != nil {
			return m, er
		}
		if er = limits.add(h); er != nil {
			return m, er
		}
		name, er := safeName(h.Name)
		if er != nil {
			return m, er
		}
		target := filepath.Join(dst, name)
		if name == "palhelm-backup.json" {
			if h.Size > 1<<20 {
				return m, errors.New("backup manifest exceeds the 1 MiB limit")
			}
			limited := &contextReader{ctx: ctx, r: tr}
			if er = json.NewDecoder(io.LimitReader(limited, h.Size)).Decode(&m); er != nil {
				return m, er
			}
			found = true
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			er = os.MkdirAll(target, fs.FileMode(h.Mode)&0o777)
		case tar.TypeReg, tar.TypeRegA:
			if er = os.MkdirAll(filepath.Dir(target), 0o700); er == nil {
				var f *os.File
				f, er = os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.FileMode(h.Mode)&0o777)
				if er == nil {
					var copied int64
					copied, er = copyNContext(ctx, f, tr, h.Size)
					if er == nil && copied != h.Size {
						er = fmt.Errorf("archive entry %q copied %d bytes, expected %d", h.Name, copied, h.Size)
					}
					ce := f.Close()
					if er == nil {
						er = ce
					}
					if er == nil {
						er = os.Chtimes(target, h.ModTime, h.ModTime)
					}
				}
			}
		default:
			er = fmt.Errorf("unsupported tar entry %q", h.Name)
		}
		if er != nil {
			return m, er
		}
	}
	if !found || m.WorldGUID == "" {
		return m, errors.New("backup manifest is missing or invalid")
	}
	if m.WorldGUID != filepath.Base(m.WorldGUID) || m.WorldGUID == "." || m.WorldGUID == ".." {
		return m, errors.New("backup manifest contains an unsafe worldGuid")
	}
	return m, nil
}

type fileMeta struct {
	path  string
	size  int64
	mtime time.Time
}

func tree(ctx context.Context, root string) (map[string]fileMeta, error) {
	out := map[string]fileMeta{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		i, e := d.Info()
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		out[filepath.ToSlash(rel)] = fileMeta{p, i.Size(), i.ModTime()}
		return nil
	})
	return out, err
}
func compareTrees(ctx context.Context, before, after string) ([]Change, error) {
	a, err := tree(ctx, before)
	if err != nil {
		return nil, err
	}
	b, err := tree(ctx, after)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	out := []Change{}
	for _, k := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		x, xok := a[k]
		y, yok := b[k]
		switch {
		case !xok:
			v := y.size
			out = append(out, Change{Path: k, Kind: "add", ToSize: &v})
		case !yok:
			v := x.size
			out = append(out, Change{Path: k, Kind: "delete", FromSize: &v})
		default:
			equal := x.size == y.size && x.mtime.Equal(y.mtime)
			if x.size == y.size && !equal {
				hx, e := hash(ctx, x.path)
				if e != nil {
					return nil, e
				}
				hy, e := hash(ctx, y.path)
				if e != nil {
					return nil, e
				}
				equal = hx == hy
			}
			if !equal {
				u, v := x.size, y.size
				out = append(out, Change{Path: k, Kind: "modify", FromSize: &u, ToSize: &v})
			}
		}
	}
	return out, nil
}
func hash(ctx context.Context, path string) ([32]byte, error) {
	var z [32]byte
	f, e := os.Open(path)
	if e != nil {
		return z, e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = copyContext(ctx, h, f); e != nil {
		return z, e
	}
	copy(z[:], h.Sum(nil))
	return z, nil
}

func copyTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported restored file type at %s", rel)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := copyContext(ctx, out, in)
		inErr, outErr := in.Close(), out.Close()
		if copyErr != nil {
			return copyErr
		}
		if inErr != nil {
			return inErr
		}
		if outErr != nil {
			return outErr
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, &contextReader{ctx: ctx, r: src})
}

func copyNContext(ctx context.Context, dst io.Writer, src io.Reader, n int64) (int64, error) {
	return io.CopyN(dst, &contextReader{ctx: ctx, r: src}, n)
}

// GetSchedule returns persisted schedule settings with defaults.
func (e *Engine) GetSchedule(ctx context.Context) (Schedule, error) {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()
	return e.getScheduleLocked(ctx, true)
}

func (e *Engine) getScheduleLocked(ctx context.Context, persistDeadline bool) (Schedule, error) {
	s := Schedule{Enabled: true, EveryMinutes: 240, KeepDays: 30}
	raw, err := e.repo.GetKV(ctx, "backups_schedule")
	if err == nil {
		if err = json.Unmarshal([]byte(raw), &s); err != nil {
			return s, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	if s.EveryMinutes < 1 || s.KeepDays < 1 {
		return s, errors.New("persisted backup schedule is invalid")
	}
	changed := false
	if !s.Enabled && s.NextRunAt != nil {
		s.NextRunAt = nil
		changed = true
	}
	if s.Enabled && s.NextRunAt == nil {
		next := e.clock.Now().UTC().Add(time.Duration(s.EveryMinutes) * time.Minute)
		s.NextRunAt = &next
		changed = true
	}
	if changed && persistDeadline {
		if err := e.persistScheduleLocked(ctx, s); err != nil {
			return s, err
		}
	}
	return s, nil
}

// PutSchedule validates and persists schedule settings.
func (e *Engine) PutSchedule(ctx context.Context, s Schedule) (Schedule, error) {
	if s.EveryMinutes < 1 || s.KeepDays < 1 {
		return s, errors.New("everyMinutes and keepDays must be positive")
	}
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()
	if s.Enabled {
		next := e.clock.Now().UTC().Add(time.Duration(s.EveryMinutes) * time.Minute)
		s.NextRunAt = &next
	} else {
		s.NextRunAt = nil
	}
	if err := e.persistScheduleLocked(ctx, s); err != nil {
		return s, err
	}
	e.scheduleRevision.Add(1)
	e.signalScheduleReset()
	return s, nil
}

func (e *Engine) persistScheduleLocked(ctx context.Context, s Schedule) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return e.repo.SetKV(ctx, "backups_schedule", string(b))
}

func (e *Engine) signalScheduleReset() {
	select {
	case e.scheduleReset <- struct{}{}:
	default:
	}
}

// Run executes the dynamic scheduler until cancellation.
func (e *Engine) Run(ctx context.Context) {
	_ = e.Reconcile(ctx)
	for {
		schedule, err := e.GetSchedule(ctx)
		if err != nil {
			e.log.Error("load backup schedule", "error", err)
			next := e.clock.Now().UTC().Add(240 * time.Minute)
			schedule = Schedule{Enabled: true, EveryMinutes: 240, KeepDays: 30, NextRunAt: &next}
		}
		if !schedule.Enabled {
			select {
			case <-ctx.Done():
				return
			case <-e.scheduleReset:
				continue
			}
		}
		wait := schedule.NextRunAt.Sub(e.clock.Now())
		if wait < 0 {
			wait = 0
		}
		timer := e.clock.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-e.scheduleReset:
			timer.Stop()
			continue
		case <-timer.Chan():
		}
		current, loadErr := e.GetSchedule(ctx)
		if loadErr != nil {
			e.log.Error("reload backup schedule", "error", loadErr)
			continue
		}
		if !current.Enabled || current.NextRunAt == nil || current.NextRunAt.After(e.clock.Now()) {
			continue
		}
		revision := e.scheduleRevision.Load()
		if _, err = e.Create(ctx, "scheduled"); err != nil {
			e.log.Error("scheduled backup failed", "error", err)
		}
		e.advanceSchedule(ctx, current, revision)
	}
}

func (e *Engine) advanceSchedule(ctx context.Context, previous Schedule, revision uint64) {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()
	if e.scheduleRevision.Load() != revision {
		return
	}
	current, err := e.getScheduleLocked(ctx, false)
	if err != nil || !current.Enabled || current.NextRunAt == nil || previous.NextRunAt == nil || current.EveryMinutes != previous.EveryMinutes || !current.NextRunAt.Equal(*previous.NextRunAt) {
		return
	}
	interval := time.Duration(current.EveryMinutes) * time.Minute
	now := e.clock.Now().UTC()
	next := current.NextRunAt.Add(interval)
	for !next.After(now) {
		next = next.Add(interval)
	}
	current.NextRunAt = &next
	if err := e.persistScheduleLocked(ctx, current); err != nil {
		e.log.Error("persist next backup deadline", "error", err)
	}
}

// Prune removes archives older than keepDays.
func (e *Engine) Prune(ctx context.Context, keepDays int) error {
	if !e.running.CompareAndSwap(false, true) {
		return ErrRunning
	}
	defer e.running.Store(false)
	return e.prune(ctx, keepDays)
}

func (e *Engine) prune(ctx context.Context, keepDays int) error {
	rows, err := e.repo.Backups(ctx)
	if err != nil {
		return err
	}
	cut := e.clock.Now().Add(-time.Duration(keepDays) * 24 * time.Hour)
	for _, b := range rows {
		if b.CreatedAt.Before(cut) {
			if err = e.delete(ctx, b.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
func (e *Engine) event(message string, meta any) {
	if e.emit != nil {
		e.emit(message, meta)
	}
}
