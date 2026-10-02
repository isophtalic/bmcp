// Command mcp-server is a Browser MCP server written in Go. It speaks MCP over
// stdio to an AI app and relays each tool call to the Browser MCP Chrome
// extension over a loopback WebSocket, so the existing extension works
// unchanged.
//
// By default it only brokers: start it, then click Connect in the extension.
// Pass --launch=chrome (or BMCP_LAUNCH=chrome) to also start Chrome with the
// extension loaded and auto-connect a tab.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ngxuanth/mcp-server/internal/bridge"
	"github.com/ngxuanth/mcp-server/internal/browser"
	"github.com/ngxuanth/mcp-server/internal/config"
	"github.com/ngxuanth/mcp-server/internal/mcp"
	"github.com/ngxuanth/mcp-server/internal/tools"
)

// version is reported to the MCP client; override at build time with
// -ldflags "-X main.version=x.y.z".
var version = "0.1.6"

func main() {
	// Logs go to stderr so they never corrupt the stdio JSON-RPC stream.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[bmcp] ")

	cfg, err := config.Load(version)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hub := bridge.New(cfg.ExtensionID)
	go func() {
		if err := hub.ListenAndServe(ctx, cfg.WsPort, cfg.NoKill); err != nil {
			log.Fatalf("websocket server: %v", err)
		}
	}()

	// Optionally launch Chrome and auto-connect it. Failure here is logged but
	// does not stop the server: the user can still connect a browser manually.
	if cfg.Launch == "chrome" {
		go func() {
			if err := browser.Launch(ctx, cfg); err != nil {
				log.Printf("launch chrome: %v", err)
			}
		}()
	}

	server := mcp.NewServer(cfg.Name, cfg.Version, cfg.MCPVersion, tools.All(hub))
	if err := server.Serve(ctx); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
