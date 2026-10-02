package bridge

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// startTestHub starts a hub on a free loopback port and returns it with its port.
func startTestHub(t *testing.T) (*Hub, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	h := New("")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = h.ListenAndServe(ctx, port, true) }()

	// Wait for the listener to come up.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
			c.Close()
			return h, port
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("hub did not start listening")
	return nil, 0
}

// dialExtension connects like the Chrome extension would, with a chrome-extension Origin.
func dialExtension(t *testing.T, port int) *websocket.Conn {
	t.Helper()
	hdr := map[string][]string{"Origin": {"chrome-extension://abcdefghijklmnop"}}
	conn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:"+strconv.Itoa(port)+"/", hdr)
	if err != nil {
		t.Fatalf("extension dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestSendRoundTrip(t *testing.T) {
	h, port := startTestHub(t)
	conn := dialExtension(t, port)

	// The fake extension echoes a result for whatever it receives.
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				ID      string          `json:"id"`
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
			_ = json.Unmarshal(data, &msg)
			resp := map[string]any{
				"type": "messageResponse",
				"payload": map[string]any{
					"requestId": msg.ID,
					"result":    "ok:" + msg.Type,
				},
			}
			out, _ := json.Marshal(resp)
			_ = conn.WriteMessage(websocket.TextMessage, out)
		}
	}()

	// Give the server a moment to adopt the connection.
	time.Sleep(100 * time.Millisecond)

	raw, err := h.Send(context.Background(), "browser_snapshot", map[string]any{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var got string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got != "ok:browser_snapshot" {
		t.Fatalf("got %q, want ok:browser_snapshot", got)
	}
}

func TestSendNilPayloadOmitted(t *testing.T) {
	h, port := startTestHub(t)
	conn := dialExtension(t, port)

	received := make(chan string, 1)
	go func() {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		received <- string(data)
		// Reply so Send returns.
		var msg struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(data, &msg)
		out, _ := json.Marshal(map[string]any{
			"type":    "messageResponse",
			"payload": map[string]any{"requestId": msg.ID, "result": "x"},
		})
		_ = conn.WriteMessage(websocket.TextMessage, out)
	}()

	time.Sleep(100 * time.Millisecond)
	if _, err := h.Send(context.Background(), "getUrl", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	raw := <-received
	if strings.Contains(raw, "payload") {
		t.Fatalf("nil payload should be omitted, got: %s", raw)
	}
}

func TestSendErrorTranslation(t *testing.T) {
	h, port := startTestHub(t)
	conn := dialExtension(t, port)

	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(data, &msg)
			out, _ := json.Marshal(map[string]any{
				"type":    "messageResponse",
				"payload": map[string]any{"requestId": msg.ID, "error": noConnectedTab},
			})
			_ = conn.WriteMessage(websocket.TextMessage, out)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	_, err := h.Send(context.Background(), "browser_click", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "no tab is attached") {
		t.Fatalf("want translated no-tab message, got: %v", err)
	}
}

func TestSendNoConnection(t *testing.T) {
	h := New("")
	// No ListenAndServe, no extension: Send should fail with the connect hint
	// after the reconnect grace period.
	start := time.Now()
	_, err := h.Send(context.Background(), "browser_snapshot", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "No connection to browser extension") {
		t.Fatalf("want no-connection message, got: %v", err)
	}
	if time.Since(start) < reconnectGrace {
		t.Fatalf("Send returned before the reconnect grace elapsed")
	}
}
