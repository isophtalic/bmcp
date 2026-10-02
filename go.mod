module github.com/ngxuanth/mcp-server

go 1.27

require (
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f
	github.com/chromedp/chromedp v0.16.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674
)

require (
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	// Pin to versions available in the local module cache (gorilla/websocket and
	// chromedp request older ones that are not cached).
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
