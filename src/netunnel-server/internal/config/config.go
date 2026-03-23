package config

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultDatabaseURL = "postgresql://ai_ssh_user:ai_ssh_password@127.0.0.1:5432/netunnel"
const defaultListenAddr = ":40061"
const defaultBridgeListenAddr = ":40062"
const defaultSettlementInterval = 1 * time.Minute
const defaultConfigPath = "config.yaml"

type PortRange struct {
	Start int `yaml:"start"`
	End   int `yaml:"end"`
}

type yamlConfig struct {
	TCPPortRanges    []PortRange `yaml:"tcp_port_ranges"`
	PublicHost       string      `yaml:"public_host"`
	PublicAPIBaseURL string      `yaml:"public_api_base_url"`
	HostDomainSuffix string      `yaml:"host_domain_suffix"`
}

type Config struct {
	DatabaseURL        string
	MigrationsDir      string
	ListenAddr         string
	BridgeListenAddr   string
	SettlementInterval time.Duration
	TCPPortRanges      []PortRange
	PublicHost         string
	PublicAPIBaseURL   string
	HostDomainSuffix   string
}

func Load() (Config, error) {
	var cfg Config

	cfg.MigrationsDir = resolveMigrationsDir()
	databaseURLFlag := flag.String("database-url", envOrDefault("NETUNNEL_DATABASE_URL", defaultDatabaseURL), "PostgreSQL connection string")
	configPathFlag := flag.String("config", envOrDefault("NETUNNEL_CONFIG", defaultConfigPath), "YAML config file path")
	listenAddrFlag := flag.String("listen-addr", envOrDefault("NETUNNEL_LISTEN_ADDR", defaultListenAddr), "HTTP listen address")
	bridgeListenAddrFlag := flag.String("bridge-listen-addr", envOrDefault("NETUNNEL_BRIDGE_LISTEN_ADDR", defaultBridgeListenAddr), "TCP bridge listen address")
	settlementIntervalFlag := flag.Duration("settlement-interval", envDurationOrDefault("NETUNNEL_SETTLEMENT_INTERVAL", defaultSettlementInterval), "Automatic settlement interval, 0 to disable")
	flag.Parse()

	cfg.DatabaseURL = *databaseURLFlag
	cfg.ListenAddr = *listenAddrFlag
	cfg.BridgeListenAddr = *bridgeListenAddrFlag
	cfg.SettlementInterval = *settlementIntervalFlag

	yamlPath := *configPathFlag
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return Config{}, errors.New("failed to read config file: " + err.Error())
		}
	} else {
		var yc yamlConfig
		if err := yaml.Unmarshal(data, &yc); err != nil {
			return Config{}, errors.New("failed to parse config file: " + err.Error())
		}
		cfg.TCPPortRanges = yc.TCPPortRanges
		cfg.PublicHost = yc.PublicHost
		cfg.PublicAPIBaseURL = yc.PublicAPIBaseURL
		cfg.HostDomainSuffix = yc.HostDomainSuffix
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("database url is required")
	}
	if cfg.TCPPortRanges == nil {
		cfg.TCPPortRanges = []PortRange{{Start: 40000, End: 45000}, {Start: 50000, End: 60000}}
	}

	return cfg, nil
}

func resolveMigrationsDir() string {
	candidates := []string{
		filepath.Clean(filepath.Join(".", "sql")),
		filepath.Clean(filepath.Join("..", "..", "sql")),
	}

	for _, candidate := range candidates {
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate
		}
	}

	return candidates[0]
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}
