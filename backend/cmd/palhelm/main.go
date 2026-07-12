// Command palhelm runs the Palhelm server or parses a Palworld save.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/sav"
	"github.com/8tp/palhelm/internal/server"
	"github.com/8tp/palhelm/internal/store"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	server.PanelVersion = version
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "palhelm:", err)
		os.Exit(1)
	}
}
func run() error {
	args := os.Args[1:]
	command := "serve"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	switch command {
	case "serve":
		if len(args) > 0 {
			return errors.New("usage: palhelm [serve]")
		}
		return serve()
	case "parse":
		if len(args) != 1 {
			return errors.New("usage: palhelm parse <file.sav>")
		}
		return parse(args[0])
	default:
		return fmt.Errorf("unknown subcommand %q (expected serve or parse)", command)
	}
}
func parse(path string) error {
	var v any
	var err error
	if strings.EqualFold(filepath.Base(path), "LevelMeta.sav") {
		v, err = sav.ParseLevelMeta(path)
	} else {
		v, err = sav.ParseLevel(path, sav.Options{})
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err = cfg.ValidateServe(); err != nil {
		return err
	}
	if err = cfg.EnsureSessionSecret(); err != nil {
		return fmt.Errorf("session secret: %w", err)
	}
	if err = os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "palhelm.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	if err = st.SetKV(context.Background(), "session_secret", cfg.SessionSecret); err != nil {
		return fmt.Errorf("persist session secret: %w", err)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app, handler := server.New(cfg, st, log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	backgroundDone := make(chan struct{})
	go func() {
		defer close(backgroundDone)
		app.RunPollers(ctx)
	}()
	defer func() {
		stop()
		<-backgroundDone
	}()
	httpServer := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() { log.Info("palhelm listening", "addr", cfg.Addr); errCh <- httpServer.ListenAndServe() }()
	select {
	case err = <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		app.CloseStreams()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err = httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err = <-errCh
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}
