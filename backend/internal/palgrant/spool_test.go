package palgrant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSpoolStaysFailClosedUntilExactBridgeIsReady(t *testing.T) {
	dir := t.TempDir()
	catalog := NewCatalog()
	spool := Spool{Dir: dir}
	now := time.Now().UTC().Truncate(time.Second)
	if got := spool.Capability(false, catalog, now); got.Ready {
		t.Fatal("disabled capability reported ready")
	}
	body := `{"protocolVersion":1,"state":"validation_required","gameVersion":"1.0.1","catalogVersion":"palworld_1.0_pinned","catalogItems":` + fmtInt(catalog.Count()) + `,"at":"` + now.Format(time.RFC3339) + `"}`
	if err := os.WriteFile(filepath.Join(dir, "capability.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := spool.Capability(true, catalog, now); got.Ready || got.Reason == "" {
		t.Fatalf("unvalidated bridge = %#v", got)
	}
}

func TestResultRequiresCompleteBoundedGeneratedSpecimen(t *testing.T) {
	dir := t.TempDir()
	requestID := "1234567890abcdef"
	value := 72
	result := Result{
		ProtocolVersion:   BridgeProtocolVersion,
		RequestID:         requestID,
		Status:            "succeeded",
		InstanceID:        "abcdef1234567890",
		Level:             MaxSupportedLevel,
		Gender:            "female",
		TalentHP:          &value,
		TalentMelee:       &value,
		TalentShot:        &value,
		TalentDefense:     &value,
		PassiveSkillIDs:   []string{"CraftSpeed_up2"},
		CondensationStars: 2,
		CompletedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "results"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "results", requestID+".json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Spool{Dir: dir}).Result(requestID); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}

	result.TalentDefense = nil
	body, _ = json.Marshal(result)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Spool{Dir: dir}).Result(requestID); err == nil {
		t.Fatal("incomplete successful result accepted")
	}
}

func fmtInt(v int) string {
	if v == 0 {
		return "0"
	}
	b := make([]byte, 0, 8)
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
