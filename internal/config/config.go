// Package config reads the server's settings from environment variables and
// command-line flags. The names mirror the Node implementation so existing
// setups keep working (BMCP_WS_PORT, BMCP_EXTENSION_ID, BMCP_NO_KILL,
// BMCP_MCP_VERSION).
package config

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/ngxuanth/mcp-server/internal/mcp"
)

const DefaultWsPort = 9009

// Config holds everything the server needs to run.
type Config struct {
	// Name and Version are reported to the MCP client.
	Name    string
	Version string

	// MCPVersion is the MCP protocol revision the server prefers; it must be one
	// of mcp.SupportedProtocolVersions. A client may still pin the other one.
	MCPVersion string

	// WsPort is the loopback port the Chrome extension connects to.
	WsPort int
	// ExtensionID, when set, restricts the WebSocket to one extension's Origin.
	ExtensionID string
	// NoKill keeps a busy port as an error instead of killing its listener.
	NoKill bool

	// Launch selects the browser to start and auto-connect on boot. Empty means
	// "do not launch anything": the server only brokers and waits for the user
	// to press Connect in the extension. The only supported value is "chrome".
	Launch string
	// Browser launch options, only used when Launch != "".
	ChromePath   string
	ExtensionDir string
	StartURL     string
	ProfileDir   string
	Headless     string
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load parses flags and environment variables into a Config. Flags take
// precedence over environment variables.
func Load(version string) (Config, error) {
	cfg := Config{
		Name:         "Browser MCP",
		Version:      version,
		MCPVersion:   env("BMCP_MCP_VERSION", mcp.ProtocolV20250618),
		ExtensionID:  os.Getenv("BMCP_EXTENSION_ID"),
		NoKill:       os.Getenv("BMCP_NO_KILL") != "",
		Launch:       os.Getenv("BMCP_LAUNCH"),
		ChromePath:   os.Getenv("BMCP_CHROME_PATH"),
		ExtensionDir: os.Getenv("BMCP_EXTENSION_DIR"),
		StartURL:     env("BMCP_START_URL", "https://example.com"),
		ProfileDir:   os.Getenv("BMCP_PROFILE_DIR"),
		Headless:     os.Getenv("BMCP_HEADLESS"),
	}

	port, err := parsePort(os.Getenv("BMCP_WS_PORT"))
	if err != nil {
		return Config{}, err
	}
	cfg.WsPort = port

	// Flags override the environment. Defaults come from the values above so an
	// unset flag leaves the environment-derived value untouched.
	fs := flag.NewFlagSet("mcp-server", flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.IntVar(&cfg.WsPort, "port", cfg.WsPort, "WebSocket port the extension connects to")
	fs.StringVar(&cfg.Launch, "launch", cfg.Launch, `launch and auto-connect a browser on boot; only "chrome" is supported (default: off)`)
	fs.StringVar(&cfg.ExtensionDir, "extension-dir", cfg.ExtensionDir, "unpacked extension directory (required with --launch)")
	fs.StringVar(&cfg.StartURL, "start-url", cfg.StartURL, "page opened in the connected tab when --launch is used")
	fs.StringVar(&cfg.MCPVersion, "mcp-version", cfg.MCPVersion, fmt.Sprintf("MCP protocol revision to prefer: %q or %q", mcp.ProtocolV20250618, mcp.ProtocolV20260728))
	if err := fs.Parse(os.Args[1:]); err != nil {
		return Config{}, err
	}

	if *showVersion {
		fmt.Println(cfg.Version)
		os.Exit(0)
	}

	if cfg.Launch != "" && cfg.Launch != "chrome" {
		return Config{}, fmt.Errorf("unsupported --launch %q: only \"chrome\" is supported", cfg.Launch)
	}

	if !slices.Contains(mcp.SupportedProtocolVersions, cfg.MCPVersion) {
		return Config{}, fmt.Errorf("unsupported --mcp-version %q: supported versions are %v", cfg.MCPVersion, mcp.SupportedProtocolVersions)
	}

	return cfg, nil
}

func parsePort(raw string) (int, error) {
	if raw == "" {
		return DefaultWsPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid BMCP_WS_PORT: %s", raw)
	}
	return port, nil
}
