package config

import (
	"errors"
	"flag"
	"os"
	"strconv"
)

const (
	defaultServerURL    = "http://127.0.0.1:40061"
	defaultBridgeAddr   = "127.0.0.1:40062"
	defaultAgentName    = "local-agent"
	defaultMachineCode  = "machine-local"
	defaultClientVer    = "0.1.0"
	defaultOSType       = "windows"
	defaultSyncInterval = 30
)

type Config struct {
	ServerURL     string
	BridgeAddr    string
	UserID        string
	AgentName     string
	MachineCode   string
	ClientVersion string
	OSType        string
	SyncIntervalS int
}

func Load() (Config, error) {
	serverURL := flag.String("server-url", envOrDefault("NETUNNEL_SERVER_URL", defaultServerURL), "Netunnel server base URL")
	bridgeAddr := flag.String("bridge-addr", envOrDefault("NETUNNEL_BRIDGE_ADDR", defaultBridgeAddr), "Netunnel TCP bridge address")
	userID := flag.String("user-id", envOrDefault("NETUNNEL_USER_ID", ""), "Netunnel user id")
	agentName := flag.String("agent-name", envOrDefault("NETUNNEL_AGENT_NAME", defaultAgentName), "Agent display name")
	machineCode := flag.String("machine-code", envOrDefault("NETUNNEL_MACHINE_CODE", defaultMachineCode), "Stable machine code")
	clientVersion := flag.String("client-version", envOrDefault("NETUNNEL_CLIENT_VERSION", defaultClientVer), "Agent version")
	osType := flag.String("os-type", envOrDefault("NETUNNEL_OS_TYPE", defaultOSType), "Agent OS type")
	syncInterval := flag.Int("sync-interval", envOrDefaultInt("NETUNNEL_SYNC_INTERVAL", defaultSyncInterval), "Config sync interval in seconds")
	flag.Parse()

	cfg := Config{
		ServerURL:     *serverURL,
		BridgeAddr:    *bridgeAddr,
		UserID:        *userID,
		AgentName:     *agentName,
		MachineCode:   *machineCode,
		ClientVersion: *clientVersion,
		OSType:        *osType,
		SyncIntervalS: *syncInterval,
	}

	if cfg.ServerURL == "" {
		return Config{}, errors.New("server url is required")
	}
	if cfg.BridgeAddr == "" {
		return Config{}, errors.New("bridge addr is required")
	}
	if cfg.UserID == "" {
		return Config{}, errors.New("user id is required")
	}
	if cfg.AgentName == "" || cfg.MachineCode == "" {
		return Config{}, errors.New("agent name and machine code are required")
	}
	if cfg.SyncIntervalS <= 0 {
		return Config{}, errors.New("sync interval must be greater than 0")
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
