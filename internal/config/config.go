package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

type Config struct {
	Neo4jURI      string
	Neo4jUser     string
	Neo4jPassword string
	Neo4jDatabase string

	LogFilePath string
	ServerName  string
	ServerRole  string

	BatchSize       int
	WorkerCount     int
	FlushInterval   time.Duration
	MaxWriteRetries int
	RetryDelay      time.Duration
	OffsetStatePath string
	DryRun          bool

	IgnoreReverseLookup bool
	IgnoredDomains      []string
	LocalDomains        []string
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Neo4jURI:        "neo4j://127.0.0.1:7687",
		Neo4jUser:       "neo4j",
		Neo4jDatabase:   "neo4j",
		LogFilePath:     "dns.log",
		ServerName:      hostname(),
		ServerRole:      "primary",
		BatchSize:       2000,
		WorkerCount:     4,
		FlushInterval:   5 * time.Second,
		MaxWriteRetries: 3,
		RetryDelay:      time.Second,
		OffsetStatePath: "state/dnslog.offset.json",
		DryRun:          true,

		IgnoreReverseLookup: true,
	}

	if _, err := os.Stat(path); err == nil {
		file, err := ini.Load(path)
		if err != nil {
			return nil, fmt.Errorf("load config %q: %w", path, err)
		}

		cfg.Neo4jURI = file.Section("neo4j").Key("uri").MustString(cfg.Neo4jURI)
		cfg.Neo4jUser = file.Section("neo4j").Key("user").MustString(cfg.Neo4jUser)
		cfg.Neo4jPassword = file.Section("neo4j").Key("password").MustString(cfg.Neo4jPassword)
		cfg.Neo4jDatabase = file.Section("neo4j").Key("database").MustString(cfg.Neo4jDatabase)

		cfg.LogFilePath = file.Section("log").Key("file_path").MustString(cfg.LogFilePath)
		cfg.ServerName = file.Section("server").Key("name").MustString(cfg.ServerName)
		cfg.ServerRole = file.Section("server").Key("role").MustString(cfg.ServerRole)

		cfg.BatchSize = file.Section("ingest").Key("batch_size").MustInt(cfg.BatchSize)
		cfg.WorkerCount = file.Section("ingest").Key("worker_count").MustInt(cfg.WorkerCount)
		cfg.FlushInterval = file.Section("ingest").Key("flush_interval").MustDuration(cfg.FlushInterval)
		cfg.MaxWriteRetries = file.Section("ingest").Key("max_write_retries").MustInt(cfg.MaxWriteRetries)
		cfg.RetryDelay = file.Section("ingest").Key("retry_delay").MustDuration(cfg.RetryDelay)
		cfg.OffsetStatePath = file.Section("ingest").Key("offset_state_path").MustString(cfg.OffsetStatePath)
		cfg.DryRun = file.Section("ingest").Key("dry_run").MustBool(cfg.DryRun)

		cfg.IgnoreReverseLookup = file.Section("filter").Key("ignore_reverse_lookup").MustBool(cfg.IgnoreReverseLookup)
		cfg.IgnoredDomains = splitList(file.Section("filter").Key("ignored_domains").String())
		cfg.LocalDomains = splitList(file.Section("filter").Key("local_domains").String())
	}

	overrideString("DNSLOG_NEO4J_URI", &cfg.Neo4jURI)
	overrideString("DNSLOG_NEO4J_USER", &cfg.Neo4jUser)
	overrideString("DNSLOG_NEO4J_PASSWORD", &cfg.Neo4jPassword)
	overrideString("DNSLOG_NEO4J_DATABASE", &cfg.Neo4jDatabase)
	overrideString("DNSLOG_FILE_PATH", &cfg.LogFilePath)
	overrideString("DNSLOG_SERVER_NAME", &cfg.ServerName)
	overrideString("DNSLOG_SERVER_ROLE", &cfg.ServerRole)
	overrideInt("DNSLOG_BATCH_SIZE", &cfg.BatchSize)
	overrideInt("DNSLOG_WORKER_COUNT", &cfg.WorkerCount)
	overrideDuration("DNSLOG_FLUSH_INTERVAL", &cfg.FlushInterval)
	overrideInt("DNSLOG_MAX_WRITE_RETRIES", &cfg.MaxWriteRetries)
	overrideDuration("DNSLOG_RETRY_DELAY", &cfg.RetryDelay)
	overrideString("DNSLOG_OFFSET_STATE_PATH", &cfg.OffsetStatePath)
	overrideBool("DNSLOG_DRY_RUN", &cfg.DryRun)
	overrideBool("DNSLOG_IGNORE_REVERSE_LOOKUP", &cfg.IgnoreReverseLookup)
	overrideStringList("DNSLOG_IGNORED_DOMAINS", &cfg.IgnoredDomains)
	overrideStringList("DNSLOG_LOCAL_DOMAINS", &cfg.LocalDomains)

	if cfg.BatchSize <= 0 {
		return nil, fmt.Errorf("batch_size must be greater than zero")
	}
	if cfg.WorkerCount <= 0 {
		return nil, fmt.Errorf("worker_count must be greater than zero")
	}
	if cfg.FlushInterval <= 0 {
		return nil, fmt.Errorf("flush_interval must be greater than zero")
	}
	if cfg.MaxWriteRetries < 0 {
		return nil, fmt.Errorf("max_write_retries must not be negative")
	}
	if cfg.RetryDelay < 0 {
		return nil, fmt.Errorf("retry_delay must not be negative")
	}
	if cfg.OffsetStatePath == "" {
		return nil, fmt.Errorf("offset_state_path must not be empty")
	}
	if cfg.ServerName == "" {
		return nil, fmt.Errorf("server name must not be empty")
	}
	if cfg.Neo4jPassword == "" && !cfg.DryRun {
		return nil, fmt.Errorf("neo4j password must be set when dry_run=false")
	}

	return cfg, nil
}

func overrideString(name string, target *string) {
	if value := os.Getenv(name); value != "" {
		*target = value
	}
}

func overrideInt(name string, target *int) {
	value := os.Getenv(name)
	if value == "" {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err == nil {
		*target = parsed
	}
}

func overrideBool(name string, target *bool) {
	value := os.Getenv(name)
	if value == "" {
		return
	}
	parsed, err := strconv.ParseBool(value)
	if err == nil {
		*target = parsed
	}
}

func overrideDuration(name string, target *time.Duration) {
	value := os.Getenv(name)
	if value == "" {
		return
	}
	parsed, err := time.ParseDuration(value)
	if err == nil {
		*target = parsed
	}
}

func overrideStringList(name string, target *[]string) {
	value := os.Getenv(name)
	if value == "" {
		return
	}
	*target = splitList(value)
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "dns-server"
	}
	return name
}
