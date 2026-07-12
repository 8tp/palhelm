package sav

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTruncatedFixtureHeader(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "Level.gvas"))
	if err != nil {
		t.Fatal(err)
	}
	r := newReader(data)
	if _, err := readGVASHeader(r); err != nil {
		t.Fatal(err)
	}
	headerEnd := r.position()
	for n := 0; n < headerEnd; n += 16 {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			stats := newStats()
			if _, err := parseGVAS(data[:n], &stats); err == nil {
				t.Fatalf("%d-byte truncated fixture unexpectedly parsed", n)
			}
		})
	}
}

func TestHostileCountsFailWithoutLargeAllocations(t *testing.T) {
	const hostile = ^uint32(0)
	tests := []struct {
		name string
		fn   func() error
	}{
		{"array", func() error {
			r := newReader(append(ansiFString("IntProperty"), 0, 0xff, 0xff, 0xff, 0xff))
			_, err := readArray(r, 0, ".array", ptrStats())
			return err
		}},
		{"map", func() error {
			b := append(ansiFString("IntProperty"), ansiFString("IntProperty")...)
			b = append(b, make([]byte, 9)...)
			binary.LittleEndian.PutUint32(b[len(b)-4:], hostile)
			_, err := readMap(newReader(b), ".map", ptrStats())
			return err
		}},
		{"string", func() error {
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, hostile>>1)
			_, err := newReader(b).fstring()
			return err
		}},
		{"custom-version", func() error {
			b := minimalGVASHeader(hostile)
			_, err := readGVASHeader(newReader(b))
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatal("hostile count unexpectedly accepted")
			}
			allocs := testing.AllocsPerRun(10, func() { _ = tc.fn() })
			if allocs > 20 {
				t.Fatalf("hostile count used %.1f allocations", allocs)
			}
		})
	}
}

func TestZlibOutputCannotExceedRawLength(t *testing.T) {
	raw := append([]byte("GVAS"), bytes.Repeat([]byte("bomb"), 1024)...)
	compressed := zlibFixture(raw)
	b := make([]byte, 12+len(compressed))
	binary.LittleEndian.PutUint32(b, 16)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(compressed)))
	copy(b[8:], "PlZ")
	b[11] = 0x31
	copy(b[12:], compressed)
	if _, _, err := readContainer(b); err == nil || !strings.Contains(err.Error(), "exceeds declared raw length") {
		t.Fatalf("got %v, want raw-length error", err)
	}
}

func TestResolveOodleLibraryOrderAndDownload(t *testing.T) {
	originalURL, originalHash := oodleDownloadURL, oodleExpectedHash
	t.Cleanup(func() {
		oodleDownloadURL, oodleExpectedHash = originalURL, originalHash
	})

	t.Run("explicit wins", func(t *testing.T) {
		dataDir := t.TempDir()
		dataLib := filepath.Join(dataDir, oodleLibrary)
		if err := os.WriteFile(dataLib, []byte("data"), 0o755); err != nil {
			t.Fatal(err)
		}
		explicit := filepath.Join(t.TempDir(), "explicit.so")
		if err := os.WriteFile(explicit, []byte("explicit"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PALHELM_DATA_DIR", dataDir)
		t.Setenv("PALHELM_OODLE_LIB", explicit)
		got, err := resolveOodleLibrary()
		if err != nil || got != explicit {
			t.Fatalf("resolve = %q, %v; want %q", got, err, explicit)
		}
	})

	t.Run("explicit must be absolute", func(t *testing.T) {
		t.Setenv("PALHELM_OODLE_LIB", "relative.so")
		if _, err := resolveOodleLibrary(); err == nil {
			t.Fatal("relative explicit path unexpectedly accepted")
		}
	})

	t.Run("data directory", func(t *testing.T) {
		t.Setenv("PALHELM_OODLE_LIB", "")
		dataDir := t.TempDir()
		lib := filepath.Join(dataDir, oodleLibrary)
		if err := os.WriteFile(lib, []byte("data"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PALHELM_DATA_DIR", dataDir)
		got, err := resolveOodleLibrary()
		if err != nil || got != lib {
			t.Fatalf("resolve = %q, %v; want %q", got, err, lib)
		}
	})

	t.Run("verified atomic download", func(t *testing.T) {
		t.Setenv("PALHELM_OODLE_LIB", "")
		artifact := []byte("test oodle artifact")
		sum := sha256.Sum256(artifact)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(artifact)
		}))
		defer server.Close()
		oodleDownloadURL = server.URL
		oodleExpectedHash = fmt.Sprintf("%x", sum[:])
		dataDir := t.TempDir()
		t.Setenv("PALHELM_DATA_DIR", dataDir)
		got, err := resolveOodleLibrary()
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(dataDir, oodleLibrary) {
			t.Fatalf("path %q", got)
		}
		stored, err := os.ReadFile(got)
		if err != nil || !bytes.Equal(stored, artifact) {
			t.Fatalf("stored artifact = %q, %v", stored, err)
		}
		if matches, _ := filepath.Glob(filepath.Join(dataDir, ".oodle.tmp-*")); len(matches) != 0 {
			t.Fatalf("temporary files remain: %v", matches)
		}
	})

	t.Run("hash mismatch cleans temp", func(t *testing.T) {
		t.Setenv("PALHELM_OODLE_LIB", "")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("wrong"))
		}))
		defer server.Close()
		oodleDownloadURL = server.URL
		oodleExpectedHash = strings.Repeat("0", 64)
		dataDir := t.TempDir()
		t.Setenv("PALHELM_DATA_DIR", dataDir)
		if _, err := resolveOodleLibrary(); err == nil {
			t.Fatal("bad digest unexpectedly accepted")
		}
		matches, _ := filepath.Glob(filepath.Join(dataDir, ".oodle.tmp-*"))
		if len(matches) != 0 {
			t.Fatalf("temporary files remain: %v", matches)
		}
	})
}

