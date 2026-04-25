package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/manju-backend/core-services-object-store/internal/api"
	"github.com/manju-backend/core-services-object-store/pkg/filestore"
)

type config struct {
	platformDSN    string
	minioEndpoint  string
	minioAccessKey string
	minioSecretKey string
	minioUseSSL    bool
	port           int
}

func loadConfig() config {
	return config{
		platformDSN:    getEnv("PLATFORM_DB_DSN", "host=localhost port=5432 dbname=manju user=manju password=manju sslmode=disable"),
		minioEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		minioAccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		minioSecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
		minioUseSSL:    getEnvBool("MINIO_USE_SSL", false),
		port:           getEnvInt("PORT", 8081),
	}
}

func main() {
	cfg := loadConfig()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.platformDSN)
	if err != nil {
		log.Fatalf("connect platform db: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping platform db: %v", err)
	}

	backend, err := filestore.NewMinioBackend(filestore.MinioConfig{
		Endpoint:        cfg.minioEndpoint,
		AccessKeyID:     cfg.minioAccessKey,
		SecretAccessKey: cfg.minioSecretKey,
		UseSSL:          cfg.minioUseSSL,
	})
	if err != nil {
		log.Fatalf("create minio backend: %v", err)
	}

	store := filestore.New(backend, pool,
		filestore.WithEventEmitter(&filestore.LogEmitter{Logger: slog.Default()}),
	)

	e := api.New(store)
	log.Printf("object store service listening on :%d", cfg.port)
	if err := e.Start(fmt.Sprintf(":%d", cfg.port)); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}
