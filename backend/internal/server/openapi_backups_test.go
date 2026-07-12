package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/palhelm/palhelm/internal/config"
	"github.com/palhelm/palhelm/internal/store"
)

func TestBackupsOpenAPIContract(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(openapi, &doc); err != nil {
		t.Fatal(err)
	}
	paths := object(t, doc, "paths")
	want := map[string]map[string][]string{
		"/api/v1/backups":                      {"get": {"200"}, "post": {"201", "409"}},
		"/api/v1/backups/{id}/download":        {"get": {"200", "400", "404"}, "head": {"200", "400", "404"}},
		"/api/v1/backups/{id}/contents":        {"get": {"200", "400", "404"}},
		"/api/v1/backups/{id}/restore/dry-run": {"post": {"200", "400", "404", "409"}},
		"/api/v1/backups/{id}/restore":         {"post": {"200", "400", "404", "409"}},
		"/api/v1/backups/{id}":                 {"delete": {"200", "400", "404", "409"}},
		"/api/v1/backups/schedule":             {"get": {"200"}, "put": {"200", "400"}},
	}
	for path, methods := range want {
		pathItem := object(t, paths, path)
		for method, statuses := range methods {
			op := object(t, pathItem, method)
			responses := object(t, op, "responses")
			got := make([]string, 0, len(responses))
			for status := range responses {
				got = append(got, status)
			}
			sort.Strings(got)
			sort.Strings(statuses)
			if !reflect.DeepEqual(got, statuses) {
				t.Errorf("%s %s statuses = %v, want %v", method, path, got, statuses)
			}
			if _, obsolete := responses["501"]; obsolete {
				t.Errorf("%s %s still advertises 501", method, path)
			}
		}
	}

	assertResponseRef(t, paths, "/api/v1/backups", "post", "201", "#/components/schemas/Backup")
	assertResponseRef(t, paths, "/api/v1/backups/{id}/restore/dry-run", "post", "200", "#/components/schemas/BackupDiff")
	assertResponseRef(t, paths, "/api/v1/backups/schedule", "get", "200", "#/components/schemas/BackupSchedule")

	components := object(t, doc, "components")
	schemas := object(t, components, "schemas")
	for _, name := range []string{"Backup", "BackupEntry", "BackupChange", "BackupDiff", "BackupSchedule", "BackupScheduleUpdate", "RestoreRequest"} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("missing backup schema %s", name)
		}
	}
	schedule := object(t, schemas, "BackupSchedule")
	required, ok := schedule["required"].([]any)
	if !ok || len(required) != 4 {
		t.Fatalf("BackupSchedule.required = %#v", schedule["required"])
	}
	restorePath := object(t, paths, "/api/v1/backups/{id}/restore")
	restorePost := object(t, restorePath, "post")
	if _, ok := restorePost["requestBody"]; !ok {
		t.Error("restore request body is undocumented")
	}
}

func TestBackupDownloadHEADMatchesGETWithoutBody(t *testing.T) {
	root := t.TempDir()
	saveDir := filepath.Join(root, "Saved")
	worldDir := filepath.Join(saveDir, "SaveGames", "0", "GUID")
	if err := os.MkdirAll(worldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "Level.sav"), []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := config.Config{
		DataDir: root, SaveDir: saveDir, AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48),
		MetricsInterval: time.Hour, PlayersInterval: time.Hour, SaveSyncInterval: time.Hour,
	}
	app, handler := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	backup, err := app.backups.Create(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"password":"panelpass"}`))
	login.Header.Set("Content-Type", "application/json")
	lr := httptest.NewRecorder()
	handler.ServeHTTP(lr, login)
	if lr.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", lr.Code, lr.Body.String())
	}
	cookie := lr.Result().Cookies()[0]
	request := func(method string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/backups/"+strconv.FormatInt(backup.ID, 10)+"/download", nil)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	get := request(http.MethodGet)
	head := request(http.MethodHead)
	if get.Code != http.StatusOK || head.Code != http.StatusOK {
		t.Fatalf("GET=%d HEAD=%d", get.Code, head.Code)
	}
	if get.Body.Len() == 0 || head.Body.Len() != 0 {
		t.Fatalf("GET body=%d bytes, HEAD body=%d bytes", get.Body.Len(), head.Body.Len())
	}
	if head.Header().Get("Content-Length") != get.Header().Get("Content-Length") {
		t.Fatalf("HEAD length %q != GET length %q", head.Header().Get("Content-Length"), get.Header().Get("Content-Length"))
	}
	if head.Header().Get("Content-Disposition") == "" || head.Header().Get("Content-Type") != "application/gzip" {
		t.Fatalf("HEAD headers = %#v", head.Header())
	}
}

func assertResponseRef(t *testing.T, paths map[string]any, path, method, status, want string) {
	t.Helper()
	op := object(t, object(t, paths, path), method)
	response := object(t, object(t, op, "responses"), status)
	content := object(t, response, "content")
	media := object(t, content, "application/json")
	schema := object(t, media, "schema")
	if got, _ := schema["$ref"].(string); got != want {
		t.Errorf("%s %s response %s schema = %q, want %q", method, path, status, got, want)
	}
}

func object(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := parent[key]
	if !ok {
		t.Fatalf("missing object %s", key)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s is %T, want object (%s)", key, v, fmt.Sprint(v))
	}
	return obj
}