func TestPropertyDepthLimitIsTyped(t *testing.T) {
	r := newReader(nil)
	r.propertyDepth = maxPropertyDepth
	_, err := readProperties(r, ".nested", ptrStats())
	var limitErr *parseLimitError
	if !errors.As(err, &limitErr) || limitErr.Kind != "property depth" {
		t.Fatalf("got %v, want typed depth error", err)
	}
}

func TestUTF16FStringRequiresTerminator(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b, ^uint32(1))
	binary.LittleEndian.PutUint16(b[4:], 'x')
	binary.LittleEndian.PutUint16(b[6:], 'y')
	if _, err := newReader(b).fstring(); err == nil {
		t.Fatal("unterminated UTF-16 FString unexpectedly accepted")
	}
}

func TestUnknownPropertyConsumesOptionalGUIDBeforePayload(t *testing.T) {
	payload := []byte{1, 2, 3}
	b := append([]byte{1}, make([]byte, 16)...)
	b = append(b, payload...)
	r := newReader(b)
	p, err := readProperty(r, "FutureProperty", uint64(len(payload)), ".future", ptrStats())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.Value.([]byte); !ok || !bytes.Equal(got, payload) {
		t.Fatalf("payload = %#v, want %v", p.Value, payload)
	}
	if r.remaining() != 0 {
		t.Fatalf("%d bytes remain", r.remaining())
	}
}

func TestDecodedBudgetRejectsBoxedMapBeforeAllocation(t *testing.T) {
	stats := ptrStats()
	stats.decodedNodes = maxDecodedNodes - 2
	b := append(ansiFString("BoolProperty"), ansiFString("BoolProperty")...)
	b = append(b, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1)
	_, err := readMap(newReaderWithStats(b, stats), ".map", stats)
	var limitErr *parseLimitError
	if !errors.As(err, &limitErr) || !strings.Contains(limitErr.Kind, "decoded nodes") {
		t.Fatalf("got %v, want decoded-node limit", err)
	}
}

func TestDecodedByteBudgetCoversCustomBlob(t *testing.T) {
	stats := ptrStats()
	stats.decodedBytes = maxDecodedBytes - 2
	b := append(ansiFString("ByteProperty"), 0, 3, 0, 0, 0, 1, 2, 3)
	_, err := readArray(newReaderWithStats(b, stats), 0, ".blob", stats)
	var limitErr *parseLimitError
	if !errors.As(err, &limitErr) || !strings.Contains(limitErr.Kind, "decoded bytes") {
		t.Fatalf("got %v, want decoded-byte limit", err)
	}
}

func TestKnownOpaqueByteBlobDoesNotIncrementSkipStats(t *testing.T) {
	stats := ptrStats()
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	b := append(ansiFString("ByteProperty"), 0, byte(len(payload)), 0, 0, 0)
	b = append(b, payload...)
	v, err := readArray(newReaderWithStats(b, stats), uint64(len(payload)), ".worldSaveData.KnownOpaque.RawData", stats)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v.([]byte); !ok || !bytes.Equal(got, payload) {
		t.Fatalf("opaque payload = %#v", v)
	}
	if stats.SkippedProperties != 0 || stats.SkippedStructs != 0 || len(stats.DecodeFailures) != 0 {
		t.Fatalf("known opaque blob reported format drift stats: %#v", stats)
	}
}

