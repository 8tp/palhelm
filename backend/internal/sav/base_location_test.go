package sav

import (
	"encoding/binary"
	"math"
	"testing"
	"unicode/utf16"
)

// encodeBaseRawData builds a PalBaseCampSaveData.RawData blob in the proven
// retail 1.x layout: id GUID, name fstring, state byte, FTransform (rotation
// quaternion + translation + scale, all f64), area_range f32, group GUID, and
// some trailing bytes the decoder must ignore. wide selects the UTF-16 fstring
// encoding retail saves use for base names; false writes the ANSI form.
func encodeBaseRawData(idBytes [16]byte, name string, wide bool, tx, ty, tz float64) []byte {
	var b []byte
	f64 := func(v float64) {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
		b = append(b, buf[:]...)
	}
	b = append(b, idBytes[:]...) // id GUID
	var l [4]byte
	if wide {
		// UTF-16 fstring: negative unit count including the null terminator.
		units := utf16.Encode([]rune(name))
		binary.LittleEndian.PutUint32(l[:], uint32(-(int32(len(units)) + 1)))
		b = append(b, l[:]...)
		for _, u := range units {
			var ub [2]byte
			binary.LittleEndian.PutUint16(ub[:], u)
			b = append(b, ub[:]...)
		}
		b = append(b, 0, 0)
	} else {
		// ANSI fstring: positive byte length including the null terminator.
		binary.LittleEndian.PutUint32(l[:], uint32(len(name)+1))
		b = append(b, l[:]...)
		b = append(b, name...)
		b = append(b, 0)
	}
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

func TestDecodeBaseRawTranslationAndName(t *testing.T) {
	id := [16]byte{0x00, 0xd9, 0x34, 0x5d, 0x4e, 0x43, 0xa4, 0xea, 0x24, 0x48, 0xe6, 0x99, 0xcf, 0xb5, 0x3c, 0x79}
	// UTF-16 name, as retail saves store base names.
	raw := encodeBaseRawData(id, "北の拠点 Alpha", true, -304214.09, 227626.09, 2883.5)

	// The embedded GUID as readGUID renders it is the canonical key; matching it
	// is part of the decoder's contract, so derive it the same way.
	key, err := readGUID(newReader(id[:]))
	if err != nil {
		t.Fatal(err)
	}

	name, loc, ok := decodeBaseRaw(raw, key)
	if !ok {
		t.Fatalf("decodeBaseRaw returned !ok for a well-formed blob")
	}
	if name != "北の拠点 Alpha" {
		t.Fatalf("decoded name = %q, want %q", name, "北の拠点 Alpha")
	}
	if loc == nil {
		t.Fatalf("decodeBaseRaw returned nil location for a well-formed blob")
	}
	if math.Abs(loc.X-(-304214.09)) > 1e-3 || math.Abs(loc.Y-227626.09) > 1e-3 || math.Abs(loc.Z-2883.5) > 1e-3 {
		t.Fatalf("decoded translation = (%.3f,%.3f,%.3f), want (-304214.09,227626.09,2883.5)", loc.X, loc.Y, loc.Z)
	}

	// An ANSI-encoded name decodes identically.
	ansiName, ansiLoc, ok := decodeBaseRaw(encodeBaseRawData(id, "East Camp", false, 1, 2, 3), key)
	if !ok || ansiName != "East Camp" || ansiLoc == nil || ansiLoc.X != 1 {
		t.Fatalf("ANSI blob = (%q,%v,%v)", ansiName, ansiLoc, ok)
	}

	// An empty baseID skips the key check and must still decode.
	if name2, loc2, ok := decodeBaseRaw(raw, ""); !ok || name2 != name || loc2 == nil || loc2.X != loc.X {
		t.Fatalf("decodeBaseRaw with empty baseID = (%q,%v,%v)", name2, loc2, ok)
	}
}

func TestDecodeBaseRawRejectsDrift(t *testing.T) {
	id := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	key, _ := readGUID(newReader(id[:]))
	raw := encodeBaseRawData(id, "camp", false, 100, 200, 300)

	// A GUID that does not match the map key is treated as drift, not a base.
	if _, _, ok := decodeBaseRaw(raw, "ffffffff-ffff-ffff-ffff-ffffffffffff"); ok {
		t.Fatalf("decodeBaseRaw accepted a blob whose embedded GUID mismatched the key")
	}
	// A buffer truncated after the name but before the translation keeps the name
	// and fails the location closed (nil), never reading garbage coordinates.
	if name, loc, ok := decodeBaseRaw(raw[:40], key); !ok || name != "camp" || loc != nil {
		t.Fatalf("truncated blob = (%q,%v,%v), want (camp,nil,true)", name, loc, ok)
	}
	// A buffer truncated inside the name fails the whole decode.
	if _, _, ok := decodeBaseRaw(raw[:20], key); ok {
		t.Fatalf("decodeBaseRaw accepted a blob truncated inside the name")
	}
	// Empty input must not panic and must fail closed.
	if _, _, ok := decodeBaseRaw(nil, key); ok {
		t.Fatalf("decodeBaseRaw accepted nil input")
	}
}

func TestNormalizeBaseName(t *testing.T) {
	cases := map[string]string{
		"North Fort":     "North Fort",
		"  North Fort  ": "North Fort",
		"":               "",
		"   ":            "",
		"\t\n":           "",
		"新規生成拠点テンプレート名0(仮)":  "", // engine placeholder, live-save shape
		"新規生成拠点テンプレート名19(仮)": "",
		"新規生成拠点テンプレート名":      "", // prefix alone is still the placeholder
	}
	for in, want := range cases {
		if got := normalizeBaseName(in); got != want {
			t.Errorf("normalizeBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestBaseFromEntryDecodesRawDataPosition proves the property-tree path: a base
// entry carrying only a RawData byte property (no plain Position/Location
// property, as retail 1.x saves are shaped) yields a decoded Position and Name.
func TestBaseFromEntryDecodesRawDataPosition(t *testing.T) {
	id := [16]byte{0xaa, 0xbb, 0xcc, 0xdd, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	key, _ := readGUID(newReader(id[:]))
	raw := encodeBaseRawData(id, "Outpost 7", true, -12345.5, 67890.25, -42)
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
	if base.Name != "Outpost 7" {
		t.Fatalf("baseFromEntry Name = %q, want %q", base.Name, "Outpost 7")
	}

	// The engine's unnamed placeholder collapses to "" (served as null upstream).
	unnamed := baseFromEntry(mapEntry{Key: key, Value: propertyMap{
		"RawData": &property{Value: encodeBaseRawData(id, "新規生成拠点テンプレート名3(仮)", true, 1, 2, 3)},
	}}, &stats)
	if unnamed.Name != "" {
		t.Fatalf("placeholder base name = %q, want empty", unnamed.Name)
	}

	// A base whose RawData cannot be decoded must yield a nil Position (served as
	// null), never a zero vector, and record the tolerated skip.
	badStats := newStats()
	bad := baseFromEntry(mapEntry{Key: key, Value: propertyMap{"RawData": &property{Value: []byte{1, 2, 3}}}}, &badStats)
	if bad.Position != nil {
		t.Fatalf("undecodable RawData produced a non-nil Position %+v", *bad.Position)
	}
	if bad.Name != "" {
		t.Fatalf("undecodable RawData produced a name %q", bad.Name)
	}
	if badStats.SkippedProperties == 0 {
		t.Fatalf("undecodable base transform was not recorded as a tolerated skip")
	}
}
