package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains application configuration
type Config struct {
	// PostgreSQL
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBMaxConns int
	DBMinConns int

	// Kafka
	KafkaBrokers []string
	KafkaTopic   string
	KafkaGroupId string

	// HTTP Server
	HTTPPort string

	// Cache
	CacheMaxItems int
	CacheTTL      time.Duration
}

// Load returns app configuration from the environment variables
func Load() *Config {
	return &Config{
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnvAsInt("DB_PORT", 5432),
		DBUser:        getEnv("DB_USER", "orders_user"),
		DBPassword:    getEnv("DB_PASSWORD", "orders_password"),
		DBName:        getEnv("DB_NAME", "orders_db"),
		DBMaxConns:    getEnvAsInt("DB_MAX_CONNS", 10),
		DBMinConns:    getEnvAsInt("DB_MIN_CONNS", 2),
		KafkaBrokers:  strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
		KafkaTopic:    getEnv("KAFKA_TOPIC", "orders"),
		KafkaGroupId:  getEnv("KAFKA_GROUP_ID", "orders-service"),
		HTTPPort:      getEnv("HTTP_PORT", "8081"),
		CacheMaxItems: getEnvAsInt("CACHE_MAX_ITEMS", 1000),
		CacheTTL:      getEnvAsDuration("CACHE_TTL", 30*time.Minute),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
