package sav

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	oodle "github.com/new-world-tools/go-oodle"
)

const (
	oodleLibrary          = "liboo2corelinux64.so.9"
	defaultOodleURL       = "https://github.com/new-world-tools/go-oodle/releases/download/v0.2.3-files/liboo2corelinux64.so.9"
	maxOodleDownloadBytes = 64 << 20
	// SHA-256 of the official go-oodle v0.2.3-files release artifact.
	oodleArtifactSHA256 = "7354655eb25b587dc34cbf98696b91e30e6d7a3f0eefad3872e6c1b76ef86a6e"
)

// Variables allow the download path to be exercised with an httptest server
// and a test-only digest without reaching the network.
var (
	oodleDownloadURL  = defaultOodleURL
	oodleExpectedHash = oodleArtifactSHA256
)

var oodleSetup struct {
	sync.Once
	err error
}

func oodleDecompress(src []byte, rawLen int) ([]byte, error) {
	oodleSetup.Do(func() { oodleSetup.err = prepareOodle() })
	if oodleSetup.err != nil {
		return nil, oodleSetup.err
	}
	out, err := oodle.Decompress(src, int64(rawLen))
	if err != nil {
		return nil, fmt.Errorf("sav: Oodle decompress: %w", err)
	}
	return out, nil
}

func prepareOodle() error {
	path, err := resolveOodleLibrary()
	if err != nil {
		return err
	}
	if err := oodle.LoadFrom(path); err != nil {
		return fmt.Errorf("sav: load Oodle library: %w", err)
	}
	return nil
}

func resolveOodleLibrary() (string, error) {
	if explicit := os.Getenv("PALHELM_OODLE_LIB"); explicit != "" {
		if !filepath.IsAbs(explicit) {
			return "", fmt.Errorf("sav: PALHELM_OODLE_LIB must be an absolute path: %q", explicit)
		}
		if err := validateOodleFile(explicit); err != nil {
			return "", fmt.Errorf("sav: PALHELM_OODLE_LIB: %w", err)
		}
		return explicit, nil
	}

	dataDir := os.Getenv("PALHELM_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("sav: resolve data directory: %w", err)
	}
	dest := filepath.Join(absDataDir, oodleLibrary)
	if err := validateOodleFile(dest); err == nil {
		return dest, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("sav: Oodle library: %w", err)
	}

	log.Printf("palhelm: Oodle library missing; downloading %s into %s", oodleLibrary, absDataDir)
	if err := downloadOodle(absDataDir, dest); err != nil {
		return "", fmt.Errorf("sav: download Oodle library: %w", err)
	}
	return dest, nil
}

func validateOodleFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}

func downloadOodle(dataDir, dest string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	tmp, err := os.CreateTemp(dataDir, ".oodle.tmp-")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(oodleDownloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download returned %s", resp.Status)
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxOodleDownloadBytes+1))
	if err != nil {
		return err
	}
	if n > maxOodleDownloadBytes {
		return fmt.Errorf("download exceeds %d bytes", maxOodleDownloadBytes)
	}
	got := fmt.Sprintf("%x", h.Sum(nil))
	if got != oodleExpectedHash {
		return fmt.Errorf("SHA-256 mismatch: got %s, want %s", got, oodleExpectedHash)
	}
	if err := tmp.Chmod(0o755); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return err
	}
	keep = true
	return nil
}
