package server

import (
	"net/http"
	"syscall"
)

// diskStatFunc reports the total and available bytes of the filesystem backing a
// path. It is a field on Server so tests can drive the stat-failure branch.
type diskStatFunc func(path string) (total, avail uint64, err error)

// statfsDiskUsage reads real filesystem capacity via statfs(2). Total and available
// bytes come from the block count and block size reported by the kernel.
func statfsDiskUsage(path string) (total, avail uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return st.Blocks * bsize, st.Bavail * bsize, nil
}

// backupStorage reports the real disk capacity and free space of the filesystem
// holding the backup volume. Host paths are never exposed. If the stat fails the
// fields are reported as null so callers degrade to the bytes they already know.
func (s *Server) backupStorage(w http.ResponseWriter, r *http.Request) {
	total, avail, err := s.diskStat(s.backups.Dir())
	if err != nil {
		s.log.Warn("backup storage statfs failed", "error", err)
		writeJSON(w, 200, map[string]any{"totalBytes": nil, "freeBytes": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"totalBytes": total, "freeBytes": avail})
}
