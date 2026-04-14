package config

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Env        string
	ServerPort string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Bayse API
	BayseRelayURL string
	BayseWSURL    string

	// Polling intervals
	DiscoveryInterval time.Duration
	PollInterval      time.Duration

	// Rate limiting (our API)
	RateLimitRPS   float64
	RateLimitBurst int
}

func (c *Config) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetConfigFile(".env")
	v.SetConfigType("env")

	if err := v.ReadInConfig(); err != nil {
		log.Print(err.Error())
		log.Print("No config file found, falling back to environment variables")
	}

	discoverySeconds := getIntDefault(v, "DISCOVERY_INTERVAL_SECONDS", 300)
	pollSeconds := getIntDefault(v, "POLL_INTERVAL_SECONDS", 30)

	cfg := &Config{
		Env:        getStringDefault(v, "APP_ENV", "development"),
		ServerPort: getStringDefault(v, "SERVER_PORT", "8080"),

		DBHost:     getStringDefault(v, "DB_HOST", "localhost"),
		DBPort:     getStringDefault(v, "DB_PORT", "5432"),
		DBUser:     getStringDefault(v, "DB_USER", "snapshot"),
		DBPassword: v.GetString("DB_PASSWORD"),
		DBName:     getStringDefault(v, "DB_NAME", "orderbook_snapshots"),
		DBSSLMode:  getStringDefault(v, "DB_SSLMODE", "disable"),

		BayseRelayURL: getStringDefault(v, "BAYSE_RELAY_URL", "https://relay.bayse.markets"),
		BayseWSURL:    getStringDefault(v, "BAYSE_WS_URL", "wss://socket.bayse.markets/ws/v1/markets"),

		DiscoveryInterval: time.Duration(discoverySeconds) * time.Second,
		PollInterval:      time.Duration(pollSeconds) * time.Second,

		RateLimitRPS:   getFloat64Default(v, "RATE_LIMIT_RPS", 10),
		RateLimitBurst: getIntDefault(v, "RATE_LIMIT_BURST", 20),
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	var missing []string
	if cfg.DBPassword == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
}

func getStringDefault(v *viper.Viper, key, fallback string) string {
	if value := v.GetString(key); value != "" {
		return value
	}
	return fallback
}

func getIntDefault(v *viper.Viper, key string, fallback int) int {
	if v.IsSet(key) {
		return v.GetInt(key)
	}
	return fallback
}

func getFloat64Default(v *viper.Viper, key string, fallback float64) float64 {
	if v.IsSet(key) {
		return v.GetFloat64(key)
	}
	return fallback
}
