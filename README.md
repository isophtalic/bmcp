# mcp-server (Browser MCP, in Go)

A Go rewrite of the Browser MCP server. It speaks MCP over stdio to an AI app
and relays every tool call to the **Browser MCP Chrome extension** over a
loopback WebSocket. The extension does the actual browser work, so it is reused
**unchanged** — this project only replaces the Node server.

```
AI app ⇄ (stdio) ⇄ mcp-server (Go) ⇄ (WebSocket 127.0.0.1:9009) ⇄ Chrome extension ⇄ tab
```

## Build

```sh
go build -o bmcp-go .
```

Produces a single static binary (no Node runtime needed). To stamp a version:

```sh
go build -ldflags "-X main.version=0.1.6" -o bmcp-go .
```

## Use

Point your AI app at the binary instead of the Node package.

Claude Code:

```sh
claude mcp add browsermcp -e BMCP_UPLOAD_DIR=/path/to/uploads -- /path/to/bmcp-go
```

Cursor (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "browsermcp": {
      "command": "/path/to/bmcp-go",
      "env": { "BMCP_UPLOAD_DIR": "/path/to/uploads" }
    }
  }
}
```

Then open the tab to automate, click the extension icon and press **Connect**.

## Launching Chrome automatically (optional)

By default the server **only brokers**: it waits for you to press Connect in the
extension, using your real browser and profile.

The launcher from the Node project's `docker/` folder is available here as an
opt-in option, **off by default**. It starts a browser with the extension
loaded and auto-connects a tab (what pressing Connect does). **Only Chrome is
supported.**

Turn it on with `--launch chrome` (or `BMCP_LAUNCH=chrome`); it requires the
path to the unpacked extension:

```sh
./bmcp-go --launch chrome --extension-dir /path/to/browsermcp-extension
```

If launching fails, the server logs the error and keeps brokering, so you can
still connect a browser manually.

## Configuration

| Variable / flag                     | Default               | Description                                                                 |
| ----------------------------------- | --------------------- | --------------------------------------------------------------------------- |
| `BMCP_WS_PORT` / `--port`           | `9009`                | Loopback WebSocket port the extension connects to.                          |
| `BMCP_EXTENSION_ID`                 | (any extension)       | Restrict the WebSocket to one extension's `Origin`.                         |
| `BMCP_NO_KILL`                      | unset                 | Treat a busy port as an error instead of killing its listener.             |
| `BMCP_UPLOAD_DIR`                   | unset (upload off)    | Only directory whose files `browser_upload_file` may attach.                |
| `BMCP_LAUNCH` / `--launch`          | unset (off)           | Launch and auto-connect a browser on boot. Only `chrome` is supported.      |
| `BMCP_EXTENSION_DIR` / `--extension-dir` | unset            | Unpacked extension directory (required with `--launch`).                    |
| `BMCP_START_URL` / `--start-url`    | `https://example.com` | Page opened in the connected tab when `--launch` is used.                   |
| `BMCP_CHROME_PATH`                  | auto-detect           | Chrome executable to run when launching.                                    |
| `BMCP_PROFILE_DIR`                  | temporary profile     | Chrome user-data dir when launching.                                        |
| `BMCP_HEADLESS`                     | unset (windowed)      | Set to `new` to run the launched Chrome headless.                           |

## Tools

Same 15 tools as the Node server, with identical names and schemas:
`browser_navigate`, `browser_go_back`, `browser_go_forward`, `browser_snapshot`,
`browser_click`, `browser_drag`, `browser_hover`, `browser_type`,
`browser_select_option`, `browser_press_key`, `browser_wait`,
`browser_get_console_logs`, `browser_screenshot`, `browser_upload_file`,
`browser_evaluate`.

## Layout

```
main.go                     wiring: config → ws hub → (optional) launcher → stdio MCP
internal/config             flags + BMCP_* environment variables
internal/mcp                MCP server over stdio (JSON-RPC 2.0)
internal/bridge             WebSocket server + request/response relay to the extension
internal/tools              the 15 browser tools
internal/browser            optional Chrome launcher (chromedp), off by default
```

## Test

```sh
go test ./...
```
