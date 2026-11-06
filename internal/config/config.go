package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the complete runtime configuration, read from environment
// variables. Secret values are never logged.
type Config struct {
	Host         string
	Port         int
	PublicURL    string
	DatabasePath string

	APIToken      string
	EncryptionKey []byte

	LogLevel string

	SyncInterval    time.Duration
	SyncOnStart     bool
	SyncOnCreate    bool
	SyncConcurrency int
	ProbeWorkers    int
	ProbeTimeout    time.Duration
	RequestTimeout  time.Duration

	RateLimitPerMinute int
	CORSOrigins        []string

	RexURL         string
	RexSourceName  string
	RexEnabled     bool
	RexPushBatch   int
	RexSendTMDBIDs bool

	RexSourceUser     string
	RexSourcePassword string
	RexToken          string

	SyncMaxMovies int
	SyncMaxSeries int
	SyncMaxLive   int
	EPGDays       int

	AllowPrivateHosts bool
	FFprobePath       string

	Version string
}

const (
	DefaultPort           = 9981
	DefaultDatabasePath   = "/data/rex-xstream.db"
	DefaultSyncInterval   = 6 * time.Hour
	DefaultProbeTimeout   = 30 * time.Second
	DefaultRequestTimeout = 45 * time.Second
	DefaultRexSourceName  = "rex-xstream"
	DefaultRexPushBatch   = 100
	DefaultEPGDays        = 7
)

// Load reads the environment and validates required secrets.
func Load() (*Config, error) {
	cfg := &Config{
		Host:               env("APP_HOST", "0.0.0.0"),
		Port:               envInt("APP_PORT", DefaultPort),
		PublicURL:          strings.TrimRight(env("PUBLIC_URL", ""), "/"),
		DatabasePath:       env("DATABASE_PATH", DefaultDatabasePath),
		APIToken:           env("API_TOKEN", ""),
		LogLevel:           strings.ToLower(env("LOG_LEVEL", "info")),
		SyncInterval:       envDuration("SYNC_INTERVAL", DefaultSyncInterval),
		SyncOnStart:        envBool("SYNC_ON_START", true),
		SyncOnCreate:       envBool("SYNC_ON_CREATE", true),
		SyncConcurrency:    envInt("SYNC_CONCURRENCY", 4),
		ProbeWorkers:       envInt("PROBE_WORKERS", 4),
		ProbeTimeout:       envDuration("PROBE_TIMEOUT", DefaultProbeTimeout),
		RequestTimeout:     envDuration("HTTP_TIMEOUT", DefaultRequestTimeout),
		RateLimitPerMinute: envInt("RATE_LIMIT_PER_MINUTE", 120),
		CORSOrigins:        splitCSV(env("CORS_ORIGINS", "")),
		RexURL:             strings.TrimRight(env("REX_URL", ""), "/"),
		RexSourceName:      env("REX_SOURCE_NAME", DefaultRexSourceName),
		RexEnabled:         envBool("REX_ENABLED", true),
		RexPushBatch:       envInt("REX_PUSH_BATCH", DefaultRexPushBatch),
		RexSourceUser:      env("REX_SOURCE_USER", "rex"),
		RexSourcePassword:  env("REX_SOURCE_PASSWORD", ""),
		RexToken:           env("REX_TOKEN", ""),
		SyncMaxMovies:      envInt("SYNC_MAX_MOVIES", 0),
		SyncMaxSeries:      envInt("SYNC_MAX_SERIES", 0),
		SyncMaxLive:        envInt("SYNC_MAX_LIVE", 0),
		EPGDays:            envInt("EPG_DAYS", DefaultEPGDays),
		AllowPrivateHosts:  envBool("XC_ALLOW_PRIVATE_HOSTS", false),
		FFprobePath:        env("FFPROBE_PATH", "ffprobe"),
		Version:            env("APP_VERSION", "dev"),
	}
	key, err := decodeKey(env("ENCRYPTION_KEY", ""))
	if err != nil {
		return nil, err
	}
	cfg.EncryptionKey = key
	if len(cfg.APIToken) < 16 {
		return nil, errors.New("API_TOKEN must be set and at least 16 characters")
	}
	if cfg.PublicURL == "" {
		return nil, errors.New("PUBLIC_URL is required because rex fetches streams from it")
	}
	if cfg.RexEnabled && cfg.RexURL == "" {
		return nil, errors.New("REX_URL is required when REX_ENABLED is true")
	}
	if cfg.RexEnabled && len(cfg.RexSourcePassword) < 8 {
		return nil, errors.New("REX_SOURCE_PASSWORD must be at least 8 characters")
	}
	if cfg.SyncConcurrency < 1 {
		cfg.SyncConcurrency = 1
	}
	if cfg.ProbeWorkers < 1 {
		cfg.ProbeWorkers = 1
	}
	if cfg.RexPushBatch < 1 {
		cfg.RexPushBatch = 1
	}
	if cfg.EPGDays < 1 {
		cfg.EPGDays = 1
	}
	return cfg, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func decodeKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, errors.New("ENCRYPTION_KEY is required (base64 of 32 bytes)")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("ENCRYPTION_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
