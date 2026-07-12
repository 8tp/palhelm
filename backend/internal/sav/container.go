package sav

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
)

const (
	defaultMaxSaveBytes int64 = 512 << 20
	hardMaxSaveBytes    int64 = 2 << 30
)

type containerHeader struct {
	RawLen, CompressedLen uint32
	Magic                 string
	SaveType              byte
	Offset                int
}

func readContainer(data []byte) ([]byte, containerHeader, error) {
	h, err := parseContainerHeader(data)
	if err != nil {
		return nil, h, err
	}
	maxBytes, err := maxSaveBytes()
	if err != nil {
		return nil, h, err
	}
	if int64(h.RawLen) > maxBytes {
		return nil, h, &parseLimitError{Kind: "decompressed save bytes", Value: uint64(h.RawLen), Limit: uint64(maxBytes)}
	}
	bodyStart := h.Offset + 12
	if int(h.CompressedLen) > len(data)-bodyStart {
		return nil, h, fmt.Errorf("sav: compressed length %d exceeds %d-byte body", h.CompressedLen, len(data)-bodyStart)
	}
	src := data[bodyStart : bodyStart+int(h.CompressedLen)]
	var raw []byte
	switch h.Magic {
	case "PlZ":
		if h.SaveType != 0x31 && h.SaveType != 0x32 {
			return nil, h, fmt.Errorf("sav: unsupported PlZ save type %#x", h.SaveType)
		}
		raw, err = zlibBytes(src, int64(h.RawLen))
		if err == nil && h.SaveType == 0x32 {
			raw, err = zlibBytes(raw, int64(h.RawLen))
		}
	case "PlM":
		if h.SaveType != 0x31 {
			return nil, h, fmt.Errorf("sav: unsupported PlM save type %#x", h.SaveType)
		}
		raw, err = oodleDecompress(src, int(h.RawLen))
	default:
		err = fmt.Errorf("sav: unsupported container magic %q", h.Magic)
	}
	if err != nil {
		return nil, h, err
	}
	if len(raw) != int(h.RawLen) {
		return nil, h, fmt.Errorf("sav: decompressed length %d, expected %d", len(raw), h.RawLen)
	}
	if !bytes.HasPrefix(raw, []byte("GVAS")) {
		return nil, h, fmt.Errorf("sav: decompressed body does not begin with GVAS")
	}
	return raw, h, nil
}

func parseContainerHeader(data []byte) (containerHeader, error) {
	for _, base := range []int{0, 12} {
		if len(data) < base+12 {
			continue
		}
		magic := string(data[base+8 : base+11])
		if magic != "PlZ" && magic != "PlM" {
			continue
		}
		return containerHeader{
			RawLen: binary.LittleEndian.Uint32(data[base:]), CompressedLen: binary.LittleEndian.Uint32(data[base+4:]),
			Magic: magic, SaveType: data[base+11], Offset: base,
		}, nil
	}
	return containerHeader{}, fmt.Errorf("sav: PlZ/PlM header not found at normal or CNK offset")
}

func zlibBytes(src []byte, rawLen int64) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("sav: zlib: %w", err)
	}
	defer zr.Close()
	b, err := io.ReadAll(io.LimitReader(zr, rawLen+1))
	if err != nil {
		return nil, fmt.Errorf("sav: zlib: %w", err)
	}
	if int64(len(b)) > rawLen {
		return nil, fmt.Errorf("sav: zlib output exceeds declared raw length %d", rawLen)
	}
	return b, nil
}

func readSave(path string) ([]byte, error) {
	maxBytes, err := maxSaveBytes()
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("sav: stat %s: %w", path, err)
	}
	if st.Size() > maxBytes {
		return nil, &parseLimitError{Kind: "save file bytes", Value: uint64(st.Size()), Limit: uint64(maxBytes)}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sav: read %s: %w", path, err)
	}
	if int64(len(b)) > maxBytes {
		return nil, &parseLimitError{Kind: "save file bytes", Value: uint64(len(b)), Limit: uint64(maxBytes)}
	}
	raw, _, err := readContainer(b)
	return raw, err
}

func maxSaveBytes() (int64, error) {
	s := os.Getenv("PALHELM_SAV_MAX_BYTES")
	if s == "" {
		return defaultMaxSaveBytes, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("sav: invalid PALHELM_SAV_MAX_BYTES %q", s)
	}
	if n > hardMaxSaveBytes {
		return 0, fmt.Errorf("sav: PALHELM_SAV_MAX_BYTES %d exceeds hard cap %d", n, hardMaxSaveBytes)
	}
	return n, nil
}
