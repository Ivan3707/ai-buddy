package database

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func OpenPostgres() (*sql.DB, error) {
	config, err := postgresConfig()
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDB(*config)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return db, nil
}

func postgresConfig() (*pgx.ConnConfig, error) {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "app")
	password := getEnv("DB_PASSWORD", "change_me")
	dbname := getEnv("DB_NAME", "app")
	sslmode := getEnv("DB_SSLMODE", "disable")

	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}
	config, err := pgx.ParseConfig("sslmode=" + sslmode)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_SSLMODE: %w", err)
	}
	config.Host = host
	config.Port = uint16(portNumber)
	config.User = user
	config.Password = password
	config.Database = dbname
	for _, fallback := range config.Fallbacks {
		fallback.Host = host
		fallback.Port = uint16(portNumber)
	}
	return config, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
