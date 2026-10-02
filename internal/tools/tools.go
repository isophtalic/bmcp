// Package tools defines the browser automation tools exposed over MCP. Each
// tool forwards a message to the Chrome extension through the bridge and shapes
// the reply, mirroring src/tools/*.ts from the Node implementation. Names,
// descriptions and input schemas are kept identical so the extension and any
// client prompts behave the same.
package tools

import (
	"context"
	"encoding/json"

	"github.com/ngxuanth/mcp-server/internal/bridge"
	"github.com/ngxuanth/mcp-server/internal/mcp"
)

// sender is the subset of *bridge.Hub the tools need (eases testing).
type sender interface {
	Send(ctx context.Context, msgType string, payload any) (json.RawMessage, error)
}

// All returns every tool, in the same order the Node server registered them.
func All(hub *bridge.Hub) []mcp.Tool {
	return []mcp.Tool{
		navigate(hub),
		goBack(hub),
		goForward(hub),
		snapshot(hub),
		click(hub),
		drag(hub),
		hover(hub),
		typeText(hub),
		selectOption(hub),
		pressKey(hub),
		wait(hub),
		getConsoleLogs(hub),
		screenshot(hub),
		uploadFile(hub),
		evaluate(hub),
	}
}

// forward relays the raw arguments as the message payload unchanged. Empty
// argument objects are sent as {} to match the Node server.
func forward(ctx context.Context, hub sender, msgType string, args json.RawMessage) (json.RawMessage, error) {
	return hub.Send(ctx, msgType, decodeArgs(args))
}

// decodeArgs turns the incoming arguments into a payload. A missing or null
// argument object becomes an empty object.
func decodeArgs(args json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &m)
	}
	return m
}
