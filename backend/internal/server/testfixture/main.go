// Command testfixture runs the real Palhelm router for the frontend Config contract test.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/server"
	"github.com/8tp/palhelm/internal/store"
)

func main() {
	dir, err := os.MkdirTemp("", "palhelm-config-contract-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	compose := filepath.Join(dir, "docker-compose.yml")
	contents := "services:\n  palworld:\n    environment:\n      SERVER_NAME: contract\n      PLAYERS: 16\n      ADMIN_PASSWORD: secret\n"
	if err = os.WriteFile(compose, []byte(contents), 0o640); err != nil {
		panic(err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		panic(err)
	}
	defer st.Close()
	cfg := config.Config{
		AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48),
		DataDir: filepath.Join(dir, "data"), ComposeFile: compose, GameService: "palworld",
		SaveDir: filepath.Join(dir, "saved"),
	}
	_, handler := server.New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Printf("LISTEN http://%s\n", listener.Addr())
	if err := http.Serve(listener, handler); err != nil {
		panic(err)
	}
}
