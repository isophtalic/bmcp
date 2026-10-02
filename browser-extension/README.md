# Browser MCP extension (TypeScript source)

A clean, from-scratch TypeScript rewrite of the Browser MCP Chrome extension,
implementing exactly the wire protocol of the Go MCP server in the parent
directory (`../`). It replaces the opaque minified build in
`../browsermcp-extension/`.

## What it does

The extension is a WebSocket **client**. The Go server listens on a loopback
port; this extension connects to it and answers each tool request against the
connected tab.

```
server -> extension: { id, type, payload? }
extension -> server: { id, type: "messageResponse",
                       payload: { requestId, result, error } }
```

Supported message types (one per server tool): `getUrl`, `getTitle`,
`browser_navigate`, `browser_go_back`, `browser_go_forward`, `browser_wait`,
`browser_snapshot`, `browser_click`, `browser_hover`, `browser_drag`,
`browser_type`, `browser_select_option`, `browser_press_key`,
`browser_get_console_logs`, `browser_screenshot`, `browser_evaluate`,
`browser_upload_file`.

## Build

```bash
npm install
npm run build      # -> dist/   (load this folder as an unpacked extension)
npm run watch      # rebuild on change
npm run typecheck  # tsc --noEmit
```

Then in `chrome://extensions` → enable Developer mode → **Load unpacked** →
select `dist/`. Click the toolbar icon on the tab you want to automate and press
**Connect**.

The server auto-launch flag points here too:

```bash
../bmcp-go --launch=chrome --extension-dir=/abs/path/to/browser-extension/dist
```

## Layout

| Path | Role |
| --- | --- |
| `src/shared/protocol.ts` | Wire types + constants shared across contexts |
| `src/background/ws.ts` | WebSocket client: connect, reconnect, route |
| `src/background/index.ts` | Dispatch each message type to a handler |
| `src/background/tabs.ts` | Connected-tab tracking + navigation |
| `src/background/page.ts` | Screenshot, `eval` in main world, CDP upload |
| `src/background/content-bridge.ts` | Talk to the content script |
| `src/content/content.ts` | Accessibility snapshot + DOM interactions |
| `src/content/console-hook.ts` | Main-world console capture |
| `src/popup/` | Connect/Disconnect UI |
| `public/` | `manifest.json`, `popup.html`, icons (copied verbatim) |

## Protocol notes

- Default port is `9009` (server `config.DefaultWsPort`). To match a server
  started with `--port N`, set `storage.local.wsPort = N`.
- Refs in snapshots (`[ref=eN]`) are valid only until the next snapshot; the
  content script maps them to live elements and reports a stale ref so the model
  re-snapshots.
- `browser_upload_file` uses `chrome.debugger` (`DOM.setFileInputFiles`); the
  server passes an absolute path it has already validated against
  `BMCP_UPLOAD_DIR`.
