package sav

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
)

func TestZlibContainers(t *testing.T) {
	raw := append([]byte("GVAS"), bytes.Repeat([]byte(" tiny fixture"), 32)...)
	for _, tc := range []struct {
		name     string
		saveType byte
		cnk      bool
	}{{"once", 0x31, false}, {"twice", 0x32, false}, {"cnk", 0x31, true}} {
		t.Run(tc.name, func(t *testing.T) {
			body := zlibFixture(raw)
			if tc.saveType == 0x32 {
				body = zlibFixture(body)
			}
			prefix := 0
			if tc.cnk {
				prefix = 12
			}
			b := make([]byte, prefix+12+len(body))
			copy(b[prefix+8:], []byte("PlZ"))
			b[prefix+11] = tc.saveType
			binary.LittleEndian.PutUint32(b[prefix:], uint32(len(raw)))
			binary.LittleEndian.PutUint32(b[prefix+4:], uint32(len(body)))
			copy(b[prefix+12:], body)
			got, h, err := readContainer(b)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, raw) {
				t.Fatalf("got %q, want %q", got, raw)
			}
			if h.Offset != prefix {
				t.Fatalf("offset %d, want %d", h.Offset, prefix)
			}
		})
	}
}

func TestHeaderRejectsBadMagic(t *testing.T) {
	if _, err := parseContainerHeader(make([]byte, 32)); err == nil {
		t.Fatal("expected bad-magic error")
	}
}

func TestParsePlMHeader(t *testing.T) {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b, 1234)
	binary.LittleEndian.PutUint32(b[4:], 99)
	copy(b[8:], "PlM")
	b[11] = 0x31
	h, err := parseContainerHeader(b)
	if err != nil {
		t.Fatal(err)
	}
	if h.Magic != "PlM" || h.RawLen != 1234 || h.CompressedLen != 99 || h.SaveType != 0x31 {
		t.Fatalf("unexpected header: %#v", h)
	}
}

func TestTruncatedGVASReturnsError(t *testing.T) {
	for n := 0; n < 64; n++ {
		data := make([]byte, n)
		copy(data, "GVAS")
		stats := newStats()
		if _, err := parseGVAS(data, &stats); err == nil {
			t.Fatalf("%d-byte GVAS unexpectedly parsed", n)
		}
	}
}

func zlibFixture(raw []byte) []byte {
	var b bytes.Buffer
	zw := zlib.NewWriter(&b)
	_, _ = zw.Write(raw)
	_ = zw.Close()
	return b.Bytes()
}
