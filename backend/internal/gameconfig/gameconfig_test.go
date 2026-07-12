package gameconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCatalogMappings(t *testing.T) {
	want := map[string]string{"SERVER_NAME": "ServerName", "PLAYERS": "ServerPlayerMaxNum", "PAL_CAPTURE_RATE": "PalCaptureRate", "MULTITHREADING": "bUseMultithreadForDS"}
	for env, ini := range want {
		entry, ok := byEnv(env)
		if !ok || entry.INI != ini {
			t.Errorf("%s mapping = %#v", env, entry)
		}
	}
	admin, _ := byEnv("ADMIN_PASSWORD")
	if !admin.Masked {
		t.Error("ADMIN_PASSWORD must be masked")
	}
	server, _ := byEnv("SERVER_PASSWORD")
	if !server.Masked {
		t.Error("SERVER_PASSWORD must be masked")
	}
}

func TestCatalogHas10Additions(t *testing.T) {
	want := map[string]struct {
		ini string
		typ ValueType
		def any
	}{
		"ENABLE_VOICE_CHAT":                       {"bEnableVoiceChat", Boolean, false},
		"VOICE_CHAT_MAX_VOLUME_DISTANCE":          {"VoiceChatMaxVolumeDistance", Number, 3000.0},
		"VOICE_CHAT_ZERO_VOLUME_DISTANCE":         {"VoiceChatZeroVolumeDistance", Number, 15000.0},
		"ENABLE_BUILDING_PLAYER_UID_DISPLAY":      {"bEnableBuildingPlayerUIdDisplay", Boolean, false},
		"MONSTER_FARM_ACTION_SPEED_RATE":          {"MonsterFarmActionSpeedRate", Number, 1.0},
		"PHYSICS_ACTIVE_DROP_ITEM_MAX_NUM":        {"PhysicsActiveDropItemMaxNum", Integer, -1},
		"BUILDING_NAME_DISPLAY_CACHE_TTL_SECONDS": {"BuildingNameDisplayCacheTTLSeconds", Integer, 60},
	}
	for env, w := range want {
		entry, ok := byEnv(env)
		if !ok {
			t.Errorf("%s missing from catalog", env)
			continue
		}
		if entry.INI != w.ini || entry.Type != w.typ || entry.Default != w.def {
			t.Errorf("%s = %#v, want ini=%s type=%v default=%v", env, entry, w.ini, w.typ, w.def)
		}
	}
	if e, _ := byEnv("BUILDING_NAME_DISPLAY_CACHE_TTL_SECONDS"); e.Group != "advanced" {
		t.Errorf("BUILDING_NAME_DISPLAY_CACHE_TTL_SECONDS group = %q, want advanced", e.Group)
	}
}

func TestVoiceChatDistanceCrossValidation(t *testing.T) {
	dir := t.TempDir()
	src, _ := os.ReadFile(filepath.Join("testdata", "docker-compose.yml"))
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, src, 0o640); err != nil {
		t.Fatal(err)
	}
	e := Editor{ComposeFile: path, Service: "palworld"}

	// Zero-volume distance below the (default) max-volume distance must be rejected.
	if err := e.Update(map[string]any{"VOICE_CHAT_ZERO_VOLUME_DISTANCE": 100}); err == nil {
		t.Fatal("expected rejection of zero-volume distance below max-volume distance")
	}

	// A consistent pair should be accepted together.
	if err := e.Update(map[string]any{"VOICE_CHAT_MAX_VOLUME_DISTANCE": 500, "VOICE_CHAT_ZERO_VOLUME_DISTANCE": 1000}); err != nil {
		t.Fatalf("expected consistent pair to be accepted: %v", err)
	}

	// Raising zero-volume distance alone (max already 500 from the prior update) should pass.
	if err := e.Update(map[string]any{"VOICE_CHAT_ZERO_VOLUME_DISTANCE": 2000}); err != nil {
		t.Fatalf("expected raise-only update to be accepted: %v", err)
	}

	// Now dropping max above the persisted zero (2000) should be rejected.
	if err := e.Update(map[string]any{"VOICE_CHAT_MAX_VOLUME_DISTANCE": 5000}); err == nil {
		t.Fatal("expected rejection when max-volume distance would exceed persisted zero-volume distance")
	}

	// Unrelated updates must not be blocked by the voice-chat pair at all.
	if err := e.Update(map[string]any{"PLAYERS": 20}); err != nil {
		t.Fatalf("unrelated update should not trip voice-chat validation: %v", err)
	}
}

