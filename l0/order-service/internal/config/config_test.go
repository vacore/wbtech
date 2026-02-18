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
	if cfg.DBHost != "localhost" {
		t.Errorf("Expected DBHost=localhost, got %s", cfg.DBHost)
	}
	if cfg.DBPort != 5432 {
		t.Errorf("Expected DBPort=5432, got %d", cfg.DBPort)
	}
	if cfg.DBUser != "orders_user" {
		t.Errorf("Expected DBUser=orders_user, got %s", cfg.DBUser)
	}
	if cfg.DBPassword != "orders_password" {
		t.Errorf("Expected DBPassword=orders_password, got %s", cfg.DBPassword)
	}
	if cfg.DBName != "orders_db" {
		t.Errorf("Expected DBName=orders_db, got %s", cfg.DBName)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("Expected DBMaxConns=10, got %d", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 2 {
		t.Errorf("Expected DBMinConns=2, got %d", cfg.DBMinConns)
	}
	if len(cfg.KafkaBrokers) != 1 || cfg.KafkaBrokers[0] != "localhost:9092" {
		t.Errorf("Expected KafkaBrokers=[localhost:9092], got %v", cfg.KafkaBrokers)
	}
	if cfg.KafkaTopic != "orders" {
		t.Errorf("Expected KafkaTopic=orders, got %s", cfg.KafkaTopic)
	}
	if cfg.KafkaGroupId != "orders-service" {
		t.Errorf("Expected KafkaGroupId=orders-service, got %s", cfg.KafkaGroupId)
	}
	if cfg.HTTPPort != "8081" {
		t.Errorf("Expected HTTPPort=8081, got %s", cfg.HTTPPort)
	}
	if cfg.CacheMaxItems != 1000 {
		t.Errorf("Expected CacheMaxItems=1000, got %d", cfg.CacheMaxItems)
	}
	if cfg.CacheTTL != 30*time.Minute {
		t.Errorf("Expected CacheTTL=30m, got %v", cfg.CacheTTL)
	}
}

func TestLoad_FromEnv(t *testing.T) {
	// Set custom env vars
	os.Setenv("DB_HOST", "custom-host")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("DB_USER", "custom_user")
	os.Setenv("DB_PASSWORD", "custom_pass")
	os.Setenv("DB_NAME", "custom_db")
	os.Setenv("DB_MAX_CONNS", "20")
	os.Setenv("DB_MIN_CONNS", "5")
	os.Setenv("KAFKA_BROKERS", "kafka:9092")
	os.Setenv("KAFKA_TOPIC", "custom-topic")
	os.Setenv("KAFKA_GROUP_ID", "custom-group")
	os.Setenv("HTTP_PORT", "9090")
	os.Setenv("CACHE_MAX_ITEMS", "500")
	os.Setenv("CACHE_TTL", "1h")

	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("DB_MAX_CONNS")
		os.Unsetenv("DB_MIN_CONNS")
		os.Unsetenv("KAFKA_BROKERS")
		os.Unsetenv("KAFKA_TOPIC")
		os.Unsetenv("KAFKA_GROUP_ID")
		os.Unsetenv("HTTP_PORT")
		os.Unsetenv("CACHE_MAX_ITEMS")
		os.Unsetenv("CACHE_TTL")
	}()

	cfg := Load()

	if cfg.DBHost != "custom-host" {
		t.Errorf("Expected DBHost=custom-host, got %s", cfg.DBHost)
	}
	if cfg.DBPort != 5433 {
		t.Errorf("Expected DBPort=5433, got %d", cfg.DBPort)
	}
	if cfg.DBUser != "custom_user" {
		t.Errorf("Expected DBUser=custom_user, got %s", cfg.DBUser)
	}
	if cfg.DBPassword != "custom_pass" {
		t.Errorf("Expected DBPassword=custom_pass, got %s", cfg.DBPassword)
	}
	if cfg.DBName != "custom_db" {
		t.Errorf("Expected DBName=custom_db, got %s", cfg.DBName)
	}
	if cfg.DBMaxConns != 20 {
		t.Errorf("Expected DBMaxConns=20, got %d", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 5 {
		t.Errorf("Expected DBMinConns=5, got %d", cfg.DBMinConns)
	}
	if cfg.KafkaBrokers[0] != "kafka:9092" {
		t.Errorf("Expected KafkaBrokers=[kafka:9092], got %v", cfg.KafkaBrokers)
	}
	if cfg.KafkaTopic != "custom-topic" {
		t.Errorf("Expected KafkaTopic=custom-topic, got %s", cfg.KafkaTopic)
	}
	if cfg.KafkaGroupId != "custom-group" {
		t.Errorf("Expected KafkaGroupId=custom-group, got %s", cfg.KafkaGroupId)
	}
	if cfg.HTTPPort != "9090" {
		t.Errorf("Expected HTTPPort=9090, got %s", cfg.HTTPPort)
	}
	if cfg.CacheMaxItems != 500 {
		t.Errorf("Expected CacheMaxItems=500, got %d", cfg.CacheMaxItems)
	}
	if cfg.CacheTTL != time.Hour {
		t.Errorf("Expected CacheTTL=1h, got %v", cfg.CacheTTL)
	}
}

func TestLoad_InvalidIntFallsBackToDefault(t *testing.T) {
	os.Setenv("DB_PORT", "invalid")
	os.Setenv("CACHE_MAX_ITEMS", "not-a-number")
	os.Setenv("DB_MAX_CONNS", "abc")
	os.Setenv("DB_MIN_CONNS", "xyz")
	defer func() {
		os.Unsetenv("DB_PORT")
		os.Unsetenv("CACHE_MAX_ITEMS")
		os.Unsetenv("DB_MAX_CONNS")
		os.Unsetenv("DB_MIN_CONNS")
	}()

	cfg := Load()

	if cfg.DBPort != 5432 {
		t.Errorf("Expected DBPort=5432 (default), got %d", cfg.DBPort)
	}
	if cfg.CacheMaxItems != 1000 {
		t.Errorf("Expected CacheMaxItems=1000 (default), got %d", cfg.CacheMaxItems)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("Expected DBMaxConns=10 (default), got %d", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 2 {
		t.Errorf("Expected DBMinConns=2 (default), got %d", cfg.DBMinConns)
	}
}

func TestLoad_InvalidDurationFallsBackToDefault(t *testing.T) {
	os.Setenv("CACHE_TTL", "invalid-duration")
	defer os.Unsetenv("CACHE_TTL")

	cfg := Load()

	if cfg.CacheTTL != 30*time.Minute {
		t.Errorf("Expected CacheTTL=30m (default), got %v", cfg.CacheTTL)
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
				os.Setenv(tt.key, tt.envValue)
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
				os.Setenv(tt.key, tt.envValue)
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
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvAsDuration(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
