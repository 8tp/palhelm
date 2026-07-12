package sav

import "testing"

func TestDecodePlayerProgressUsesAuthoritativeRecordData(t *testing.T) {
	data := propertyMap{
		"RecordData": {Type: "StructProperty", Value: structData{Value: propertyMap{
			"TribeCaptureCount": {Type: "IntProperty", Value: int32(123)},
			"PalCaptureCount": {Type: "MapProperty", Value: []mapEntry{
				{Key: "SheepBall", Value: int32(4)},
				{Key: "Anubis", Value: int32(1)},
				{Key: "NeverCaught", Value: int32(0)},
			}},
			"PaldeckUnlockFlag": {Type: "MapProperty", Value: []mapEntry{
				{Key: "SheepBall", Value: true},
				{Key: "Anubis", Value: true},
				{Key: "Unknown", Value: false},
			}},
		}}},
	}
	var got Player
	decodePlayerProgress(data, &got)
	if got.CaptureTotal == nil || *got.CaptureTotal != 123 {
		t.Fatalf("CaptureTotal = %v, want 123", got.CaptureTotal)
	}
	if got.UniquePalsCaptured == nil || *got.UniquePalsCaptured != 2 {
		t.Fatalf("UniquePalsCaptured = %v, want 2", got.UniquePalsCaptured)
	}
	if got.PaldeckUnlocked == nil || *got.PaldeckUnlocked != 2 {
		t.Fatalf("PaldeckUnlocked = %v, want 2", got.PaldeckUnlocked)
	}
}

func TestDecodePlayerProgressPreservesUnavailableVersusZero(t *testing.T) {
	var missing Player
	decodePlayerProgress(propertyMap{}, &missing)
	if missing.CaptureTotal != nil || missing.UniquePalsCaptured != nil || missing.PaldeckUnlocked != nil {
		t.Fatalf("missing RecordData should remain unavailable: %+v", missing)
	}

	zeroData := propertyMap{"RecordData": {Value: structData{Value: propertyMap{
		"TribeCaptureCount": {Value: int32(0)},
		"PalCaptureCount":   {Value: []mapEntry{}},
		"PaldeckUnlockFlag": {Value: []mapEntry{}},
	}}}}
	var zero Player
	decodePlayerProgress(zeroData, &zero)
	if zero.CaptureTotal == nil || *zero.CaptureTotal != 0 || zero.UniquePalsCaptured == nil || *zero.UniquePalsCaptured != 0 || zero.PaldeckUnlocked == nil || *zero.PaldeckUnlocked != 0 {
		t.Fatalf("real zeros must remain present: %+v", zero)
	}
}