func TestComposeSurgeryPreservesUntouchedText(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("testdata", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := surgery(string(original), "palworld", map[string]string{"SERVER_NAME": "new name"})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(original), `SERVER_NAME: "fixture server" # keep this comment`, `SERVER_NAME: "new name" # keep this comment`, 1)
	if updated != want {
		t.Fatalf("surgery changed unrelated bytes\nwant:\n%s\ngot:\n%s", want, updated)
	}
}

func TestComposeSurgeryAddsMissingKey(t *testing.T) {
	original, _ := os.ReadFile(filepath.Join("testdata", "docker-compose.yml"))
	updated, err := surgery(string(original), "palworld", map[string]string{"EXP_RATE": "2.5"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated, "      EXP_RATE: 2.5\n    volumes:") {
		t.Fatalf("missing insertion:\n%s", updated)
	}
	if !strings.Contains(updated, `SERVER_NAME: "untouched"`) {
		t.Fatal("other service was modified")
	}
}

func TestEditorUpdateBackupAndValidation(t *testing.T) {
	dir := t.TempDir()
	src, _ := os.ReadFile(filepath.Join("testdata", "docker-compose.yml"))
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, src, 0o640); err != nil {
		t.Fatal(err)
	}
	e := Editor{ComposeFile: path, Service: "palworld"}
	if err := e.Update(map[string]any{"PLAYERS": 24}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "PLAYERS: 24") {
		t.Fatalf("value not edited: %s", b)
	}
	matches, _ := filepath.Glob(path + ".*.palhelm.bak")
	if len(matches) != 1 {
		t.Fatalf("daily backups=%v", matches)
	}
	if err := e.Update(map[string]any{"PLAYERS": "many"}); err == nil {
		t.Fatal("invalid integer accepted")
	}
}

func TestGetContractTypedEditableAndWriteOnlySecrets(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  palworld:
    environment:
      SERVER_NAME: "desired"
      SERVER_PASSWORD: "join-secret"
      ADMIN_PASSWORD: "admin-secret"
      PLAYERS: 20
      EXP_RATE: 1.5
      RCON_PORT: 25575
`
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(compose), 0o640); err != nil {
		t.Fatal(err)
	}
	e := Editor{
		ComposeFile: path,
		Service:     "palworld",
		Effective: func(context.Context) (map[string]any, error) {
			return map[string]any{"ServerName": "effective", "ServerPlayerMaxNum": 16, "ExpRate": 1.0}, nil
		},
	}
	doc, err := e.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Source != "compose" || doc.Version == "" || !doc.Capabilities.Write.Available || doc.Capabilities.Apply.Available {
		t.Fatalf("unexpected capability contract: %#v", doc)
	}
	settings := make(map[string]Setting, len(doc.Settings))
	for _, setting := range doc.Settings {
		settings[setting.Key] = setting
	}
	for _, key := range []string{"SERVER_PASSWORD", "ADMIN_PASSWORD"} {
		setting := settings[key]
		if setting.Value != "•••" || setting.EffectiveValue != "•••" {
			t.Fatalf("%s leaked through placeholder: %#v", key, setting)
		}
		if !setting.Editable || setting.ReadOnly {
			t.Fatalf("%s write-only field must accept replacement values: %#v", key, setting)
		}
	}
	if _, ok := settings["PLAYERS"].Value.(int64); !ok {
		t.Fatalf("PLAYERS value type = %T, want int64", settings["PLAYERS"].Value)
	}
	if _, ok := settings["EXP_RATE"].Value.(float64); !ok {
		t.Fatalf("EXP_RATE value type = %T, want float64", settings["EXP_RATE"].Value)
	}
	if settings["RCON_PORT"].Editable || !settings["RCON_PORT"].ReadOnly {
		t.Fatalf("panel-managed setting contract inverted: %#v", settings["RCON_PORT"])
	}
}

func TestGetReadOnlyCapabilityWithoutCompose(t *testing.T) {
	e := Editor{Service: "palworld", SaveDir: t.TempDir()}
	doc, err := e.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Source != "ini" || doc.Capabilities.Write.Available || doc.Capabilities.Write.Reason == "" {
		t.Fatalf("unexpected read-only contract: %#v", doc)
	}
	for _, setting := range doc.Settings {
		if setting.Editable || !setting.ReadOnly {
			t.Fatalf("read-only deployment exposed editable setting: %#v", setting)
		}
	}
}

func TestGetUnavailableComposeStillReturnsNormalizedReadOnlySettings(t *testing.T) {
	e := Editor{ComposeFile: filepath.Join(t.TempDir(), "missing", "docker-compose.yml"), Service: "palworld", SaveDir: t.TempDir()}
	doc, err := e.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Source != "ini" || doc.Capabilities.Write.Available || len(doc.Settings) != len(Catalog()) {
		t.Fatalf("unavailable compose lost normalized settings/state: %#v", doc)
	}
	for _, setting := range doc.Settings {
		if setting.Editable || !setting.ReadOnly {
			t.Fatalf("unavailable compose exposed editable setting: %#v", setting)
		}
	}
}

func TestUpdateYAMLEncodesChangedStrings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	original := "services:\n  palworld:\n    environment:\n      SERVER_DESCRIPTION: old # preserved\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	e := Editor{ComposeFile: path, Service: "palworld"}
	value := `raid: alpha # keep [literal] {text}`
	if err := e.Update(map[string]any{"SERVER_DESCRIPTION": value}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `SERVER_DESCRIPTION: "raid: alpha # keep [literal] {text}" # preserved`) {
		t.Fatalf("changed string was not safely YAML encoded: %s", b)
	}
	desired, _, err := parseEnvironment(string(b), "palworld")
	if err != nil || desired["SERVER_DESCRIPTION"] != value {
		t.Fatalf("parsed value = %q, err=%v", desired["SERVER_DESCRIPTION"], err)
	}
}

