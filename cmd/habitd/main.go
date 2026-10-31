// Command habitd is a self-hosted habit tracker with gamification.
package main

import (
	"context"
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
	_ "time/tzdata" // the scratch image has no zone files

	"github.com/seals187/habitd/internal/db"
	server "github.com/seals187/habitd/internal/http"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/web"
)

// version is set at build time.
var version = "0.0.3"

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "seed":
		err = seed()
	case "healthcheck":
		err = healthcheck()
	default:
		err = fmt.Errorf("unknown command %q (use serve, seed or healthcheck)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "habitd:", err)
		os.Exit(1)
	}
}

func open(ctx context.Context) (*service.Service, error) {
	loc := time.Local
	if tz := os.Getenv("TZ"); tz != "" {
		var err error
		if loc, err = time.LoadLocation(tz); err != nil {
			return nil, fmt.Errorf("time zone %q: %w", tz, err)
		}
	}
	path := env("HABITD_DB", "/data/habitd.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	store, err := db.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	svc := service.New(store, loc)
	if err := svc.Init(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return svc, nil
}

func serve() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc, err := open(ctx)
	if err != nil {
		return err
	}
	defer svc.Store.Close()

	if err := initialCredentials(ctx, svc); err != nil {
		return err
	}
	cfg := server.Config{
		Version:      version,
		Runtime:      env("HABITD_RUNTIME", "Binary"),
		CookieSecure: env("HABITD_COOKIE_SECURE", "true") != "false",
		APIToken:     os.Getenv("HABITD_API_TOKEN"),
	}
	app, err := server.New(cfg, svc, web.FS)
	if err != nil {
		return err
	}
	if !cfg.CookieSecure {
		slog.Warn("session cookie is not marked Secure: use this on local HTTP only")
	}
	go svc.RunBackground(ctx)

	addr := ":" + env("HABITD_PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 2 * time.Minute}
	errs := make(chan error, 1)
	go func() {
		slog.Info("habitd listening", "addr", addr, "version", version)
		errs <- srv.ListenAndServe()
	}()
	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// initialCredentials applies HABITD_USER on every start and HABITD_PASSWORD only
// once; later password changes happen in the app.
func initialCredentials(ctx context.Context, svc *service.Service) error {
	if user := strings.TrimSpace(os.Getenv("HABITD_USER")); user != "" {
		if err := svc.Store.Q(ctx).SetSetting(service.KeyAdminUser, user); err != nil {
			return err
		}
	}
	password := os.Getenv("HABITD_PASSWORD")
	if password == "" {
		return nil
	}
	hash, err := svc.Store.Q(ctx).Setting(service.KeyPasswordHash, "")
	if err != nil || hash != "" {
		return err
	}
	if password == "changeme" {
		slog.Warn("HABITD_PASSWORD is still the default: change it before exposing habitd")
	}
	return server.SetPassword(ctx, svc.Store, password)
}

func seed() error {
	ctx := context.Background()
	svc, err := open(ctx)
	if err != nil {
		return err
	}
	defer svc.Store.Close()
	if err := svc.Seed(ctx); err != nil {
		return err
	}
	fmt.Println("demo data created")
	return nil
}

// healthcheck backs the Docker HEALTHCHECK, the image has no curl.
func healthcheck() error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + env("HABITD_PORT", "8080") + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}
