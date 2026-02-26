package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	MaxConns int
	MinConns int
}

type KafkaConfig struct {
	Brokers  []string
	Topic    string
	GroupID  string
	DLQTopic string
}

type HTTPConfig struct {
	Port string
}

type CacheConfig struct {
	MaxItems int
	TTL      time.Duration
}

type MetricsConfig struct {
	Enabled bool
}

type TracingConfig struct {
	Enabled  bool
	Exporter string // "stdout", "noop"
}

type LogConfig struct {
	Level string // "debug", "info", "warn", "error"
}

// Config contains application configuration
type Config struct {
	DB      DBConfig
	Kafka   KafkaConfig
	HTTP    HTTPConfig
	Cache   CacheConfig
	Metrics MetricsConfig
	Tracing TracingConfig
	Log     LogConfig
}

// Load returns app configuration from the environment variables
func Load() *Config {
	return &Config{
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvAsInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "orders_user"),
			Password: getEnv("DB_PASSWORD", "orders_password"),
			Name:     getEnv("DB_NAME", "orders_db"),
			MaxConns: getEnvAsInt("DB_MAX_CONNS", 10),
			MinConns: getEnvAsInt("DB_MIN_CONNS", 2),
		},
		Kafka: KafkaConfig{
			Brokers:  strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
			Topic:    getEnv("KAFKA_TOPIC", "orders"),
			GroupID:  getEnv("KAFKA_GROUP_ID", "orders-service"),
			DLQTopic: getEnv("KAFKA_DLQ_TOPIC", "orders-dlq"),
		},
		HTTP: HTTPConfig{
			Port: getEnv("HTTP_PORT", "8081"),
		},
		Cache: CacheConfig{
			MaxItems: getEnvAsInt("CACHE_MAX_ITEMS", 1000),
			TTL:      getEnvAsDuration("CACHE_TTL", 30*time.Minute),
		},
		Metrics: MetricsConfig{
			Enabled: getEnvAsBool("METRICS_ENABLED", true),
		},
		Tracing: TracingConfig{
			Enabled:  getEnvAsBool("TRACING_ENABLED", false),
			Exporter: getEnv("TRACING_EXPORTER", "stdout"),
		},
		Log: LogConfig{
			Level: getEnv("LOG_LEVEL", "info"),
		},
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

func getEnvAsBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