func TestQuotedBackslashStringRoundTripsThroughGET(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services:\n  palworld:\n    environment:\n      SERVER_DESCRIPTION: old\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	e := Editor{ComposeFile: path, Service: "palworld", SaveDir: filepath.Join(dir, "saved")}
	value := `say "hi" \ path # literal`
	if err := e.Update(map[string]any{"SERVER_DESCRIPTION": value}); err != nil {
		t.Fatal(err)
	}
	doc, err := e.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range doc.Settings {
		if setting.Key == "SERVER_DESCRIPTION" {
			if setting.Value != value {
				t.Fatalf("GET value = %q, want %q", setting.Value, value)
			}
			return
		}
	}
	t.Fatal("SERVER_DESCRIPTION missing from GET document")
}

func TestUpdateRejectsControlAndNewlineInjection(t *testing.T) {
	for name, value := range map[string]string{
		"newline":         "safe\n    HOSTILE: value",
		"carriage-return": "safe\rHOSTILE: value",
		"control":         "safe\x00value",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "docker-compose.yml")
			if err := os.WriteFile(path, []byte("services:\n  palworld:\n    environment:\n      SERVER_DESCRIPTION: old\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			e := Editor{ComposeFile: path, Service: "palworld"}
			if err := e.Update(map[string]any{"SERVER_DESCRIPTION": value}); err == nil {
				t.Fatal("injection value accepted")
			}
			b, _ := os.ReadFile(path)
			if strings.Contains(string(b), "HOSTILE") {
				t.Fatalf("compose file was modified: %s", b)
			}
		})
	}
}

func TestUpdateRejectsWriteOnlyPlaceholder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services:\n  palworld:\n    environment:\n      ADMIN_PASSWORD: secret\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	e := Editor{ComposeFile: path, Service: "palworld"}
	if err := e.Update(map[string]any{"ADMIN_PASSWORD": "•••"}); err == nil {
		t.Fatal("write-only placeholder was accepted as a real password")
	}
}

func TestConcurrentUpdatesAreSerializedWithoutLostChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services:\n  palworld:\n    environment:\n      SERVER_NAME: old\n      PLAYERS: 16\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	e := Editor{ComposeFile: path, Service: "palworld"}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for key, value := range map[string]any{"SERVER_NAME": "new", "PLAYERS": 24} {
		go func(key string, value any) {
			ready.Done()
			<-start
			errs <- e.Update(map[string]any{key: value})
		}(key, value)
	}
	ready.Wait()
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `SERVER_NAME: "new"`) || !strings.Contains(string(b), "PLAYERS: 24") {
		t.Fatalf("acknowledged update was lost: %s", b)
	}
}

func TestUpdateVersionDetectsExternalChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	original := []byte("services:\n  palworld:\n    environment:\n      SERVER_NAME: old\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	e := Editor{ComposeFile: path, Service: "palworld"}
	doc, err := e.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	external := []byte("services:\n  palworld:\n    environment:\n      SERVER_NAME: externally-edited\n")
	if err := os.WriteFile(path, external, 0o640); err != nil {
		t.Fatal(err)
	}
	err = e.UpdateVersion(map[string]any{"SERVER_NAME": "panel-edit"}, doc.Version)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(external) {
		t.Fatalf("external edit was overwritten: %s", b)
	}
}
