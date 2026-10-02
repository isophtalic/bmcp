# Browser MCP (Go)

A Go implementation of the Browser MCP server. It speaks **MCP over stdio** to an
AI app (Claude Code, Cursor, …) and relays every tool call to a **Chrome
extension** over a loopback WebSocket. The extension performs the actual browser
work on a tab you connect.

```
AI app ⇄ (stdio, JSON-RPC) ⇄ bmcp-go ⇄ (WebSocket 127.0.0.1:9009) ⇄ Chrome extension ⇄ tab
```

This repo contains both halves:

| Path | What it is |
| --- | --- |
| `./` (Go) | The MCP server — a single static binary, `bmcp-go`. |
| `browser-extension/` | A clean **TypeScript** source for the extension (build it to `dist/`). |
| `browsermcp-extension/` | The old prebuilt/minified extension (kept for reference only). |

> **How the two protocols relate.** The MCP protocol version (`2025-06-18` /
> `2026-07-28`) is negotiated on the **stdio** side only. The WebSocket bridge to
> the extension is a separate, unversioned protocol, so the extension never needs
> to change when you switch MCP versions.

---

## 1. Build the server

Requires Go (see `go.mod` for the version).

```sh
go build -ldflags "-X main.version=0.1.6" -o bmcp-go .
```

This produces a single binary `bmcp-go` with no runtime dependencies.

## 2. Build the extension

Requires Node.js 18+.

```sh
cd browser-extension
npm install
npm run build        # outputs browser-extension/dist/
```

Other scripts: `npm run watch` (rebuild on change), `npm run typecheck`.

## 3. Load the extension in Chrome

1. Open `chrome://extensions`.
2. Enable **Developer mode** (top-right).
3. Click **Load unpacked** and select `browser-extension/dist/`.

The **Browser MCP** icon appears in the toolbar.

## 4. Register the server with your AI app

**Claude Code** (run from this directory):

```sh
claude mcp add browsermcp -- "$(pwd)/bmcp-go"
```

Optional env/flags (see the table below), e.g. file upload + the 2026 protocol:

```sh
claude mcp add browsermcp \
  -e BMCP_UPLOAD_DIR=/path/to/uploads \
  -e BMCP_MCP_VERSION=2026-07-28 \
  -- "$(pwd)/bmcp-go"
```

**Cursor** (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "browsermcp": {
      "command": "/absolute/path/to/bmcp-go",
      "env": { "BMCP_UPLOAD_DIR": "/path/to/uploads" }
    }
  }
}
```

## 5. Connect and use

1. Make sure the AI app has the server loaded (it launches `bmcp-go`, which then
   listens on port 9009 — see the lifecycle note below).
2. Open the tab you want to automate.
3. Click the **Browser MCP** icon → **Connect**. The badge turns green (`on`).
4. Ask the AI to drive the page, e.g. *"open facebook.com and snapshot it"*,
   *"click the Search box and type hello"*, *"screenshot the page"*.

> **Lifecycle note (important).** Because transport is stdio, the server only
> runs — and only holds port 9009 open — **while the AI app keeps it loaded**. If
> the extension popup is stuck on *Connecting…*, nothing is listening on 9009:
> start/restart your AI session so it launches `bmcp-go`. The extension retries
> every second and connects automatically once the port is open. Run only one
> server instance at a time (a new one frees the port by killing the old
> listener unless `BMCP_NO_KILL` is set).

### Running the server standalone (for testing)

Stdio servers exit on end-of-input, so hold stdin open:

```sh
sleep infinity | ./bmcp-go --port 9009
```

---

## Launching Chrome automatically (optional)

By default the server **only brokers** and waits for you to press Connect, using
your real browser and profile. You can instead have it launch Chrome with the
extension preloaded and auto-connect a tab. **Only Chrome is supported.**

```sh
./bmcp-go --launch chrome --extension-dir "$(pwd)/browser-extension/dist"
```

If launching fails, the server logs the error and keeps brokering, so you can
still connect a browser manually.

## MCP protocol versions

The server speaks two revisions and prefers one via `--mcp-version` /
`BMCP_MCP_VERSION` (default `2025-06-18`):

- **`2025-06-18`** — classic: `initialize` handshake, no `resultType`.
- **`2026-07-28`** — stateless: adds the `server/discover` RPC, stamps
  `resultType: "complete"` on every result, and reads a client's version pinned
  in each request's `_meta`.

A client may still pin the other supported version via its `initialize` request
or `_meta`; the server negotiates per client.

## Configuration

| Variable / flag | Default | Description |
| --- | --- | --- |
| `BMCP_WS_PORT` / `--port` | `9009` | Loopback WebSocket port the extension connects to. |
| `BMCP_MCP_VERSION` / `--mcp-version` | `2025-06-18` | MCP revision to prefer: `2025-06-18` or `2026-07-28`. |
| `BMCP_EXTENSION_ID` | (any extension) | Restrict the WebSocket to one extension's `Origin`. |
| `BMCP_NO_KILL` | unset | Treat a busy port as an error instead of killing its listener. |
| `BMCP_UPLOAD_DIR` | unset (upload off) | Only directory whose files `browser_upload_file` may attach. |
| `BMCP_LAUNCH` / `--launch` | unset (off) | Launch and auto-connect a browser on boot. Only `chrome`. |
| `BMCP_EXTENSION_DIR` / `--extension-dir` | unset | Unpacked extension directory (required with `--launch`). |
| `BMCP_START_URL` / `--start-url` | `https://example.com` | Page opened in the connected tab when `--launch` is used. |
| `BMCP_CHROME_PATH` | auto-detect | Chrome executable to run when launching. |
| `BMCP_PROFILE_DIR` | temporary profile | Chrome user-data dir when launching. |
| `BMCP_HEADLESS` | unset (windowed) | Set to `new` to run the launched Chrome headless. |

## Tools

15 tools with identical names and schemas to the Node server:
`browser_navigate`, `browser_go_back`, `browser_go_forward`, `browser_snapshot`,
`browser_click`, `browser_drag`, `browser_hover`, `browser_type`,
`browser_select_option`, `browser_press_key`, `browser_wait`,
`browser_get_console_logs`, `browser_screenshot`, `browser_upload_file`,
`browser_evaluate`.

Interaction tools take a `ref` from the most recent `browser_snapshot`
(e.g. `[ref=e6]`); navigation and interaction tools return a fresh snapshot so
the next action always has up-to-date refs.

## Troubleshooting

- **Popup stuck on *Connecting…*** — no server is listening on 9009. Start/restart
  the AI session (or run the standalone command above). Only run one instance.
- **`No connection to browser extension`** — the extension is not connected: open
  the target tab and press **Connect**.
- **`No tab is connected` / stale tab** — the connected tab was closed or
  switched; press **Connect** again on the tab you want.
- **File upload disabled** — set `BMCP_UPLOAD_DIR`; only regular, non-hidden files
  inside it can be attached.
- **Port already in use** — another `bmcp-go` holds 9009. It is freed
  automatically unless `BMCP_NO_KILL` is set; otherwise stop the other instance.

## Project layout

```
main.go                     wiring: config → ws hub → (optional) launcher → stdio MCP
internal/config             flags + BMCP_* environment variables
internal/mcp                MCP server over stdio (JSON-RPC 2.0), version negotiation
internal/bridge             WebSocket server + request/response relay to the extension
internal/tools              the 15 browser tools
internal/browser            optional Chrome launcher (chromedp), off by default
browser-extension/          clean TypeScript extension source (build → dist/)
```

## Test

```sh
go test ./...
cd browser-extension && npm run typecheck
```
