package sav

import (
	"encoding/binary"
	"math"
	"testing"
)

// encodeBaseRawData builds a PalBaseCampSaveData.RawData blob in the proven
// retail 1.x layout: id GUID, name fstring, state byte, FTransform (rotation
// quaternion + translation + scale, all f64), area_range f32, group GUID, and
// some trailing bytes the decoder must ignore.
func encodeBaseRawData(idBytes [16]byte, name string, tx, ty, tz float64) []byte {
	var b []byte
	f64 := func(v float64) {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
		b = append(b, buf[:]...)
	}
	b = append(b, idBytes[:]...) // id GUID
	// name: positive-length ANSI fstring (length includes the null terminator).
	var l [4]byte
	binary.LittleEndian.PutUint32(l[:], uint32(len(name)+1))
	b = append(b, l[:]...)
	b = append(b, name...)
	b = append(b, 0)
	b = append(b, 0x01) // state byte
	f64(0.0)            // quat.x
	f64(0.0)            // quat.y
	f64(0.7071)         // quat.z
	f64(0.7071)         // quat.w
	f64(tx)             // translation.x
	f64(ty)             // translation.y
	f64(tz)             // translation.z
	f64(1.0)            // scale.x
	f64(1.0)            // scale.y
	f64(1.0)            // scale.z
	var area [4]byte
	binary.LittleEndian.PutUint32(area[:], math.Float32bits(3500))
	b = append(b, area[:]...)          // area_range f32
	b = append(b, make([]byte, 16)...) // group_id_belong_to GUID
	b = append(b, make([]byte, 40)...) // trailing worker/module bytes to ignore
	return b
}

func TestBaseLocationDecodesTranslation(t *testing.T) {
	id := [16]byte{0x00, 0xd9, 0x34, 0x5d, 0x4e, 0x43, 0xa4, 0xea, 0x24, 0x48, 0xe6, 0x99, 0xcf, 0xb5, 0x3c, 0x79}
	raw := encodeBaseRawData(id, "拠点テンプレート", -304214.09, 227626.09, 2883.5)

	// The embedded GUID as readGUID renders it is the canonical key; matching it
	// is part of the decoder's contract, so derive it the same way.
	key, err := readGUID(newReader(id[:]))
	if err != nil {
		t.Fatal(err)
	}

	loc, ok := baseLocation(raw, key)
	if !ok {
		t.Fatalf("baseLocation returned !ok for a well-formed blob")
	}
	if math.Abs(loc.X-(-304214.09)) > 1e-3 || math.Abs(loc.Y-227626.09) > 1e-3 || math.Abs(loc.Z-2883.5) > 1e-3 {
		t.Fatalf("decoded translation = (%.3f,%.3f,%.3f), want (-304214.09,227626.09,2883.5)", loc.X, loc.Y, loc.Z)
	}

	// An empty baseID skips the key check and must still decode.
	if loc2, ok := baseLocation(raw, ""); !ok || loc2.X != loc.X {
		t.Fatalf("baseLocation with empty baseID = %v,%v", loc2, ok)
	}
}

func TestBaseLocationRejectsDrift(t *testing.T) {
	id := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	key, _ := readGUID(newReader(id[:]))
	raw := encodeBaseRawData(id, "camp", 100, 200, 300)

	// A GUID that does not match the map key is treated as drift, not a location.
	if _, ok := baseLocation(raw, "ffffffff-ffff-ffff-ffff-ffffffffffff"); ok {
		t.Fatalf("baseLocation accepted a blob whose embedded GUID mismatched the key")
	}
	// A buffer truncated before the translation must fail closed, not read garbage.
	if _, ok := baseLocation(raw[:40], key); ok {
		t.Fatalf("baseLocation accepted a truncated blob")
	}
	// Empty input must not panic and must fail closed.
	if _, ok := baseLocation(nil, key); ok {
		t.Fatalf("baseLocation accepted nil input")
	}
}

// TestBaseFromEntryDecodesRawDataPosition proves the property-tree path: a base
// entry carrying only a RawData byte property (no plain Position/Location
// property, as retail 1.x saves are shaped) yields a decoded Position.
func TestBaseFromEntryDecodesRawDataPosition(t *testing.T) {
	id := [16]byte{0xaa, 0xbb, 0xcc, 0xdd, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	key, _ := readGUID(newReader(id[:]))
	raw := encodeBaseRawData(id, "camp", -12345.5, 67890.25, -42)
	entry := mapEntry{
		Key: key,
		Value: propertyMap{
			"RawData": &property{Value: raw},
		},
	}
	stats := newStats()
	base := baseFromEntry(entry, &stats)
	if base.Position == nil {
		t.Fatalf("baseFromEntry did not decode a Position from RawData")
	}
	if base.Position.X != -12345.5 || base.Position.Y != 67890.25 || base.Position.Z != -42 {
		t.Fatalf("baseFromEntry Position = %+v, want (-12345.5,67890.25,-42)", *base.Position)
	}

	// A base whose RawData cannot be decoded must yield a nil Position (served as
	// null), never a zero vector, and record the tolerated skip.
	badStats := newStats()
	bad := baseFromEntry(mapEntry{Key: key, Value: propertyMap{"RawData": &property{Value: []byte{1, 2, 3}}}}, &badStats)
	if bad.Position != nil {
		t.Fatalf("undecodable RawData produced a non-nil Position %+v", *bad.Position)
	}
	if badStats.SkippedProperties == 0 {
		t.Fatalf("undecodable base transform was not recorded as a tolerated skip")
	}
}
