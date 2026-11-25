package mcp_compiler

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// LogLevel defines logging verbosity.
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelNotice
	LogLevelWarning
	LogLevelError
	LogLevelCritical
)

// String returns human-readable level name.
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelNotice:
		return "NOTICE"
	case LogLevelWarning:
		return "WARNING"
	case LogLevelError:
		return "ERROR"
	case LogLevelCritical:
		return "CRITICAL"
	default:
		return "INFO"
	}
}

// ParseLogLevel parses string into LogLevel.
func ParseLogLevel(str string) LogLevel {
	switch strings.ToUpper(strings.TrimSpace(str)) {
	case "DEBUG":
		return LogLevelDebug
	case "INFO":
		return LogLevelInfo
	case "NOTICE":
		return LogLevelNotice
	case "WARN", "WARNING":
		return LogLevelWarning
	case "ERROR":
		return LogLevelError
	case "CRIT", "CRITICAL":
		return LogLevelCritical
	default:
		return LogLevelInfo
	}
}

// Config holds production runtime options for the MCP server & Maestro compiler.
type Config struct {
	AppID              string        `json:"app_id"`
	LogLevel           LogLevel      `json:"log_level"`
	MaxMessageSizeBytes int          `json:"max_message_size_bytes"`
	RequestTimeout     time.Duration `json:"request_timeout"`
	DrainTimeout       time.Duration `json:"drain_timeout"`
	DefaultRepeatCount int           `json:"default_repeat_count"`
	AutoOptimizeFlow   bool          `json:"auto_optimize_flow"`
	IncludeComments    bool          `json:"include_comments"`
	EnableMetrics      bool          `json:"enable_metrics"`
}

// DefaultConfig provides sensible production defaults.
func DefaultConfig() *Config {
	return &Config{
		AppID:               "com.prism.v",
		LogLevel:            LogLevelInfo,
		MaxMessageSizeBytes: 16 * 1024 * 1024, // 16 MB buffer for base64 screens and dense ASTs
		RequestTimeout:      60 * time.Second,
		DrainTimeout:        5 * time.Second,
		DefaultRepeatCount:  3,
		AutoOptimizeFlow:    true,
		IncludeComments:     true,
		EnableMetrics:       true,
	}
}

// LoadConfigFromEnv populates configuration overrides from environment variables.
func LoadConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if appID := os.Getenv("NANO_MCP_APP_ID"); appID != "" {
		cfg.AppID = appID
	}
	if lvl := os.Getenv("NANO_MCP_LOG_LEVEL"); lvl != "" {
		cfg.LogLevel = ParseLogLevel(lvl)
	}
	if maxMsg := os.Getenv("NANO_MCP_MAX_MSG_MB"); maxMsg != "" {
		if val, err := strconv.Atoi(maxMsg); err == nil && val > 0 {
			cfg.MaxMessageSizeBytes = val * 1024 * 1024
		}
	}
	if reqTimeout := os.Getenv("NANO_MCP_REQUEST_TIMEOUT_SEC"); reqTimeout != "" {
		if val, err := strconv.Atoi(reqTimeout); err == nil && val > 0 {
			cfg.RequestTimeout = time.Duration(val) * time.Second
		}
	}
	if repeat := os.Getenv("NANO_MCP_REPEAT_COUNT"); repeat != "" {
		if val, err := strconv.Atoi(repeat); err == nil && val > 0 {
			cfg.DefaultRepeatCount = val
		}
	}

	return cfg
}
