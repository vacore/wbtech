package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear relevant env vars
	envVars := []string{
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
		"DB_MAX_CONNS", "DB_MIN_CONNS",
		"KAFKA_BROKERS", "KAFKA_TOPIC", "KAFKA_GROUP_ID",
		"HTTP_PORT", "CACHE_MAX_ITEMS", "CACHE_TTL",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}

	cfg := Load()

	// Check defaults
	if cfg.DB.Host != "localhost" {
		t.Errorf("Expected DBHost=localhost, got %s", cfg.DB.Host)
	}
	if cfg.DB.Port != 5432 {
		t.Errorf("Expected DBPort=5432, got %d", cfg.DB.Port)
	}
	if cfg.DB.User != "orders_user" {
		t.Errorf("Expected DBUser=orders_user, got %s", cfg.DB.User)
	}
	if cfg.DB.Password != "orders_password" {
		t.Errorf("Expected DBPassword=orders_password, got %s", cfg.DB.Password)
	}
	if cfg.DB.Name != "orders_db" {
		t.Errorf("Expected DBName=orders_db, got %s", cfg.DB.Name)
	}
	if cfg.DB.MaxConns != 10 {
		t.Errorf("Expected DBMaxConns=10, got %d", cfg.DB.MaxConns)
	}
	if cfg.DB.MinConns != 2 {
		t.Errorf("Expected DBMinConns=2, got %d", cfg.DB.MinConns)
	}
	if len(cfg.Kafka.Brokers) != 1 || cfg.Kafka.Brokers[0] != "localhost:9092" {
		t.Errorf("Expected KafkaBrokers=[localhost:9092], got %v", cfg.Kafka.Brokers)
	}
	if cfg.Kafka.Topic != "orders" {
		t.Errorf("Expected KafkaTopic=orders, got %s", cfg.Kafka.Topic)
	}
	if cfg.Kafka.GroupID != "orders-service" {
		t.Errorf("Expected KafkaGroupId=orders-service, got %s", cfg.Kafka.GroupID)
	}
	if cfg.HTTP.Port != "8081" {
		t.Errorf("Expected HTTPPort=8081, got %s", cfg.HTTP.Port)
	}
	if cfg.Cache.MaxItems != 1000 {
		t.Errorf("Expected CacheMaxItems=1000, got %d", cfg.Cache.MaxItems)
	}
	if cfg.Cache.TTL != 30*time.Minute {
		t.Errorf("Expected CacheTTL=30m, got %v", cfg.Cache.TTL)
	}
}

func TestLoad_FromEnv(t *testing.T) {
	// Set custom env vars
	t.Setenv("DB_HOST", "custom-host")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "custom_user")
	t.Setenv("DB_PASSWORD", "custom_pass")
	t.Setenv("DB_NAME", "custom_db")
	t.Setenv("DB_MAX_CONNS", "20")
	t.Setenv("DB_MIN_CONNS", "5")
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("KAFKA_TOPIC", "custom-topic")
	t.Setenv("KAFKA_GROUP_ID", "custom-group")
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("CACHE_MAX_ITEMS", "500")
	t.Setenv("CACHE_TTL", "1h")

	cfg := Load()

	if cfg.DB.Host != "custom-host" {
		t.Errorf("Expected DBHost=custom-host, got %s", cfg.DB.Host)
	}
	if cfg.DB.Port != 5433 {
		t.Errorf("Expected DBPort=5433, got %d", cfg.DB.Port)
	}
	if cfg.DB.User != "custom_user" {
		t.Errorf("Expected DBUser=custom_user, got %s", cfg.DB.User)
	}
	if cfg.DB.Password != "custom_pass" {
		t.Errorf("Expected DBPassword=custom_pass, got %s", cfg.DB.Password)
	}
	if cfg.DB.Name != "custom_db" {
		t.Errorf("Expected DBName=custom_db, got %s", cfg.DB.Name)
	}
	if cfg.DB.MaxConns != 20 {
		t.Errorf("Expected DBMaxConns=20, got %d", cfg.DB.MaxConns)
	}
	if cfg.DB.MinConns != 5 {
		t.Errorf("Expected DBMinConns=5, got %d", cfg.DB.MinConns)
	}
	if cfg.Kafka.Brokers[0] != "kafka:9092" {
		t.Errorf("Expected KafkaBrokers=[kafka:9092], got %v", cfg.Kafka.Brokers)
	}
	if cfg.Kafka.Topic != "custom-topic" {
		t.Errorf("Expected KafkaTopic=custom-topic, got %s", cfg.Kafka.Topic)
	}
	if cfg.Kafka.GroupID != "custom-group" {
		t.Errorf("Expected KafkaGroupId=custom-group, got %s", cfg.Kafka.GroupID)
	}
	if cfg.HTTP.Port != "9090" {
		t.Errorf("Expected HTTPPort=9090, got %s", cfg.HTTP.Port)
	}
	if cfg.Cache.MaxItems != 500 {
		t.Errorf("Expected CacheMaxItems=500, got %d", cfg.Cache.MaxItems)
	}
	if cfg.Cache.TTL != time.Hour {
		t.Errorf("Expected CacheTTL=1h, got %v", cfg.Cache.TTL)
	}
}