func TestPrimitiveArraysAvoidInterfaceBoxing(t *testing.T) {
	b := append(ansiFString("BoolProperty"), 0, 3, 0, 0, 0, 0, 1, 1)
	v, err := readArray(newReader(b), 0, ".bools", ptrStats())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v.([]bool); !ok || len(got) != 3 || got[0] || !got[1] || !got[2] {
		t.Fatalf("decoded bool array = %#v", v)
	}
}

func TestLargeEarlyNoneTrailerIsNotDuplicated(t *testing.T) {
	header := minimalGVASHeader(0)
	header = append(header, 0, 0, 0, 0) // empty class name
	header = append(header, ansiFString("None")...)
	const trailerSize = 8 << 20
	data := append(header, bytes.Repeat([]byte{0xa5}, trailerSize)...)
	stats := ptrStats()
	g, err := parseGVAS(data, stats)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Trailer) != trailerSize {
		t.Fatalf("trailer length = %d, want %d", len(g.Trailer), trailerSize)
	}
	if &g.Trailer[0] != &data[len(header)] {
		t.Fatal("GVAS trailer was copied instead of aliasing the decompressed input")
	}
	allocs := testing.AllocsPerRun(10, func() {
		runStats := ptrStats()
		if _, parseErr := parseGVAS(data, runStats); parseErr != nil {
			panic(parseErr)
		}
	})
	if allocs > 25 {
		t.Fatalf("large trailer parse used %.1f allocations; want size-independent parsing", allocs)
	}
}

func FuzzContainerHeaders(f *testing.F) {
	f.Add([]byte("short"))
	f.Add(make([]byte, 32))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _ = parseContainerHeader(data)
	})
}

func FuzzFString(f *testing.F) {
	f.Add(ansiFString("hello"))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _ = newReader(data).fstring()
	})
}

func FuzzPropertyTree(f *testing.F) {
	f.Add(ansiFString("None"))
	f.Add([]byte{1, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		stats := ptrStats()
		_, _ = readProperties(newReaderWithStats(data, stats), ".fuzz", stats)
	})
}

func FuzzArrayAndMap(f *testing.F) {
	f.Add(uint8(0), []byte{})
	f.Add(uint8(1), []byte{})
	f.Fuzz(func(t *testing.T, kind uint8, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		stats := ptrStats()
		r := newReaderWithStats(data, stats)
		if kind&1 == 0 {
			_, _ = readArray(r, uint64(len(data)), ".fuzz", stats)
		} else {
			_, _ = readMap(r, ".fuzz", stats)
		}
	})
}

func FuzzCustomBlobs(f *testing.F) {
	f.Add(uint8(0), []byte{})
	f.Add(uint8(1), make([]byte, 32))
	f.Fuzz(func(t *testing.T, kind uint8, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		stats := ptrStats()
		if kind&1 == 0 {
			_, _ = decodeCharacter(data, stats)
		} else {
			_, _ = decodeGroup(data, groupGuild, stats)
		}
	})
}

func ptrStats() *ParseStats {
	s := newStats()
	return &s
}

func ansiFString(s string) []byte {
	b := make([]byte, 4+len(s)+1)
	binary.LittleEndian.PutUint32(b, uint32(len(s)+1))
	copy(b[4:], s)
	return b
}

func minimalGVASHeader(customCount uint32) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint32(0x53415647))
	_ = binary.Write(&b, binary.LittleEndian, int32(3))
	_ = binary.Write(&b, binary.LittleEndian, int32(0))
	_ = binary.Write(&b, binary.LittleEndian, int32(0))
	_ = binary.Write(&b, binary.LittleEndian, uint16(0))
	_ = binary.Write(&b, binary.LittleEndian, uint16(0))
	_ = binary.Write(&b, binary.LittleEndian, uint16(0))
	_ = binary.Write(&b, binary.LittleEndian, uint32(0))
	_ = binary.Write(&b, binary.LittleEndian, int32(0)) // empty branch
	_ = binary.Write(&b, binary.LittleEndian, int32(3))
	_ = binary.Write(&b, binary.LittleEndian, customCount)
	return b.Bytes()
}
