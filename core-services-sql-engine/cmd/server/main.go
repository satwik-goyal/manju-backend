package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/manju-backend/core-services-sql-engine/internal/api"
	"github.com/manju-backend/core-services-sql-engine/internal/digestion"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := loadConfig()

	ctx := context.Background()

	// Connect to platform DB.
	pool, err := pgxpool.New(ctx, cfg.platformDSN)
	if err != nil {
		slog.Error("connect to platform DB", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("ping platform DB", "err", err)
		os.Exit(1)
	}
	slog.Info("platform DB connected", "dsn", maskPassword(cfg.platformDSN))

	// Build engine and scheduler.
	engine := digestion.New(pool)
	scheduler := digestion.NewScheduler(engine)

	if err := scheduler.Start(ctx); err != nil {
		slog.Error("start scheduler", "err", err)
		os.Exit(1)
	}
	defer scheduler.Stop()

	// Start HTTP server.
	e := api.New(engine, scheduler)
	addr := fmt.Sprintf(":%d", cfg.port)
	slog.Info("server starting", "addr", addr)
	if err := e.Start(addr); err != nil {
		slog.Error("server stopped", "err", err)
	}
}

type config struct {
	platformDSN string
	port        int
}

func loadConfig() config {
	return config{
		platformDSN: getEnv(
			"PLATFORM_DB_DSN",
			"host=localhost port=5432 dbname=manju user=manju password=manju sslmode=disable",
		),
		port: getEnvInt("PORT", 8080),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// maskPassword replaces the password in a DSN with *****.
func maskPassword(dsn string) string {
	// Simple mask: hide anything after "password="
	for i := 0; i < len(dsn); i++ {
		if len(dsn[i:]) > 9 && dsn[i:i+9] == "password=" {
			end := i + 9
			for end < len(dsn) && dsn[end] != ' ' {
				end++
			}
			return dsn[:i+9] + "*****" + dsn[end:]
		}
	}
	return dsn
}
