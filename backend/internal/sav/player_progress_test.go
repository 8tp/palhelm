package sav

import (
	"fmt"
	"testing"
)

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
	if got.PalCaptureCounts["SheepBall"] != 4 || got.PalCaptureCounts["Anubis"] != 1 || got.PalCaptureCounts["NeverCaught"] != 0 {
		t.Fatalf("PalCaptureCounts = %#v", got.PalCaptureCounts)
	}
	if !got.PaldeckUnlockFlags["SheepBall"] || !got.PaldeckUnlockFlags["Anubis"] || got.PaldeckUnlockFlags["Unknown"] {
		t.Fatalf("PaldeckUnlockFlags = %#v", got.PaldeckUnlockFlags)
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
	if zero.PalCaptureCounts == nil || len(zero.PalCaptureCounts) != 0 || zero.PaldeckUnlockFlags == nil || len(zero.PaldeckUnlockFlags) != 0 {
		t.Fatalf("authoritative empty maps must remain distinguishable from unavailable: %+v", zero)
	}
}

func TestDecodePlayerProgressBoundsSpeciesMapsWithoutChangingAggregateCounters(t *testing.T) {
	captures := make([]mapEntry, 2050)
	unlocks := make([]mapEntry, 2050)
	for i := range captures {
		id := fmt.Sprintf("Species_%04d", i)
		captures[i] = mapEntry{Key: id, Value: int32(1)}
		unlocks[i] = mapEntry{Key: id, Value: true}
	}
	data := propertyMap{"RecordData": {Value: structData{Value: propertyMap{
		"PalCaptureCount":   {Value: captures},
		"PaldeckUnlockFlag": {Value: unlocks},
	}}}}
	var got Player
	decodePlayerProgress(data, &got)
	if len(got.PalCaptureCounts) != 2048 || !got.PalCaptureCountsTruncated || len(got.PaldeckUnlockFlags) != 2048 || !got.PaldeckUnlockFlagsTruncated {
		t.Fatalf("bounded maps = captures %d/%v unlocks %d/%v", len(got.PalCaptureCounts), got.PalCaptureCountsTruncated, len(got.PaldeckUnlockFlags), got.PaldeckUnlockFlagsTruncated)
	}
	if got.UniquePalsCaptured == nil || *got.UniquePalsCaptured != 2050 || got.PaldeckUnlocked == nil || *got.PaldeckUnlocked != 2050 {
		t.Fatalf("aggregate counters must describe the complete decoded maps: unique=%v unlocked=%v", got.UniquePalsCaptured, got.PaldeckUnlocked)
	}
}