func TestLoad_InvalidIntFallsBackToDefault(t *testing.T) {
	t.Setenv("DB_PORT", "invalid")
	t.Setenv("CACHE_MAX_ITEMS", "not-a-number")
	t.Setenv("DB_MAX_CONNS", "abc")
	t.Setenv("DB_MIN_CONNS", "xyz")

	cfg := Load()

	if cfg.DB.Port != 5432 {
		t.Errorf("Expected DBPort=5432 (default), got %d", cfg.DB.Port)
	}
	if cfg.Cache.MaxItems != 1000 {
		t.Errorf("Expected CacheMaxItems=1000 (default), got %d", cfg.Cache.MaxItems)
	}
	if cfg.DB.MaxConns != 10 {
		t.Errorf("Expected DBMaxConns=10 (default), got %d", cfg.DB.MaxConns)
	}
	if cfg.DB.MinConns != 2 {
		t.Errorf("Expected DBMinConns=2 (default), got %d", cfg.DB.MinConns)
	}
}

func TestLoad_InvalidDurationFallsBackToDefault(t *testing.T) {
	t.Setenv("CACHE_TTL", "invalid-duration")
	defer os.Unsetenv("CACHE_TTL")

	cfg := Load()

	if cfg.Cache.TTL != 30*time.Minute {
		t.Errorf("Expected CacheTTL=30m (default), got %v", cfg.Cache.TTL)
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue string
		expected     string
		setEnv       bool
	}{
		{
			name:         "returns env value when set",
			key:          "TEST_VAR",
			envValue:     "custom_value",
			defaultValue: "default",
			expected:     "custom_value",
			setEnv:       true,
		},
		{
			name:         "returns default when not set",
			key:          "TEST_VAR_UNSET",
			envValue:     "",
			defaultValue: "default",
			expected:     "default",
			setEnv:       false,
		},
		{
			name:         "returns empty string if env is empty",
			key:          "TEST_VAR_EMPTY",
			envValue:     "",
			defaultValue: "default",
			expected:     "",
			setEnv:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnv(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetEnvAsInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue int
		expected     int
		setEnv       bool
	}{
		{
			name:         "parses valid int",
			key:          "TEST_INT",
			envValue:     "42",
			defaultValue: 10,
			expected:     42,
			setEnv:       true,
		},
		{
			name:         "returns default for invalid int",
			key:          "TEST_INT_INVALID",
			envValue:     "not-a-number",
			defaultValue: 10,
			expected:     10,
			setEnv:       true,
		},
		{
			name:         "returns default when not set",
			key:          "TEST_INT_UNSET",
			envValue:     "",
			defaultValue: 10,
			expected:     10,
			setEnv:       false,
		},
		{
			name:         "parses negative int",
			key:          "TEST_INT_NEG",
			envValue:     "-5",
			defaultValue: 10,
			expected:     -5,
			setEnv:       true,
		},
		{
			name:         "parses zero",
			key:          "TEST_INT_ZERO",
			envValue:     "0",
			defaultValue: 10,
			expected:     0,
			setEnv:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvAsInt(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetEnvAsDuration(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue time.Duration
		expected     time.Duration
		setEnv       bool
	}{
		{
			name:         "parses minutes",
			key:          "TEST_DUR_MIN",
			envValue:     "15m",
			defaultValue: time.Hour,
			expected:     15 * time.Minute,
			setEnv:       true,
		},
		{
			name:         "parses hours",
			key:          "TEST_DUR_HOUR",
			envValue:     "2h",
			defaultValue: time.Minute,
			expected:     2 * time.Hour,
			setEnv:       true,
		},
		{
			name:         "parses seconds",
			key:          "TEST_DUR_SEC",
			envValue:     "30s",
			defaultValue: time.Minute,
			expected:     30 * time.Second,
			setEnv:       true,
		},
		{
			name:         "parses complex duration",
			key:          "TEST_DUR_COMPLEX",
			envValue:     "1h30m",
			defaultValue: time.Minute,
			expected:     90 * time.Minute,
			setEnv:       true,
		},
		{
			name:         "returns default for invalid",
			key:          "TEST_DUR_INVALID",
			envValue:     "invalid",
			defaultValue: time.Hour,
			expected:     time.Hour,
			setEnv:       true,
		},
		{
			name:         "returns default when not set",
			key:          "TEST_DUR_UNSET",
			envValue:     "",
			defaultValue: time.Hour,
			expected:     time.Hour,
			setEnv:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvAsDuration(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
