// Package oodle provides the decode API from new-world-tools/go-oodle without
// its accidental C.GoString dependency.
package oodle

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var library struct {
	sync.Once
	err        error
	decompress func(unsafe.Pointer, int, unsafe.Pointer, int64, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) uintptr
}

// LoadFrom loads Oodle from exactly absPath. The first call, successful or not,
// fixes the process-wide result used by Decompress.
func LoadFrom(absPath string) error {
	library.Do(func() {
		library.err = loadFrom(absPath)
	})
	return library.err
}

func loadFrom(absPath string) error {
	if !filepath.IsAbs(absPath) {
		return fmt.Errorf("oodle: library path must be absolute: %q", absPath)
	}
	st, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("oodle: stat %s: %w", absPath, err)
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("oodle: library path is not a regular file: %s", absPath)
	}
	h, err := purego.Dlopen(absPath, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return fmt.Errorf("oodle: load %s: %w", absPath, err)
	}
	sym, err := purego.Dlsym(h, "OodleLZ_Decompress")
	if err != nil {
		_ = purego.Dlclose(h)
		return fmt.Errorf("oodle: resolve OodleLZ_Decompress in %s: %w", absPath, err)
	}
	purego.RegisterFunc(&library.decompress, sym)
	return nil
}

// Decompress expands an Oodle stream to outputSize bytes.
func Decompress(input []byte, outputSize int64) ([]byte, error) {
	if len(input) == 0 || outputSize <= 0 {
		return nil, fmt.Errorf("oodle: invalid input/output size")
	}
	if library.err != nil {
		return nil, library.err
	}
	if library.decompress == nil {
		return nil, fmt.Errorf("oodle: library has not been loaded")
	}
	out := make([]byte, outputSize)
	n := library.decompress(unsafe.Pointer(&input[0]), len(input), unsafe.Pointer(&out[0]), outputSize, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3)
	if n == 0 {
		return nil, fmt.Errorf("oodle: decompression failed")
	}
	if int64(n) != outputSize {
		return nil, fmt.Errorf("oodle: decompressed %d bytes, expected %d", n, outputSize)
	}
	return out, nil
}
