// Package bridge runs the WebSocket server the Chrome extension connects to and
// relays request/response messages over it. The wire format matches the Node
// implementation exactly so the existing extension works unchanged:
//
//	server -> extension: {"id": "<uuid>", "type": "<name>", "payload": <any>}
//	extension -> server: {"type": "messageResponse",
//	                      "payload": {"requestId": "<uuid>", "result": <any>, "error": "<string>"}}
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// These messages mirror the Node server's wording so the model sees the same
// guidance when the browser side is not ready.
const (
	noConnectionMessage = "No connection to browser extension. In order to proceed, you must first connect a tab by clicking the Browser MCP extension icon in the browser toolbar and clicking the 'Connect' button."
	noTabMessage        = "The browser extension is connected, but no tab is attached to it (the tab may have been closed or switched). Click the Browser MCP extension icon on the tab you want to automate and click the 'Connect' button again."
	staleTabMessage     = "The connected tab no longer exists (it was closed, discarded or replaced by the browser). Click the Browser MCP extension icon on the tab you want to automate and click the 'Connect' button again."

	// noConnectedTab is the extension's own error for a missing tab; it is
	// translated into the friendlier noTabMessage above.
	noConnectedTab = "No tab is connected"

	// reconnectGrace is how long a request waits for the extension to
	// (re)connect. The extension retries every second.
	reconnectGrace = 5 * time.Second

	// defaultTimeout bounds how long we wait for a single response.
	defaultTimeout = 30 * time.Second
)

type response struct {
	RequestID string          `json:"requestId"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error"`
}

type envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Hub owns the single extension connection and routes responses back to the
// callers waiting on them.
type Hub struct {
	extensionID string

	mu   sync.Mutex // guards conn and serialises writes (gorilla forbids concurrent writes)
	conn *websocket.Conn

	pending sync.Map // requestID -> chan response
}

// New returns a Hub that accepts the extension whose ID is extensionID, or any
// chrome-extension origin when extensionID is empty.
func New(extensionID string) *Hub {
	return &Hub{extensionID: extensionID}
}

// ListenAndServe binds the loopback WebSocket port and serves until ctx is
// cancelled. If the port is busy it kills the listener holding it, unless
// noKill is set, in which case it returns an error.
func (h *Hub) ListenAndServe(ctx context.Context, port int, noKill bool) error {
	addr := "127.0.0.1:" + strconv.Itoa(port)

	ln, err := listen(addr, noKill)
	if err != nil {
		return err
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: h.checkOrigin,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return // Upgrade already wrote the error response.
		}
		h.adopt(conn)
	})

	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	log.Printf("websocket listening on ws://%s", addr)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// checkOrigin only accepts Chrome extensions (web pages cannot forge Origin).
func (h *Hub) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if h.extensionID != "" {
		return origin == "chrome-extension://"+h.extensionID
	}
	return strings.HasPrefix(origin, "chrome-extension://")
}

// adopt makes conn the current connection, closing any previous one, and reads
// from it until it closes.
func (h *Hub) adopt(conn *websocket.Conn) {
	h.mu.Lock()
	if h.conn != nil {
		_ = h.conn.Close()
	}
	h.conn = conn
	h.mu.Unlock()
	log.Printf("extension connected")

	defer func() {
		h.mu.Lock()
		if h.conn == conn {
			h.conn = nil
		}
		h.mu.Unlock()
		_ = conn.Close()
		log.Printf("extension disconnected")
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil || env.Type != "messageResponse" {
			continue
		}
		var resp response
		if err := json.Unmarshal(env.Payload, &resp); err != nil {
			continue
		}
		if ch, ok := h.pending.LoadAndDelete(resp.RequestID); ok {
			ch.(chan response) <- resp
		}
	}
}

// waitConn returns the current connection, waiting up to reconnectGrace for the
// extension to connect if there is none yet.
func (h *Hub) waitConn(ctx context.Context) (*websocket.Conn, error) {
	deadline := time.Now().Add(reconnectGrace)
	for {
		h.mu.Lock()
		conn := h.conn
		h.mu.Unlock()
		if conn != nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New(noConnectionMessage)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Send sends a message to the extension and returns its result. A nil payload
// is omitted from the message entirely (matching the Node server's handling of
// getUrl/getTitle), while an empty object must be passed as an empty map.
func (h *Hub) Send(ctx context.Context, msgType string, payload any) (json.RawMessage, error) {
	conn, err := h.waitConn(ctx)
	if err != nil {
		return nil, err
	}

	id := uuid.NewString()
	msg := map[string]any{"id": id, "type": msgType}
	if payload != nil {
		msg["payload"] = payload
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	ch := make(chan response, 1)
	h.pending.Store(id, ch)
	defer h.pending.Delete(id)

	h.mu.Lock()
	writeErr := conn.WriteMessage(websocket.TextMessage, data)
	h.mu.Unlock()
	if writeErr != nil {
		return nil, errors.New(noConnectionMessage)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(defaultTimeout):
		return nil, errors.New("WebSocket response timeout")
	case resp := <-ch:
		if resp.Error != "" {
			return nil, translateError(resp.Error)
		}
		return resp.Result, nil
	}
}

// translateError maps the extension's raw errors to the friendlier wording the
// Node server used.
func translateError(msg string) error {
	switch {
	case msg == noConnectedTab:
		return errors.New(noTabMessage)
	case strings.Contains(msg, "No tab with given id"):
		return errors.New(staleTabMessage)
	default:
		return errors.New(msg)
	}
}

// listen binds addr, freeing it first if a listener is holding it (unless
// noKill is set).
func listen(addr string, noKill bool) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	if noKill {
		return nil, errors.New("port " + addr + " is already in use")
	}

	_, port, _ := net.SplitHostPort(addr)
	killListener(port)
	// Wait for the port to free up.
	for i := 0; i < 50; i++ {
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, err
}

// killListener kills only the process listening on port; clients connected to
// it (Chrome, via the extension) are left alone.
func killListener(port string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C",
			`FOR /F "tokens=5" %a in ('netstat -ano ^| findstr LISTENING ^| findstr :`+port+`') do taskkill /F /PID %a`)
	} else {
		cmd = exec.Command("sh", "-c", "lsof -ti tcp:"+port+" -sTCP:LISTEN | xargs -r kill -9")
	}
	if err := cmd.Run(); err != nil {
		log.Printf("failed to free port %s: %v", port, err)
	}
}
