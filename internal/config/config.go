package config

import (
	"os"
	"strconv"
)

type Config struct {
	// Database (PostgreSQL - relational data)
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// ClickHouse (bulk data - emails, blacklist)
	ClickHouseHost     string
	ClickHousePort     string
	ClickHouseUser     string
	ClickHousePassword string
	ClickHouseDB       string

	// Redis
	RedisHost     string
	RedisPort     string
	RedisPassword string

	// API
	APIPort   string
	APISecret string

	// Engine
	WorkersCount      int
	ConnectionsPerSMTP int

	// Tracking
	TrackingDomain string
}

func Load() *Config {
	return &Config{
		// Database (PostgreSQL)
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "smtpenviador"),
		DBPassword: getEnv("DB_PASSWORD", "smtpenviador123"),
		DBName:     getEnv("DB_NAME", "smtpenviador"),

		// ClickHouse
		ClickHouseHost:     getEnv("CLICKHOUSE_HOST", "localhost"),
		ClickHousePort:     getEnv("CLICKHOUSE_PORT", "9000"),
		ClickHouseUser:     getEnv("CLICKHOUSE_USER", "smtpenviador"),
		ClickHousePassword: getEnv("CLICKHOUSE_PASSWORD", "smtpenviador123"),
		ClickHouseDB:       getEnv("CLICKHOUSE_DB", "smtpenviador"),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		// API
		APIPort:   getEnv("API_PORT", "80"),
		APISecret: getEnv("API_SECRET", "your_super_secret_jwt_key"),

		// Engine
		WorkersCount:      getEnvInt("WORKERS_COUNT", 10),
		ConnectionsPerSMTP: getEnvInt("CONNECTIONS_PER_SMTP", 5),

		// Tracking
		TrackingDomain: getEnv("TRACKING_DOMAIN", "http://localhost"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
